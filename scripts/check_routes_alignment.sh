#!/usr/bin/env bash
# scripts/check_routes_alignment.sh
#
# Stage 71 · C8 三方路径对齐契约脚本
#
# 目的：永久关闭 todo-pile §C8 "前端 / BFF / APISIX 三方路径不一致"问题。
#       解析两份 source-of-truth：
#         - emotion-echo-web-bff/main_test.go 的 wantRoutes + wantRoutesWithEmotionQ
#         - emotion-echo-web/app/lib/apiRoutes.ts（含 knownOrphans）
#       规范化（去 /api/v1 前缀、归 method 大写）后断言前端 ⊆ BFF。
#
# 用法：
#   bash scripts/check_routes_alignment.sh
#
# 设计原则：
#   - 路径作为单一字符串事实源：method + path（归一化后）
#   - 前端 path 字段省略 /api/v1 前缀（BFF 真实 HTTP 路径含）；脚本加回前缀后比较
#   - 前端 knownOrphans（孤儿路径）单独列出 — 不参与硬对齐，避免契约误报
#   - APISIX seed.sh 用通配符 /api/v1/*（不需对齐）— 文档化在 README
#
# 调研依据：
#   - emotion-echo-web-bff/main_test.go:48-100 wantRoutes 切片
#   - emotion-echo-web-bff/main_test.go:103-107 wantRoutesWithEmotionQ
#   - emotion-echo-web/app/lib/apiRoutes.ts API_ROUTES + knownOrphans
#   - todo-pile-2026-09-04.md §C8（"前端/BFF/APISIX 三方路径 source of truth + 契约测试"）

set -eu

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
BFF_TEST="$REPO_ROOT/emotion-echo-web-bff/main_test.go"
FRONT_ROUTES="$REPO_ROOT/emotion-echo-web/app/lib/apiRoutes.ts"

PASS=0
FAIL=0

if [ ! -f "$BFF_TEST" ]; then
  echo "[check] FATAL: $BFF_TEST 不存在" >&2
  exit 1
fi
if [ ! -f "$FRONT_ROUTES" ]; then
  echo "[check] FATAL: $FRONT_ROUTES 不存在" >&2
  exit 1
fi

# 解析 BFF wantRoutes：从 main_test.go 提取 {Method: "X", Path: "/api/v1/..."} 行
BFF_ROUTES=$(grep -oE '\{Method: "[A-Z]+", Path: "[^"]+"\}' "$BFF_TEST" | \
  sed -E 's/\{Method: "([A-Z]+)", Path: "([^"]+)"\}/\1 \2/' | sort -u)

# 解析前端 API_ROUTES：从 apiRoutes.ts 提取 method/path
#   path 用单引号字符串（TypeScript 风格）
#   跳过 knownOrphans: { ... } 块
# 实现：grep 直接匹配，再用 awk 行号范围排除 knownOrphans 段
ORPHAN_START=$(grep -n 'knownOrphans:' "$FRONT_ROUTES" | head -1 | cut -d: -f1)
TOTAL=$(wc -l < "$FRONT_ROUTES")
if [ -n "$ORPHAN_START" ]; then
  # knownOrphans 结束 = 下一个 ^\},[[:space:]]*$ 行
  ORPHAN_END=$(awk -v start="$ORPHAN_START" '
    NR > start && /^[[:space:]]*\},?[[:space:]]*$/ { print NR; exit }
  ' "$FRONT_ROUTES")
  if [ -z "$ORPHAN_END" ]; then ORPHAN_END=$TOTAL; fi
else
  ORPHAN_END=0
fi

