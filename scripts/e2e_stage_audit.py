#!/usr/bin/env python3
"""E2E 阶段收口审计器（MVA —— 最小可用版）

用途：机械校验 E2E 阶段是否真的满足收口契约，替代"执行者自己宣布完成"。
依据：docs/e2e-roadmap/RUNBOOK.md §13「收口审计」+
      docs/e2e-roadmap/anti-patterns.md（14 类已真实发生的失真反例）

MVA 实现 11 条断言（对应 docs/e2e-roadmap/remediation.md §R-00 + §R-03 #1）：
  A1  report.md 存在且含必填章节                     (AP-12)
  A2  汇总行非占位符，且计数 == 测试点表行数          (AP-12)
  A3  plan 的测试点编号集合 ⊆ report 的编号集合       (AP-07)
  A4  证据列不含"存在性措辞"（已创建/已新增/...）      (AP-01/02)
  A5  属本阶段且未解决的 E2E-F-xx 存在时，状态不得 done (AP-04)
  A6  收口自检项无 [x]+"待…"                          (AP-06)
  A7  [V] 点有截图证据                                (AP-01)
  A8  账本编号连续                                    (AP-14)
  A9  plan/roadmap/report 三处 status 一致            (AP-07)
  A10 相对链接可达                                    (AP-12)
  A11 四值合法基值（PASS/FAIL/BLOCKED/N/A）           (AP-12)

设计约束（RUNBOOK §13.4）：
  - 必须能复现已知缺口（--selftest 用 E2E-03/04/05 做回归样本）
  - 不得"只会喊狼来了"（对最接近合规的 E2E-02 不应误报）
  - 输出机器可读（--json），退出码可被 CI 消费

用法：
  python scripts/e2e_stage_audit.py --stage e2e-04
  python scripts/e2e_stage_audit.py --all
  python scripts/e2e_stage_audit.py --selftest
  python scripts/e2e_stage_audit.py --all --json
"""

from __future__ import annotations

import argparse
import json
import re
import sys
from dataclasses import dataclass, field
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
ROADMAP = ROOT / "docs/e2e-roadmap/roadmap.md"
LEDGER = ROOT / "docs/e2e-roadmap/discovered-unresolved.md"
STAGES_DIR = ROOT / "docs/e2e-roadmap/stages"

# A1：report.md 的必填章节（硬性）与建议章节（软性/警告）
REQUIRED_SECTIONS = ["测试点", "收口自检"]
RECOMMENDED_SECTIONS = ["环境基线", "发现", "修复清单", "回归钉", "待决策"]

# A4：存在性措辞 —— 只证明"文件存在"，未证明"行为正确"
EXISTENCE_PHRASES = [
    "已创建", "已新增", "已完成创建", "已配置", "已实现", "已落地",
    "已添加", "已接入完成",
]

# A4b：证据若"只有文件路径、没有任何执行信号"，同样不算有效证据（RUNBOOK §4.1）
# 执行信号：出现任一即视为"记录了执行结果"
EXECUTION_SIGNALS = [
    "输出", "退出码", "断言", "日志", "→", "->", "PASS", "FAIL", "结果",
    "数", "stdout", "查询", "截图", "对比", "diff", "HTTP",
]
EXECUTION_SIGNALS_LOWER = [s.lower() for s in EXECUTION_SIGNALS]
PATH_TOKEN_RE = re.compile(r"`[^`]*\.(?:sh|py|yml|yaml|md|ts|js|vue|go|sql|json)`")

# 账本中表示"已了结"的状态关键词（不含"待修复""部分解决"——那些仍算未了结）
RESOLVED_MARKERS = [
    "已解决", "已修复", "全部修复", "降级并记录", "已知限制", "不归属阶段",
]

# 汇总行里的占位符（未填实数的证据）
PLACEHOLDER_RE = re.compile(r"PASS\s+(?!\d)(\w+)", re.IGNORECASE)


@dataclass
class Finding:
    check: str
    level: str  # FAIL | WARN
    message: str


