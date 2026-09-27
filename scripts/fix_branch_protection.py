#!/usr/bin/env python3
"""scripts/fix_branch_protection.py — 把 main 分支保护的 required_status_checks 对齐到现有 4 个 workflow

为什么需要（反例 AP-11 + E2E-19 PR #93 merge 失败实证）：
    现状：分支保护要求的 status check 名字引用了**已被改名/合并/删除的旧 workflow**
    （commit/statuses legacy 命名空间，27 个 check-runs API 已迁 GitHub Apps，但 23 个
    required 名字仍指向老 workflow 名字）。结果：Check Runs API 27/27 SUCCESS，
    Commit Statuses API 0/0，分支保护说 "23 of 23 expected"，PR #93 merge 被拒。

修复目标：
    把 required_status_checks.contexts 改为 4 个**现存**的 workflow 名（来自
    .github/workflows/），全部无 paths 过滤（D-08 兼容）。workflow 内部的 job
    失败由 PR 作者自行看（不细化到每个 job，避免锁死）。

用法：
    # 1. dry-run（不调 API，只看现状 + 提议修复）：
    python scripts/fix_branch_protection.py --dry-run

    # 2. 实际修复（需要 admin 权限 token）：
    GH_TOKEN=<admin PAT> python scripts/fix_branch_protection.py --apply

    # 3. 只打印 gh CLI 命令（用户复制粘贴到网页端 / 终端执行）：
    python scripts/fix_branch_protection.py --gh-command

退出码：
    0 = dry-run 完成 / apply 成功 / gh-command 已打印
    1 = 现状与期望一致无需修复 / API 调用失败
    2 = token 无权限

参考：
    - scripts/check_required_checks.py（只检查不修；R-03 #16 收口必跑）
    - E2E-19 PR #93：「MCP get_pull_request_status 查旧 Statuses API 恒空是误判」→
      「应修到绿+MCP 自助合并」（commit 4e57456 STATUS.md）
"""

from __future__ import annotations

import argparse
import json
import os
import re
import subprocess
import sys
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
WORKFLOWS_DIR = REPO_ROOT / ".github" / "workflows"


def gh_api(path: str, *, method: str = "GET", body: dict | None = None) -> dict:
    """走 gh CLI（复用其认证）。失败抛 RuntimeError。"""
    cmd = ["gh", "api", path, "--method", method]
    if body is not None:
        cmd += ["--input", "-"]
    try:
        proc = subprocess.run(
            cmd, capture_output=True, text=True, timeout=60,
            input=json.dumps(body) if body is not None else None,
        )
    except FileNotFoundError as e:
        raise RuntimeError("gh CLI 不可用（请先 `gh auth login`）") from e
    if proc.returncode != 0:
        stderr = proc.stderr.strip()[:300]
        # 把"未登录/无 token"与"权限不足/403"区分开（前者=本地无 token 应 SKIP；后者=有 token 但 admin scope 不够）
        if "auth login" in stderr or "GH_TOKEN" in stderr:
            raise RuntimeError(f"NO_TOKEN: {stderr}")
        raise RuntimeError(stderr)
    return json.loads(proc.stdout or "{}")


def current_workflow_names() -> list[str]:
    """从 .github/workflows/*.yml 读出每个 workflow 的顶层 name: 字段。"""
    names: list[str] = []
    for wf in sorted(WORKFLOWS_DIR.glob("*.yml")):
        text = wf.read_text(encoding="utf-8")
        # re.findall + re.MULTILINE 才能在多行匹配每行行首
        hits = re.findall(r"^name:\s*(.+)$", text, re.M)
        if hits:
            names.append(hits[0].strip())
    return names


def check_workflows_have_paths_filter() -> dict[str, bool]:
    """返回 {workflow_name: has_paths_filter}。任何带 paths 过滤的 workflow 必须排除。"""
    out: dict[str, bool] = {}
    for wf in sorted(WORKFLOWS_DIR.glob("*.yml")):
        text = wf.read_text(encoding="utf-8")
        name = re.match(r"^name:\s*(.+)$", text, re.M)
        if not name:
            continue
        # paths: 出现在 events 段（缩进 2 空格，与 push/pull_request 同级）即认为有过滤
        has_paths = bool(re.search(r"^\s{2,}paths:\s*$", text, re.M))
        out[name.group(1).strip()] = has_paths
    return out


def get_current_protection(branch: str = "main") -> dict:
    """读 main 分支保护。"""
    return gh_api(f"repos/:owner/:repo/branches/{branch}/protection")


def compute_target_contexts(current_workflows: list[str],
                            has_paths: dict[str, bool]) -> list[str]:
    """提议的修复目标：所有无 paths 过滤的 workflow 名（同名 status check context）。"""
    return sorted({w for w in current_workflows if not has_paths.get(w, False)})


def diff_contexts(current: list[str], target: list[str]) -> dict:
    return {
        "keep": sorted(set(current) & set(target)),
        "drop": sorted(set(current) - set(target)),
        "add": sorted(set(target) - set(current)),
    }