FRONT_RAW=$(awk -v orph_start="$ORPHAN_START" -v orph_end="$ORPHAN_END" '
  /method: '\''[A-Z]+'\'',[[:space:]]+path: '\''\/[a-z]/ {
    if (orph_start != "" && NR >= orph_start && NR <= orph_end) next
    # 提取 method
    match($0, /method: '\''([A-Z]+)'\''/, m)
    match($0, /path: '\''([^'\'']+)'\''/, p)
    if (m[1] != "" && p[1] != "") {
      printf "%s /api/v1%s\n", m[1], p[1]
    }
  }
' "$FRONT_ROUTES" | sort -u)

BFF_COUNT=$(echo "$BFF_ROUTES" | wc -l | tr -d ' ')
FRONT_COUNT=$(echo "$FRONT_RAW" | wc -l | tr -d ' ')

echo "[check] BFF wantRoutes 条数: $BFF_COUNT"
echo "[check] 前端 API_ROUTES 主路径: $FRONT_COUNT"
echo ""

# 校验：每个前端 method+path 是否能在 BFF 中找到对应（含 :WILD 通配匹配）
echo "[check] 路径对齐校验（前端主路径 ⊆ BFF 通配骨架）..."

# 用 awk 做精细匹配
MISSING=$(awk '
  BEGIN { while ((getline line < "'"$BFF_TEST"'") > 0) bff[++bc] = line }
  function check_route(front_method, front_path,    i, b, fp, bp, fc, matched, seg) {
    # 拆分前端 path 为段
    fc = split(front_path, fp, "/")
    for (b = 1; b <= bc; b++) {
      line = bff[b]
      if (line !~ /\{Method:/) continue
      # 提取 method + path
      b_method = ""
      b_path = ""
      if (match(line, /Method: "([A-Z]+)"/)) {
        b_method = substr(line, RSTART+9, RLENGTH-10)
      }
      if (match(line, /Path: "([^"]+)"/)) {
        b_path = substr(line, RSTART+7, RLENGTH-8)
      }
      if (b_method == "" || b_path == "") continue
      if (b_method != front_method) continue
      # 拆分 BFF path 为段
      bc2 = split(b_path, bp, "/")
      if (bc2 != fc) continue
      matched = 1
      for (i = 1; i <= fc; i++) {
        seg = bp[i]
        if (seg == ":action" || seg == ":kind" || seg == ":id" || seg == ":messageId" || seg == ":conversationId" || seg == ":resultId") continue
        if (fp[i] != seg) { matched = 0; break }
      }
      if (matched) return 1
    }
    return 0
  }
  # 跳过已知 orphan
  {
    method = $1; path = $2
    if (path ~ /^\/upload\// || path ~ /^\/voice\// || path ~ /^\/face\//) next
    # Stage 71：路径以 /pin 结尾（pinConversationOrphan）也跳过
    if (path ~ /\/pin$/) next
    # 前端 path 字段省略 /api/v1 前缀,这里 path 已经是 /api/v1/... 形式
    front_method = method
    front_path = path
    if (check_route(front_method, front_path)) {
      pass++
    } else {
      print "  " front_method " " front_path
      fail++
    }
  }
  END {
    print "pass=" pass
    print "fail=" fail
  }
' pass=0 fail=0 <<< "$FRONT_RAW")

echo "$MISSING"
PASS_LINE=$(echo "$MISSING" | grep "^pass=" | tail -1)
FAIL_LINE=$(echo "$MISSING" | grep "^fail=" | tail -1)
PASS_COUNT=$(echo "$PASS_LINE" | sed 's/pass=//')
FAIL_COUNT=$(echo "$FAIL_LINE" | sed 's/fail=//')

if [ "$FAIL_COUNT" = "0" ]; then
  echo "[PASS] 所有 $PASS_COUNT 个前端主路径在 BFF 中能找到对应（含通配匹配）"
  PASS=$((PASS+1))
else
  echo "[FAIL] $FAIL_COUNT 个前端路径在 BFF 中找不到对应（详见上方）"
  FAIL=$((FAIL+1))
fi

# knownOrphans 提示
echo ""
echo "[check] knownOrphans 提示（不参与对齐校验）："
echo "$FRONT_RAW" | grep -E "^\S+ /upload/|^\S+ /voice/|^\S+ /face/" | head -5 | sed 's/^/  /'

# 反向提示：BFF 业务路径但前端未直接调用
echo ""
echo "[check] 反向：BFF 业务路径 vs 前端调用（去除 /health /metrics /ai/stream /ai/health /:action 通配）..."
# 计算差集
BFF_FILTERED=$(echo "$BFF_ROUTES" | grep -vE "/health|/metrics|/ai/stream|/ai/health|auth/:action|/uploads/:kind" | sort -u)
FRONT_FILTERED=$(echo "$FRONT_RAW" | grep -vE "^POST /upload/|^POST /voice/|^POST /face/" | sort -u)
ORPHANED=$(comm -23 <(echo "$BFF_FILTERED") <(echo "$FRONT_FILTERED"))
if [ -n "$ORPHANED" ]; then
  echo "[WARN] 以下 BFF 路径前端未直接调用（可能是 BFF 内部 handler 或可接受 orphan）："
  echo "$ORPHANED" | head -10 | sed 's/^/  /'
  if [ "$(echo "$ORPHANED" | wc -l)" -gt 10 ]; then
    echo "  ... ($(echo "$ORPHANED" | wc -l) 总)"
  fi
  # WARN 不计入 FAIL — 仅提醒
fi

echo ""
echo "==========="
echo "PASS=$PASS FAIL=$FAIL"
echo "==========="

if [ "$FAIL" -gt 0 ]; then
  exit 1
fi
exit 0