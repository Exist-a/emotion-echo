# FER-tflite image (PR-TTS-VENDOR Phase 2)
# 轻量人脸情绪识别：tflite (92KB model) + Haar cascade，避免 tensorflow 572MB。
FROM python:3.10-slim

WORKDIR /app

# 镜像预计 < 500MB（含 tflite + opencv-contrib + fastapi + 模型权重）
ENV PIP_DISABLE_PIP_VERSION_CHECK=1 \
    PIP_NO_CACHE_DIR=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    LOG_FORMAT=json \
    LOG_LEVEL=INFO

COPY requirements.txt .
# numpy<2 + contrib-headless 是已知依赖组合（tflite 2.14 ABI 不兼容 numpy 2.x）
RUN pip install --no-cache-dir --index-url https://mirrors.aliyun.com/pypi/simple \
      -r requirements.txt

COPY server.py logging_setup.py metrics_setup.py ./
COPY emotion_model_quantized.tflite ./
COPY haarcascade_frontalface_default.xml ./

EXPOSE 8004

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD python -c "import urllib.request; urllib.request.urlopen('http://localhost:8004/health', timeout=3).read()" || exit 1

CMD ["python", "server.py"]