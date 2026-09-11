#!/usr/bin/env bash
# download-sv-wheels.sh — SV-fastbuild 本地 wheels 预下脚本
#
# 解决的问题（commit 50f2a3d + force-push 验证）：
#   emotion-echo-models/SV-fastbuild/wheels/ 6 个 .whl（190MB torch + 5 个小包）
#   原 commit 进 git → push 阻塞 GH001 (>100MB)。
#   已 git-filter-repo 从历史删除 + 加 .gitignore + .gitkeep。
#
# 但 Docker build (SV-fastbuild/Dockerfile COPY wheels/ + pip --find-links /build/wheels/)
# 仍需要这些 wheels 存在于本地。本脚本负责首次/重建时预下。
#
# 用法：
#   bash scripts/download-sv-wheels.sh            # 下载到默认 emotion-echo-models/SV-fastbuild/wheels/
#   WHEELS_DIR=/path/to/wheels bash ...           # 自定义目标目录
#   PYTORCH_INDEX=https://... bash ...            # 自定义 torch 镜像（默认官方 download.pytorch.org）
#   PIP_INDEX_URL=https://... bash ...            # 自定义其他包 PyPI 镜像（默认 pypi.org/simple）
#
# 退出码：0 = 全部成功；非 0 = 至少 1 个 wheel 失败
#
# 实现要点（commit 验证）：
#   1. 不使用 pip download。原因：
#      - pip 26+ 列 PyTorch CPU index 时 torch==1.13.1+cpu 目录已被 PyTorch 官方
#        删除（目录返回 403，但文件还在 S3），pip 找不到候选版本
#      - pip 26 的 --extra-index-url + --index-url 行为有变化
#      - 我们只需 6 个 .whl，curl 直拉更可控
#   2. URLs 写死但用变量覆盖：PYTORCH_INDEX / PIP_INDEX_URL 允许切镜像。
#   3. 重试逻辑：MAX_RETRY / RETRY_DELAY（跟 scripts/build_dev_images.sh 一致）。
#   4. 大小校验：torch wheel < 100MB 视为下载残缺。
#   5. PyPI simple index 解析：grep 出 href 中匹配 file 名的链接，strip sha256 后缀
#      （如 .whl#sha256=abc → .whl）；再选 cp310/cp310-manylinux1_x86_64 这种 linux x86_64 优先
#      （因为 Dockerfile 基于 python:3.10-slim，--platform linux/amd64）。
#
# 重新生成 wheels/ 的时机：
#   - requirements.txt 变更（加/升/降依赖）
#   - 现有 wheel 文件损坏或缺失
#   - 切换 Python 版本（3.10 → 3.11 等）

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
SV_DIR="${SV_DIR:-$REPO_ROOT/emotion-echo-models/SV-fastbuild}"
WHEELS_DIR="${WHEELS_DIR:-$SV_DIR/wheels}"

# 镜像源
PYTORCH_INDEX="${PYTORCH_INDEX:-https://download.pytorch.org/whl/cpu}"
PIP_INDEX_URL="${PIP_INDEX_URL:-https://pypi.org/simple}"

# 重试
MAX_RETRY="${MAX_RETRY:-3}"
RETRY_DELAY="${RETRY_DELAY:-3}"

echo "==> download-sv-wheels.sh"
echo "    sv_dir       = $SV_DIR"
echo "    wheels_dir   = $WHEELS_DIR"
echo "    pytorch_idx  = $PYTORCH_INDEX"
echo "    pip_idx      = $PIP_INDEX_URL"
echo

mkdir -p "$WHEELS_DIR"

# 6 个 wheel 的 (package_name_on_pypi, target_filename, base_url) 配置。
# - torch 必须用 PyTorch 官方 CPU index（aliyun 不镜像 +cpu 变体；pypi.org/simple 也不含）
# - 其他 5 个用 pypi.org/simple 解析
#
# 解析后缀要求：cp310-cp310-linux_x86_64 / manylinux1_x86_64 / none-any 皆可。
# 选 linux 优先（Dockerfile 用 python:3.10-slim --platform linux/amd64）。

# 格式: package_name|target_filename|base_url（base_url 必须以 / 结尾）
WHEELS=(
  "torch|torch-1.13.1+cpu-cp310-cp310-linux_x86_64.whl|${PYTORCH_INDEX}/"
  "funasr|funasr-1.4.15-py3-none-any.whl|${PIP_INDEX_URL}/funasr/"
  "torchaudio|torchaudio-0.12.1-cp310-cp310-manylinux1_x86_64.whl|${PIP_INDEX_URL}/torchaudio/"
  "modelscope|modelscope-1.40.0-py3-none-any.whl|${PIP_INDEX_URL}/modelscope/"
  "gradio|gradio-6.26.0-py3-none-any.whl|${PIP_INDEX_URL}/gradio/"
  "huggingface-hub|huggingface_hub-1.30.0-py3-none-any.whl|${PIP_INDEX_URL}/huggingface-hub/"
)

