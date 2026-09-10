# SenseVoice fastbuild image (PR-TTS-VENDOR Phase 5)
# 解决 dev 网络下 torch 800MB+ 卡死的 wheels 预下 + --find-links 方案
FROM python:3.10-slim AS builder

WORKDIR /build

ENV PIP_DISABLE_PIP_VERSION_CHECK=1 \
    PIP_NO_CACHE_DIR=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1

RUN for i in 1 2 3; do \
      apt-get update && \
      apt-get install -y --no-install-recommends --fix-missing \
        gcc g++ git libsndfile1 ffmpeg \
        && break; \
      echo "apt-get install attempt $i failed, retrying in 10s..."; \
      sleep 10; \
    done && \
    rm -rf /var/lib/apt/lists/*

COPY requirements.txt /build/wheels_requirements.txt
COPY wheels/ /build/wheels/
# pip install 阶段：清华为主索引，aliyun 备份，避免 download.pytorch.org 大包断链
# wheels 已预下到 context 的 wheels/（构建期中间产物，不进入 runtime 镜像）
# 注意：--find-links 优先本地 wheels，找不到才走 --index-url
# 这样 prometheus-client/fastapi/uvicorn/python-json-logger 等小包走 aliyun 下载
RUN pip install --prefix=/install \
      --find-links /build/wheels/ \
      --index-url https://mirrors.aliyun.com/pypi/simple \
      -r /build/wheels_requirements.txt \
      --timeout 600


# Stage 2: runtime
FROM python:3.10-slim AS runtime

ARG TZ=Asia/Shanghai
ENV TZ=${TZ} \
    PIP_DISABLE_PIP_VERSION_CHECK=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    LOG_FORMAT=json \
    LOG_LEVEL=INFO \
    SENSEVOICE_DEVICE=cpu

# runtime 只装最小集（libsndfile + tini + curl）；ffmpeg 已在 builder 阶段用于编译 funasr C 扩展。
# 加 retry loop 处理 deb.debian.org 偶发 502/EOF（stage-36 §5.4 记录）。
RUN for i in 1 2 3; do \
      apt-get update && \
      apt-get install -y --no-install-recommends --fix-missing \
        libsndfile1 ca-certificates curl tini \
        && break; \
      echo "apt-get install attempt $i failed, retrying in 10s..."; \
      sleep 10; \
    done && \
    rm -rf /var/lib/apt/lists/*

RUN ln -snf /usr/share/zoneinfo/$TZ /etc/localtime && echo $TZ > /etc/timezone
RUN groupadd -g 65532 app && useradd -u 65532 -g app -m -s /usr/sbin/nologin app

WORKDIR /app
COPY --from=builder /install /usr/local

COPY server.py logging_setup.py metrics_setup.py ./

# Stage 36-B5: 预烘焙 SenseVoiceSmall 模型（含 model.pt 893MB）
COPY model.pt am.mvn chn_jpn_yue_eng_ko_spectok.bpe.model config.yaml configuration.json ./model/
RUN mkdir -p /app/cache && chown -R app:app /app/cache /app/model

ENV HF_HOME=/app/cache \
    MODELSCOPE_CACHE=/app/cache \
    FUNASR_MODEL_DIR=/app/model

EXPOSE 8002

USER app

HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 \
    CMD python -c "import urllib.request; urllib.request.urlopen('http://localhost:8002/health', timeout=5).read()" || exit 1

ENTRYPOINT ["/usr/bin/tini", "--"]
CMD ["python", "server.py", "--host", "0.0.0.0", "--port", "8002"]