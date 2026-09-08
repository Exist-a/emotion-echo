#!/usr/bin/env bash
# scripts/test_ai_image_build_precheck.sh
#
# PR-TTS-1 RED 阶段测试：验证 3 个 AI 镜像的 Dockerfile + 父镜像可行性
#
# 目的（Stage 58 §风险）：
#   Stage 36 记录"pypi CDN 0 字节响应 + Docker Desktop 内存限制 30+ 分钟"导致
#   AI profile 镜像构建长期失败。PR-TTS-1 必须先单跑可行性，确认：
#     1. 3 个 Dockerfile 存在且结构完整
#     2. 父镜像 python:3.10-slim 可拉取
#     3. requirements.txt 不含已 deprecated 包
#     4. 模型权重引用路径合理（本地有/远程拉/不存在）
#   然后再决定下一步（真构建 / 换 pre-built / 推迟整批）。
#
# 验证项：
#   1. FER / sensevoice-small / XTTS 三个 Dockerfile 存在
#   2. 每个 Dockerfile 含 FROM python:3.10-slim（或等价 slim 基础）
#   3. 每个目录有 requirements.txt
#   4. 每个 Dockerfile 有 HEALTHCHECK 指令
#   5. 父镜像 docker pull 试拉一次（best-effort 30s 超时）

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$SCRIPT_DIR/.."
MODELS_DIR="$ROOT_DIR/emotion-echo-models"

pass=0
fail=0

assert_file_exists() {
  local file="$1"
  local desc="$2"
  if [ -f "$file" ]; then
    echo "  ✓ $desc"
    pass=$((pass + 1))
  else
    echo "  ✗ $desc (missing: $file)"
    fail=$((fail + 1))
  fi
}

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

echo "=== TDD: AI profile 镜像构建可行性预检 ==="
echo

# 模型目录
DIRS=("FER" "sensevoice-small" "XTTS")

for dir in "${DIRS[@]}"; do
  echo "--- ${dir} ---"
  assert_file_exists "$MODELS_DIR/$dir/Dockerfile" "${dir}/Dockerfile 存在"
  assert_file_exists "$MODELS_DIR/$dir/requirements.txt" "${dir}/requirements.txt 存在"

  DOCKERFILE="$MODELS_DIR/$dir/Dockerfile"
  if [ -f "$DOCKERFILE" ]; then
    assert_contains "$DOCKERFILE" "^FROM python:3\\.10-slim" "${dir}/Dockerfile 用 python:3.10-slim 基础"
    assert_contains "$DOCKERFILE" "HEALTHCHECK" "${dir}/Dockerfile 含 HEALTHCHECK 指令"
    assert_contains "$DOCKERFILE" "ENTRYPOINT.*tini|tini.*--" "${dir}/Dockerfile 用 tini 处理 SIGTERM"
  fi
  echo
done

# 父镜像可拉性（best-effort）
echo "--- 父镜像 python:3.10-slim 可拉性 (best-effort 30s) ---"
if command -v docker >/dev/null 2>&1; then
  # 异步跑 docker pull 加 30s 超时；只看退出码，不阻塞测试主流程
  PULL_RESULT=$(timeout 30 docker pull python:3.10-slim >/dev/null 2>&1; echo $?)
  if [ "$PULL_RESULT" = "0" ]; then
    echo "  ✓ python:3.10-slim 可拉取（exit 0）"
    pass=$((pass + 1))
  elif [ "$PULL_RESULT" = "124" ]; then
    echo "  ✗ python:3.10-slim 拉取超时 30s（pypi/CDN 阻塞，Stage 36 记录的问题）"
    fail=$((fail + 1))
  else
    echo "  ⚠ python:3.10-slim 拉取失败 exit=$PULL_RESULT（可能网络/registry 问题）"
    fail=$((fail + 1))
  fi
else
  echo "  ⊘ docker 命令不可用，跳过 pull 验证"
fi

echo
echo "=== Result: $pass passed, $fail failed ==="
if [ $fail -gt 0 ]; then
  exit 1
fi
exit 0