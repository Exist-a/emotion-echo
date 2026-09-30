#!/usr/bin/env bash
# test_audit_ledger_parser.sh — 审计器账本解析器的回归钉
#
# 为什么需要（E2E-23 第三轮复核抓到的**真门禁漏洞**，不是文档问题）：
#   `e2e_stage_audit.py` 的 `parse_ledger()` 逐**行**读账本，
#   对每行做 `s.strip("|").split("|")`，**格数 < 6 就 `continue` 静默丢弃**。
#   而 `discovered-unresolved.md` 里有 4 条账本（F-165 / 166 / 167 / 171）
#   单元格内含换行 —— 在 Markdown 表格里这是一条**跨多行**的行，
#   解析器只看得到它的第一行（3 格）⇒ **整条账本对审计器完全不可见**。
#
# 后果有两层，第二层才是致命的：
#   ① A8 误报「账本编号不连续，缺失：165,166,167」——它们明明存在。
#      常红的告警会训练出"看见就跳过"（AP-11 的变体）。
#   ② **A5 是"阶段判 done 前账本必须对账干净"的唯一执行者**。
#      解析器看不见的账本行 ⇒ **A5 也看不见** ⇒
#      一个阶段可以带着一条未解决的账本判 done，而门禁全绿。
#      本阶段 E2E-23 侥幸没被绕过（那 4 条都不归它），
#      **但这是靠运气，不是靠机制**。
#
# 修法：解析前先把 Markdown 表格里**跨多行的行拼回一行**再切格。
#
# 用法：bash scripts/test_audit_ledger_parser.sh
# 退出码：0 = 通过；1 = 有 FAIL；2 = 找不到可用解释器
#
# ── 写这个守卫时踩的两个坑，都固化成了断言 ──
#   ① `command -v python3 || command -v python` 在 Windows 上会选中
#      Microsoft Store 的 python3 **别名桩**：`command -v` 找得到，
#      但它是个什么都不做的转发器，**存在却静默不执行**。
#      结果守卫有两条断言**空跑成 PASS** —— 半真半假的守卫比没有守卫更坏。
#      故：解释器必须**实跑一次并要求有版本号输出**才认。
#   ② 测试 1 原本写成"若解析结果为空则 PASS"，
#      而 python 片段一旦抛异常，stdout 也是空 ⇒ **异常被读成"没缺任何账本"= PASS**。
#      这是典型的弱断言（AP-01 近亲）：**"没输出"被当成了"结果是好的"**。
#      故：每条 python 断言都必须**先校验退出码**，再解释输出。

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$REPO_ROOT"

LEDGER="docs/e2e-roadmap/discovered-unresolved.md"
AUDIT="scripts/e2e_stage_audit.py"

# ---------------------------------------------------------------- 解释器选择
# 必须实际跑一次并要求有版本号输出 —— 见文件头坑 ①。
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
  echo "       绝不在解释器不可用时继续跑 —— 那正是本项目吃过的'假绿'成因。" >&2
  exit 2
fi

PASS=0
FAIL=0
ok()  { PASS=$((PASS+1)); echo "  PASS  $1"; }
bad() { FAIL=$((FAIL+1)); echo "  FAIL  $1"; }

# 加载审计器的公共片段。
# 关键：必须先 `sys.modules[name] = mod` 再 `exec_module` ——
# 否则脚本里的 `@dataclass` 会在 `dataclasses._is_type` 里查
# `sys.modules.get(cls.__module__)` 拿到 None 而 AttributeError。
LOADER='
import importlib.util, sys
def load_audit(path):
    spec = importlib.util.spec_from_file_location("e2e_audit_under_test", path)
    mod = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = mod
    spec.loader.exec_module(mod)
    return mod
'

echo "== 审计器账本解析器回归钉 =="
echo "解释器：$PYTHON_BIN（$PY_VER）"

# ---------------------------------------------------------------- 1. 当前账本无盲区
echo
echo "-- 1. 真实账本里 164~171 必须全部可见 --"
out="$("$PYTHON_BIN" - "$LEDGER" "$AUDIT" <<PY
$LOADER
import pathlib, sys
mod = load_audit(sys.argv[2])
mod.LEDGER = pathlib.Path(sys.argv[1])
seen = {e["id"] for e in mod.parse_ledger()}
want = [f"E2E-F-{n}" for n in range(164, 172)]
print("MISSING:" + " ".join(i for i in want if i not in seen))
PY
)"
rc=$?
# 先看退出码 —— 见文件头坑 ②：空输出不等于"结果正确"，也可能是脚本崩了
if [ $rc -ne 0 ]; then
  bad "检查脚本自身崩了（rc=$rc），本条断言无效：$out"
elif ! printf '%s' "$out" | grep -q '^MISSING:'; then
  bad "检查脚本没有按约定输出 MISSING: 前缀，无法判断结果：$out"