@dataclass
class StageResult:
    stage: str
    slug: str
    findings: list[Finding] = field(default_factory=list)

    @property
    def failed(self) -> list[Finding]:
        return [f for f in self.findings if f.level == "FAIL"]

    @property
    def warned(self) -> list[Finding]:
        return [f for f in self.findings if f.level == "WARN"]


# ---------------------------------------------------------------- 解析辅助


def read(path: Path) -> str:
    return path.read_text(encoding="utf-8") if path.exists() else ""


def section_lines(text: str, heading_keyword: str) -> str:
    """取 '## ...<heading_keyword>...' 到下一个 '## ' 之间的内容。"""
    out, capturing = [], False
    for line in text.splitlines():
        if line.startswith("## "):
            capturing = heading_keyword in line
            continue
        if capturing:
            out.append(line)
    return "\n".join(out)


def table_rows(block: str, first_col_is_int: bool = False) -> list[list[str]]:
    """解析 markdown 表格的数据行（跳过表头与分隔行）。"""
    rows = []
    for line in block.splitlines():
        s = line.strip()
        if not s.startswith("|"):
            continue
        cells = [c.strip() for c in s.strip("|").split("|")]
        if not cells:
            continue
        if set(cells[0]) <= set("-: "):  # 分隔行
            continue
        if first_col_is_int:
            if not re.fullmatch(r"\d+", cells[0]):
                continue
        else:
            if cells[0] in ("#", "编号", "阶段"):
                continue
        rows.append(cells)
    return rows


def col_index(header_cells: list[str], keyword: str) -> int | None:
    for i, c in enumerate(header_cells):
        if keyword in c:
            return i
    return None


def find_header(block: str, must_contain: str) -> list[str] | None:
    for line in block.splitlines():
        s = line.strip()
        if s.startswith("|") and must_contain in s:
            return [c.strip() for c in s.strip("|").split("|")]
    return None


def parse_plan_testpoints(text: str) -> set[str]:
    block = section_lines(text, "测试点清单")
    return {r[0] for r in table_rows(block, first_col_is_int=True)}


def parse_report_testpoints(text: str) -> tuple[set[str], list[list[str]], list[str] | None]:
    block = section_lines(text, "测试点结果") or section_lines(text, "测试点")
    rows = table_rows(block, first_col_is_int=True)
    header = find_header(block, "测试点")
    return {r[0] for r in rows}, rows, header


def parse_roadmap_states() -> dict[str, str]:
    """roadmap 排期总表 → {e2e-NN: 原始状态文本}

    注意：必须只在「排期总表」章节内解析——文件后面的「详档约定」表也有 E2E-NN 行，
    会把状态覆盖成 plan 路径。行首可能带标记（如 `E2E-04 🔧`），故不能 fullmatch。
    """
    text = read(ROADMAP)
    start = text.find("## 排期总表")
    end = text.find("\n## ", start + 1) if start != -1 else -1
    scope = text[start:end] if start != -1 and end != -1 else text

    states: dict[str, str] = {}
    for line in scope.splitlines():
        s = line.strip()
        if not s.startswith("|"):
            continue
        cells = [c.strip() for c in s.strip("|").split("|")]
        m = re.match(r"E2E-(\d+)(?:\s|$)", cells[0])
        if m and len(cells) >= 5:
            states[f"e2e-{m.group(1)}"] = cells[-1]
    return states


def roadmap_state_kind(raw: str) -> str:
    """从 roadmap 状态单元格判定状态种类。

    注意：不能用"全文是否含某关键词"——状态单元格里描述瑕疵时可能**提到**这些词
    （如 E2E-02 的 `✅ done（…唯一瑕疵：#6 可验证却标 BLOCKED）` 含 "BLOCKED"，
    若按全文匹配会被误判为 blocked）。故按优先级 + 限定措辞判定。
    """
    low = raw.lower()
    if "partial" in low:
        return "partial"
    if raw.lstrip().startswith("⏸") or "blocked by" in low:
        return "blocked"
    if low.lstrip().startswith("⏳") or re.match(r"^\s*pending", low):
        return "pending"
    if "done" in low:
        return "done"
    return "pending"


