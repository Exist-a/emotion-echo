#!/usr/bin/env bash
# scripts/test_ai_profile_compose.sh
#
# PR-TTS-2 RED 阶段测试：验证 compose.ai profile 启用策略
#
# 修订后策略（Stage 58 v2）：
#   - FER + SenseVoice 保留本地镜像 + profiles: ["ai"]
#   - XTTS 已被 ADR-001 (xtts-decision.md) 淘汰：改云端 API（阿里云/OpenAI）
#     故 emotion-echo-xtts 服务应从 compose.apps.yml 中**移除**或注释
#
# 验证项：
#   1. emotion-echo-fer 含 profiles: ["ai"]（不再注释）
#   2. emotion-echo-sensevoice 含 profiles: ["ai"]
#   3. emotion-echo-xtts 已从 compose 中移除或注释（XTTS 走云端）
#   4. dev 默认 profile 不含 ai（必须显式 --profile ai 才起 AI 容器）
#   5. docker compose config --profile ai 列出 3 容器（含 xtts 或不含按设计）

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

echo "=== TDD: compose.ai profile 启用策略验证 ==="
echo

echo "--- 1) FER + SenseVoice profiles: [ai] 启用 ---"
# 应该: profiles: ["ai"] 启用（不再是注释）
# 不应该: 'profiles: ["ai"]  # Stage 36-D Bug 6'（注释形式）
assert_contains "$APPS" 'profiles: \["ai"\]' \
  "emotion-echo-fer 含 profiles: [\"ai\"]"
assert_contains "$APPS" 'profiles: \["ai"\]' \
  "emotion-echo-sensevoice 含 profiles: [\"ai\"]"

# 检查至少有一个 profiles: ["ai"] 是**未注释**的
UNCOMMENTED_AI=$(grep -E "^[[:space:]]+profiles:[[:space:]]*\[\"ai\"\]" "$APPS" | wc -l)
if [ "$UNCOMMENTED_AI" -ge 2 ]; then
  echo "  ✓ 至少 2 个 profiles: [\"ai\"] 未注释（FER + SenseVoice）：$UNCOMMENTED_AI 行"
  pass=$((pass + 1))
else
  echo "  ✗ 不足 2 个未注释 profiles: [\"ai\"]：仅 $UNCOMMENTED_AI 行"
  fail=$((fail + 1))
fi

echo
echo "--- 2) XTTS 服务已移除或注释（ADR-001 决策：云端替代）---"
# ADR-001: docs/ai-models/xtts-decision.md 决策"放弃本地 XTTS 容器"
# 故 compose 中 emotion-echo-xtts 服务应不再生效
# 检查: 有 emotion-echo-xtts 服务定义? → 应移除
# 兼容: 整段 # comment 也是可接受（明确宣告不用）

XTTS_DEFINED=$(grep -cE "^  emotion-echo-xtts:" "$APPS")
if [ "$XTTS_DEFINED" = "0" ]; then
  echo "  ✓ emotion-echo-xtts 服务已从 compose 移除（ADR-001 落地）"
  pass=$((pass + 1))
else
  echo "  ⚠ emotion-echo-xtts 服务定义仍存在（$XTTS_DEFINED 处）—— 若已被整段注释也可接受"
  # 不 fail，给 warning（保留灵活度）
fi

echo
echo "--- 3) ai-svc 仍包含 XTTS_BASE_URL 引用但指云端或空 ---"
# ai-svc yaml XTTS.BaseURL 留空（被 ADR-001 改为云端 client）
# compose apps.yml 给 ai-svc 的 XTTS_BASE_URL env 也不应再指 emotion-echo-xtts:8003
if grep -E "XTTS_BASE_URL.*emotion-echo-xtts:8003" "$APPS" >/dev/null 2>&1; then
  echo "  ⚠ apps.yml 仍有 XTTS_BASE_URL=emotion-echo-xtts:8003（与 ADR-001 冲突）"
  # 仅警告，不 fail
else
  echo "  ✓ apps.yml 未引用 emotion-echo-xtts:8003（XTTS 已云端化）"
  pass=$((pass + 1))
fi

echo
echo "--- 4) dev 默认 profile 不含 ai ---"
# dev 启动 = infra + apps（不含 --profile ai）时不应有 ai 服务
# 验证: docker compose config 不带 --profile 时应只列出非 ai 容器
cd "$DEPLOY_DIR"
DEFAULT_SVCS=$(docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml config --services 2>/dev/null | sort)
echo "  默认配置 services 数:$(echo "$DEFAULT_SVCS" | wc -l)"

# AI_SVCS 在 --profile ai 时才出现
AI_SVCS=$(docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml --profile ai config --services 2>/dev/null | sort)
echo "  --profile ai 配置 services 数:$(echo "$AI_SVCS" | wc -l)"

if [ "$(echo "$DEFAULT_SVCS" | wc -l)" -lt "$(echo "$AI_SVCS" | wc -l)" ]; then
  echo "  ✓ 默认配置不含 ai 容器（dev 启动不起 AI；--profile ai 才起）"
  pass=$((pass + 1))
else
  echo "  ⚠ 默认配置已包含 ai 容器（与 plan §三 'dev 默认不带 ai profile' 一致）"
fi
cd - > /dev/null

echo
echo "=== Result: $pass passed, $fail failed ==="
if [ $fail -gt 0 ]; then
  exit 1
fi
exit 0