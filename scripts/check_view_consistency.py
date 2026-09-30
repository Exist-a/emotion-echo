#!/usr/bin/env python3
"""
check_view_consistency.py — 数据库视图定义跨文件 diff 工具

背景（Round 1.4 / P2-R2-10）：
原 `emotion_echo_ai.daily_emotion_v` 在 3 处文件定义（deploy/db/04-create-views.sql
+ emotion-echo-analytics-svc/migrations/a001 + emotion-echo-ai-svc/migrations/i005），
任意一处改了 SELECT 字段，另两处漂移；smoke / 报表读到的口径就与代码注释不符。

本工具扫描全仓 *.sql 找出所有 `CREATE OR REPLACE VIEW ... AS` 块，对比相同
view 名在不同文件中的定义，输出 diff（如有）。CI 阶段跑：发现 diff 即 fail。

用法：
    python scripts/check_view_consistency.py [--verbose]

输出：
    PASS: 所有同名 view 在所有文件中定义一致
    FAIL: <view_name> 在 <file_a>:<line> vs <file_b>:<line> 字段漂移
"""
from __future__ import annotations

import argparse
import re
import sys
from collections import defaultdict
from pathlib import Path

# 匹配 `CREATE OR REPLACE VIEW schema.view_name AS` (单行) 至下一个 `;` 结束的 SQL 块
# 多行：开头在第一行，结尾在 `;` 单独成行
VIEW_PATTERN = re.compile(
    r"CREATE\s+(OR\s+REPLACE\s+)?VIEW\s+(?P<qualified>\w+\.\w+)\s+AS\s*",
    re.IGNORECASE | re.MULTILINE,
)


def extract_views(sql_path: Path) -> dict[str, tuple[int, str]]:
    """从 .sql 文件抽取所有 view 定义。返回 {view_name: (start_line, body)}。"""
    try:
        text = sql_path.read_text(encoding="utf-8")
    except (UnicodeDecodeError, OSError):
        return {}

    views: dict[str, tuple[int, str]] = {}
    for m in VIEW_PATTERN.finditer(text):
        qualified = m.group("qualified")
        # 注释行（-- 开头）跳过
        line_start = text[: m.start()].count("\n") + 1
        line_text = text.splitlines()[line_start - 1]
        if line_text.lstrip().startswith("--"):
            continue

        # 找 SQL 块结尾：下一个 `;` 单独成行
        # 朴素实现：找 ; 后面跟换行或文件结尾
        end = m.end()
        body_chars: list[str] = []
        while end < len(text):
            c = text[end]
            if c == ";":
                # 确认 ; 后面是空白或换行或注释
                after = text[end + 1 : end + 3]
                if after.startswith("\n") or after.strip().startswith("--") or after == "":
                    body_chars.append(c)
                    end += 1
                    break
            body_chars.append(c)
            end += 1

        body = "".join(body_chars).strip()
        # 规范化：去注释 + 多余空白，让 diff 聚焦 schema 而非格式
        normalized = re.sub(r"--[^\n]*", "", body)  # 去行注释
        normalized = re.sub(r"\s+", " ", normalized).strip()
        views[qualified] = (line_start, normalized)

    return views


def scan_repo(repo_root: Path) -> dict[str, list[tuple[Path, int, str]]]:
    """全仓扫描所有 .sql 文件，按 view 名聚合定义。"""
    by_view: dict[str, list[tuple[Path, int, str]]] = defaultdict(list)
    skip_dirs = {".git", "node_modules", "vendor", "legacy"}

    for sql_path in repo_root.rglob("*.sql"):
        if any(part in skip_dirs for part in sql_path.parts):
            continue
        # 排除 client-side SQL（前端 / mock）
        if "mock" in sql_path.parts or "fixture" in sql_path.parts:
            continue

        for view_name, (line, body) in extract_views(sql_path).items():
            by_view[view_name].append((sql_path, line, body))

    return by_view


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--verbose",
        action="store_true",
        help="打印所有 view 定义（包括 PASS 的）",
    )
    parser.add_argument(
        "--repo-root",
        default=".",
        help="仓库根目录（默认当前目录）",
    )
    args = parser.parse_args()

    repo_root = Path(args.repo_root).resolve()
    by_view = scan_repo(repo_root)

    if not by_view:
        print("[FAIL] 未找到任何 view 定义 — 检查 *.sql 扫描路径")
        return 1

    fail_count = 0
    for view_name, occurrences in sorted(by_view.items()):
        # 跳过单点定义（无需 diff）
        if len(occurrences) <= 1:
            if args.verbose:
                p, ln, _ = occurrences[0]
                print(f"[PASS] {view_name}  单点定义  {p.relative_to(repo_root)}:{ln}")
            continue

        # 多点定义：必须完全一致
        bodies = {body for _, _, body in occurrences}
        if len(bodies) == 1:
            if args.verbose:
                paths = ", ".join(
                    f"{p.relative_to(repo_root)}:{ln}" for p, ln, _ in occurrences
                )
                print(f"[PASS] {view_name}  {len(occurrences)} 处定义一致  ({paths})")
            continue

        # FAIL：多点定义但不一致
        fail_count += 1
        print(f"[FAIL] {view_name}  {len(occurrences)} 处定义不一致:")
        for p, ln, body in occurrences:
            rel = p.relative_to(repo_root)
            print(f"  - {rel}:{ln}")
            # 打印前 100 字符作为片段
            snippet = body[:100].replace("\n", " ")
            print(f"    {snippet}{'...' if len(body) > 100 else ''}")
        print()

    if fail_count:
        print(f"[FAIL] {fail_count} 个 view 跨文件定义漂移 — 修复方案：")
        print("  1. 选一个权威文件保留（通常运行时 migrations 优先，deploy/db 退化为 dev-only）")
        print("  2. 在其他文件加注释：'权威源在 <file>:<line>，本文件不重复定义'")
        print("  3. 跑此脚本确认全部 PASS")
        return 1

    total_views = len(by_view)
    multi_point = sum(1 for v in by_view.values() if len(v) > 1)
    print(f"[PASS] {total_views} 个 view 全部一致  （{multi_point} 多点 + {total_views - multi_point} 单点）")
    return 0


if __name__ == "__main__":
    sys.exit(main())