def parse_ledger() -> list[dict[str, str]]:
    """账本 → 条目列表（id / 归属 / 状态）"""
    entries = []
    for line in read(LEDGER).splitlines():
        s = line.strip()
        if not s.startswith("| E2E-F-"):
            continue
        cells = [c.strip() for c in s.strip("|").split("|")]
        if len(cells) < 6:
            continue
        entries.append({
            "id": cells[0],
            "summary": cells[2] if len(cells) > 2 else "",
            "owner": cells[-2],
            "status": cells[-1],
        })
    return entries


def stage_mentions(cell: str) -> set[str]:
    return {f"e2e-{n}" for n in re.findall(r"E2E-(\d+)", cell)}


# ---------------------------------------------------------------- 检查项


def check_a1(text: str, res: StageResult) -> None:
    headings = [l for l in text.splitlines() if l.startswith("## ")]
    joined = "\n".join(headings)
    for kw in REQUIRED_SECTIONS:
        if kw not in joined:
            res.findings.append(Finding("A1", "FAIL", f"report 缺必填章节「{kw}」"))
    for kw in RECOMMENDED_SECTIONS:
        if kw not in joined:
            res.findings.append(Finding("A1", "WARN", f"report 缺建议章节「{kw}」"))


def check_a2(text: str, rows: list[list[str]], res: StageResult) -> None:
    m = re.search(r"汇总[：:]\s*(.+)", text)
    if not m:
        res.findings.append(Finding("A2", "FAIL", "report 无汇总行"))
        return
    line = m.group(1)
    ph = PLACEHOLDER_RE.search(line)
    if ph:
        res.findings.append(
            Finding("A2", "FAIL", f"汇总行为未填实数的占位符：{line.strip()[:70]}")
        )
        return
    nums = {k: int(v) for k, v in re.findall(r"(PASS|FAIL|BLOCKED|N/A)\s*(\d+)", line)}
    if not nums:
        res.findings.append(Finding("A2", "FAIL", f"汇总行无法解析计数：{line.strip()[:70]}"))
        return
    total = sum(nums.values())
    if total != len(rows):
        res.findings.append(
            Finding(
                "A2", "FAIL",
                f"汇总计数合计 {total} 与测试点表行数 {len(rows)} 不一致（{nums}）",
            )
        )


def check_a3(plan_text: str, report_nums: set[str], res: StageResult) -> None:
    plan_nums = parse_plan_testpoints(plan_text)
    if not plan_nums:
        res.findings.append(Finding("A3", "WARN", "plan 未解析到测试点编号，跳过 A3"))
        return
    missing = sorted(plan_nums - report_nums, key=int)
    if missing:
        res.findings.append(
            Finding(
                "A3", "FAIL",
                f"plan 共 {len(plan_nums)} 个测试点，report 只覆盖 {len(report_nums)} 个；"
                f"缺失编号：{','.join(missing[:15])}"
                + ("…" if len(missing) > 15 else ""),
            )
        )


def row_claims_pass(row: list[str], header: list[str]) -> bool:
    """该行是否在「结果」列**声称通过**。

    A4 的判定前提是"该行声称成功、但证据只证明了文件存在"。若该行结果本身
    就是 FAIL / 未验证 / BLOCKED / N/A，那证据里的存在性措辞（如"脚本已创建
    **但**产物不存在"）是在**诚实陈述缺口**，不是假 PASS。

    不加这条前提，审计器会反过来惩罚诚实的报告（E2E-F-64：E2E-04 的报告已被
    R-02 如实改写为"❌ 假 PASS / ⚠️ 未验证"，却仍被 A4 按关键词判 FAIL，
    且该误报还被写进了 SELFTEST_EXPECT 固化下来）。

    取不到「结果」列时保守返回 True（宁可报，不可漏）。
    """
    idx = col_index(header, "结果")
    if idx is None or idx >= len(row):
        return True
    cell = row[idx].strip()
    low = cell.lower()
    NEGATIVE_MARKERS = (
        "fail", "未验证", "未做", "未运行", "未执行", "未补",
        "假 pass", "blocked", "n/a", "❌", "⚠️",
    )
    if any(m in low for m in NEGATIVE_MARKERS):
        return False
    return "pass" in low or "通过" in cell


