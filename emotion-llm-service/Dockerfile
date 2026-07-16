# emotion-llm-service Dockerfile
# 同时提供 HTTP (uvicorn :8000) 和 gRPC (grpc_server :50051) 两套协议
#
# Build context 必须是仓库根（emotion-llm-service 的父目录）：
#   docker build -f emotion-llm-service/Dockerfile .
# 或在 docker-compose.yml 中：
#   build:
#     context: .
#     dockerfile: emotion-llm-service/Dockerfile
#
# 阶段 1: builder  - 安装 requirements 到 /install（独立 layer 缓存）
# 阶段 2: runtime  - 仅 python:3.12-slim + tini + 业务代码（非 root 运行）

# ============ Stage 1: builder ============
FROM python:3.12-slim AS builder

WORKDIR /build

# 配置 pip 国内镜像源（PyPI 在容器内被墙，必须换源）
RUN pip config set global.index-url https://pypi.tuna.tsinghua.edu.cn/simple \
    && pip config set global.trusted-host pypi.tuna.tsinghua.edu.cn

# 复制并安装 Python 依赖（注意：所有路径相对 build context 根）
# grpcio>=1.60.0 官方提供 manylinux wheel，不需要 C 编译，省去 build-essential
COPY emotion-llm-service/requirements.txt ./requirements.txt
RUN pip install --no-cache-dir --prefix=/install -r requirements.txt

# 复制 proto 生成代码（grpc_server.py 强依赖）
COPY emotion-llm-service/emotion_llm_pb2.py      ./emotion_llm_pb2.py
COPY emotion-llm-service/emotion_llm_pb2_grpc.py ./emotion_llm_pb2_grpc.py

# 复制业务代码（含 logging_setup + metrics_setup）
COPY emotion-llm-service/main.py          ./main.py
COPY emotion-llm-service/grpc_server.py   ./grpc_server.py
COPY emotion-llm-service/logging_setup.py ./logging_setup.py
COPY emotion-llm-service/metrics_setup.py ./metrics_setup.py
COPY emotion-llm-service/entrypoint.sh    ./entrypoint.sh

# ============ Stage 2: runtime ============
FROM python:3.12-slim AS runtime

# 容器内时区 & 语言
ENV TZ=Asia/Shanghai \
    LANG=C.UTF-8 \
    PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    # gRPC/HTTP 端口
    HTTP_PORT=8000 \
    GRPC_PORT=50051

# tini: 轻量 init，提供 PID 1 + 信号转发（SIGTERM → 子进程优雅退出）
ARG TINI_VERSION=v0.19.0
ADD --chmod=755 https://github.com/krallin/tini/releases/download/${TINI_VERSION}/tini /usr/local/bin/tini

# 创建非 root 用户（UID 65532，对应 distroless nonroot）
RUN groupadd --system --gid 65532 app \
    && useradd  --system --uid 65532 --gid app --home-dir /app --shell /usr/sbin/nologin app

WORKDIR /app

# 从 builder 复制已安装的 Python 包（保留 prefix=/install 结构）
COPY --from=builder /install /usr/local
# 复制业务代码（关键：logging_setup.py + metrics_setup.py 都必须在，import 才能成功）
COPY --from=builder /build/main.py                 ./main.py
COPY --from=builder /build/grpc_server.py          ./grpc_server.py
COPY --from=builder /build/logging_setup.py        ./logging_setup.py
COPY --from=builder /build/metrics_setup.py        ./metrics_setup.py
COPY --from=builder /build/emotion_llm_pb2.py      ./emotion_llm_pb2.py
COPY --from=builder /build/emotion_llm_pb2_grpc.py ./emotion_llm_pb2_grpc.py
# 复制 entrypoint
COPY --from=builder /build/entrypoint.sh           ./entrypoint.sh
RUN chmod +x /app/entrypoint.sh

# 切到非 root
USER app:app

EXPOSE 8000 50051

# tini 作为 PID 1，转发信号给 shell 启动的两个 Python 进程
ENTRYPOINT ["/usr/local/bin/tini", "--", "/app/entrypoint.sh"]