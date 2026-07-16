# syntax=docker/dockerfile:1.7
# Stage 1: builder - install dependencies into /install prefix
FROM python:3.10-slim AS builder

WORKDIR /build

ENV PIP_DISABLE_PIP_VERSION_CHECK=1 \
    PIP_NO_CACHE_DIR=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1

# System deps needed for opencv-python / fer libraries
RUN apt-get update && apt-get install -y --no-install-recommends \
    gcc \
    g++ \
    git \
    libopencv-dev \
    && rm -rf /var/lib/apt/lists/*

COPY requirements.txt .

# Install Python packages to a separate prefix so they can be copied cleanly.
RUN pip install --prefix=/install -r requirements.txt \
    -i https://pypi.tuna.tsinghua.edu.cn/simple \
    --timeout 120


# Stage 2: runtime - lean image with non-root user + tini
FROM python:3.10-slim AS runtime

ARG TZ=Asia/Shanghai
ENV TZ=${TZ} \
    PIP_DISABLE_PIP_VERSION_CHECK=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    LOG_FORMAT=json \
    LOG_LEVEL=INFO

# Runtime libs only (no compilers). libgl1 needed by opencv at runtime.
RUN apt-get update && apt-get install -y --no-install-recommends \
    libopencv-core4.5 \
    libopencv-imgproc4.5 \
    libopencv-objdetect4.5 \
    libopencv-dnn4.5 \
    libopencv-imgcodecs4.5 \
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
COPY --chown=app:app logging_setup.py metrics_setup.py server.py deploy.prototxt.txt ./

EXPOSE 8004

USER app

HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 \
    CMD python -c "import urllib.request; urllib.request.urlopen('http://localhost:8004/health', timeout=5).read()" || exit 1

# tini as PID 1 handles SIGTERM properly (uvicorn will receive and graceful-shutdown)
ENTRYPOINT ["/usr/bin/tini", "--"]
CMD ["python", "server.py", "--host", "0.0.0.0", "--port", "8004"]
