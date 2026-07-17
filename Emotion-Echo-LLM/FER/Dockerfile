# Stage 1: builder - install dependencies into /install prefix
FROM python:3.10-slim AS builder

WORKDIR /build

ENV PIP_DISABLE_PIP_VERSION_CHECK=1 \
    PIP_NO_CACHE_DIR=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1

# System deps needed for opencv-python / fer libraries
# 国内 build 加速：清华 TUNA 镜像（已确认有 trixie main 包，aliyun 缺）
RUN sed -i 's|deb.debian.org|mirrors.tuna.tsinghua.edu.cn|g' /etc/apt/sources.list.d/debian.sources 2>/dev/null \
 || sed -i 's|deb.debian.org|mirrors.tuna.tsinghua.edu.cn|g' /etc/apt/sources.list \
 && apt-get update && apt-get install -y --no-install-recommends \
    gcc \
    g++ \
    git \
    libopencv-dev \
    && rm -rf /var/lib/apt/lists/*

COPY FER/requirements.txt .

# Install Python packages to a separate prefix so they can be copied cleanly.
RUN pip install --prefix=/install -r requirements.txt \
    -i https://pypi.tuna.tsinghua.edu.cn/simple \
    --timeout 180


# Stage 2: runtime - lean image with non-root user + tini
FROM python:3.10-slim AS runtime

ARG TZ=Asia/Shanghai
ENV TZ=${TZ} \
    PIP_DISABLE_PIP_VERSION_CHECK=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    LOG_FORMAT=json \
    LOG_LEVEL=INFO

# Runtime libs only (no compilers). libopencv-*410 on Debian 13 (trixie).
RUN sed -i 's|deb.debian.org|mirrors.tuna.tsinghua.edu.cn|g' /etc/apt/sources.list.d/debian.sources 2>/dev/null \
 || sed -i 's|deb.debian.org|mirrors.tuna.tsinghua.edu.cn|g' /etc/apt/sources.list \
 && apt-get update && apt-get install -y --no-install-recommends \
    libopencv-core410 \
    libopencv-imgproc410 \
    libopencv-objdetect410 \
    libopencv-dnn410 \
    libopencv-imgcodecs410 \
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
COPY --chown=app:app FER/logging_setup.py FER/metrics_setup.py FER/server.py FER/deploy.prototxt.txt ./

EXPOSE 8004

USER app

HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 \
    CMD python -c "import urllib.request; urllib.request.urlopen('http://localhost:8004/health', timeout=5).read()" || exit 1

# tini as PID 1 handles SIGTERM properly (uvicorn will receive and graceful-shutdown)
ENTRYPOINT ["/usr/bin/tini", "--"]
CMD ["python", "server.py", "--host", "0.0.0.0", "--port", "8004"]