def check_a4(rows: list[list[str]], header: list[str] | None, res: StageResult) -> None:
    if not header:
        res.findings.append(Finding("A4", "WARN", "report 测试点表无表头，跳过 A4"))
        return
    idx = col_index(header, "证据")
    if idx is None:
        res.findings.append(Finding("A4", "FAIL", "report 测试点表缺「证据」列"))
        return

    phrase_hits, path_only_hits = [], []
    for r in rows:
        if idx >= len(r):
            continue
        # 只审"声称通过"的行：非通过行里的存在性措辞是在陈述缺口（见 row_claims_pass）
        if not row_claims_pass(r, header):
            continue
        ev = r[idx]
        for phrase in EXISTENCE_PHRASES:
            if phrase in ev:
                phrase_hits.append(f"#{r[0]}「{ev[:36]}」含「{phrase}」")
                break
        else:
            # 证据里出现文件路径，却没有任何执行信号 ⇒ 只证明文件存在
            if PATH_TOKEN_RE.search(ev) and not any(
                sig in ev.lower() for sig in EXECUTION_SIGNALS_LOWER
            ):
                path_only_hits.append(f"#{r[0]}「{ev[:44]}」")

    if phrase_hits:
        res.findings.append(
            Finding(
                "A4", "FAIL",
                f"{len(phrase_hits)} 个测试点的证据含存在性措辞（只证明文件存在）："
                + "；".join(phrase_hits[:4]) + ("…" if len(phrase_hits) > 4 else ""),
            )
        )
    if path_only_hits:
        res.findings.append(
            Finding(
                "A4", "FAIL",
                f"{len(path_only_hits)} 个测试点的证据只有文件路径、无执行信号"
                f"（须补可复现命令 + 实际输出，见 RUNBOOK §4.1）："
                + "；".join(path_only_hits[:4]) + ("…" if len(path_only_hits) > 4 else ""),
            )
        )


def check_a5(stage: str, state_kind: str, ledger: list[dict[str, str]], res: StageResult) -> None:
    if state_kind != "done":
        return
    conflicts = []
    for e in ledger:
        if any(m in e["status"] for m in RESOLVED_MARKERS):
            continue
        if stage in stage_mentions(e["owner"]):
            conflicts.append(f"{e['id']}（{e['status'][:24]}）")
    if conflicts:
        res.findings.append(
            Finding(
                "A5", "FAIL",
                f"阶段标 done，但账本有 {len(conflicts)} 条归属本阶段的未解决条目："
                + "、".join(conflicts[:6]) + ("…" if len(conflicts) > 6 else ""),
            )
        )


def check_a6(text: str, res: StageResult) -> None:
    """A6: 收口自检项不能勾选但留"待"字（[x] + 待 = 假完成）"""
    section = section_lines(text, "收口自检")
    if not section:
        return
    for line in section.splitlines():
        s = line.strip()
        if re.match(r"-\s*\[x\]", s) and "待" in s:
            res.findings.append(
                Finding("A6", "FAIL", f"收口自检项已勾选但含「待」字：{s[:60]}")
            )


