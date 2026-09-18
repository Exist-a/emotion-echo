#!/usr/bin/env python3
"""scripts/check_required_checks.py —— 门禁接线检查（RUNBOOK §13.3 断言 #16）

为什么需要（反例 AP-11「门禁只报不拦」）：
    仓库文档反复出现"任何 test 失败 → PR 不可 merge"这类表述，但实测
    `branches/main/protection.required_status_checks.contexts` 曾长期为**空集**
    —— 即 CI 红根本不影响合并。文档说的和仓库实际做的是两回事。
    这条断言把它变成机械可检。

检查项：
    1. required_status_checks 非空（否则 CI 全红也拦不住任何东西）。
    2. 每个 required context 必须对应一个**没有 paths 过滤**的 job
       （依据 D-08：有 paths 过滤的 job 只在相关文件变更时运行，
        若把它设为 required，无关 PR 会永远等不到该 check ⇒ 锁死）。
    3. 反向提示：无 paths 过滤 workflow 里的 job 未纳入 required 的列出（WARN）。

用法：
    python scripts/check_required_checks.py
退出码：
    0 = 通过（或缺少 token 而跳过，见下）
    1 = 未接线条目 / 配置矛盾
    2 = API 调用失败

关于 token：本检查需要读 GitHub API，故依赖 GITHUB_TOKEN / GH_TOKEN。
    无 token 时**明确跳过**（打印 SKIP）并返回 0 —— CI 上 token 必然存在，
    本地无 token 时不应因"无法访问网络"而制造假红。跳过是显式的，不静默。
"""

from __future__ import annotations

import json
import os
import re
import subprocess
import sys
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
WORKFLOWS_DIR = REPO_ROOT / ".github" / "workflows"


def gh_api(path: str) -> dict:
    """走 gh CLI（复用其认证）。失败抛 RuntimeError。"""
    try:
        out = subprocess.run(
            ["gh", "api", path],
            capture_output=True, text=True, timeout=60, cwd=str(REPO_ROOT),
        )
    except FileNotFoundError as e:
        raise RuntimeError("gh CLI 不可用") from e
    if out.returncode != 0:
        raise RuntimeError(f"gh api {path} 失败: {out.stderr.strip()[:200]}")
    return json.loads(out.stdout or "{}")


def workflow_jobs_and_paths() -> dict[str, dict[str, bool]]:
    """返回 {job_name: {"has_paths_filter": bool}}（遍历所有 workflow）。

    用文本解析而非 PyYAML：只需判断 `on.push/pull_request` 下是否出现 `paths:`，
    以及 `jobs.<name>` 的名字。避免为一条检查引入 YAML 依赖。
    """
    jobs: dict[str, dict[str, bool]] = {}
    for wf in sorted(WORKFLOWS_DIR.glob("*.yml")):
        text = wf.read_text(encoding="utf-8")
        # `paths:` 出现在 events 段（缩进 4 空格）即认为该 workflow 有路径过滤
        has_paths = bool(re.search(r"^\s{4,}paths:\s*$", text, re.M))
        # 收集 job 名：jobs: 之后缩进 2 空格、以 `:` 结尾的键
        m = re.search(r"^jobs:\s*$", text, re.M)
        names: list[str] = []
        if m:
            body = text[m.end():]
            for line in body.splitlines():
                if re.match(r"^[a-zA-Z]", line):  # 顶格 ⇒ jobs 段结束
                    break
                mm = re.match(r"^  ([A-Za-z0-9_-]+):\s*$", line)
                if mm:
                    names.append(mm.group(1))
        # 取每个 job 的 display name（`name:` 字段），GitHub 的 check context 用它
        for jn in names:
            disp = jn
            dm = re.search(
                rf"^  {re.escape(jn)}:\s*(?:#[^\n]*)?\n(?:.*\n)*?^    name:\s*(.+)$",
                text, re.M,
            )
            if dm:
                disp = dm.group(1).strip().strip('"').strip("'")
            row = {"has_paths_filter": has_paths, "workflow": wf.name, "job_key": jn}
            jobs[disp] = row
            # 矩阵 job：GitHub 的 check context 形如 "test (emotion-echo-ai-svc)"，
            # 故把 matrix 的取值展开成同名副本，避免误报"找不到同名 job"。
            for val in _matrix_values(text, jn):
                jobs[f"{disp} ({val})"] = row
    return jobs


