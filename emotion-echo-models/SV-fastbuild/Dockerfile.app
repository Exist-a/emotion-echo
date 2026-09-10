# SenseVoice 应用层（PR-TTS-VENDOR Phase 6）
# 依赖 sensevoice-base:v0.1.0（python + torch + funasr + kaldi-native-fbank 等）
# 只叠加业务代码 + 预烘焙 936MB SenseVoiceSmall 模型。
#
# 重要：FROM 必须用 ACR 完整地址，否则别人 pull 应用层镜像时找不到 base。
ARG BASE_REGISTRY=crpi-rnawo8jx69bslvbx.cn-hongkong.personal.cr.aliyuncs.com
ARG BASE_NAMESPACE=emotion-echo
ARG BASE_TAG=v0.1.0
FROM ${BASE_REGISTRY}/${BASE_NAMESPACE}/sensevoice-base:${BASE_TAG}

WORKDIR /app

COPY --chown=app:app server.py logging_setup.py metrics_setup.py ./
COPY --chown=app:app model.pt am.mvn chn_jpn_yue_eng_ko_spectok.bpe.model config.yaml configuration.json ./model/

ENV FUNASR_MODEL_DIR=/app/model

EXPOSE 8002

USER app

HEALTHCHECK --interval=30s --timeout=10s --start-period=90s --retries=3 \
    CMD python -c "import urllib.request; urllib.request.urlopen('http://localhost:8002/health', timeout=5).read()" || exit 1

ENTRYPOINT ["/usr/bin/tini", "--"]
CMD ["python", "server.py", "--host", "0.0.0.0", "--port", "8002"]