def check_a7(text: str, stage_dir: Path, res: StageResult) -> None:
    """A7: [V] 视觉测试点必须有截图证据"""
    block = section_lines(text, "测试点结果") or section_lines(text, "测试点")
    rows = table_rows(block, first_col_is_int=True)
    header = find_header(block, "测试点")
    if not header:
        return

    # 找判定列和证据列
    judge_idx = col_index(header, "判定")
    evidence_idx = col_index(header, "证据")
    if judge_idx is None or evidence_idx is None:
        return

    # 检查截图目录
    screenshots_dir = stage_dir / "screenshots"
    has_screenshots = screenshots_dir.exists() and any(screenshots_dir.iterdir())

    for r in rows:
        if judge_idx >= len(r) or evidence_idx >= len(r):
            continue
        if "[V]" in r[judge_idx]:
            # 视觉测试点需要截图证据
            if not has_screenshots and "截图" not in r[evidence_idx].lower():
                res.findings.append(
                    Finding(
                        "A7", "WARN",
                        f"#{r[0]} 含 [V] 判定但无截图证据（screenshots/ 目录为空或不存在）",
                    )
                )


def check_a8(ledger: list[dict[str, str]], res: StageResult) -> None:
    """A8: 账本编号连续（E2E-F-xx 不能跳号）"""
    ids = []
    for e in ledger:
        m = re.match(r"E2E-F-(\d+)", e["id"])
        if m:
            ids.append(int(m.group(1)))
    if not ids:
        return
    ids.sort()
    expected = set(range(ids[0], ids[-1] + 1))
    missing = sorted(expected - set(ids))
    if missing:
        res.findings.append(
            Finding(
                "A8", "WARN",
                f"账本编号不连续，缺失：{','.join(str(n) for n in missing[:10])}",
            )
        )


def check_a9(stage: str, stage_dir: Path, states: dict[str, str], res: StageResult) -> None:
    """A9: plan/roadmap/report 三处 status 一致"""
    roadmap_kind = roadmap_state_kind(states.get(stage, "pending"))

    # 读 plan status
    plan_path = stage_dir / "plan.md"
    plan_status = "pending"
    if plan_path.exists():
        plan_text = read(plan_path)
        m = re.search(r"status:\s*(\w+)", plan_text)
        if m:
            plan_status = m.group(1).lower()

    # 读 report status
    report_path = stage_dir / "report.md"
    report_status = "pending"
    if report_path.exists():
        report_text = read(report_path)
        m = re.search(r"status:\s*(\w+)", report_text)
        if m:
            report_status = m.group(1).lower()

    # 比较三处状态
    statuses = {"roadmap": roadmap_kind, "plan": plan_status, "report": report_status}
    unique = set(statuses.values())
    if len(unique) > 1:
        # 允许 pending 和 partial 混合（未开工阶段）
        if not (unique <= {"pending", "partial"}):
            res.findings.append(
                Finding(
                    "A9", "FAIL",
                    f"三处 status 不一致：{statuses}",
                )
            )


def check_a10(stage_dir: Path, res: StageResult) -> None:
    """A10: report 中的相对链接可达"""
    report_path = stage_dir / "report.md"
    if not report_path.exists():
        return
    report_text = read(report_path)
    # 匹配相对链接 [text](path)
    for m in re.finditer(r"\[([^\]]*)\]\(([^)]+)\)", report_text):
        link = m.group(2)
        # 跳过绝对链接和锚点
        if link.startswith("http") or link.startswith("#"):
            continue
        # 解析相对路径
        target = (stage_dir / link).resolve()
        if not target.exists():
            res.findings.append(
                Finding("A10", "WARN", f"report 链接不可达：{link}")
            )


def check_a11(rows: list[list[str]], header: list[str] | None, res: StageResult) -> None:
    """A11: 测试点结果列必须是合法基值（PASS/FAIL/BLOCKED/N/A）"""
    if not header:
        return
    result_idx = col_index(header, "结果")
    if result_idx is None:
        return

    VALID_RESULTS = {"PASS", "FAIL", "BLOCKED", "N/A", "⚠️"}
    for r in rows:
        if result_idx >= len(r):
            continue
        val = r[result_idx].strip()
        if val and val not in VALID_RESULTS:
            # 允许带表情符号的变体
            clean = re.sub(r"[⚠️❌✅]", "", val).strip()
            if clean and clean not in VALID_RESULTS:
                res.findings.append(
                    Finding(
                        "A11", "WARN",
                        f"#{r[0]} 结果列值不合法：「{val}」（合法基值：PASS/FAIL/BLOCKED/N/A）",
                    )
                )