def _matrix_values(text: str, job_key: str) -> list[str]:
    """抽取某 job 下 `matrix:` 段里的取值列表（形如 `- xxx`）。"""
    m = re.search(rf"^  {re.escape(job_key)}:\s*$", text, re.M)
    if not m:
        return []
    body = text[m.end():]
    # 截到下一个 job（缩进 2 空格的键）为止
    end = re.search(r"^  [A-Za-z0-9_-]+:\s*$", body, re.M)
    if end:
        body = body[: end.start()]
    vals: list[str] = []
    in_matrix = False
    for line in body.splitlines():
        if re.match(r"^\s+matrix:\s*$", line):
            in_matrix = True
            continue
        if in_matrix:
            mm = re.match(r"^\s+-\s+(\S+)\s*$", line)
            if mm:
                vals.append(mm.group(1))
            elif re.match(r"^\s+[a-zA-Z0-9_-]+:\s*", line):
                continue  # matrix 内的键（如 include:）
            elif line.strip() and not line.startswith(" " * 8):
                break
    return vals


def main() -> int:
    print("=== 门禁接线检查（required_status_checks）===")

    if not (os.environ.get("GITHUB_TOKEN") or os.environ.get("GH_TOKEN")):
        print("SKIP: 无 GITHUB_TOKEN / GH_TOKEN，无法读 GitHub API。")
        print("      （CI 上 token 必然存在，故该检查在 CI 会真实执行；本地跳过不视为失败）")
        return 0

    try:
        prot = gh_api("repos/{owner}/{repo}/branches/main/protection")
    except RuntimeError as e:
        msg = str(e)
        if "404" in msg:
            print(f"FAIL: main 分支无保护规则 → CI 失败拦不住任何东西（AP-11）：{msg}")
            return 1
        print(f"退出码 2：{msg}")
        return 2

    rsc = prot.get("required_status_checks") or {}
    contexts = rsc.get("contexts") or []
    # 新式 checks 字段（workflow 级）与 contexts（job 级）至少一个非空
    checks = rsc.get("checks") or []
    print(f"required contexts: {len(contexts)} | required checks: {len(checks)} | strict: {rsc.get('strict')}")

    if not contexts and not checks:
        print("FAIL: required_status_checks 为空集 ⇒ CI 红不影响合并（AP-11 复活）")
        print("      修法：把**无 paths 过滤**的 job 纳入 required（见 D-08）")
        return 1

    jobs = workflow_jobs_and_paths()
    # contexts 与 checks 在多数情况下是同一批（旧/新两种表示），去重
    required = sorted({c for c in list(contexts) + [c.get("context") for c in checks if isinstance(c, dict)] if c})

    bad = [c for c in required if c in jobs and jobs[c]["has_paths_filter"]]
    unknown = [c for c in required if c not in jobs]

    ok = True
    if bad:
        ok = False
        print("FAIL: 以下 required check 所属 workflow 带 paths 过滤 —— 无关 PR 永远等不到它，会锁死合并（D-08 禁止）：")
        for c in bad:
            print(f"       {c}  ← {jobs[c]['workflow']}（paths 过滤）")
    if unknown:
        print("WARN: 以下 required check 未在 .github/workflows 找到同名 job（可能是已改名/已删，检查会永远 pending）：")
        for c in unknown:
            print(f"       {c}")

    missing = [j for j, meta in jobs.items() if not meta["has_paths_filter"] and j not in required]
    if missing:
        print(f"WARN: {len(missing)} 个无 paths 过滤的 job 未纳入 required（不拦）：")
        for j in missing[:8]:
            print(f"       {j}")
        if len(missing) > 8:
            print(f"       … 共 {len(missing)} 个")

    if not ok:
        return 1
    print("GREEN: required_status_checks 非空，且没有 paths 过滤 job 被误列为 required")
    return 0


if __name__ == "__main__":
    sys.exit(main())
