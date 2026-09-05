# Stage 1: builder - install dependencies into /install prefix
FROM python:3.10-slim AS builder

WORKDIR /build

ENV PIP_DISABLE_PIP_VERSION_CHECK=1 \
    PIP_NO_CACHE_DIR=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1

# System deps needed for audio processing
# - libsndfile1: soundfile
# - ffmpeg: audio format conversion
# - git: for funasr ModelScope download
# Stage 36-B2 fix: removed tuna tsinghua mirror substitution — was breaking build
# in non-CN networks (current dev environment uses pypi.org directly).
RUN apt-get update && apt-get install -y --no-install-recommends --fix-missing \
    gcc \
    g++ \
    git \
    libsndfile1 \
    ffmpeg \
    && rm -rf /var/lib/apt/lists/*

COPY sensevoice-small/requirements.txt .

# Install Python packages to a separate prefix so they can be copied cleanly.
# 关键：torch 等大包国内 pypi 镜像没有 Python 3.10 wheel，已 fallback 到 pypi.org。
RUN pip install --prefix=/install -r requirements.txt \
    --timeout 600


# Stage 2: runtime - lean image with non-root user + tini
FROM python:3.10-slim AS runtime

ARG TZ=Asia/Shanghai
ENV TZ=${TZ} \
    PIP_DISABLE_PIP_VERSION_CHECK=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    LOG_FORMAT=json \
    LOG_LEVEL=INFO \
    SENSEVOICE_DEVICE=cpu

# Runtime libs only (no compilers)
RUN apt-get update && apt-get install -y --no-install-recommends --fix-missing \
    libsndfile1 \
    ffmpeg \
    ca-certificates \
    curl \
    tini \
    && rm -rf /var/lib/apt/lists/*

# Symlink zoneinfo for TZ
RUN ln -snf /usr/share/zoneinfo/$TZ /etc/localtime && echo $TZ > /etc/timezone

# non-root user (UID 65532)
RUN groupadd -g 65532 app && useradd -u 65532 -g app -m -s /usr/sbin/nologin app

WORKDIR /app

# Copy installed packages from builder
COPY --from=builder /install /usr/local

# Copy application source
COPY --chown=app:app sensevoice-small/logging_setup.py sensevoice-small/metrics_setup.py sensevoice-small/server.py ./

# Stage 36-B5 fix: 预烘焙 SenseVoiceSmall 完整模型仓库（含 model.pt 893MB +
# config.yaml / tokens.json / am.mvn / example/），funasr 期望路径：
#   MODELSCOPE_CACHE/iic--SenseVoiceSmall/snapshots/master/...
#
# 仓里已经下载好完整 ModelScope repo（sensevoice-small/），直接 COPY + 路径重命名。
# 容器启动后 funasr 跳过 ModelScope 下载，model_loaded=true 立即生效。
COPY --chown=app:app sensevoice-small/ /app/cache/models/iic--SenseVoiceSmall/snapshots/master/
RUN chown -R app:app /app/cache

# 应用 cache env（funasr / modelscope 都会读）
ENV HF_HOME=/app/cache \
    MODELSCOPE_CACHE=/app/cache

EXPOSE 8002

USER app

# Stage 36-B5 fix: start_period 提到 60s（模型已预烘焙，只需 funasr 加载权重 ~30s）
HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 \
    CMD python -c "import urllib.request; urllib.request.urlopen('http://localhost:8002/health', timeout=5).read()" || exit 1

# tini as PID 1 handles SIGTERM properly
ENTRYPOINT ["/usr/bin/tini", "--"]
CMD ["python", "server.py", "--host", "0.0.0.0", "--port", "8002"]