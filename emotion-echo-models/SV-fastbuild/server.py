"""SenseVoice (语音 ASR + 情绪识别) emotion-analysis service.

Stage 25-B：实现 FastAPI HTTP server，接收 multipart 音频，调用 funasr
SenseVoiceSmall 模型，返回 (text, emotion, confidence)。

对齐 ai-svc 客户端契约（internal/aiclient/sensevoice.go）：
  POST /analyze  multipart file=audio
  → {"text": str, "emotion": str, "confidence": float, "raw_text": str, "source": "sensevoice"}

设计要点：
- 复用 FER 的 logging_setup / metrics_setup（标准化）
- 模型 funasr.AutoModel，第一次请求时懒加载
- VAD 模型 fsmn-vad（funasr 内置）
- emotion 从 raw_text 中的 emotion tokens (<|HAPPY|><|zh|>...) 提取
- emotion taxonomy 与 emotion-llm-service 对齐：happy/sad/angry/neutral/surprise/fear/disgust
"""
import argparse
import asyncio
import logging
import os
import re
import signal
import sys
import tempfile

import uvicorn
from fastapi import FastAPI, File, HTTPException, UploadFile
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import JSONResponse

from logging_setup import setup_logging
from metrics_setup import (
    ANALYZE_TOTAL,
    MODEL_INFERENCE_DURATION,
    MetricsMiddleware,
    metrics_endpoint,
)

logger = setup_logging("sensevoice")

app = FastAPI(title="SenseVoice Emotion Analysis Service")

app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)
app.add_middleware(MetricsMiddleware)


# -------- Emotion taxonomy --------
# 与 emotion-llm-service / ai-svc 对齐：7 类基础情绪
EMOTIONS = ["angry", "disgust", "fear", "happy", "sad", "surprise", "neutral"]

# SenseVoice raw token → 标准 emotion 映射
# SenseVoice 输出格式: <|EMO_xxx|><|LANG|><|TEXT|> 或 <|HAPPY|>...
EMOTION_TOKEN_MAPPING = {
    "HAPPY": "happy",
    "ANGRY": "angry",
    "SAD": "sad",
    "SURPRISE": "surprise",
    "FEAR": "fear",
    "DISGUST": "disgust",
    "NEUTRAL": "neutral",
    "EMO_UNKNOWN": "neutral",  # 未识别情绪 → 归 neutral
}


# -------- 模型懒加载 --------
_MODEL = None
_MODEL_LOCK = asyncio.Lock()
_DEVICE = os.getenv("SENSEVOICE_DEVICE", "cpu")


async def _get_model():
    """懒加载 funasr AutoModel。第一次调用时加载，后续复用。

    Returns:
        funasr.AutoModel 实例
    """
    global _MODEL
    if _MODEL is not None:
        return _MODEL
    async with _MODEL_LOCK:
        if _MODEL is not None:
            return _MODEL
        logger.info("loading SenseVoice model (this may take 30-60s on first request)...")
        # 同步 funasr import + 加载会阻塞 → 丢到 thread pool
        loop = asyncio.get_event_loop()
        _MODEL = await loop.run_in_executor(None, _load_model_sync)
        logger.info("SenseVoice model loaded")
        return _MODEL


def _load_model_sync():
    """同步加载 funasr 模型（在线程池里跑）。

    E2E-F-106 修复 ①：VAD 模型改为**本地目录**（镜像已烘焙）。
    原实现用 `vad_model="fsmn-vad"` + `hub="ms"`，把 VAD 下载放在**请求路径**里
    （compose 注释自述"VAD 模型首次启动从 ModelScope 下载到 /app/cache"）⇒
    首次 /analyze 要先下载再推理，必然超过 ai-svc 的 30s 超时。
    """
    from funasr import AutoModel
    model_dir = os.getenv("FUNASR_MODEL_DIR", "/app/model")
    vad_dir = os.getenv("FUNASR_VAD_DIR", "/app/model/vad")
    # 本地 VAD 存在则用本地（离线可用）；否则退回 ModelScope 名称（兼容未重建的旧镜像）
    vad_arg = vad_dir if os.path.isdir(vad_dir) else "fsmn-vad"
    logger.info(
        "loading funasr AutoModel",
        extra={"model_dir": model_dir, "vad": vad_arg},
    )
    return AutoModel(
        model=model_dir,
        vad_model=vad_arg,
        vad_kwargs={"max_single_segment_time": 30000},
        device=_DEVICE,
        hub="ms",  # 仅当 vad_arg 回退为名称时才需要
        disable_update=True,
    )


# -------- Emotion extraction --------
EMOTION_TOKEN_RE = re.compile(r"<\|([A-Za-z_]+)\|>")


def extract_emotion_from_raw(raw_text: str) -> tuple[str, float]:
    """从 SenseVoice raw text 提取 emotion token。

    Returns:
        (emotion, confidence)
        - emotion: 标准 emotion 字符串（happy/angry/...）
        - confidence: 固定 0.85（SenseVoice 不输出概率，按行业惯例给定 0.85）
    """
    if not raw_text:
        return "neutral", 0.5
    m = EMOTION_TOKEN_RE.search(raw_text)
    if not m:
        return "neutral", 0.5
    token = m.group(1)
    emotion = EMOTION_TOKEN_MAPPING.get(token, "neutral")
    # 显式情绪（HAPPY/ANGRY/...）比 EMO_UNKNOWN 信心更高
    confidence = 0.9 if token in {"HAPPY", "ANGRY", "SAD", "SURPRISE", "FEAR", "DISGUST"} else 0.6
    return emotion, confidence


