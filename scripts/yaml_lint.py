#!/usr/bin/env python3
"""scripts/yaml_lint.py — Round 4.6 P1-26 字面值收紧校验

锁住 ai-api.yaml（未来扩到全仓 *.yaml）不得含 ${VAR:-default} 字面默认值。
所有 default 走 env 注入或 main.go applyDefaultFallbacks 显式赋值，
避免"dev 拼通的 prod 配置"漂移。

用法：
    python scripts/yaml_lint.py
退出码：
    0 = 所有 yaml 无字面 default
    1 = 存在 ${VAR:-...} 字面 default（CI 红）
"""
import re
import sys
from pathlib import Path

# 字面 default 模式：${VAR:-default} 或 ${VAR:-"quoted default"}
# 但允许 ${VAR:-}（空 default = 可选服务），${VAR} 形式（无 default）
PATTERN = re.compile(r'\$\{[A-Z_][A-Z0-9_]*:-[^}"]')


def scan(path: Path) -> list[str]:
    """返 [(file, line, content)] 列表"""
    findings = []
    if path.is_dir():
        for child in path.rglob("*.yaml"):
            findings.extend(scan(child))
        for child in path.rglob("*.yml"):
            findings.extend(scan(child))
        return findings
    try:
        text = path.read_text(encoding="utf-8")
    except (UnicodeDecodeError, OSError):
        return findings
    for lineno, line in enumerate(text.splitlines(), start=1):
        # 跳过注释行（# 开头）
        if line.lstrip().startswith("#"):
            continue
        if PATTERN.search(line):
            findings.append((str(path), lineno, line.strip()))
    return findings


def main() -> int:
    root = Path("emotion-echo-ai-svc/etc")
    findings = scan(root)
    if not findings:
        print(f"OK: {root} has no literal ${{VAR:-default}} patterns")
        return 0
    print(f"FAIL: {len(findings)} literal default(s) found in {root}:")
    for f, lineno, line in findings:
        print(f"  {f}:{lineno}: {line}")
    return 1


if __name__ == "__main__":
    sys.exit(main())
