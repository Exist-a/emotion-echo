#!/usr/bin/env python3
"""
check_git_layout.py · Emotion-Echo monorepo 仓库布局自检脚本

校验项目：
  1. 根目录存在 .git/（主仓已 init）
  2. 根目录存在 .gitignore 且包含必要条目
  3. 根目录存在 docs/git-layout.md
  4. 4 个 submodule 已在 .gitmodules 中注册
  5. 4 个 submodule 目录存在且各自有 .git（独立仓）
  6. 根目录存在 docs/ 目录

用法：
  python scripts/check_git_layout.py            # 输出检查结果
  python scripts/check_git_layout.py --strict   # 任一失败 exit 1
"""

from __future__ import annotations

import sys
from pathlib import Path

# 项目根目录（脚本位于 scripts/ 下，父目录即根）
ROOT = Path(__file__).resolve().parent.parent

# 4 个 submodule
SUBMODULES = [
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
    print("Emotion-Echo monorepo · 仓库布局自检")
    print("=" * 60)
    print(f"ROOT = {ROOT}")
    print()

    all_ok = True

    # 1. .git 存在
    print("[1/6] 主仓 git 初始化")
    git_dir = ROOT / ".git"
    all_ok &= check(".git/ 存在", git_dir.is_dir(), str(git_dir))

    # 2. .gitignore 存在且包含必要条目
    print("\n[2/6] .gitignore 完整性")
    gitignore = ROOT / ".gitignore"
    if check(".gitignore 存在", gitignore.is_file()):
        content = gitignore.read_text(encoding="utf-8")
        required = ["node_modules/", "*.exe", "__pycache__/", "*.pb.go", "deploy/certs/"]
        for token in required:
            all_ok &= check(f"  包含 '{token}'", token in content)
    else:
        all_ok = False

    # 3. docs/git-layout.md 存在
    print("\n[3/6] 布局规范文档")
    layout_doc = ROOT / "docs" / "git-layout.md"
    all_ok &= check("docs/git-layout.md 存在", layout_doc.is_file())

    # 4. .gitmodules 注册了 4 个 submodule
    print("\n[4/6] .gitmodules submodule 注册")
    gitmodules = ROOT / ".gitmodules"
    if check(".gitmodules 存在", gitmodules.is_file()):
        content = gitmodules.read_text(encoding="utf-8")
        for sub in SUBMODULES:
            all_ok &= check(f"  注册 '{sub}'", f"[submodule \"{sub}\"]" in content)
    else:
        all_ok = False

    # 5. 4 个 submodule 目录存在
    print("\n[5/6] Submodule 目录与独立 .git")
    for sub in SUBMODULES:
        sub_path = ROOT / sub
        sub_git = sub_path / ".git"
        all_ok &= check(f"{sub}/ 存在", sub_path.is_dir(), str(sub_path))
        all_ok &= check(f"  独立 .git", sub_git.exists())

    # 6. docs/ 目录
    print("\n[6/6] docs/ 目录")
    docs_dir = ROOT / "docs"
    all_ok &= check("docs/ 目录存在", docs_dir.is_dir())

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
        return 0  # 默认不阻断


if __name__ == "__main__":
    sys.exit(main())