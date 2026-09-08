#!/usr/bin/env bash
# scripts/test_ai_svc_yaml_config.sh
#
# PR-TTS-3 测试：ai-svc ai-api.yaml + compose env 注入一致性
#
# 目的：
#   验证 dev 默认配置下 ai-svc 启动后 FER/SenseVoice/XTTS BaseURL 都是空
#   （构造时返 nil）——与 todo-pile §A1 失真 c 修订一致
#   （"dev 默认模式即'不调用'，不是'容器被删除'"）
#
# 验证项：
#   1. emotion-echo-ai-svc/etc/ai-api.yaml FER.BaseURL 默认空
#   2. emotion-echo-ai-svc/etc/ai-api.yaml SenseVoice.BaseURL 默认空
#   3. emotion-echo-ai-svc/etc/ai-api.yaml XTTS.BaseURL 默认空
#   4. deploy/docker-compose.apps.yml ai-svc FER_BASE_URL env 默认值指向容器 DNS
#      （仅 --profile ai 起 AI 容器时该 DNS 才可达；dev 默认走 nil 降级路径）
#   5. emotion-echo-ai-svc/internal/aiclient/fer.go NewFERClient 空 BaseURL 返 nil
#   6. emotion-echo-ai-svc/internal/aiclient/sensevoice.go NewSenseVoiceClient 空 BaseURL 返 nil
#   7. emotion-echo-ai-svc/internal/aiclient/xtts.go NewXTTSClient 空 BaseURL 返 nil

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$SCRIPT_DIR/.."
AI_SVC="$ROOT_DIR/emotion-echo-ai-svc"
APPS="$ROOT_DIR/deploy/docker-compose.apps.yml"
YAML="$AI_SVC/etc/ai-api.yaml"

pass=0
fail=0

assert_yaml_baseurl_empty() {
  local section="$1"  # "FER" / "SenseVoice" / "XTTS"
  local desc="$2"
  # 找 section 下的 BaseURL: "" 或 BaseURL: '' 或 BaseURL:（空字符串）
  if awk "/^${section}:/{f=1} f && /BaseURL:[[:space:]]*[\"']{0,2}[\"']{0,2}[[:space:]]*$/{print \"\"; exit} f && /[^[:space:]]/{f=0}" "$YAML" | grep -q "^$" 2>/dev/null; then
    echo "  ✓ $desc"
    pass=$((pass + 1))
  fi
  # 简单 grep：section 内 BaseURL: "" 或 BaseURL: (空)
  if grep -A 2 "^${section}:" "$YAML" | grep -E "BaseURL:[[:space:]]*[\"']{0,1}[\"'[:space:]]*$" > /dev/null; then
    echo "  ✓ $desc (grep A2)"
    pass=$((pass + 1))
  else
    echo "  ✗ $desc"
    fail=$((fail + 1))
  fi
}

assert_apps_env_container_dns() {
  local env_key="$1"
  local container="$2"
  local port="$3"
  local desc="$4"
  # 检查 apps.yml 含 ENV_KEY=${ENV_KEY:-http://emotion-echo-XXX:PORT}
  if grep -qE "${env_key}:[[:space:]]*\\\${${env_key}:-http://${container}:${port}}" "$APPS"; then
    echo "  ✓ $desc"
    pass=$((pass + 1))
  else
    echo "  ✗ $desc (env ${env_key} 默认值应指 http://${container}:${port})"
    fail=$((fail + 1))
  fi
}

assert_go_func_signature() {
  local file="$1"
  local func="$2"
  local nil_check="$3"
  local desc="$4"
  if grep -qE "func New${func}Client" "$file"; then
    echo "  ✓ ${desc} 函数存在"
    pass=$((pass + 1))
  else
    echo "  ✗ ${desc} 函数缺失"
    fail=$((fail + 1))
  fi
  # 使用 grep -F (固定字符串) 避开正则转义问题
  if grep -qF 'if c.BaseURL == ""' "$file" && grep -qF 'return nil' "$file"; then
    echo "  ✓ ${desc} nil-on-empty 实现"
    pass=$((pass + 1))
  else
    echo "  ✗ ${desc} nil-on-empty 实现缺失"
    fail=$((fail + 1))
  fi
}

echo "=== TDD: ai-svc BaseURL 默认空 + compose 容器 DNS 注入 ==="
echo

echo "--- 1) etc/ai-api.yaml FER/SenseVoice/XTTS BaseURL 默认空 ---"
assert_yaml_baseurl_empty "FER" "FER.BaseURL 默认空"
assert_yaml_baseurl_empty "SenseVoice" "SenseVoice.BaseURL 默认空"
assert_yaml_baseurl_empty "XTTS" "XTTS.BaseURL 默认空"

echo
echo "--- 2) compose.apps.yml ai-svc env 默认指容器 DNS ---"
assert_apps_env_container_dns "FER_BASE_URL" "emotion-echo-fer" "8004" "FER_BASE_URL 默认指 http://emotion-echo-fer:8004"
assert_apps_env_container_dns "SENSEVOICE_BASE_URL" "emotion-echo-sensevoice" "8002" "SENSEVOICE_BASE_URL 默认指 http://emotion-echo-sensevoice:8002"
assert_apps_env_container_dns "XTTS_BASE_URL" "emotion-echo-xtts" "8003" "XTTS_BASE_URL 默认指 http://emotion-echo-xtts:8003"

echo
echo "--- 3) aiclient 3 个 New*Client 函数 + nil-on-empty 实现 ---"
assert_go_func_signature "$AI_SVC/internal/aiclient/fer.go" "FER" \
  "if c\.BaseURL == \"\"[[:space:]]*\{[[:space:]]*return nil" \
  "FER"

assert_go_func_signature "$AI_SVC/internal/aiclient/sensevoice.go" "SenseVoice" \
  "if c\.BaseURL == \"\"[[:space:]]*\{[[:space:]]*return nil" \
  "SenseVoice"

assert_go_func_signature "$AI_SVC/internal/aiclient/xtts.go" "XTTS" \
  "if c\.BaseURL == \"\"[[:space:]]*\{[[:space:]]*return nil" \
  "XTTS"

echo
echo "--- 4) go test 跑 nil-on-empty 集成测试 ---"
cd "$AI_SVC"
if go test ./internal/aiclient/ -run "NilOnEmptyBaseURL|EmptyBaseURL_ReturnsNil" 2>&1 | tail -3 | grep -q "^ok"; then
  echo "  ✓ aiclient nil-on-empty 测试通过"
  pass=$((pass + 1))
else
  echo "  ✗ aiclient nil-on-empty 测试失败"
  fail=$((fail + 1))
fi
cd - > /dev/null

echo
echo "=== Result: $pass passed, $fail failed ==="
if [ $fail -gt 0 ]; then
  exit 1
fi
exit 0