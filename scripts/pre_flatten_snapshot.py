#!/usr/bin/env python3
"""
pre_flatten_snapshot.py · 合并前快照脚本

在把 4 个 submodule 转为普通目录之前，记录当前状态：
  - 4 个目录的 git 子模块信息（commit SHA、当前文件数）
  - 各目录的 .git 是否存在
  - 主仓关键文件清单

产物：docs/flatten-snapshot-<timestamp>.json

用法：
  python scripts/pre_flatten_snapshot.py
"""

from __future__ import annotations

import json
import subprocess
import sys
from datetime import datetime
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
DOCS = ROOT / "docs"

SUBMODULE_PATHS = [
    "Emotion-Echo-Web",
    "legacy/emotion-echo-gin",
    "Emotion-Echo-LLM/sensevoice-small",
    "Emotion-Echo-LLM/XTTS/TTS",
]


def count_files(p: Path) -> int:
    """统计目录中文件数（不含 .git）"""
    if not p.exists():
        return 0
    return sum(1 for _ in p.rglob("*") if _.is_file() and ".git" not in _.parts)


def git(args: list[str]) -> str:
    return subprocess.check_output(
        ["git", "-C", str(ROOT)] + args,
        stderr=subprocess.STDOUT,
    ).decode("utf-8", errors="replace").strip()


def main() -> int:
    print("=" * 60)
    print("Emotion-Echo · 单仓合并前快照")
    print("=" * 60)

    # 1. 主仓 commit 信息
    print("\n[1/3] 主仓状态")
    head = git(["rev-parse", "HEAD"])
    head_short = git(["rev-parse", "--short", "HEAD"])
    branch = git(["branch", "--show-current"])
    print(f"  branch: {branch}")
    print(f"  HEAD:   {head_short} ({head})")

    # 2. submodule 状态
    print("\n[2/3] Submodule 状态")
    sm_status_raw = git(["submodule", "status"])
    submodules = []
    for line in sm_status_raw.splitlines():
        # 格式: " <sha> <path> (<desc>)"
        parts = line.strip().split(maxsplit=2)
        if len(parts) >= 2:
            sha, path = parts[0], parts[1]
            desc = parts[2] if len(parts) > 2 else ""
            submodules.append({
                "path": path,
                "sha": sha,
                "desc": desc.strip("()"),
            })

    for sm in submodules:
        p = ROOT / sm["path"]
        git_dir = p / ".git"
        file_count = count_files(p)
        sm["has_inner_git"] = git_dir.exists()
        sm["file_count"] = file_count
        print(f"  {sm['path']}")
        print(f"    sha: {sm['sha']}  files: {file_count}  inner_git: {sm['has_inner_git']}")

    # 3. 写快照 JSON
    print("\n[3/3] 写快照文件")
    DOCS.mkdir(parents=True, exist_ok=True)
    ts = datetime.now().strftime("%Y%m%d-%H%M%S")
    snapshot_path = DOCS / f"flatten-snapshot-{ts}.json"

    snapshot = {
        "created_at": datetime.now().isoformat(timespec="seconds"),
        "branch": branch,
        "head": head,
        "head_short": head_short,
        "submodules": submodules,
        "note": "P0-C flattening: 4 submodule → 普通目录",
    }
    snapshot_path.write_text(
        json.dumps(snapshot, indent=2, ensure_ascii=False),
        encoding="utf-8",
    )
    print(f"  ✓ {snapshot_path.relative_to(ROOT)}")

    # 4. 总结
    total_files = sum(sm["file_count"] for sm in submodules)
    print(f"\n合计：4 个 submodule / {total_files} 个文件将被转为普通目录")

    # 5. 列出迁移后的根目录布局
    print("\n迁移后根目录将包含（部分）：")
    print("  📁 Emotion-Echo-Web/              (普通目录)")
    print("  📁 legacy/emotion-echo-gin/        (普通目录)")
    print("  📁 Emotion-Echo-LLM/sensevoice-small/  (普通目录)")
    print("  📁 Emotion-Echo-LLM/XTTS/TTS/     (普通目录)")
    print("  📄 .gitmodules                      (将被删除)")
    print("  📄 docs/git-layout.md              (将被改写)")

    return 0


if __name__ == "__main__":
    sys.exit(main())