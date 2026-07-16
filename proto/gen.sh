#!/usr/bin/env bash
# proto/gen.sh · Emotion-Echo proto 一键生成脚本
#
# 用途：从 proto/*.proto 自动生成 Go + Python 代码到目标位置
#
# 输出位置：
#   - Go pb:        emotion-echo-shared/pkg/emotionllm/
#   - Go pb:        emotion-echo-shared/pkg/emotionquery/
#   - Python pb:    emotion-llm-service/
#
# 用法：
#   bash proto/gen.sh                    # 生成全部
#   bash proto/gen.sh emotion_llm.proto  # 只生成指定 proto
#
# 前置依赖：
#   - protoc          (从 https://github.com/protocolbuffers/protobuf/releases 下载)
#   - protoc-gen-go   (go install google.golang.org/protobuf/cmd/protoc-gen-go@latest)
#   - protoc-gen-go-grpc (go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest)
#   - grpcio-tools    (pip install grpcio-tools)
#
# 为什么有这个脚本：
#   避免 pb 文件散落到仓库根目录，统一归位到 emotion-echo-shared/pkg/

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PROTO_DIR="${ROOT}/proto"
SHARED_PKG="${ROOT}/emotion-echo-shared/pkg"
PYTHON_OUT="${ROOT}/emotion-llm-service"

# 颜色输出
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

log_info() { echo -e "${GREEN}[INFO]${NC} $*"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $*"; }
log_err()  { echo -e "${RED}[ERROR]${NC} $*"; }

# 依赖检查
check_deps() {
    local missing=0
    for cmd in protoc protoc-gen-go protoc-gen-go-grpc; do
        if ! command -v "$cmd" &> /dev/null; then
            log_err "缺失依赖: $cmd"
            missing=1
        fi
    done
    if ! python -c "import grpc_tools" &> /dev/null; then
        log_warn "缺失 Python 依赖: grpc_tools (pip install grpcio-tools)"
    fi
    if [[ $missing -eq 1 ]]; then
        log_err "请先安装缺失的依赖，参见本文件头注释"
        exit 1
    fi
}

# 确定要生成的 proto（默认全部）
PROTOS="${1:-}"
if [[ -z "$PROTOS" ]]; then
    PROTOS="$(ls "${PROTO_DIR}"/*.proto | xargs -n1 basename | tr '\n' ' ')"
fi

# ============================================
# Go 代码生成
# ============================================
gen_go() {
    local proto_name="$1"
    local pkg_name=""

    case "$proto_name" in
        emotion_llm.proto)  pkg_name="emotionllm" ;;
        emotion_query.proto) pkg_name="emotionquery" ;;
        *)
            log_warn "未知 proto: $proto_name，跳过 Go 生成"
            return
            ;;
    esac

    local out_dir="${SHARED_PKG}/${pkg_name}"
    mkdir -p "$out_dir"
    log_info "生成 Go pb: $proto_name → $out_dir"

    protoc \
        --proto_path="$PROTO_DIR" \
        --go_out="$out_dir" \
        --go_opt=paths=source_relative \
        --go-grpc_out="$out_dir" \
        --go-grpc_opt=paths=source_relative \
        "$PROTO_DIR/$proto_name"
}

# ============================================
# Python 代码生成
# ============================================
gen_python() {
    local proto_name="$1"
    log_info "生成 Python pb: $proto_name → $PYTHON_OUT"

    python -m grpc_tools.protoc \
        --proto_path="$PROTO_DIR" \
        --python_out="$PYTHON_OUT" \
        --grpc_python_out="$PYTHON_OUT" \
        "$PROTO_DIR/$proto_name"
}

# ============================================
# 主流程
# ============================================
main() {
    log_info "Emotion-Echo proto 生成脚本"
    log_info "ROOT: $ROOT"

    check_deps

    for proto_name in $PROTOS; do
        gen_go "$proto_name"
        gen_python "$proto_name"
    done

    log_info "✅ 全部生成完成"
    log_info ""
    log_info "下一步："
    log_info "  cd emotion-echo-shared && go build ./..."
    log_info "  python scripts/check_proto_layout.py   # 验证无散落文件"
}

main "$@"