# ---------------------------------------------------------------- 主流程


def resolve_stage_dir(stage: str) -> Path | None:
    hits = [p for p in STAGES_DIR.glob(f"{stage}-*") if p.is_dir()]
    return hits[0] if hits else None


def audit_stage(stage: str, states: dict[str, str], ledger: list[dict[str, str]]) -> StageResult:
    kind = roadmap_state_kind(states.get(stage, "pending"))
    d = resolve_stage_dir(stage)

    if d is None:
        res = StageResult(stage, "?", [])
        # 详档按 just-in-time 约定撰写：未开工阶段本就没有目录，不算缺陷。
        # 只有已宣称完成/部分的阶段缺目录，才是真的契约缺失。
        if kind in ("done", "partial"):
            res.findings.append(
                Finding("A0", "FAIL", f"阶段状态为 {kind}，但没有阶段目录/详档 {stage}-*")
            )
        return res

    res = StageResult(stage, d.name, [])

    report_path = d / "report.md"
    if not report_path.exists():
        if kind in ("done", "partial"):
            res.findings.append(Finding("A1", "FAIL", "阶段已宣称完成，但 report.md 不存在（收口契约 #1）"))
        else:
            res.findings.append(Finding("A0", "WARN", "阶段未开工，无 report.md（符合 just-in-time 约定）"))
        return res

    report = read(report_path)
    plan = read(d / "plan.md")

    check_a1(report, res)
    nums, rows, header = parse_report_testpoints(report)
    check_a2(report, rows, res)
    check_a3(plan, nums, res)
    check_a4(rows, header, res)
    check_a5(stage, kind, ledger, res)
    check_a6(report, res)
    check_a7(report, d, res)
    check_a8(ledger, res)
    check_a9(stage, d, states, res)
    check_a10(d, res)
    check_a11(rows, header, res)
    return res


# ---------------------------------------------------------------- 自校验

# 回归样本：本次审查（2026-09-18）实测出的已知缺口。
# 跑不出这些 = 审计器无效（RUNBOOK §13.4）。
# 注：R-02 修复后部分缺口已消除，更新期望值。
SELFTEST_EXPECT = {
    # 2026-09-18 状态对齐后更新：E2E-03/04/05/06 由 done 改为 partial
    # ⇒ A5（"标 done 却有未解决账本条目"）不再触发 —— 这是**修复后的正确行为**，
    # 故期望值同步下调。剩余项均为真实未还的契约欠账（R-02 #1~#3）。
    "e2e-03": ["A1", "A2", "A3"],  # report 非模板（缺「收口自检」）；无汇总行；21 点中 13 点无结果
    "e2e-04": ["A2", "A6"],        # 汇总计数与表行数不符；收口自检项 "[x] …（待 push）"
    "e2e-05": ["A4"],              # 3 个测试点的证据只有文件路径、无执行信号（RUNBOOK §4.1）
}

# 反误报样本：最接近合规的阶段，不应触发 A1~A5
SELFTEST_NOFAIL = ["e2e-02"]

# 不得误报的断言（E2E-F-64）：
#   A4 的前提是"该行声称 PASS 但证据只证明存在"。E2E-04 的报告行 #4~#7 已被 R-02
#   如实改写为"❌ 假 PASS / ⚠️ 未验证"，其证据里的"已创建/已新增"是在**陈述缺口**。
#   修 A4 前这些行被误判为假 PASS，且误报被写进了本期望表固化。
#   在此显式登记"不得触发"，防止误报再次被当成"复现结论"通过。
SELFTEST_MUSTNOT = {
    "e2e-04": ["A4"],
}

