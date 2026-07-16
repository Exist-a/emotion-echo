# XTTS has a vendored TTS source tree that must be present (sys.path.insert).
# Strategy:
#   builder : pip-install torch / Coqui TTS / modelscope / fastapi
#   runtime : copy builder's site-packages + local TTS/ folder + model files

FROM python:3.11-slim AS builder

WORKDIR /build

ENV PIP_DISABLE_PIP_VERSION_CHECK=1 \
    PIP_NO_CACHE_DIR=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PIP_INDEX_URL=https://pypi.tuna.tsinghua.edu.cn/simple \
    # Tell huggingface to use Aliyun mirror
    HF_ENDPOINT=https://hf-mirror.com

RUN sed -i 's|deb.debian.org|mirrors.aliyun.com|g' /etc/apt/sources.list.d/debian.sources 2>/dev/null \
 || sed -i 's|deb.debian.org|mirrors.aliyun.com|g' /etc/apt/sources.list \
 && apt-get update && apt-get install -y --no-install-recommends \
    gcc \
    g++ \
    git \
    && rm -rf /var/lib/apt/lists/*

COPY XTTS/requirements.txt .
RUN pip install --prefix=/install -r requirements.txt --timeout 600 \
    && python -c "import TTS; print('TTS version:', TTS.__version__)"


FROM python:3.11-slim AS runtime

ARG TZ=Asia/Shanghai
ENV TZ=${TZ} \
    PIP_DISABLE_PIP_VERSION_CHECK=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    LOG_FORMAT=json \
    LOG_LEVEL=INFO \
    HF_ENDPOINT=https://hf-mirror.com \
    MODELSCOPE_CACHE=/app/cache

# Coqui-TTS runtime deps
RUN sed -i 's|deb.debian.org|mirrors.aliyun.com|g' /etc/apt/sources.list.d/debian.sources 2>/dev/null \
 || sed -i 's|deb.debian.org|mirrors.aliyun.com|g' /etc/apt/sources.list \
 && apt-get update && apt-get install -y --no-install-recommends \
    libsndfile1 \
    libgomp1 \
    libespeak-ng1 \
    ca-certificates \
    tini \
    && rm -rf /var/lib/apt/lists/*

RUN ln -snf /usr/share/zoneinfo/$TZ /etc/localtime && echo $TZ > /etc/timezone

# non-root user
RUN groupadd -g 65532 app && useradd -u 65532 -g app -m -s /usr/sbin/nologin app

WORKDIR /app

COPY --from=builder /install /usr/local

# vendored TTS source (server.py uses `import TTS`)
COPY --chown=app:app XTTS/TTS/ ./TTS/

# application code
COPY --chown=app:app XTTS/logging_setup.py XTTS/metrics_setup.py XTTS/server.py ./

# model + sample WAV
COPY --chown=app:app XTTS/AI-ModelScope/ ./AI-ModelScope/

RUN mkdir -p /app/cache && chown -R app:app /app/cache

EXPOSE 8003

USER app

HEALTHCHECK --interval=30s --timeout=10s --start-period=180s --retries=3 \
    CMD python -c "import urllib.request; urllib.request.urlopen('http://localhost:8003/health', timeout=5).read()" || exit 1

ENTRYPOINT ["/usr/bin/tini", "--"]
CMD ["python", "server.py", "--host", "0.0.0.0", "--port", "8003", "--device", "cpu"]
