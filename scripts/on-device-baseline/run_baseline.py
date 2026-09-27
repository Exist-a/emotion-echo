"""baseline CLI 入口：注入 model_fn 跑既有 golden_set.jsonl 出报告。

用法（worktree 根或仓库根）：
    python -m scripts.on_device_baseline.run_baseline --impl cloud_grpc --report baseline_report.md
    python -m scripts.on_device_baseline.run_baseline --impl cloud_deepseek --api-key sk-... --report baseline_report.md
    python -m scripts.on_device_baseline.run_baseline --impl record --replay-dir /tmp/baseline-cache --inner-impl cloud_grpc --report baseline_report.md

报告 = `scripts/on-device-baseline/baseline_report.md`（默认路径）+ 命令行 summary。

设计：
- 复用 `scripts.on_device_golden.metrics.evaluate_case / summarize` 做评分（不重写）
- model_fn 返回 `ReplyResult`，把 `.text` 喂 evaluate_case；其余字段聚合到 baseline 报告
- 一致性：与 `scripts.on-device-perf/` 同款 CLI 风格（便于 E2E-17 同型记忆复用）
"""
from __future__ import annotations

import argparse
import json
import logging
import sys
from pathlib import Path
from typing import Any

# 允许 `python run_baseline.py`（不靠包导入）与 `python -m ...` 两种调用
_HERE = Path(__file__).resolve().parent
_REPO_ROOT = _HERE.parents[2]
for p in (_HERE, _HERE.parent):
    sp = str(p)
    if sp not in sys.path:
        sys.path.insert(0, sp)

from model_fn_factory import BaselineModelFn, ReplyResult, make_model_fn  # noqa: E402
from golden_bridge import evaluate_case, load_cases, summarize  # noqa: E402

logger = logging.getLogger(__name__)


def baseline_run(
    model_fn: BaselineModelFn, cases: list[dict[str, Any]] | None = None
) -> dict[str, Any]:
    """跑一组 case、折 id / layer / ok / violations / extras 进报告。"""
    cases = cases if cases is not None else load_cases()
    if not cases:
        raise ValueError("no golden cases loaded")

    rows: list[dict[str, Any]] = []
    for c in cases:
        result: ReplyResult = model_fn(c)
        ev = evaluate_case(c, result.text)
        rows.append(
            {
                "id": c["id"],
                "layer": c["layer"],
                "ok": ev["ok"],
                "violations": ev["violations"],
                "reply_len": len(result.text.strip()),
                "model": result.model,
                "fallback_reason": result.fallback_reason,
                "latency_ms": result.latency_ms,
                "replay_hit": result.extras.get("replay_hit"),
            }
        )

    summary = summarize([{"id": r["id"], "ok": r["ok"], "violations": r["violations"], "layer": r["layer"]} for r in rows])
    return {
        "n": summary["n"],
        "pass_rate": summary["pass_rate"],
        "length_pass_rate": summary["length_pass_rate"],
        "guardrail_pass_rate": summary["guardrail_pass_rate"],
        "rows": rows,
    }


def render_report(report: dict[str, Any], meta: dict[str, Any]) -> str:
    """生成 markdown 报告（决策材料输入格式）。"""
    lines: list[str] = []
    lines.append(f"# 云端基线跑分报告（{meta.get('impl', '?')} · {meta.get('generated_at', '?')})")
    lines.append("")
    lines.append("> **报告范围**：" + meta.get("scope", ""))
    lines.append("> **生成命令**：`" + meta.get("command", "") + "`")
    if meta.get("warnings"):
        for w in meta["warnings"]:
            lines.append("> **警告**：" + w)
    lines.append("")
    lines.append("## 一、总览（5 层 × 三率）")
    lines.append("")
    lines.append(f"- 用例数 N = **{report['n']}**")
    lines.append(f"- pass_rate（总通过率） = **{report['pass_rate']:.2%}**")
    lines.append(f"- length_pass_rate（长度合规率） = **{report['length_pass_rate']:.2%}**")
    lines.append(f"- guardrail_pass_rate（护栏通过率） = **{report['guardrail_pass_rate']:.2%}**")
    lines.append("")

    # 按层分组
    by_layer: dict[str, list[dict[str, Any]]] = {}
    for r in report["rows"]:
        by_layer.setdefault(r["layer"], []).append(r)

    lines.append("## 二、按层细分")
    lines.append("")
    lines.append("| 层 | 用例数 | pass_rate | length_pass | guardrail |")
    lines.append("|----|--------|-----------|-------------|-----------|")
    for layer in sorted(by_layer.keys()):
        rs = by_layer[layer]
        n = len(rs)
        pr = sum(1 for r in rs if r["ok"]) / n
        lr = sum(1 for r in rs if not any(v.startswith("length:") for v in r["violations"])) / n
        gr = sum(1 for r in rs if not any(v.startswith("guardrail:") for v in r["violations"])) / n
        lines.append(f"| `{layer}` | {n} | {pr:.0%} | {lr:.0%} | {gr:.0%} |")
    lines.append("")

    lines.append("## 三、逐用例明细")
    lines.append("")
    lines.append("| case_id | layer | ok | reply_len | model | fallback_reason | latency_ms |")
    lines.append("|---------|-------|----|-----------|-------|------------------|------------|")
    for r in report["rows"]:
        ok_mark = "✅" if r["ok"] else "❌"
        fr = r["fallback_reason"] or ""
        lines.append(
            f"| `{r['id']}` | `{r['layer']}` | {ok_mark} | {r['reply_len']} | "
            f"`{r['model'] or '-'}` | `{fr}` | {r['latency_ms']} |"
        )
    lines.append("")

    # 失败明细
    failed = [r for r in report["rows"] if not r["ok"]]
    if failed:
        lines.append("## 四、失败用例 violations 明细")
        lines.append("")
        for r in failed:
            lines.append(f"- `{r['id']}`（layer=`{r['layer']}`）：")
            for v in r["violations"]:
                lines.append(f"  - `{v}`")
        lines.append("")

    lines.append("## 五、与 v0.2 §8.3 阈值对比")
    lines.append("")
    lines.append("| 指标 | v0.2 §8.3 目标 | 本次 baseline | 评估 |")
    lines.append("|------|----------------|---------------|------|")
    for name, target, value in [
        ("pass_rate", "≥ 阈值（待 §十二 决策 2 拍板后定）", report["pass_rate"]),
        ("length_pass_rate", "≥ 阈值", report["length_pass_rate"]),
        ("guardrail_pass_rate", "≥ 阈值", report["guardrail_pass_rate"]),
    ]:
        lines.append(f"| {name} | {target} | {value:.2%} | 待决策 |")
    lines.append("")
    lines.append("> 阈值由 §十二 决策 2 拍板后从 golden set v0.2 §8.3 推到具体数字；本报告先给基线分。")
    lines.append("")

    return "\n".join(lines)


