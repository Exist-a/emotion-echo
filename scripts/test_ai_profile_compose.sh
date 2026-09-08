#!/usr/bin/env bash
# scripts/test_ai_profile_compose.sh
#
# PR-TTS-2 v2 测试：3 个 AI 容器（FER/SenseVoice/XTTS）一起启用 profiles: [ai]
#
# 修订策略（Stage 58 v2 + 用户核实 Stage 36 v0.1.0 build 成功事实）：
#   - 3 个 AI 容器（FER/SV/XTTS）都启用 profiles: ["ai"]
#   - dev 默认不起 AI，必须显式 --profile ai 才起
#   - XTTS 服务定义保留（不删）：Stage 36 v0.1.0 build 成功过；
#     ADR-001 云端化决策与 Stage 36 实践矛盾，待重审
#   - ai-svc XTTS_BASE_URL env 默认值仍是 http://emotion-echo-xtts:8003
#     （PR-TTS-3 阶段把 ai-svc yaml 留空 env 默认值，让 --profile ai 时才注入）

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
DEPLOY_DIR="$SCRIPT_DIR/../deploy"
APPS="$DEPLOY_DIR/docker-compose.apps.yml"

pass=0
fail=0

assert_contains() {
  local file="$1"
  local pattern="$2"
  local desc="$3"
  if grep -qE "$pattern" "$file"; then
    echo "  ✓ $desc"
    pass=$((pass + 1))
  else
    echo "  ✗ $desc (pattern: $pattern)"
    fail=$((fail + 1))
  fi
}

assert_not_contains() {
  local file="$1"
  local pattern="$2"
  local desc="$3"
  if grep -qE "$pattern" "$file"; then
    echo "  ✗ $desc (不应出现: $pattern)"
    fail=$((fail + 1))
  else
    echo "  ✓ $desc"
    pass=$((pass + 1))
  fi
}

echo "=== TDD: AI profile 镜像构建策略 v2（FER+SV+XTTS 都 profiles:[ai]）==="
echo

echo "--- 1) 3 个 AI 容器都启用 profiles: [\"ai\"] ---"
assert_contains "$APPS" '^    profiles: \["ai"\]' \
  "profiles: [\"ai\"] 行已启用（未注释）"

# 检查至少 3 行未注释
UNCOMMENTED_AI=$(grep -cE "^    profiles:[[:space:]]*\[\"ai\"\]" "$APPS")
if [ "$UNCOMMENTED_AI" -ge 3 ]; then
  echo "  ✓ 至少 3 个 profiles: [\"ai\"] 未注释（FER+SV+XTTS）：$UNCOMMENTED_AI 行"
  pass=$((pass + 1))
else
  echo "  ✗ 不足 3 行未注释 profiles: [\"ai\"]：仅 $UNCOMMENTED_AI 行"
  fail=$((fail + 1))
fi

# 检查注释形式的 'profiles: ["ai"]' 不应再存在（避免新旧混用）
assert_not_contains "$APPS" '# profiles: \["ai"\]' \
  "注释形式的 profiles: [\"ai\"] 已清理"

echo
echo "--- 2) 3 个 AI 服务定义都在（FER + SenseVoice + XTTS 完整）---"
assert_contains "$APPS" '^  emotion-echo-fer:' "emotion-echo-fer 服务定义存在"
assert_contains "$APPS" '^  emotion-echo-sensevoice:' "emotion-echo-sensevoice 服务定义存在"
assert_contains "$APPS" '^  emotion-echo-xtts:' "emotion-echo-xtts 服务定义存在（Stage 36 v0.1.0 build 成功过）"

echo
echo "--- 3) ai-svc 仍含 XTTS_BASE_URL 容器 DNS（dev 默认不起 AI，URL 无害）---"
assert_contains "$APPS" 'XTTS_BASE_URL.*emotion-echo-xtts:8003' \
  "ai-svc XTTS_BASE_URL 默认指向容器 DNS（--profile ai 启动时才可达）"

echo
echo "--- 4) dev 默认 vs --profile ai service 数差异 ---"
cd "$DEPLOY_DIR"
DEFAULT_SVCS=$(docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml config --services 2>/dev/null | sort)
AI_SVCS=$(docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml --profile ai config --services 2>/dev/null | sort)

DEFAULT_COUNT=$(echo "$DEFAULT_SVCS" | wc -l)
AI_COUNT=$(echo "$AI_SVCS" | wc -l)

echo "  默认配置 services 数：$DEFAULT_COUNT"
echo "  --profile ai 配置 services 数：$AI_COUNT"

if [ "$((AI_COUNT - DEFAULT_COUNT))" = "3" ]; then
  echo "  ✓ 默认 vs profile ai 差异恰好 +3（FER+SV+XTTS）"
  pass=$((pass + 1))
else
  echo "  ✗ 默认 vs profile ai 差异 = $((AI_COUNT - DEFAULT_COUNT))（期望 +3）"
  fail=$((fail + 1))
fi
cd - > /dev/null

echo
echo "=== Result: $pass passed, $fail failed ==="
if [ $fail -gt 0 ]; then
  exit 1
fi
exit 0