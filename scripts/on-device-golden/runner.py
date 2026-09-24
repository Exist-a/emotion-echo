"""golden set runner：加载用例 + 注入 model_fn 执行评分。

model_fn(case) -> reply_str —— 调用方注入（云端 BFF / WebLLM stub / 记录回放），
runner 自身零网络（AGENTS §3.1 依赖反转 + §一.1.3 测试不依赖网络）。
云端基线实测（v0.3 §C.1 任务 5）由后续脚本注入真实 model_fn 完成，不在本骨架内。
"""
from __future__ import annotations

import json
from pathlib import Path
from typing import Any, Callable

from metrics import evaluate_case, summarize

GOLDEN_FILE = Path(__file__).parent / "golden_set.jsonl"

ModelFn = Callable[[dict[str, Any]], str]


def load_cases(path: Path | None = None) -> list[dict[str, Any]]:
    p = path or GOLDEN_FILE
    cases = [json.loads(ln) for ln in p.read_text(encoding="utf-8").splitlines() if ln.strip()]
    return cases


def run_golden_set(model_fn: ModelFn, cases: list[dict[str, Any]] | None = None) -> dict[str, Any]:
    cases = cases if cases is not None else load_cases()
    results = [evaluate_case(c, model_fn(c)) for c in cases]
    return {**summarize(results), "results": results}
