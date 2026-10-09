#!/usr/bin/env bash
# scripts/test_helm_main_render.sh — E2E-30 组 B / L6：helm **主 chart** 渲染回归守卫
#
# 背景（stages/e2e-30-data-contract-closure/plan.md §0.1 F5）：
#   helm 侧**只有 loki / prometheus 两个子 chart** 有渲染断言
#   （`scripts/test_helm_loki_render.sh` / `test_helm_prometheus_render.sh`），
#   主 chart（`charts/emotion-echo`，23 个子 chart）在 2026-10-09 之前**零渲染回归**。
#   而"渲染了但内容是空的"正是本项目真实发生过的静默失效形态
#   （E2E-22：k8s 侧 alertmanager/prometheus `rule_files` 指向空 glob ⇒ 0 条规则且零报错）。
#   ⇒ 只断言"总行数 > 0"是弱断言，本守卫断言**每个子 chart 都可证明被渲染**。
#
# 本守卫钉三条不变量（静态；只需 helm + python，**不需要容器栈**）：
#   A. **接线（wiring）**：`charts/` 下每个子 chart 目录都必须在 Chart.yaml 里声明为
#      dependency **且带 `condition:`** —— 防"chart 存在于仓库但永远渲不出来"（孤儿 chart）。
#      子 chart 数量须等于 EXPECTED_SUBCHARTS（增删必须是一次显式动作）。
#   B. **可达（reachability）**：把**所有** condition 置 true 渲染时，**每个**子 chart 都必须
#      出现在产物里 ⇒ 证明"条件渲染"不是"永不渲染"（这是 E2E-22 空 glob 的同型防线）。
#   C. **默认面（default）**：values.yaml 里显式 `enabled: false` 的子 chart **不得**出现在
#      默认渲染中 —— 把"默认开哪些"固化成可核对的事实，也防"哪天默认被悄悄全开"。
#   另加：`helm lint` rc=0 且零 ERROR；关键资源（web-bff / apisix / postgres / kafka / web）在位。
# 含负向对照：把主 chart 副本里某个 dependency 的 `condition:` 删掉 ⇒ **不变量 A 必须 RED**。

set -uo pipefail
cd "$(dirname "$0")/.." || exit 2

CHART_DIR="${CHART_DIR:-charts/emotion-echo}"
# 2026-10-09 实测 = 23（建档期文档写 22 系笔误，E2E-31 已更正）。增删子 chart 须同步此数。
EXPECTED_SUBCHARTS=23

fail=0
err() { echo "FAIL: $*"; fail=1; }

echo "== helm 主 chart 渲染回归守卫（E2E-30 组 B / L6）=="

if ! command -v helm >/dev/null 2>&1; then
  echo "FAIL 环境无 helm ⇒ 本项**未验证**（不是通过）"
  exit 1
fi
if ! command -v python >/dev/null 2>&1; then
  echo "FAIL 环境无 python ⇒ 本项**未验证**（不是通过）"
  exit 1
fi
if [ ! -f "$CHART_DIR/Chart.yaml" ]; then
  echo "FAIL 缺 $CHART_DIR/Chart.yaml"
  exit 1
fi

# ---------- 不变量 A：接线（对给定 chart 目录；负向对照复用本函数）----------
invariant_wiring() {
  local dir="$1"
  python - "$dir" <<'PY'
import os, sys, yaml
d = sys.argv[1]
chart = yaml.safe_load(open(os.path.join(d, 'Chart.yaml'), encoding='utf-8'))
deps = {x['name']: (x.get('condition') or '') for x in (chart.get('dependencies') or [])}
sub = sorted(
    n for n in os.listdir(os.path.join(d, 'charts'))
    if os.path.isdir(os.path.join(d, 'charts', n))
) if os.path.isdir(os.path.join(d, 'charts')) else []
bad = []
for n in sub:
    if n not in deps:
        bad.append(f"{n}: 目录存在但 Chart.yaml 未声明为 dependency（孤儿 chart）")
    elif not deps[n]:
        bad.append(f"{n}: 已声明 dependency 但无 condition（渲染不可控）")
for n in deps:
    if n not in sub:
        bad.append(f"{n}: Chart.yaml 声明了 dependency 但 charts/ 下无该目录")
print(f"SUBCHART_COUNT={len(sub)}")
for b in bad:
    print(f"BAD={b}")
PY
}

wiring_out="$(invariant_wiring "$CHART_DIR")"
n_sub="$(printf '%s\n' "$wiring_out" | sed -n 's/^SUBCHART_COUNT=//p')"
bad_lines="$(printf '%s\n' "$wiring_out" | sed -n 's/^BAD=//p')"
if [ -n "$bad_lines" ]; then
  while IFS= read -r b; do err "接线：$b"; done <<< "$bad_lines"
else
  echo "   接线：全部子 chart 均已声明且带 condition"
fi
if [ "${n_sub:-0}" != "$EXPECTED_SUBCHARTS" ]; then
  err "子 chart 数 $n_sub ≠ 预期 $EXPECTED_SUBCHARTS（增删子 chart 须同步本脚本 EXPECTED_SUBCHARTS 并说明）"
else
  echo "   子 chart 数 = $n_sub"
fi

