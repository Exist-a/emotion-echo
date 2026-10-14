#!/usr/bin/env bash
# scripts/check_rpc_status.sh — CL-02「gRPC 契约闭环」RPC 状态守卫
#
# 取代/叠加 E2E-31 的 scripts/check_dead_grpc_inventory.sh（旧守卫只守"标记数=10"，
# 即"清单真实"；本守卫守"每条 RPC 有明确状态 + live 必有调用方"）。
#
# 背景：43 条 RPC 中 7 条处于"server 在、无生产调用方"的挂空状态（E2E-F-208 清点）。
# 旧处置 = 保留 + 注释 + 计数守卫 ⇒ 用户 2026-10-13 判定"不算坐实"。
# 本守卫把状态**外置成清单**（scripts/rpc_status.tsv），断言：
#   A1 覆盖   proto 里每条 rpc 在清单中恰一行（不重不漏）
#   A2 live   evidence_path 内必须出现 evidence_symbol 的调用（**不能靠 RPC 名 grep**：
#             如 ReportsTrend→TrendReport、GetEmotionByMessage→ByMessage 均不同名）
#   A3 标记   清单 marker_sites 之和 == 代码里实际的 E2E-31「已知未接线」标记数
#             （标记串从 check_dead_grpc_inventory.sh 读取；此处不写字面量，避免被旧守卫扫到）
#             （取代硬编码 EXPECTED=10：数字由清单推导，改一处不必两处同步）；
#             marker_sites>0 的行只能是 undecided（标记 = 未决，不许挂在已定态上）
#   A4 终态   archived 必带 reserved/deprecated 注释；deleted 不得残留于 proto
#   A5 豁免   5 个 `*_TRANSPORT` 回滚开关声明在位（HTTP 降级 ≠ 死链，见 CL-02 plan G4）
#
# 退出码：0 = 全绿；1 = 有 FAIL；2 = 前置缺失（清单不存在等）
#
# 注意：本脚本**尚未接入 CI**（CL-02 Phase 6 才接线，见 plan §3 与 T-24）。

set -uo pipefail
cd "$(dirname "$0")/.." || exit 2