# 合成样本：直接喂给 row_claims_pass / check_a4，验证"诚实陈述缺口"不被误报。
# 不依赖任何真实文档，故文档怎么改都不会让这条回归失效。
A4_FIXTURE_HEADER = ["编号", "测试点", "判定", "结果", "证据"]
A4_FIXTURE_ROWS = [
    # 声称 PASS + 只证明存在 ⇒ 必须触发（真缺口）
    ["1", "构建产物 smoke 通过", "[A]", "PASS", "脚本已创建"],
    # 声称 PASS + 只有路径、无执行信号 ⇒ 必须触发（真缺口）
    ["2", "校验脚本接入 CI", "[A]", "PASS", "`scripts/foo.sh`"],
    # 如上但证据含执行信号 ⇒ 不得触发
    ["3", "端到端跑通", "[A]", "PASS", "curl → HTTP 200，退出码 0"],
    # 如实陈述缺口 ⇒ 不得触发（修 A4 的目标）
    ["4", "构建产物 smoke 通过", "[A]", "⚠️ **未验证**", "脚本已创建但 `.output/public/index.html` 不存在"],
    ["5", "lint 接入 CI", "[A]", "❌ **假 PASS**", "web-test.yml 无 typecheck 步骤"],
    ["6", "mobile project 可跑", "[A]", "FAIL", "配置已新增，但未实际运行"],
]


def run_a4_fixture() -> bool:
    """A4 合成样本回归（含正例与反例，故既防漏报也防误报）。"""
    header = A4_FIXTURE_HEADER
    rows = A4_FIXTURE_ROWS
    res = StageResult(stage="fixture", slug="fixture")
    check_a4(rows, header, res)
    fired = sorted({f.check for f in res.failed})

    # 逐行核对：编号 1/2 应触发，3/4/5/6 不应触发
    hit_ids = set()
    for f in res.failed:
        for m in re.finditer(r"#(\d+)", f.message):
            hit_ids.add(m.group(1))

    ok = True
    print("\n[A4 合成样本] 正例（应触发）编号 1,2；反例（不得触发）编号 3,4,5,6")
    print(f"              实际触发编号：{sorted(hit_ids, key=int) or '无'} (findings={fired})")
    if not {"1", "2"} <= hit_ids:
        print("         ❌ 漏报：声称 PASS 且证据只证明存在的行未被判红 → A4 失效")
        ok = False
    bad = hit_ids & {"3", "4", "5", "6"}
    if bad:
        print(f"         ❌ 误报：编号 {sorted(bad, key=int)} 未声称通过，却被 A4 判红")
        ok = False
    if ok:
        print("         ✅ 既不漏报也不误报")
    return ok


def row_claims_pass_fixture() -> bool:
    """直接对 row_claims_pass 做表驱动断言（更贴近单元测试）。"""
    cases = [
        ("PASS", True),
        ("PASS（附退出码 0）", True),
        ("⚠️ **未验证**", False),
        ("❌ **假 PASS**", False),
        ("FAIL", False),
        ("BLOCKED", False),
        ("N/A", False),
        ("未运行", False),
    ]
    ok = True
    print("\n[row_claims_pass 表驱动]")
    for cell, want in cases:
        row = ["1", "x", "[A]", cell, "证据"]
        got = row_claims_pass(row, A4_FIXTURE_HEADER)
        flag = "✅" if got == want else "❌"
        if got != want:
            ok = False
        print(f"         {flag} 结果列「{cell}」→ claims_pass={got}（期望 {want}）")
    return ok


