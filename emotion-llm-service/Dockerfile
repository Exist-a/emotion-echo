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
# Round D: 基础镜像 digest pin（Dockerfile.digests.lock 提供 PYTHON_3_12_SLIM_DIGEST）
# dev compose 不强制；prod CI 同步真 digest 后 build 锁版本
ARG PYTHON_3_12_SLIM_IMAGE
ARG PYTHON_3_12_SLIM_DIGEST
FROM ${PYTHON_3_12_SLIM_IMAGE:-python:3.12-slim}${PYTHON_3_12_SLIM_DIGEST:+@${PYTHON_3_12_SLIM_DIGEST}} AS builder

WORKDIR /build

# 配置 pip 镜像源（默认清华 tuna；可由 build-arg PIP_INDEX_URL 覆盖为阿里云 / pypi.org）
# 2026-09-16: 默认 timeout + retries 拉大, 避免大文件（grpcio 7.2MB）在 build context 网络下 read timeout
ARG PIP_INDEX_URL=https://pypi.tuna.tsinghua.edu.cn/simple
ARG PIP_TRUSTED_HOST=pypi.tuna.tsinghua.edu.cn
RUN pip config set global.index-url ${PIP_INDEX_URL} \
    && pip config set global.trusted-host ${PIP_TRUSTED_HOST}

# 复制并安装 Python 依赖（注意：所有路径相对 build context 根）
# grpcio>=1.60.0 官方提供 manylinux wheel，不需要 C 编译，省去 build-essential
COPY emotion-llm-service/requirements.txt ./requirements.txt
RUN pip install --no-cache-dir --default-timeout=120 --retries 5 --prefix=/install -r requirements.txt

# 复制 proto 生成代码（grpc_server.py 强依赖）
COPY emotion-llm-service/emotion_llm_pb2.py      ./emotion_llm_pb2.py
COPY emotion-llm-service/emotion_llm_pb2_grpc.py ./emotion_llm_pb2_grpc.py

# 复制业务代码（含 logging_setup + metrics_setup）
COPY emotion-llm-service/main.py           ./main.py
COPY emotion-llm-service/chat_completion.py ./chat_completion.py
COPY emotion-llm-service/intent.py           ./intent.py
COPY emotion-llm-service/intent_llm.py      ./intent_llm.py
COPY emotion-llm-service/file_context.py   ./file_context.py
COPY emotion-llm-service/grpc_server.py   ./grpc_server.py
COPY emotion-llm-service/logging_setup.py ./logging_setup.py
COPY emotion-llm-service/metrics_setup.py ./metrics_setup.py
COPY emotion-llm-service/nacos_client.py  ./nacos_client.py
COPY emotion-llm-service/entrypoint.sh    ./entrypoint.sh

# ============ Stage 2: runtime ============
# Round D: 基础镜像 digest pin
ARG PYTHON_3_12_SLIM_IMAGE
ARG PYTHON_3_12_SLIM_DIGEST
FROM ${PYTHON_3_12_SLIM_IMAGE:-python:3.12-slim}${PYTHON_3_12_SLIM_DIGEST:+@${PYTHON_3_12_SLIM_DIGEST}} AS runtime

# 容器内时区 & 语言
ENV TZ=Asia/Shanghai \
    LANG=C.UTF-8 \
    PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    # gRPC/HTTP 端口
    HTTP_PORT=8000 \
    GRPC_PORT=50051

# tini: 轻量 init，提供 PID 1 + 信号转发（SIGTERM → 子进程优雅退出）
# P1-R2-14: 加 SHA256 校验——ADD https:// 默认不校验二进制完整性，
# tini 仓库若被攻陷 → 后门注入到所有 llm-service 容器。
ARG TINI_VERSION=v0.19.0
# 权威 SHA：emotion-echo/llm-service:v0.1.2 镜像 /usr/local/bin/tini 真值（2026-09-16 验证）
# 修复历史：原 93dccd091001... 是字符级抄写错误，前 6 位匹配但后续完全不同
ARG TINI_SHA256=93dcc18adc78c65a028a84799ecf8ad40c936fdfc5f2a57b1acda5a8117fa82c
ADD --chmod=755 https://github.com/krallin/tini/releases/download/${TINI_VERSION}/tini /usr/local/bin/tini
# 校验：失败则中断构建（防止恶意二进制被 RUN 阶段加载）
RUN echo "${TINI_SHA256}  /usr/local/bin/tini" | sha256sum -c - || \
    (echo "FATAL: tini SHA256 mismatch (expected ${TINI_SHA256})" && exit 1)

# 创建非 root 用户（UID 65532，对应 distroless nonroot）
RUN groupadd --system --gid 65532 app \
    && useradd  --system --uid 65532 --gid app --home-dir /app --shell /usr/sbin/nologin app

WORKDIR /app

# 从 builder 复制已安装的 Python 包（保留 prefix=/install 结构）
COPY --from=builder /install /usr/local
# 复制业务代码（关键：logging_setup.py / metrics_setup.py / nacos_client.py 都必须在，import 才能成功）
COPY --from=builder /build/main.py                 ./main.py
COPY --from=builder /build/chat_completion.py      ./chat_completion.py
COPY --from=builder /build/intent.py               ./intent.py
COPY --from=builder /build/intent_llm.py           ./intent_llm.py
COPY --from=builder /build/file_context.py         ./file_context.py
COPY --from=builder /build/grpc_server.py          ./grpc_server.py
COPY --from=builder /build/logging_setup.py        ./logging_setup.py
COPY --from=builder /build/metrics_setup.py        ./metrics_setup.py
COPY --from=builder /build/nacos_client.py         ./nacos_client.py
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