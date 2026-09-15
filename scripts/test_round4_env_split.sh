#!/usr/bin/env bash
# scripts/test_round4_env_split.sh
#
# Round 4 env split + dev log level + CORS 文档对齐 RED 阶段测试
#
# 验证项：
#   1. docker-compose.apps.yml 中 ai-svc 暴露 AI_LLM_INTERNAL_API_KEY env（${...:-} 形式）
#   2. docker-compose.apps.yml 中 web-bff 暴露 BFF_LLM_INTERNAL_API_KEY env（${...:-} 形式）
#   3. compose.dev.yml 覆盖 LOG_LEVEL=DEBUG（dev 调试便利）
#   4. configuration.md §5 列出 5 业务 svc CORS override（文档与实际行为对齐）
#
# 关联文档：
#   - docs/architecture/roadmap.md Round 3.5 跨 svc 隔离 API key
#   - deploy/configuration.md §4 差异表（dev LOG_LEVEL=DEBUG）
#   - deploy/configuration.md §5 当前覆盖项

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$SCRIPT_DIR/.."
APPS_FILE="$ROOT_DIR/deploy/docker-compose.apps.yml"
DEV_FILE="$ROOT_DIR/deploy/compose.dev.yml"
CONFIG_DOC="$ROOT_DIR/deploy/configuration.md"

pass=0
fail=0

assert_uses_default_in_block() {
  # 在指定 service block 下查找 ${VAR:-} 形式
  local file="$1"
  local svc="$2"
  local var="$3"
  local desc="$4"
  # 用 awk 提取 svc block 范围（service: 后到下一个 同级 service: 或文件尾）
  local pattern
  pattern='\$\{'${var}':-'
  # 简化：grep 多行，用 sed 提取 svc block 后再 grep
  local block
  block=$(awk -v target="^  ${svc}:" '
    $0 ~ target { in_block = 1; next }
    in_block && /^  [a-zA-Z0-9_-]+:/ && $0 !~ target { in_block = 0 }
    in_block { print }
  ' "$file")
  if echo "$block" | grep -qE "$pattern"; then
    echo "  ✓ $desc"
    pass=$((pass + 1))
  else
    echo "  ✗ $desc (在 ${svc} block 中找不到 \${${var}:-})"
    fail=$((fail + 1))
  fi
}

assert_file_contains() {
  local file="$1"
  local pattern="$2"
  local desc="$3"
  if [ -f "$file" ] && grep -qE "$pattern" "$file"; then
    echo "  ✓ $desc"
    pass=$((pass + 1))
  else
    echo "  ✗ $desc (file: $file, pattern: $pattern)"
    fail=$((fail + 1))
  fi
}

echo "=== TDD Round 4: env split + dev log level + CORS 文档对齐 ==="
echo

echo "--- 1) ai-svc 暴露 AI_LLM_INTERNAL_API_KEY env ---"
assert_uses_default_in_block "$APPS_FILE" "emotion-echo-ai-svc" \
  "AI_LLM_INTERNAL_API_KEY" \
  "ai-svc block 含 \${AI_LLM_INTERNAL_API_KEY:-}"

echo
echo "--- 2) web-bff 暴露 BFF_LLM_INTERNAL_API_KEY env ---"
assert_uses_default_in_block "$APPS_FILE" "emotion-echo-web-bff" \
  "BFF_LLM_INTERNAL_API_KEY" \
  "web-bff block 含 \${BFF_LLM_INTERNAL_API_KEY:-}"

echo
echo "--- 3) dev override 覆盖 LOG_LEVEL=DEBUG ---"
assert_file_contains "$DEV_FILE" "LOG_LEVEL" \
  "compose.dev.yml 含 LOG_LEVEL 配置（dev 调试便利）"

echo
echo "--- 4) configuration.md §5 解释 5 业务 svc CORS override 实际行为 ---"
# §5 当前是 yaml 代码块列了 5 业务 svc CORS 行（compose.dev.yml:118-120），
# 文档应明确说明这一覆盖项的实际行为（dev 默认 = localhost:3000，可由 .env.local 覆盖）
# 抽取 §5 整段（§5 起 → §6 起），检查是否含 CORS_ALLOW_ORIGINS + localhost:3000 + 5 svc 之一
SECTION5=$(awk '/^## 5\./{flag=1; next} /^## 6\./{flag=0} flag' "$CONFIG_DOC")
if echo "$SECTION5" | grep -q "CORS_ALLOW_ORIGINS" \
   && echo "$SECTION5" | grep -q "localhost:3000" \
   && echo "$SECTION5" | grep -qE "emotion-echo-(chat|user|ai|analytics|assessment)-svc"; then
  echo "  ✓ configuration.md §5 解释 5 业务 svc CORS 实际覆盖行为"
  pass=$((pass + 1))
else
  echo "  ✗ configuration.md §5 缺 5 业务 svc CORS 解释（CORS_ALLOW_ORIGINS + localhost:3000 + svc 名 缺一）"
  fail=$((fail + 1))
fi

echo
echo "=== Result: $pass passed, $fail failed ==="
if [ $fail -gt 0 ]; then
  exit 1
fi
exit 0