def extract_text_only(raw_text: str) -> str:
    """从 raw_text 移除所有 <|...|> tokens，返回纯文本。"""
    return EMOTION_TOKEN_RE.sub("", raw_text).strip()


# -------- Startup warm-up (E2E-F-106 修复 ③) --------
#
# 原实现是纯懒加载：模型在**第一个 /analyze 请求**里才加载。server.py 自己的注释
# 写"this may take 30-60s on first request"，而 ai-svc 的 SenseVoice 客户端超时是
# **30s**（etc/ai-api.yaml `SenseVoice.Timeout: 30`）⇒ 冷启动的第一个请求必然超时，
# 表现为"语音上传恒 504/503"。
#
# 现在在启动时预热：uvicorn 起来之前把模型加载完，/analyze 第一个请求就是热的。
# 失败**不致命**（不 crash-loop）：记 ERROR，/health 保持 model_loaded=false，
# 由 healthcheck 判 unhealthy（Dockerfile 已改为校验 model_loaded）。
@app.on_event("startup")
async def _warmup_model():
    try:
        await _get_model()
        logger.info("startup warm-up done; service ready")
    except Exception as e:  # noqa: BLE001 — 启动期任何失败都只记录，不阻断服务
        logger.exception("startup warm-up failed; /health stays model_loaded=false", extra={"err": str(e)})


# -------- Routes --------
@app.get("/health")
async def health():
    """健康检查：model_loaded 是**真实**就绪判据（Dockerfile healthcheck 依此判定）。"""
    model_loaded = _MODEL is not None
    return JSONResponse({
        "status": "ok" if model_loaded else "loading",
        "service": "sensevoice",
        "device": _DEVICE,
        "model_loaded": model_loaded,
    })


@app.post("/analyze")
async def analyze(file: UploadFile = File(...)):
    """接收 multipart 音频，返回 ASR 文本 + emotion。

    Args:
        file: multipart/form-data 音频文件（wav/mp3/webm/...）

    Returns:
        {"text": str, "emotion": str, "confidence": float,
         "raw_text": str, "source": "sensevoice"}

    Raises:
        HTTPException 400: 文件为空或格式错
        HTTPException 500: 模型推理失败
    """
    if not file or not file.filename:
        ANALYZE_TOTAL.labels(emotion="unknown", status="bad_request").inc()
        raise HTTPException(status_code=400, detail="missing file")

    # 读取音频字节
    audio_bytes = await file.read()
    if len(audio_bytes) == 0:
        ANALYZE_TOTAL.labels(emotion="unknown", status="bad_request").inc()
        raise HTTPException(status_code=400, detail="empty audio bytes")

    # 写到临时文件（funasr AutoModel.generate 需要文件路径）
    suffix = os.path.splitext(file.filename)[1] or ".wav"
    tmp_path = None
    try:
        with tempfile.NamedTemporaryFile(suffix=suffix, delete=False) as tmp:
            tmp.write(audio_bytes)
            tmp_path = tmp.name

        # 加载模型 + 推理（CPU 推理耗时 100ms-2s）
        with MODEL_INFERENCE_DURATION.time():
            model = await _get_model()
            loop = asyncio.get_event_loop()
            res = await loop.run_in_executor(
                None, _infer_sync, model, tmp_path
            )

        # 解析结果
        raw_text = res[0]["text"] if res else ""
        emotion, confidence = extract_emotion_from_raw(raw_text)
        text = extract_text_only(raw_text)

        ANALYZE_TOTAL.labels(emotion=emotion, status="ok").inc()
        logger.info(
            "analyze ok",
            extra={
                # 注意：不能用 "filename" —— 与 LogRecord 保留字段冲突会抛 KeyError
                "audio_file": file.filename,
                "size_bytes": len(audio_bytes),
                "emotion": emotion,
                "confidence": confidence,
                "text_len": len(text),
            },
        )
        return JSONResponse({
            "text": text,
            "emotion": emotion,
            "confidence": confidence,
            "raw_text": raw_text,
            "source": "sensevoice",
        })
    except HTTPException:
        raise
    except Exception as e:
        ANALYZE_TOTAL.labels(emotion="unknown", status="err").inc()
        logger.exception("analyze failed", extra={"audio_file": file.filename})
        raise HTTPException(status_code=500, detail=f"inference failed: {e}")
    finally:
        if tmp_path and os.path.exists(tmp_path):
            try:
                os.unlink(tmp_path)
            except Exception:
                pass


def _infer_sync(model, audio_path: str) -> list:
    """同步调用 funasr 模型（在线程池里跑）。"""
    return model.generate(
        input=audio_path,
        cache={},
        language="auto",
        use_itn=True,
        batch_size_s=60,
        merge_vad=True,
        merge_length_s=15,
    )


@app.get("/metrics")
async def metrics():
    return await metrics_endpoint()


# -------- Main --------
def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--host", default="0.0.0.0")
    parser.add_argument("--port", type=int, default=8002)
    args = parser.parse_args()

    # SIGTERM graceful shutdown (k8s/docker stop 友好)
    def _sigterm_handler(signum, frame):
        logger.info("SIGTERM received, shutting down")
        sys.exit(0)
    signal.signal(signal.SIGTERM, _sigterm_handler)

    logger.info("starting SenseVoice server", extra={"host": args.host, "port": args.port, "device": _DEVICE})
    uvicorn.run(app, host=args.host, port=args.port, log_config=None)


if __name__ == "__main__":
    main()