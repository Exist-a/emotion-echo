"""golden set 桥接层：把 on-device-golden 的纯函数 re-export 到 on-device-baseline。

为什么需要：
- on-device-golden/ 不是 Python package（无 __init__.py），pytest collection 时
  from on_device_golden.runner import load_cases 会被 pytest 路径解析干扰而失败
- baseline 的测试与运行代码只 import 同 package 内的桥接层，避免跨目录包路径

桥接而非复制：每次 import 时动态 load 源文件，保证 golden set 改动同步生效。
"""
from __future__ import annotations

import importlib.util as _ilu
from pathlib import Path
import sys as _sys

_REPO_ROOT = Path(__file__).resolve().parents[2]
_GOLDEN = _REPO_ROOT / "scripts" / "on-device-golden"


def _load(name: str) -> object:
    spec = _ilu.spec_from_file_location(f"_on_device_golden_bridge_{name}", str(_GOLDEN / f"{name}.py"))
    mod = _ilu.module_from_spec(spec)
    # 同时以裸名注入 sys.modules（on-device-golden 内部 import 用 `from metrics import ...`）
    _sys.modules[name] = mod
    _sys.modules[spec.name] = mod
    spec.loader.exec_module(mod)
    return mod


# metrics 必须先于 runner（runner.py 内部 `from metrics import ...`）
_metrics_mod = _load("metrics")
_runner_mod = _load("runner")

evaluate_case = _metrics_mod.evaluate_case
summarize = _metrics_mod.summarize
check_guardrail = _metrics_mod.check_guardrail
check_length = _metrics_mod.check_length

load_cases = _runner_mod.load_cases
run_golden_set = _runner_mod.run_golden_set