# ---------- 条件清单（name + condition path），默认开关取自 values.yaml ----------
meta="$(python - "$CHART_DIR" <<'PY'
import os, sys, yaml
d = sys.argv[1]
chart = yaml.safe_load(open(os.path.join(d, 'Chart.yaml'), encoding='utf-8'))
vals = yaml.safe_load(open(os.path.join(d, 'values.yaml'), encoding='utf-8')) or {}
for dep in chart.get('dependencies') or []:
    name, cond = dep['name'], (dep.get('condition') or '')
    # condition 形如 "postgres.enabled"；取顶层 key 查 values 默认值（缺省 = 开）
    key = cond.split('.')[0] if cond else name
    block = vals.get(key)
    enabled = True
    if isinstance(block, dict) and block.get('enabled') is False:
        enabled = False
    print(f"{name}\t{cond}\t{'on' if enabled else 'off'}")
PY
)"

cond_args=()
while IFS=$'\t' read -r name cond state; do
  [ -z "$cond" ] && continue
  cond_args+=(--set "$cond=true")
done <<< "$meta"

origins_of() {
  helm template ee "$CHART_DIR" "$@" 2>/dev/null \
    | grep '^# Source:' | sed 's|^# Source: ||; s|/templates/.*||' | sort -u
}

default_origins="$(origins_of)"
if [ -z "$default_origins" ]; then
  err "默认渲染无任何 # Source: 产物（渲染失败或产物为空）"
else
  echo "   默认渲染 origin 数 = $(printf '%s\n' "$default_origins" | grep -c .)"
fi

all_on_origins="$(origins_of "${cond_args[@]}")"

# ---------- 不变量 B：可达性——每个子 chart 在"全开"渲染里必须出现 ----------
missing_reach=0
while IFS=$'\t' read -r name cond state; do
  if ! printf '%s\n' "$all_on_origins" | grep -qF "/charts/$name"; then
    err "可达性：子 chart '$name' 即使在 condition 置 true 时也**没有渲染出任何资源**（静默失效）"
    missing_reach=1
  fi
done <<< "$meta"
[ "$missing_reach" -eq 0 ] && echo "   可达性：${EXPECTED_SUBCHARTS} 个子 chart 全部可渲染"

# ---------- 不变量 C：默认面——显式关掉的子 chart 不得出现在默认渲染 ----------
while IFS=$'\t' read -r name cond state; do
  if [ "$state" = "off" ] && printf '%s\n' "$default_origins" | grep -qF "/charts/$name"; then
    err "默认面：子 chart '$name' 在 values.yaml 标 enabled:false，却出现在默认渲染里"
  fi
done <<< "$meta"
echo "   默认面：显式关闭的子 chart 未泄漏进默认渲染"

# ---------- helm lint ----------
lint_out="$(helm lint "$CHART_DIR" 2>&1)"
lint_rc=$?
if [ "$lint_rc" -ne 0 ]; then
  err "helm lint rc=$lint_rc（期望 0）"
fi
if printf '%s\n' "$lint_out" | grep -qE '\[ERROR\]'; then
  err "helm lint 含 ERROR：$(printf '%s\n' "$lint_out" | grep -E '\[ERROR\]' | head -3)"
else
  echo "   helm lint：rc=$lint_rc，零 ERROR（$(printf '%s\n' "$lint_out" | tail -1)）"
fi

# ---------- 关键资源在位（防"渲染了但关键 Deployment 丢了"）----------
for want in "Deployment web-bff" "Deployment apisix" "Deployment web" "StatefulSet postgres" "StatefulSet kafka" "Service web-bff"; do
  kind="${want%% *}"; rname="${want##* }"
  if helm template ee "$CHART_DIR" 2>/dev/null \
      | awk -v k="$kind" -v n="$rname" '
          /^kind:/{ck=$2} /^  name:/{if(ck==k && $2==n) found=1} END{exit found?0:1}'; then
    :
  else
    err "关键资源缺失：$kind/$rname"
  fi
done
echo "   关键资源在位：web-bff / apisix / web / postgres / kafka（Deployment·StatefulSet·Service）"

# ---------- 负向对照：删掉某 dependency 的 condition ⇒ 不变量 A 必须 RED ----------
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
cp -r "$CHART_DIR" "$tmp/chart"
python - "$tmp/chart/Chart.yaml" <<'PY'
import io, re, sys
p = sys.argv[1]
s = io.open(p, encoding='utf-8').read()
# 删掉第一条 condition 行（模拟"声明了 dependency 但渲染不可控"）
s2 = re.sub(r'^\s+condition:.*\n', '', s, count=1, flags=re.M)
assert s2 != s, 'negative control: no condition line removed'
io.open(p, 'w', encoding='utf-8').write(s2)
PY
neg_out="$(CHART_DIR_UNUSED=1 invariant_wiring "$tmp/chart")"
neg_bad="$(printf '%s\n' "$neg_out" | sed -n 's/^BAD=//p')"
if [ -n "$neg_bad" ]; then
  echo "   负向对照：删 1 条 condition → 接线不变量命中「$neg_bad」"
else
  err "负向对照失败：删掉 condition 后接线不变量未报错（守卫是弱断言）"
fi

if [ "$fail" -ne 0 ]; then
  echo "FAIL: 主 chart 渲染回归不成立（见上）"
  exit 1
fi
echo "PASS: 主 chart 接线/可达/默认面三不变量 + lint + 关键资源 全部成立，负向对照可检测"
exit 0