def run_selftest(states, ledger) -> int:
    print("=" * 74)
    print("E2E 审计器自校验（--selftest）")
    print("=" * 74)
    ok = True

    for stage, expect in SELFTEST_EXPECT.items():
        res = audit_stage(stage, states, ledger)
        got = sorted({f.check for f in res.failed})
        print(f"\n[{stage}] 期望检出 {expect}")
        print(f"         实际检出 {got}")
        for f in res.failed:
            print(f"           - {f.check} {f.message[:110]}")
        missing = [c for c in expect if c not in got]
        if missing:
            print(f"         ❌ 未能检出期望项：{missing} → 审计器无效")
            ok = False
        else:
            print("         ✅ 复现审查结论")

    for stage in SELFTEST_NOFAIL:
        res = audit_stage(stage, states, ledger)
        got = sorted({f.check for f in res.failed})
        print(f"\n[{stage}] 期望不误报（最接近合规）")
        print(f"         实际检出 {got}")
        for f in res.warned:
            print(f"           ~ {f.check} {f.message[:110]}")
        if got:
            print(f"         ⚠️ 出现 FAIL：{got} → 可能误报，需人工确认")
        else:
            print("         ✅ 无误报")

    for stage, mustnot in SELFTEST_MUSTNOT.items():
        res = audit_stage(stage, states, ledger)
        got = sorted({f.check for f in res.failed})
        print(f"\n[{stage}] 不得误报 {mustnot}")
        print(f"         实际检出 {got}")
        bad = [c for c in mustnot if c in got]
        if bad:
            print(f"         ❌ 出现已知误报：{bad}（见 SELFTEST_MUSTNOT 注释）")
            ok = False
        else:
            print("         ✅ 未误报")

    if not run_a4_fixture():
        ok = False
    if not row_claims_pass_fixture():
        ok = False

    print("\n" + "=" * 74)
    print("自校验结果：" + ("✅ 通过（审计器可信）" if ok else "❌ 失败（审计器无效，不得进入 R-01）"))
    print("=" * 74)
    return 0 if ok else 1


# ---------------------------------------------------------------- 入口


def main() -> int:
    ap = argparse.ArgumentParser(description="E2E 阶段收口审计器（MVA）")
    ap.add_argument("--stage", help="阶段编号，如 e2e-04")
    ap.add_argument("--all", action="store_true", help="审计 roadmap 中全部阶段")
    ap.add_argument("--selftest", action="store_true", help="用已知缺口做回归自校验")
    ap.add_argument("--json", action="store_true", help="输出机器可读 JSON")
    args = ap.parse_args()

    states = parse_roadmap_states()
    ledger = parse_ledger()

    if args.selftest:
        return run_selftest(states, ledger)

    targets = sorted(states) if args.all else ([args.stage] if args.stage else [])
    if not targets:
        ap.print_help()
        return 2

    results = [audit_stage(s, states, ledger) for s in targets]

    if args.json:
        print(json.dumps(
            [
                {
                    "stage": r.stage, "slug": r.slug,
                    "status": roadmap_state_kind(states.get(r.stage, "pending")),
                    "fail": [{"check": f.check, "message": f.message} for f in r.failed],
                    "warn": [{"check": f.check, "message": f.message} for f in r.warned],
                }
                for r in results
            ],
            ensure_ascii=False, indent=2,
        ))
    else:
        for r in results:
            kind = roadmap_state_kind(states.get(r.stage, "pending"))
            mark = "✅" if not r.failed else "❌"
            print(f"\n{mark} {r.stage} ({r.slug})  roadmap 状态: {kind}")
            if not r.findings:
                print("     无 A1~A5 问题")
            for f in r.failed:
                print(f"     [FAIL] {f.check}  {f.message}")
            for f in r.warned:
                print(f"     [WARN] {f.check}  {f.message}")

        n_fail = sum(1 for r in results if r.failed)
        print(f"\n合计：{len(results)} 个阶段，{n_fail} 个存在 FAIL")
        print("提示：审计器覆盖 11 条断言（A1~A11）。其余：soft-assert→check_soft_asserts.sh；"
              "TDD→check_tdd_gate.sh；孤儿→check_orphan_outputs.sh；ADR→check_adr_gate.sh；"
              "残留→check_residual.sh；分支保护→check_required_checks.py（需管理员 token）")

    return 1 if any(r.failed for r in results) else 0


if __name__ == "__main__":
    sys.exit(main())
