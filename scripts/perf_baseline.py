#!/usr/bin/env python3
"""E2E-28 性能基线测量脚本（plan §3 C1，stdlib only 零新依赖）。

模式：
  http    顺序发 N 个请求，输出逐样本耗时 + p50/p95（默认）
  ladder  小规模并发阶梯（--levels 1,2,4,8），每档独立分位数
  sse     POST SSE 流：TTFT（首个 data 块）+ 逐块到达时间戳 + 突发占比
          （plan #7 伪流式定性：首块后 0.5s 内到达的块占比 ≥80% ⇒ 整段缓冲）

退出码：0=全部成功；1=存在请求失败/不可达（stderr 有显式 ERROR）；2=用法错误。
stdout 恒为 JSON（含 errors 字段）；stderr 只放诊断。

分位数方法：官方线性插值（docs.python.org statistics "simpler alternative"：
i=(len-1)*p/100，两端线性插值）。不用 statistics.quantiles(n=100) 是因为它
要求样本数 > 100，而 plan #2 只需 N≥50。

路径纪律：--json-out 建议传相对路径 —— Git Bash 的 /tmp 等 MSYS 路径
Windows python 解析不了（E2E-28 计划期实测）。

容器网用法（llm-service 无宿主端口，plan F-c）：
  docker exec -i emotion-llm-service python - \
    --url http://emotion-llm-service:8000/analyze --n 50 \
    -H "Internal-API-Key:$KEY" --method POST --body '{"text":"..."}' \
    < scripts/perf_baseline.py
"""

from __future__ import annotations

import argparse
import json
import sys
import time
import urllib.error
import urllib.request

TOOL_VERSION = 1
BURST_WINDOW_S = 0.5  # plan #7：首块后 0.5s 内到达视为"突发"


def percentile(data: list[float], p: float) -> float:
    """线性插值分位数（官方 simpler-alternative 公式）。p ∈ [0,100]。"""
    if not data:
        raise ValueError("percentile of empty data")
    if not 0 <= p <= 100:
        raise ValueError(f"percentile p out of range: {p}")
    ordered = sorted(data)
    if len(ordered) == 1:
        return ordered[0]
    idx = (len(ordered) - 1) * p / 100.0
    lo = int(idx)
    hi = min(lo + 1, len(ordered) - 1)
    return ordered[lo] + (ordered[hi] - ordered[lo]) * (idx - lo)


def summarize(samples_ms: list[float]) -> dict:
    if not samples_ms:
        return {"n": 0}
    return {
        "n": len(samples_ms),
        "min": round(min(samples_ms), 3),
        "max": round(max(samples_ms), 3),
        "mean": round(sum(samples_ms) / len(samples_ms), 3),
        "p50": round(percentile(samples_ms, 50), 3),
        "p95": round(percentile(samples_ms, 95), 3),
    }


def build_request(args: argparse.Namespace) -> urllib.request.Request:
    headers = {"User-Agent": f"perf-baseline/{TOOL_VERSION}"}
    for h in args.header or []:
        if ":" not in h:
            raise SystemExit(f"ERROR: bad --header (want 'K:V'): {h!r}")
        k, v = h.split(":", 1)
        headers[k.strip()] = v.strip()
    if args.token:
        headers["Authorization"] = f"Bearer {args.token}"
    data = None
    if args.body is not None:
        data = args.body.encode("utf-8")
        headers.setdefault("Content-Type", "application/json")
    return urllib.request.Request(args.url, data=data, headers=headers,
                                  method=args.method)


def one_http(args: argparse.Namespace) -> tuple[float, int | None, str | None]:
    """单次请求 → (耗时ms, HTTP状态码, 错误串)。"""
    req = build_request(args)
    start = time.perf_counter()
    try:
        with urllib.request.urlopen(req, timeout=args.timeout) as resp:
            resp.read()
            code = resp.status
    except urllib.error.HTTPError as e:
        return (time.perf_counter() - start) * 1000, e.code, f"HTTP {e.code}"
    except Exception as e:  # URLError/timeout/socket 等
        return (time.perf_counter() - start) * 1000, None, f"{type(e).__name__}: {e}"
    elapsed = (time.perf_counter() - start) * 1000
    if code != 200:
        return elapsed, code, f"HTTP {code}"
    return elapsed, code, None


def run_http(args: argparse.Namespace) -> dict:
    samples: list[float] = []
    errors: list[str] = []
    for _ in range(args.n):
        elapsed, code, err = one_http(args)
        if err is not None:
            errors.append(err)
        else:
            samples.append(elapsed)
    return {
        "mode": "http",
        "url": args.url,
        "requested": args.n,
        "samples_ms": [round(x, 3) for x in samples],
        "summary": summarize(samples),
        "errors": errors,
    }


