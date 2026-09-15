#!/usr/bin/env bash
# scripts/test_dockerfile_arg_pattern.sh
#
# Round D digest pin ARG 模式回归测试（2026-09-16）
#
# 背景：Round D ded2efc 把 8 个 Dockerfile 改为 ARG + ${VAR_DIGEST:-tag}
# 形式钉住基础镜像。设计意图 = dev compose 不强制（VAR 未填时 fallback 到 tag），
# 但 buildkit 静态分析时拒绝 image@${VAR:-tag} 形式（@ 后必须 sha256 digest），
# 导致 dev 模式 build 全部失败：failed to parse stage name "alpine@alpine:3.19"
#
# 正确模式（buildkit 官方支持）：
#   ARG XXX_IMAGE=image:tag       # 完整 image:tag 作为默认 image
#   ARG XXX_DIGEST                # digest 可选
#   FROM ${XXX_IMAGE}${XXX_DIGEST:+@${XXX_DIGEST}}
#
# dev 模式（不传 DIGEST）→ FROM alpine:3.19
# prod 模式（传 DIGEST=sha256:abc）→ FROM alpine:3.19@sha256:abc
#
# 验证项：
#   1. 8 个业务 Dockerfile 都用 ${VAR_DIGEST:+@${VAR_DIGEST}} 模式（不是 :-tag）
#   2. 每个 FROM 行至少含一个 ${VAR_DIGEST} 变量名（digest pin 形式）
#   3. 不再出现 "${VAR:-image:tag}" 这种被 buildkit 拒绝的 @ 后 fallback
#
# 关联：
#   docs/evidence/round-d-dockerfile-digest-pin.md
#   scripts/check_docker_digests.sh（已接受 ${VAR_DIGEST} 形式）

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$SCRIPT_DIR/.."

# 8 个业务 Dockerfile（与 round-d 登记一致）
DOCKERFILES=(
  "emotion-echo-chat-svc/Dockerfile"
  "emotion-echo-user-svc/Dockerfile"
  "emotion-echo-analytics-svc/Dockerfile"
  "emotion-echo-assessment-svc/Dockerfile"
  "emotion-echo-web-bff/Dockerfile"
  "emotion-echo-web/Dockerfile"
  "emotion-echo-web/Dockerfile.dev"
  "emotion-llm-service/Dockerfile"
)

pass=0
fail=0

assert_not_contains() {
  local file="$1"
  local pattern="$2"
  local desc="$3"
  if grep -qE "$pattern" "$file" 2>/dev/null; then
    echo "  ✗ $desc"
    grep -nE "$pattern" "$file" | head -3 | sed 's/^/      /'
    fail=$((fail + 1))
  else
    echo "  ✓ $desc"
    pass=$((pass + 1))
  fi
}

assert_contains() {
  local file="$1"
  local pattern="$2"
  local desc="$3"
  if grep -qE "$pattern" "$file" 2>/dev/null; then
    echo "  ✓ $desc"
    pass=$((pass + 1))
  else
    echo "  ✗ $desc (缺 pattern: $pattern)"
    fail=$((fail + 1))
  fi
}

echo "=== TDD: Round D Dockerfile digest ARG 模式回归 ==="
echo

cd "$ROOT_DIR"

for df in "${DOCKERFILES[@]}"; do
  if [ ! -f "$df" ]; then
    echo "  ⊘ skip $df (not found)"
    continue
  fi
  echo "--- $df ---"

  # 反向断言：不能出现 ${VAR:-image:tag} 形式（buildkit 拒绝）
  # 这是 buildkit 警告 "InvalidDefaultArgInFrom" 的根因
  # 匹配 @ 后跟 ${...:-...} 的形式
  assert_not_contains "$df" '@\$\{[A-Z_0-9]+_DIGEST:-' \
    "无 '@\${XXX_DIGEST:-default}' 形式（buildkit 拒绝 image@\${...:-tag}）"

  # 反向断言：不能出现 ARG XXX_DIGEST="..." 默认值非空
  # （ARG 默认值非空 + @ fallback 仍会被 buildkit 警告）
  assert_not_contains "$df" '^ARG [A-Z_0-9]+_DIGEST="[^"]' \
    "ARG XXX_DIGEST 不应有非空默认值"

  # 正向断言：必须含 \${XXX_DIGEST:+@\${XXX_DIGEST}} 形式（buildkit 支持）
  assert_contains "$df" '\$\{[A-Z_0-9]+_DIGEST:\+@' \
    "含 \${XXX_DIGEST:+@\${...}} 标准 digest pin 模式"

  echo
done

echo "=== Result: $pass passed, $fail failed ==="
if [ $fail -gt 0 ]; then
  exit 1
fi
exit 0