else
  missing="$(printf '%s' "${out#MISSING:}" | tr -d '\r' | xargs || true)"
  if [ -z "$missing" ]; then
    ok "164~171 全部 8 条对 parse_ledger 可见"
  else
    bad "以下账本行对解析器不可见（多行表格行被静默丢弃）：$missing"
  fi
fi

# ---------------------------------------------------------------- 2. 结构性断言
echo
echo "-- 2. 结构断言：跨多行的账本行必须被拼回 --"
if "$PYTHON_BIN" - "$AUDIT" <<PY
$LOADER
import os, pathlib, sys, tempfile
mod = load_audit(sys.argv[1])
# E2E-900 的现象格里含换行 —— Markdown 表格里就是一跨多行的行
sample = (
    "| ID | 标题 | 现象 | 影响 | 归属 | 状态 |\n"
    "|---|---|---|---|---|---|\n"
    "| E2E-F-900 | 跨行条目 | 现象很长\n第二行继续写 | 根因 | E2E-23 | 🟡 未解决 |\n"
    "| E2E-F-901 | 普通条目 | 现象 | 根因 | E2E-23 | ✅ 已解决 |\n"
)
fd, path = tempfile.mkstemp(suffix=".md")
os.write(fd, sample.encode("utf-8")); os.close(fd)
mod.LEDGER = pathlib.Path(path)
ids = {e["id"] for e in mod.parse_ledger()}
os.unlink(path)
assert "E2E-F-900" in ids, f"跨多行的账本行被丢弃了，实得 {sorted(ids)}"
assert "E2E-F-901" in ids, f"普通账本行丢了（修法引入回归），实得 {sorted(ids)}"
assert len(ids) == 2, f"解析出 {len(ids)} 条，期望 2：{sorted(ids)}"
PY
then
  ok "跨多行的账本行被正确拼回（E2E-900 可见）"
else
  bad "跨多行的账本行未被拼回"
fi

# ---------------------------------------------------------------- 3. 负向对照
echo
echo "-- 3. 负向对照：表头与正文不得被误当成账本条目 --"
if "$PYTHON_BIN" - "$AUDIT" <<PY
$LOADER
import os, pathlib, sys, tempfile
mod = load_audit(sys.argv[1])
sample = (
    "### 某节标题\n"
    "这是正文，不是表格行，不该被当成账本条目。\n"
    "| ID | 标题 | 现象 | 影响 | 归属 | 状态 |\n"
    "|---|---|---|---|---|---|\n"
    "| E2E-F-902 | 唯一条目 | 现象 | 根因 | E2E-23 | 🟡 未解决 |\n"
)
fd, path = tempfile.mkstemp(suffix=".md")
os.write(fd, sample.encode("utf-8")); os.close(fd)
mod.LEDGER = pathlib.Path(path)
entries = mod.parse_ledger()
os.unlink(path)
assert len(entries) == 1, f"期望恰好 1 条，实际 {len(entries)} 条：{[e['id'] for e in entries]}"
assert entries[0]["id"] == "E2E-F-902", entries[0]["id"]
PY
then
  ok "表头与正文未被误当成账本条目"
else
  bad "解析器把非账本内容当成了条目（或漏掉了唯一条目）"
fi

# ---------------------------------------------------------------- 4. A8 不再误报
echo
echo "-- 4. A8 不再因多行行而误报'缺失' --"
audit_out="$("$PYTHON_BIN" "$AUDIT" --all 2>&1)"
audit_rc=$?
if [ $audit_rc -ne 0 ]; then
  bad "审计器自身非零退出（rc=$audit_rc），本条断言无效"
elif printf '%s' "$audit_out" | grep -q 'A8.*编号不连续'; then
  bad "A8 仍在误报：$(printf '%s' "$audit_out" | grep -m1 'A8.*编号不连续' | tr -d '\r')"
else
  ok "A8 无任何'编号不连续'告警（4 条多行账本现已被解析器看见）"
fi

# ---------------------------------------------------------------- 5. 坏行不得被静默丢弃
echo
echo "-- 5. 负向对照：格数不足的账本行必须被**报出**，不能静默丢弃 --"
if "$PYTHON_BIN" - "$AUDIT" <<PY
$LOADER
import os, pathlib, sys, tempfile
mod = load_audit(sys.argv[1])
# E2E-F-903 只有 4 格：模拟"某个续行漏了行首 |"导致的坏行
sample = (
    "| ID | 标题 | 现象 | 影响 | 归属 | 状态 |\n"
    "|---|---|---|---|---|---|\n"
    "| E2E-F-903 | 坏行 | 现象很长\n续行漏了行首竖线，于是和影响格粘在一起 | E2E-23 | 🟡 未解决 |\n"
)
fd, path = tempfile.mkstemp(suffix=".md")
os.write(fd, sample.encode("utf-8")); os.close(fd)
mod.LEDGER = pathlib.Path(path)
mod.LEDGER_MALFORMED.clear()
ids = {e["id"] for e in mod.parse_ledger()}
os.unlink(path)

# 这条**不能**进 entries（格数不够，字段会错位，取 owner/status 必错），
# 但**必须**出现在 LEDGER_MALFORMED 里由 A8 报出 —— 静默丢弃才是漏洞。
assert "E2E-F-903" not in ids, "格数不足的行被当成完整条目收下了（字段会错位）"
assert any("E2E-F-903" in m for m in mod.LEDGER_MALFORMED), (
    f"坏行既没被收下、也没被报出 ⇒ 静默丢弃，正是本守卫要堵的漏洞：{mod.LEDGER_MALFORMED}"
)
PY
then
  ok "格数不足的账本行被显式报出（不会静默消失）"
else
  bad "格数不足的账本行被静默丢弃 —— A5 会看不见它"
fi

echo
echo "PASS: $PASS  FAIL: $FAIL"
if [ "$FAIL" -gt 0 ]; then
  echo "RED：账本解析器仍有盲区 —— 门禁能看见的账本少于账本里的账本"
  exit 1
fi
echo "GREEN：账本解析器无盲区，A5/A8 看到的就是账本全貌"
exit 0