def run_ladder(args: argparse.Namespace) -> dict:
    from concurrent.futures import ThreadPoolExecutor

    levels = [int(x) for x in args.levels.split(",") if x.strip()]
    out_levels = []
    total_errors: list[str] = []
    for level in levels:
        samples: list[float] = []
        errors: list[str] = []
        with ThreadPoolExecutor(max_workers=level) as pool:
            futures = [pool.submit(one_http, args) for _ in range(args.per_level)]
            for f in futures:
                elapsed, code, err = f.result()
                if err is not None:
                    errors.append(err)
                else:
                    samples.append(elapsed)
        out_levels.append({
            "concurrency": level,
            "requested": args.per_level,
            "samples_ms": [round(x, 3) for x in samples],
            "summary": summarize(samples),
            "errors": errors,
        })
        total_errors.extend(errors)
    return {
        "mode": "ladder",
        "url": args.url,
        "levels": out_levels,
        "errors": total_errors,
    }


def run_sse(args: argparse.Namespace) -> dict:
    """SSE 流式测量（plan #6/#7）：--n 轮，每轮 TTFT + 逐块时间戳 + 突发占比。

    输出：runs[]（逐轮明细）+ summary（各轮 ttfb_ms 的分位数）+ errors 汇总。
    """
    runs: list[dict] = []
    all_errors: list[str] = []
    for _ in range(args.n):
        req = build_request(args)
        start = time.perf_counter()
        chunk_times: list[dict] = []
        error: str | None = None
        try:
            with urllib.request.urlopen(req, timeout=args.timeout) as resp:
                for raw in resp:
                    now = time.perf_counter() - start
                    line = raw.decode("utf-8", "replace").strip()
                    if not line:
                        continue
                    chunk_times.append({"t_ms": round(now * 1000, 1),
                                        "line": line[:200]})
        except Exception as e:
            error = f"{type(e).__name__}: {e}"

        data_lines = [c for c in chunk_times if c["line"].startswith("data:")]
        ttfb_ms = data_lines[0]["t_ms"] if data_lines else None
        first_content = next((c["t_ms"] for c in data_lines
                              if '"delta"' in c["line"]), None)
        burst_ratio = None
        if data_lines and ttfb_ms is not None:
            cutoff = ttfb_ms + BURST_WINDOW_S * 1000
            in_window = sum(1 for c in data_lines if c["t_ms"] <= cutoff)
            burst_ratio = round(in_window / len(data_lines), 3)
        if error is not None:
            all_errors.append(error)
        runs.append({
            "ttfb_ms": ttfb_ms,
            "first_content_ms": first_content,
            "total_ms": round((time.perf_counter() - start) * 1000, 1),
            "data_chunks": len(data_lines),
            # plan #7 判定输入：>=0.8 ⇒ 整段缓冲（伪流式）疑点成立
            "burst_ratio": burst_ratio,
            "error": error,
            "chunks": chunk_times,
        })

    ttfbs = [r["ttfb_ms"] for r in runs if r["ttfb_ms"] is not None]
    return {
        "mode": "sse",
        "url": args.url,
        "requested": args.n,
        "runs": runs,
        "summary": summarize([float(t) for t in ttfbs]),
        "burst_window_s": BURST_WINDOW_S,
        "errors": all_errors,
    }


def main(argv: list[str]) -> int:
    p = argparse.ArgumentParser(description="E2E-28 performance baseline tool")
    p.add_argument("--url", required=True)
    p.add_argument("--mode", choices=["http", "ladder", "sse"], default="http")
    p.add_argument("--method", default="GET", choices=["GET", "POST"])
    p.add_argument("-H", "--header", action="append", metavar="K:V")
    p.add_argument("--token", help="shorthand → Authorization: Bearer")
    p.add_argument("--body", help="request body (with --method POST)")
    p.add_argument("--n", type=int, default=10, help="sequential requests (http)")
    p.add_argument("--levels", default="1,2,4,8", help="ladder concurrency levels")
    p.add_argument("--per-level", type=int, default=20, dest="per_level")
    p.add_argument("--timeout", type=float, default=30.0)
    p.add_argument("--json-out", dest="json_out", help="also write JSON to file")
    args = p.parse_args(argv)

    if args.n < 1 or args.per_level < 1:
        print("ERROR: --n/--per-level must be >= 1", file=sys.stderr)
        return 2
    if args.method == "POST" and args.body is None and args.mode != "sse":
        pass  # POST 无 body 也允许（部分端点如此）

    try:
        if args.mode == "http":
            result = run_http(args)
        elif args.mode == "ladder":
            result = run_ladder(args)
        else:
            result = run_sse(args)
    except SystemExit:
        raise
    except Exception as e:
        print(f"ERROR: {type(e).__name__}: {e}", file=sys.stderr)
        return 1

    result["tool"] = f"perf_baseline/{TOOL_VERSION}"
    result["ts"] = time.strftime("%Y-%m-%dT%H:%M:%S%z")
    text = json.dumps(result, ensure_ascii=False, indent=2)
    if args.json_out:
        try:
            with open(args.json_out, "w", encoding="utf-8") as f:
                f.write(text)
        except OSError as e:
            print(f"ERROR: cannot write --json-out: {e}", file=sys.stderr)
            return 1
    print(text)

    errors = result.get("errors") or []
    if errors:
        print(f"ERROR: {len(errors)} request(s) failed; first: {errors[0]}",
              file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