# 解析 PyPI simple index → 目标 wheel 的精确 URL
# 策略：
#   1. fetch simple index 页面
#   2. grep 所有 href 含 target_filename 的链接（PyPI HTML href 是完整 https URL）
#   3. 优先选 linux_x86_64（cp310-cp310-manylinux1_x86_64 / linux_x86_64），
#      其次 none-any，最后 fallback 第一个匹配
#   4. strip 末尾 #sha256=... 后缀
resolve_pypi_url() {
  local target_filename="$1"
  local index_url="$2"
  local index_html
  index_html=$(curl -sSfL --connect-timeout 15 --max-time 60 "$index_url" 2>/dev/null) || {
    echo "    [WARN] fetch $index_url 失败" >&2
    return 1
  }
  # 找所有匹配 target_filename 的 href（PyPI simple 用完整 https URL + #sha256= 后缀）
  local candidates
  candidates=$(echo "$index_html" \
    | grep -oE 'href="https://files\.pythonhosted\.org/[^"]*'"$(basename "$target_filename" .whl)"'[^"]*\.whl#sha256=[^"]*"' \
    | sed -E 's/href="//; s/#sha256=[^"]*"$//' \
    | sort -u)
  if [ -z "$candidates" ]; then
    return 1
  fi
  # 优先 linux x86_64
  local linux
  linux=$(echo "$candidates" | grep -E 'manylinux.*x86_64|linux_x86_64' | head -1)
  if [ -n "$linux" ]; then
    echo "$linux"
    return 0
  fi
  # 其次 none-any（跨平台）
  local none_any
  none_any=$(echo "$candidates" | grep -E 'none-any' | head -1)
  if [ -n "$none_any" ]; then
    echo "$none_any"
    return 0
  fi
  # 兜底第一个
  echo "$candidates" | head -1
}

# 单个 wheel 下载（带重试 + 大小校验）
download_wheel() {
  local package_name="$1"
  local target_filename="$2"
  local base_url="$3"
  local target="$WHEELS_DIR/$target_filename"
  local attempt=1

  # 跳过已存在且大小正确的
  if [ -f "$target" ]; then
    local existing_size
    existing_size=$(stat -c%s "$target" 2>/dev/null || stat -f%z "$target" 2>/dev/null || echo 0)
    if [ "$existing_size" -gt 1000 ]; then
      echo "  [SKIP] $target_filename 已存在 (${existing_size}B)"
      return 0
    fi
    echo "  [WARN] $target_filename 存在但过小 (${existing_size}B)，重新下载"
  fi

  while [ $attempt -le "$MAX_RETRY" ]; do
    echo "  [attempt $attempt/$MAX_RETRY] $target_filename"

    # 解析 URL
    local file_url
    if [[ "$package_name" == "torch" ]]; then
      # torch 直链：download.pytorch.org/whl/cpu/<target_filename>
      file_url="${base_url}${target_filename}"
    else
      # PyPI simple：解析
      file_url=$(resolve_pypi_url "$target_filename" "$base_url") || file_url=""
    fi

    if [ -z "$file_url" ]; then
      echo "  [retry] 无法解析 URL（package=$package_name index=$base_url）"
      sleep "$RETRY_DELAY"
      attempt=$((attempt + 1))
      continue
    fi

    # 下载
    if curl -sSfL --retry 0 --connect-timeout 30 --max-time 600 \
         -o "$target.tmp" "$file_url" 2>&1 | tail -3; then
      # 校验：文件大小（torch 必 >= 100MB；其他 >= 1KB）
      local size
      size=$(stat -c%s "$target.tmp" 2>/dev/null || stat -f%z "$target.tmp" 2>/dev/null || echo 0)
      local min_size=1000
      [[ "$target_filename" == torch-* ]] && min_size=100000000

      if [ "$size" -ge "$min_size" ]; then
        mv "$target.tmp" "$target"
        echo "  [OK] $target_filename (${size}B)"
        return 0
      else
        echo "  [FAIL] $target_filename 大小异常 ${size}B (< ${min_size}B)"
        rm -f "$target.tmp"
      fi
    fi
    sleep "$RETRY_DELAY"
    attempt=$((attempt + 1))
  done
  return 1
}

# 主流程
fail=0
for entry in "${WHEELS[@]}"; do
  package_name=$(echo "$entry" | cut -d'|' -f1)
  target_filename=$(echo "$entry" | cut -d'|' -f2)
  base_url=$(echo "$entry" | cut -d'|' -f3)
  if ! download_wheel "$package_name" "$target_filename" "$base_url"; then
    echo "ERROR: 下载失败: $target_filename" >&2
    fail=1
  fi
done

if [ "$fail" -ne 0 ]; then
  echo
  echo "ERROR: 至少 1 个 wheel 失败" >&2
  echo "  排查建议：" >&2
  echo "    1. curl -I $PYTORCH_INDEX/torch-1.13.1+cpu-cp310-cp310-linux_x86_64.whl" >&2
  echo "    2. 切 PyPI 镜像：PIP_INDEX_URL=https://mirrors.tuna.tsinghua.edu.cn/pypi/web/simple bash $0" >&2
  echo "    3. 检查 requirements.txt 是否仍兼容 Python 3.10 / linux_x86_64" >&2
  exit 1
fi

# 总结
echo
echo "==> 完成"
echo "    wheels 数量: $(ls -1 "$WHEELS_DIR"/*.whl 2>/dev/null | wc -l)"
echo "    wheels 大小: $(du -sh "$WHEELS_DIR" 2>/dev/null | cut -f1)"
echo
echo "下一步：构建 SV-fastbuild 镜像"
echo "    docker build -t emotion-echo/sensevoice-fastbuild:test \\"
echo "      -f emotion-echo-models/SV-fastbuild/Dockerfile \\"
echo "      emotion-echo-models/SV-fastbuild/"
