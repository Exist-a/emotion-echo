"""conftest.py：让 pytest collection 时把 scripts/ 加进 sys.path。

这样 `from on_device_golden.runner import load_cases` 才能解析（on-device-golden/
没有 __init__.py，不算包；用 path-based import 而非包 import 与既有
`scripts/on-device-golden/test_golden.py` 的运行风格一致）。
"""
import sys
from pathlib import Path

SCRIPTS_DIR = str(Path(__file__).resolve().parents[1])
if SCRIPTS_DIR not in sys.path:
    sys.path.insert(0, SCRIPTS_DIR)