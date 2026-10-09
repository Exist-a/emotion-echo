#!/usr/bin/env bash
# scripts/test_audit_plan_parser.sh — 审计器**计划测试点解析器**的回归钉（E2E-30 测试点 #17 / 账本 E2E-F-180）
#
# 背景（账本 E2E-F-180）：
#   `e2e_stage_audit.py` 的 A3（plan 测试点编号集合 ⊆ report 编号集合）依赖
#   `parse_plan_testpoints()`，而它**只认标题含「测试点清单」的章节**。
#   e2e-25 的 plan 用的是 **「测试点总表」**（+ 分组子表 `### 组 A…`）⇒
#   `section_lines` 取不到内容 ⇒ A3 直接 `WARN 未解析到测试点编号，跳过 A3`。
#
#   后果不是"少报"，而是**A3 静默失效**：A3 是"plan 承诺了 N 个测试点、report 是否
#   逐点给了结论"的唯一执行者（AP-07）。一个阶段若用了别的标题写法，A3 就完全不检查它，
#   而输出里只有一行 WARN —— 与"解析器有盲区"是同一类问题（对照 E2E-23 的
#   `test_audit_ledger_parser.sh`：账本解析器曾有静默丢弃）。
#
#   ⚠️ 根因更正（2026-10-09 实测）：账本原文的修法方向写的是"审计器 A3 识别**子表编号**"，
#   但实测子表里的编号本来就是纯整数（`| 1 | [A] | … |`），真正不被识别的是**章节标题**。
#   本守卫按实测根因钉住"标题变体容错"。
#
# 用法：bash scripts/test_audit_plan_parser.sh
# 退出码：0 = 通过；1 = 有 FAIL；2 = 找不到可用解释器

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$REPO_ROOT"

AUDIT="scripts/e2e_stage_audit.py"
PLAN25="docs/e2e-roadmap/stages/e2e-25-apisix-gateway/plan.md"

# ---------------------------------------------------------------- 解释器选择
# 必须实跑一次并要有版本号输出 —— Windows 上 python3 可能是 Store 别名桩，
# `command -v` 找得到却静默不执行，会让断言空跑成 PASS（本仓库已踩过）。
PYTHON_BIN=""
PY_VER=""
for cand in python3 python py; do
  if command -v "$cand" >/dev/null 2>&1; then
    if v="$("$cand" --version 2>&1)" && [ -n "$v" ]; then
      PYTHON_BIN="$cand"; PY_VER="$v"; break
    fi
  fi
done
if [ -z "$PYTHON_BIN" ]; then
  echo "FATAL: 找不到**可用**的 python 解释器（python3 / python / py 均不存在或静默不执行）" >&2
  exit 2
fi

PASS=0
FAIL=0
ok()  { PASS=$((PASS+1)); echo "  PASS  $1"; }
bad() { FAIL=$((FAIL+1)); echo "  FAIL  $1"; }

LOADER='
import importlib.util, sys
def load_audit(path):
    spec = importlib.util.spec_from_file_location("e2e_audit_under_test", path)
    mod = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = mod
    spec.loader.exec_module(mod)
    return mod
'

echo "== 审计器计划测试点解析器回归钉（E2E-F-180）=="
echo "解释器：$PYTHON_BIN（$PY_VER）"

# ---------------------------------------------------------------- 1. 真实 e2e-25 可解析
echo
echo "-- 1. 真实 e2e-25 plan（用「测试点总表」标题）必须可解析出 1~20 --"
out="$("$PYTHON_BIN" - "$PLAN25" "$AUDIT" <<PY
$LOADER
import pathlib, sys
mod = load_audit(sys.argv[2])
nums = mod.parse_plan_testpoints(pathlib.Path(sys.argv[1]).read_text(encoding="utf-8"))
print("NUMS:" + ",".join(sorted(nums, key=int)))
PY
)"
rc=$?
if [ $rc -ne 0 ]; then
  bad "检查脚本自身崩了（rc=$rc），本条断言无效：$out"
elif ! printf '%s' "$out" | grep -q '^NUMS:'; then
  bad "检查脚本未按约定输出 NUMS: 前缀：$out"