LIST="scripts/rpc_status.tsv"
SELF="check_rpc_status.sh"
SELF_OLD="check_dead_grpc_inventory.sh"
# 标记串**从旧守卫读取**（单一事实源）：既避免两处字面量漂移，也避免本脚本自身
# 被旧守卫的标记计数扫到（旧守卫只排除它自己，不排除本文件）。
MARKER="$(grep -oE 'MARKER="[^"]+"' "scripts/$SELF_OLD" 2>/dev/null | head -1 | sed -E 's/^MARKER="(.*)"$/\1/')"
[ -n "$MARKER" ] || { echo "FAIL: 无法从 scripts/$SELF_OLD 读取标记串"; exit 2; }
fail=0

[ -f "$LIST" ] || { echo "FAIL: 清单不存在：$LIST"; exit 2; }

echo "== RPC 状态守卫（CL-02）=="

# ---- 结构自检：7 列 ----
bad_cols="$(grep -vnE '^#|^$' "$LIST" | awk -F'\t' 'NF!=7 {print $1}' | tr '\n' ' ')"
if [ -n "$bad_cols" ]; then
  echo "FAIL: 清单存在非 7 列的行：$bad_cols（6 列会被下游静默漏读）"
  fail=1
fi

rows="$(grep -vE '^#|^$' "$LIST")"

# ---- A1 覆盖 ----
# 语义（2026-10-14 修正）：**proto 里出现的每条 RPC 必须已登记**；反向允许"清单有、proto 无"，
# 但**仅限 status=deleted**（清除项本就不该留在 proto）——否则说明清单里挂了不存在的 RPC。
proto_rpcs="$(grep -hE '^[[:space:]]*rpc[[:space:]]+[A-Za-z0-9_]+' proto/*.proto \
  | sed -E 's/.*rpc[[:space:]]+([A-Za-z0-9_]+).*/\1/' | sort)"
list_rpcs="$(printf '%s\n' "$rows" | cut -f1 | sort)"
n_proto="$(printf '%s\n' "$proto_rpcs" | grep -c . || true)"
n_list="$(printf '%s\n' "$list_rpcs" | grep -c . || true)"
n_del="$(printf '%s\n' "$rows" | awk -F'\t' '$3=="deleted"' | grep -c . || true)"

missing="$(comm -23 <(printf '%s\n' "$proto_rpcs") <(printf '%s\n' "$list_rpcs") | tr '\n' ' ')"
extra="$(comm -13 <(printf '%s\n' "$proto_rpcs") <(printf '%s\n' "$list_rpcs") | tr '\n' ' ')"
extra_bad=""
for r in $extra; do
  st="$(printf '%s\n' "$rows" | awk -F'\t' -v n="$r" '$1==n {print $3}')"
  [ "$st" = "deleted" ] || extra_bad="$extra_bad $r(status=$st)"
done
if [ -n "$missing" ] || [ -n "$extra_bad" ]; then
  echo "FAIL: A1 覆盖不一致 —— proto 缺登记：[$missing] · 清单多出非 deleted 项：[$extra_bad]"
  fail=1
else
  echo "PASS: A1 覆盖 proto $n_proto 条全部已登记；清单 $n_list 条 = proto $n_proto + 已清除 $n_del"
fi

# ---- A2 live 必有调用方 ----
a2_bad=""
a2_n=0
while IFS=$'\t' read -r rpc proto_line status ev_path ev_sym marker_sites note; do
  [ "$status" = "live" ] || continue
  a2_n=$((a2_n + 1))
  if [ -z "$ev_path" ] || [ "$ev_path" = "-" ] || [ -z "$ev_sym" ] || [ "$ev_sym" = "-" ]; then
    a2_bad="$a2_bad $rpc(证据未登记)"
    continue
  fi
  if [ ! -f "$ev_path" ]; then
    a2_bad="$a2_bad $rpc(文件不存在:$ev_path)"
    continue
  fi
  if ! grep -qE "[^A-Za-z0-9_]${ev_sym}\(" "$ev_path"; then
    a2_bad="$a2_bad $rpc(调用点${ev_path}无${ev_sym}( )"
  fi
done <<< "$rows"
if [ -n "$a2_bad" ]; then
  echo "FAIL: A2 live 但调用方证据不足 ——$a2_bad"
  fail=1
else
  echo "PASS: A2 live $a2_n 条 RPC 均有已登记且可校验的调用方证据"
fi

# ---- A3 标记数与清单一致（取代硬编码 EXPECTED） ----
actual_markers="$(grep -rn "$MARKER" --include='*.go' --include='*.py' --include='*.sh' . 2>/dev/null \
  | grep -v '/node_modules/' | grep -v "$SELF" | grep -v "$SELF_OLD" | grep -c . || true)"
expect_markers="$(printf '%s\n' "$rows" | awk -F'\t' '{s+=$6} END{print s+0}')"
if [ "$actual_markers" != "$expect_markers" ]; then
  echo "FAIL: A3 标记数 $actual_markers ≠ 清单推导的 $expect_markers"
  echo "      ⇒ 要么有 RPC 被接线（删标记 + 改本行状态为 live/archived/deleted），"
  echo "        要么新挂了未标注的死链（补标记 + 清单登记）。两者都必须是显式决策。"
  fail=1
else
  echo "PASS: A3 标记数 $actual_markers 与清单 marker_sites 之和一致"
fi

wrong_marker_status="$(printf '%s\n' "$rows" | awk -F'\t' '$6>0 && $3!="undecided" {printf "%s(%s,标记%s) ", $1, $3, $6}')"
if [ -n "$wrong_marker_status" ]; then
  echo "FAIL: A3 标记只能挂在 undecided 行 —— 违规：$wrong_marker_status"
  fail=1
fi

# ---- A4 终态规则 ----
# 注意（2026-10-14 RED 运行实测暴露的两个缺陷，已修）：
#   ① deleted 断言原写 `rpc[[:space:]]+${rpc}\(` ⇒ proto 里实际是 `rpc Logout (`（名字与括号间有空格），
#      断言**永不命中** ⇒ 是"永真断言"。改为 `${rpc}[[:space:]]*\(`。
#   ② archived 断言原只看**整文件**是否含 reserved/deprecated ⇒ 文件里任何一处出现即通过，
#      不指向被归档的那条 RPC。改为按清单的 `proto:line` 取**该声明邻域**（+0..+4 行）。
a4_bad=""
while IFS=$'\t' read -r rpc proto_line status ev_path ev_sym marker_sites note; do
  pfile="${proto_line%%:*}"
  pline="${proto_line##*:}"
  case "$status" in
    archived)
      if [ ! -f "$pfile" ]; then a4_bad="$a4_bad $rpc(proto缺失)"; continue; fi
      if ! sed -n "${pline},$((pline + 4))p" "$pfile" | grep -qE 'deprecated|reserved'; then
        a4_bad="$a4_bad $rpc(archived 但 $pfile:$pline 邻域无 deprecated/reserved)"
      fi
      ;;
    deleted)
      if grep -qE "^[[:space:]]*rpc[[:space:]]+${rpc}[[:space:]]*\(" proto/*.proto 2>/dev/null; then
        a4_bad="$a4_bad $rpc(deleted 但仍在 proto)"
      fi
      ;;
  esac
done <<< "$rows"
if [ -n "$a4_bad" ]; then
  echo "FAIL: A4 终态规则 ——$a4_bad"
  fail=1
else
  echo "PASS: A4 终态规则（archived 带标注 / deleted 无残留）——当前 archived/deleted 行数：$(printf '%s\n' "$rows" | awk -F'\t' '$3=="archived"||$3=="deleted"' | grep -c .)"
fi

# ---- A5 HTTP 回滚开关豁免声明 ----
cfg="emotion-echo-web-bff/internal/config/config.go"
exempt_ok=1
for sw in USER_TRANSPORT CHAT_TRANSPORT ASSESSMENT_TRANSPORT ANALYTICS_TRANSPORT AI_TRANSPORT; do
  grep -q "\"$sw\"" "$cfg" 2>/dev/null || { exempt_ok=0; echo "FAIL: A5 豁免开关 $sw 未在 $cfg 找到（豁免声明须与实际一致）"; fail=1; }
done
[ "$exempt_ok" = "1" ] && echo "PASS: A5 HTTP 回滚开关豁免声明在位（*_TRANSPORT=http 是降级开关，不算死链）"

# ---- 报告：undecided（收口前须清零；本守卫只报不拦，收口判据拦） ----
undecided_n="$(printf '%s\n' "$rows" | awk -F'\t' '$3=="undecided"' | grep -c . || true)"
echo "-- undecided = $undecided_n（收口判据要求 0）--"
printf '%s\n' "$rows" | awk -F'\t' '$3=="undecided" {printf "   %-28s %-24s 标记%s  %s\n", $1, $2, $6, $7}'

if [ "$fail" != "0" ]; then echo "== FAIL =="; exit 1; fi
echo "== PASS =="
exit 0
