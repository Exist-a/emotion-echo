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
    """同步加载 funasr 模型（在线程池里跑）。"""
    from funasr import AutoModel
    model_dir = "iic/SenseVoiceSmall"
    return AutoModel(
        model=model_dir,
        vad_model="fsmn-vad",
        vad_kwargs={"max_single_segment_time": 30000},
        device=_DEVICE,
        hub="ms",  # ModelScope（国内源，速度快）
        disable_update=True,
    )


# -------- Emotion extraction --------
EMOTION_TOKEN_RE = re.compile(r"<\|([A-Z_]+)\|>")


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


# -------- Routes --------
@app.get("/health")
async def health():
    """健康检查：检查模型是否已加载。"""
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
                "filename": file.filename,
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
        logger.exception("analyze failed", extra={"filename": file.filename})
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