else
  got="$(printf '%s' "${out#NUMS:}" | tr -d '\r' | xargs || true)"
  want="1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20"
  if [ "$got" = "$want" ]; then
    ok "e2e-25 plan 解析出 20 个编号（原先 WARN 跳过）"
  else
    bad "e2e-25 plan 解析结果不符：期望 $want，实得「$got」"
  fi
fi

# ---------------------------------------------------------------- 2. 结构：总表标题 + 分组子表
echo
echo "-- 2. 结构断言：「测试点总表」+ 分组子表的编号必须被收全 --"
if "$PYTHON_BIN" - "$AUDIT" <<PY
$LOADER
import sys
mod = load_audit(sys.argv[1])
sample = (
    "## 2. 测试点总表（3 个）\n"
    "\n"
    "### 组 A：甲\n"
    "\n"
    "| # | 判定 | 测试点 | 通过标准 |\n"
    "|---|------|--------|----------|\n"
    "| 1 | [A] | 甲一 | x |\n"
    "| 2 | [M] | 甲二 | y |\n"
    "\n"
    "### 组 B：乙\n"
    "\n"
    "| # | 判定 | 测试点 | 通过标准 |\n"
    "|---|------|--------|----------|\n"
    "| 3 | [A] | 乙一 | z |\n"
)
nums = mod.parse_plan_testpoints(sample)
assert nums == {"1", "2", "3"}, f"期望 {{1,2,3}}，实得 {sorted(nums)}"
PY
then
  ok "总表标题 + 分组子表的编号被收全（含跨子表续号）"
else
  bad "分组子表的编号未被收全（A3 仍会漏检）"
fi

# ---------------------------------------------------------------- 3. 无回归：原「测试点清单」标题
echo
echo "-- 3. 无回归：原「测试点清单」标题仍须解析 --"
if "$PYTHON_BIN" - "$AUDIT" <<PY
$LOADER
import sys
mod = load_audit(sys.argv[1])
sample = (
    "## 3. 测试点清单（2 个）\n"
    "\n"
    "| # | 判定 | 测试点 | 通过标准 |\n"
    "|---|------|--------|----------|\n"
    "| 1 | [A] | 一 | x |\n"
    "| 2 | [A] | 二 | y |\n"
)
assert mod.parse_plan_testpoints(sample) == {"1", "2"}
PY
then
  ok "「测试点清单」标题行为未变（22+ 个阶段依赖它）"
else
  bad "原「测试点清单」标题解析被改坏了（回归）"
fi

# ---------------------------------------------------------------- 4. 负向对照：无标题必须仍空
echo
echo "-- 4. 负向对照：没有测试点章节的 plan 必须仍解析为空（保 WARN 语义）--"
if "$PYTHON_BIN" - "$AUDIT" <<PY
$LOADER
import sys
mod = load_audit(sys.argv[1])
# 只有别的章节 + 一张与测试点无关的编号表 ⇒ 不得被误当成测试点
sample = (
    "## 1. 范围\n"
    "\n"
    "| # | 项 |\n"
    "|---|----|\n"
    "| 1 | 甲 |\n"
    "\n"
    "## 2. 风险\n"
    "\n"
    "| # | 风险 |\n"
    "|---|------|\n"
    "| 2 | 乙 |\n"
)
got = mod.parse_plan_testpoints(sample)
assert got == set(), f"无关章节被误当成测试点清单：{sorted(got)}"
PY
then
  ok "无测试点章节时仍返回空（WARN 语义保留，没有变成『什么都能解析』）"
else
  bad "解析器把无关章节当成了测试点清单 —— 容错过头等于取消 A3"
fi

# ---------------------------------------------------------------- 5. 端到端：audit 输出
echo
echo "-- 5. 端到端：audit --all 不得再出现「未解析到测试点编号」，且 0 FAIL --"
audit_out="$("$PYTHON_BIN" "$AUDIT" --all 2>&1)"
audit_rc=$?
if [ $audit_rc -ne 0 ]; then
  bad "审计器自身非零退出（rc=$audit_rc），本条断言无效"
else
  if printf '%s' "$audit_out" | grep -q '未解析到测试点编号'; then
    bad "仍有阶段报「未解析到测试点编号」：$(printf '%s' "$audit_out" | grep -m2 '未解析到测试点编号' | tr -d '\r')"
  else
    ok "无任何阶段报「未解析到测试点编号」（A3 对所有阶段生效）"
  fi
  if printf '%s' "$audit_out" | grep -qE '合计：.*0 个存在 FAIL'; then
    ok "audit --all 仍 0 FAIL（打开 A3 未引入新 FAIL）"
  else
    bad "audit --all 出现 FAIL：$(printf '%s' "$audit_out" | grep -E 'FAIL' | head -5 | tr -d '\r')"
  fi
fi

echo
echo "PASS: $PASS  FAIL: $FAIL"
if [ "$FAIL" -gt 0 ]; then
  echo "RED：计划测试点解析器仍有盲区 —— A3（AP-07）会静默跳过某些阶段"
  exit 1
fi
echo "GREEN：A3 对「测试点清单」与「测试点总表」两种标题都生效，且未引入新 FAIL"
exit 0
