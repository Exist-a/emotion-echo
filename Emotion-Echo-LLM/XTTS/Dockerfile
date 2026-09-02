# XTTS has a vendored TTS source tree that must be present (sys.path.insert).
# Strategy:
#   builder : pip-install torch / Coqui TTS / modelscope / fastapi
#   runtime : copy builder's site-packages + local TTS/ folder + model files

FROM python:3.10-slim AS builder

WORKDIR /build

ENV PIP_DISABLE_PIP_VERSION_CHECK=1 \
    PIP_NO_CACHE_DIR=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    # Stage 36-B5 fix: drop tuna mirror (境外网络 tuna 不可达), use pypi.org directly
    HF_ENDPOINT=https://hf-mirror.com

RUN for i in 1 2 3; do \
      apt-get update && \
      apt-get install -y --no-install-recommends --fix-missing \
        gcc g++ git && break; \
      echo "apt-get install attempt $i failed, retrying in 10s..."; \
      sleep 10; \
    done \
    && rm -rf /var/lib/apt/lists/*

COPY XTTS/requirements.txt .
# 简化：一次 pip install 全部 requirements（不拆 --prefix 分阶段）
# TTS 自身会拉 spacy / jieba / pypinyin（我们 requirements 已包含）
#
# Stage 36-B5 fix: pip install 加 3-retry + --no-cache-dir 关掉（pypi CDN 偶发
# 返回 200 OK with 0 bytes，sha256 = e3b0c44...；必须重试才能拉真文件）
RUN for i in 1 2 3; do \
      pip install --prefix=/install --timeout 600 --no-cache-dir -r requirements.txt && break; \
      echo "pip install attempt $i failed, retrying in 15s..."; \
      sleep 15; \
    done


FROM python:3.10-slim AS runtime

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
RUN for i in 1 2 3; do \
      apt-get update && \
      apt-get install -y --no-install-recommends --fix-missing \
        libsndfile1 libgomp1 libespeak-ng1 ca-certificates tini && break; \
      echo "apt-get install attempt $i failed, retrying in 10s..."; \
      sleep 10; \
    done \
    && rm -rf /var/lib/apt/lists/*

RUN ln -snf /usr/share/zoneinfo/$TZ /etc/localtime && echo $TZ > /etc/timezone

# non-root user
RUN groupadd -g 65532 app && useradd -u 65532 -g app -m -s /usr/sbin/nologin app

WORKDIR /app

COPY --from=builder /install /usr/local

# vendored TTS source (server.py uses `import TTS`)
COPY --chown=app:app XTTS/TTS/ ./TTS/

# Stage 36-B5 fix: vendored helper modules (server.py does `import pcm_chunk_shape`)
COPY --chown=app:app XTTS/pcm_chunk_shape.py ./

# application code
COPY --chown=app:app XTTS/logging_setup.py XTTS/metrics_setup.py XTTS/server.py ./

# model + sample WAV (1.8GB model.pth + 201MB dvae.pth + configs)
COPY --chown=app:app XTTS/AI-ModelScope/ ./AI-ModelScope/

RUN mkdir -p /app/cache && chown -R app:app /app/cache

EXPOSE 8003

USER app

HEALTHCHECK --interval=30s --timeout=10s --start-period=180s --retries=3 \
    CMD python -c "import urllib.request; urllib.request.urlopen('http://localhost:8003/health', timeout=5).read()" || exit 1

ENTRYPOINT ["/usr/bin/tini", "--"]
CMD ["python", "server.py", "--host", "0.0.0.0", "--port", "8003", "--device", "cpu"]