def main(argv: list[str] | None = None) -> int:
    p = argparse.ArgumentParser(prog="on-device-baseline")
    p.add_argument("--impl", required=True, choices=["cloud_grpc", "cloud_deepseek", "record"])
    p.add_argument("--target", default=None, help="gRPC target (default: localhost:50051)")
    p.add_argument("--api-key", default=None, help="API key for cloud_deepseek")
    p.add_argument("--base-url", default=None, help="OpenAI base URL")
    p.add_argument("--model", default=None, help="Model name")
    p.add_argument("--system-prompt", default=None, help="Override default system prompt")
    p.add_argument("--replay-dir", default=None, help="record impl: replay dir")
    p.add_argument("--inner-impl", default=None, choices=["cloud_grpc", "cloud_deepseek"], help="record impl: forward impl")
    p.add_argument("--cases", default=None, help="Path to golden_set.jsonl (default: scripts/on-device-golden/golden_set.jsonl)")
    p.add_argument("--report", default=str(_HERE / "baseline_report.md"), help="Output markdown report path")
    p.add_argument("--json", default=None, help="Output raw JSON path (machine-readable)")
    args = p.parse_args(argv)

    logging.basicConfig(level=logging.INFO, format="%(asctime)s [%(name)s] %(levelname)s %(message)s")

    if args.impl == "cloud_grpc":
        model_fn = make_model_fn("cloud_grpc", target=args.target, api_key=args.api_key, system_prompt=args.system_prompt or "")
    elif args.impl == "cloud_deepseek":
        model_fn = make_model_fn(
            "cloud_deepseek",
            api_key=args.api_key,
            base_url=args.base_url,
            model=args.model,
            system_prompt=args.system_prompt,
        )
    elif args.impl == "record":
        if not args.replay_dir or not args.inner_impl:
            p.error("--impl record requires --replay-dir and --inner-impl")
        inner = make_model_fn(
            args.inner_impl,
            target=args.target,
            api_key=args.api_key,
            base_url=args.base_url,
            model=args.model,
            system_prompt=args.system_prompt,
        )
        model_fn = make_model_fn("record", replay_dir=args.replay_dir, inner_fn=inner)
    else:
        raise SystemExit(2)

    cases = load_cases(Path(args.cases)) if args.cases else load_cases()
    report = baseline_run(model_fn, cases)

    from datetime import datetime, timezone
    meta = {
        "impl": args.impl,
        "generated_at": datetime.now(timezone.utc).isoformat(timespec="seconds"),
        "command": " ".join(argv or sys.argv[1:]),
        "scope": (
            f"scripts/on-device-baseline/ · N={report['n']} 用例 · 5 层覆盖 · "
            f"v0.3 §C.1 任务 5 + §G.1 阶段一收口契约"
        ),
        "warnings": [
            "mock fallback 路径下 length_pass_rate 必挂（mock 输出 ~50 字 vs 预算 100~300），"
            "本 baseline 报告值仅作占位；真实 Qwen3-1.7B 真机基线须 T3 借 dev mode 窗口完成。"
        ],
    }

    md = render_report(report, meta)
    Path(args.report).write_text(md, encoding="utf-8")

    if args.json:
        Path(args.json).write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8")

    # CLI summary
    print(f"[baseline] impl={args.impl} N={report['n']} "
          f"pass={report['pass_rate']:.2%} length={report['length_pass_rate']:.2%} "
          f"guardrail={report['guardrail_pass_rate']:.2%}")
    print(f"[baseline] report -> {args.report}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())