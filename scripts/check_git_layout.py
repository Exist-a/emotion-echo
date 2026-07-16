#!/usr/bin/env python3
"""
check_git_layout.py · Emotion-Echo 单仓 monorepo 布局自检脚本

校验项目：
  1. 根目录存在 .git/（主仓已 init）
  2. 根目录存在 .gitignore 且包含必要条目
  3. 根目录存在 docs/git-layout.md
  4. 根目录**不存在** .gitmodules（单仓不允许 submodule）
  5. 4 个关键目录都存在（前端 / legacy / 2 个 AI 模型）
  6. 根目录存在 docs/ 目录
  7. 根目录存在 scripts/ 目录

用法：
  python scripts/check_git_layout.py            # 输出检查结果
  python scripts/check_git_layout.py --strict   # 任一失败 exit 1
"""

from __future__ import annotations

import sys
from pathlib import Path

# 项目根目录（脚本位于 scripts/ 下，父目录即根）
ROOT = Path(__file__).resolve().parent.parent

# 4 个必须存在的关键目录（从原 submodule 转为普通目录）
KEY_DIRS = [
    "Emotion-Echo-Web",
    "legacy/emotion-echo-gin",
    "Emotion-Echo-LLM/sensevoice-small",
    "Emotion-Echo-LLM/XTTS/TTS",
]


def check(label: str, ok: bool, detail: str = "") -> bool:
    """打印一行检查结果。返回 ok。"""
    mark = "✅" if ok else "❌"
    print(f"  {mark} {label}" + (f"  ({detail})" if detail else ""))
    return ok


def main() -> int:
    print("=" * 60)
    print("Emotion-Echo monorepo · 仓库布局自检（单仓版本）")
    print("=" * 60)
    print(f"ROOT = {ROOT}")
    print()

    all_ok = True

    # 1. .git 存在
    print("[1/7] 主仓 git 初始化")
    git_dir = ROOT / ".git"
    all_ok &= check(".git/ 存在", git_dir.is_dir(), str(git_dir))

    # 2. .gitignore 存在且包含必要条目
    print("\n[2/7] .gitignore 完整性")
    gitignore = ROOT / ".gitignore"
    if check(".gitignore 存在", gitignore.is_file()):
        content = gitignore.read_text(encoding="utf-8")
        required = [
            "node_modules/",
            "*.exe",
            "__pycache__/",
            "*.pb.go",
            "*.pth",
            "deploy/certs/",
        ]
        for token in required:
            all_ok &= check(f"  包含 '{token}'", token in content)
    else:
        all_ok = False

    # 3. docs/git-layout.md 存在
    print("\n[3/7] 布局规范文档")
    layout_doc = ROOT / "docs" / "git-layout.md"
    all_ok &= check("docs/git-layout.md 存在", layout_doc.is_file())

    # 4. .gitmodules **不应存在**
    print("\n[4/7] 单仓约束：.gitmodules 不应存在")
    gitmodules = ROOT / ".gitmodules"
    all_ok &= check(".gitmodules 不存在（单仓）", not gitmodules.exists())

    # 5. 4 个关键目录存在
    print("\n[5/7] 关键目录完整性")
    for d in KEY_DIRS:
        d_path = ROOT / d
        all_ok &= check(f"{d}/ 存在", d_path.is_dir(), str(d_path))

    # 6. docs/ 目录
    print("\n[6/7] docs/ 目录")
    docs_dir = ROOT / "docs"
    all_ok &= check("docs/ 目录存在", docs_dir.is_dir())

    # 7. scripts/ 目录
    print("\n[7/7] scripts/ 目录")
    scripts_dir = ROOT / "scripts"
    all_ok &= check("scripts/ 目录存在", scripts_dir.is_dir())

    # 汇总
    print()
    print("=" * 60)
    if all_ok:
        print("✅ 所有检查通过")
        return 0
    else:
        print("❌ 部分检查未通过，请根据上面 ❌ 项修正")
        if "--strict" in sys.argv:
            return 1
        return 0


if __name__ == "__main__":
    sys.exit(main())