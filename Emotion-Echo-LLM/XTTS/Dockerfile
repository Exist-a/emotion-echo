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
# 先单独装 numpy / cython（Coqui TTS 拉老 numpy=1.22 在 Py3.10 编译失败）
# 再装 requirements.txt（--no-deps 避免老依赖冲突）
# 最后装 TTS 依赖（spacy / jieba / pypinyin 用于多语种）
# 不在 builder 验证 import TTS（runtime 阶段自然验证）
RUN pip install --prefix=/install --timeout 600 \
        numpy>=1.24.3 cython>=3.0.0 \
    -i https://mirrors.aliyun.com/pypi/simple/ \
    --extra-index-url https://pypi.org/simple/ \
 && pip install --prefix=/install --timeout 600 \
        -r XTTS/requirements.txt --no-deps \
    -i https://mirrors.aliyun.com/pypi/simple/ \
    --extra-index-url https://pypi.org/simple/ \
 && pip install --prefix=/install --timeout 600 \
        spacy jieba pypinyin \
    -i https://mirrors.aliyun.com/pypi/simple/ \
    --extra-index-url https://pypi.org/simple/


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