def dry_run(branch: str) -> int:
    print("=== fix_branch_protection.py — DRY RUN ===")
    print(f"分支：{branch}")
    print()
    try:
        prot = get_current_protection(branch)
    except RuntimeError as e:
        msg = str(e)
        if msg.startswith("NO_TOKEN"):
            # memory「check_required_checks.py:24-26」同型处理：本地无 token 应 SKIP 而非制造假红
            print("SKIP: 当前环境无 GITHUB_TOKEN / GH_TOKEN + gh CLI 未登录。")
            print("      在有 admin 权限的 CI / 终端运行：")
            print("      GH_TOKEN=<admin PAT> python scripts/fix_branch_protection.py --dry-run")
            print("      或：gh auth login 后 python scripts/fix_branch_protection.py --dry-run")
            # 本地仍打印本地能算的（不依赖 GitHub API 的部分）
            workflows = current_workflow_names()
            has_paths = check_workflows_have_paths_filter()
            target = compute_target_contexts(workflows, has_paths)
            print()
            print("--- 本地可计算的 target contexts（基于 .github/workflows/*） ---")
            print(target)
            return 0
        if "403" in msg or "Resource not accessible" in msg:
            print(f"当前 token 无读取分支保护的权限（403）：{msg}")
            print("请用 admin PAT：GH_TOKEN=<admin PAT> python scripts/fix_branch_protection.py --dry-run")
            return 2
        raise

    rsc = prot.get("required_status_checks") or {}
    contexts = rsc.get("contexts") or []
    checks = rsc.get("checks") or []
    print(f"现状：required contexts = {len(contexts)} 个，required checks = {len(checks)} 个")
    print(f"     strict = {rsc.get('strict')}")
    print()
    print("--- 现有 required context 列表 ---")
    for c in contexts:
        print(f"  {c}")
    print()

    has_paths = check_workflows_have_paths_filter()
    print("--- 当前 .github/workflows/* 实际有的 workflow name ---")
    workflows = current_workflow_names()
    for w in workflows:
        flag = "  [paths-filter]" if has_paths.get(w) else ""
        print(f"  {w}{flag}")
    print()

    target = compute_target_contexts(workflows, has_paths)
    print(f"--- 建议的修复目标：required contexts = {target} ---")
    d = diff_contexts(contexts, target)
    print(f"     keep = {d['keep']}")
    print(f"     drop = {d['drop']}  ← 这些是 stale 名字，删除")
    print(f"     add  = {d['add']}  ← 这些是现存 workflow，需要补上")
    print()

    if not d["drop"] and not d["add"]:
        print("✓ 现状与期望一致，无需修改")
        return 0

    print("--- 修复方案 ---")
    print(f"PUT repos/:owner/:repo/branches/{branch}/protection/required_status_checks")
    print("body:")
    print(json.dumps({"strict": True, "contexts": target}, indent=2))
    print()
    print("--- gh CLI 命令（用户复制粘贴执行）---")
    print(
        f'gh api --method PUT repos/:owner/:repo/branches/{branch}/protection/required_status_checks \\\n'
        f"  --input - <<< '"
    )
    print(json.dumps({"strict": True, "contexts": target}))
    print("'")
    print()
    print("⚠  执行此命令需要 admin 权限的 GitHub token / gh auth 登录账号为 repo admin。")
    return 0


def apply_fix(branch: str) -> int:
    print("=== fix_branch_protection.py — APPLY ===")
    workflows = current_workflow_names()
    has_paths = check_workflows_have_paths_filter()
    target = compute_target_contexts(workflows, has_paths)
    print(f"目标 required contexts: {target}")
    print(f"分支：{branch}")
    print()

    try:
        gh_api(
            f"repos/:owner/:repo/branches/{branch}/protection/required_status_checks",
            method="PUT",
            body={"strict": True, "contexts": target},
        )
    except RuntimeError as e:
        if "403" in str(e) or "Resource not accessible" in str(e):
            print(f"FAIL: 当前 token 无修改分支保护的权限（403）: {e}")
            print("       需要 admin PAT + repo admin scope")
            return 2
        print(f"FAIL: API 调用失败: {e}")
        return 1

    print("✓ 已更新分支保护")
    # 验证
    prot = get_current_protection(branch)
    actual = prot.get("required_status_checks", {}).get("contexts", [])
    if sorted(actual) == sorted(target):
        print(f"✓ 验证通过：required contexts = {actual}")
        return 0
    print(f"WARN: API 返回的 contexts 与目标不一致：{actual}")
    return 1


def print_gh_command(branch: str) -> int:
    workflows = current_workflow_names()
    has_paths = check_workflows_have_paths_filter()
    target = compute_target_contexts(workflows, has_paths)
    body = json.dumps({"strict": True, "contexts": target})
    print("# 在有 admin 权限的终端运行（先 gh auth login）：")
    print(
        f"gh api --method PUT repos/:owner/:repo/branches/{branch}/protection/required_status_checks \\\n"
        f"  --input - <<'EOF'\n{body}\nEOF"
    )
    return 0


def main() -> int:
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--branch", default="main")
    p.add_argument("--dry-run", action="store_true",
                   help="读现状 + 计算差异 + 打印修复方案（不修改）")
    p.add_argument("--apply", action="store_true",
                   help="实际 PUT 修改分支保护（需要 admin 权限）")
    p.add_argument("--gh-command", action="store_true",
                   help="只打印 gh CLI 命令给用户复制粘贴")
    args = p.parse_args()

    if args.apply:
        return apply_fix(args.branch)
    if args.gh_command:
        return print_gh_command(args.branch)
    return dry_run(args.branch)


if __name__ == "__main__":
    sys.exit(main())