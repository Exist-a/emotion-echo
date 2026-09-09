"""FER-tflite (Facial Expression Recognition) emotion-analysis service.

PR-TTS-VENDOR Phase 2: tflite + OpenCV Haar Cascade 后端（替代 fer+tensorflow 12.1GB）。

Backend:
  - tflite-runtime (~1MB wheel) 跑 emotion_model_quantized.tflite（92KB FER 模型量化版）
  - OpenCV Haar Cascade 做 face detection（避免 fer 库的 MTCNN 依赖）
  - 总镜像预估 < 500MB（原 12.1GB），启动 < 5s

Output contract (与 emotion-echo FER 标准化契约一致):
  {emotion, confidence, scores, source}

Emotion taxonomy 与 emotion-llm-service 对齐：
  - 7 raw (fer 库标签集)
  - 5 unified (项目统一情感)
"""
import argparse
import asyncio
import logging
import os
import signal
import sys
import tempfile

import uvicorn
from fastapi import FastAPI, File, HTTPException, UploadFile
from fastapi.middleware.cors import CORSMiddleware

from logging_setup import setup_logging
from metrics_setup import (
    ANALYZE_TOTAL,
    MODEL_INFERENCE_DURATION,
    MetricsMiddleware,
    metrics_endpoint,
)

logger = setup_logging("fer")

app = FastAPI(title="FER Emotion Analysis Service (tflite)")

app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)
app.add_middleware(MetricsMiddleware)


# -------- Emotion taxonomy --------
EMOTIONS = ["angry", "disgust", "fear", "happy", "sad", "surprise", "neutral"]

EMOTION_MAPPING = {
    "angry": "angry",
    "disgust": "neutral",
    "fear": "anxious",
    "happy": "happy",
    "sad": "sad",
    "surprise": "neutral",
    "neutral": "neutral",
}


# -------- Model loading --------
MODEL_FILE = "emotion_model_quantized.tflite"
HAAR_FILE = "haarcascade_frontalface_default.xml"

USE_TFLITE = False
interpreter = None
input_details = None
output_details = None
face_cascade = None


def _try_load_tflite() -> bool:
    """Load tflite emotion model + Haar cascade. Returns True on success."""
    global USE_TFLITE, interpreter, input_details, output_details, face_cascade

    try:
        import tflite_runtime.interpreter as tflite
        import cv2
        import numpy as np
    except Exception as e:
        logger.warning("tflite/cv2 import failed: %s", e)
        return False

    model_path = os.path.join(os.path.dirname(os.path.abspath(__file__)), MODEL_FILE)
    if not os.path.exists(model_path):
        logger.warning("tflite model file not found: %s", model_path)
        return False

    try:
        interpreter = tflite.Interpreter(model_path=model_path)
        interpreter.allocate_tensors()
        input_details = interpreter.get_input_details()
        output_details = interpreter.get_output_details()
    except Exception as e:
        logger.warning("tflite model load failed: %s", e)
        return False

    haar_path = cv2.data.haarcascades + HAAR_FILE
    face_cascade = cv2.CascadeClassifier(haar_path)
    if face_cascade.empty():
        logger.warning("Haar cascade failed to load from %s", haar_path)
        face_cascade = None
        return False

    USE_TFLITE = True
    logger.info(
        "tflite model loaded: input=%s dtype=%s, output=%s dtype=%s",
        input_details[0]["shape"], input_details[0]["dtype"],
        output_details[0]["shape"], output_details[0]["dtype"],
    )
    return True


_try_load_tflite()


# -------- Routes --------
@app.get("/health")
async def health_check():
    """Liveness probe + backend indicator."""
    return {
        "status": "ok",
        "model_loaded": USE_TFLITE,
        "backend": "tflite+haar" if USE_TFLITE else "neutral-fallback",
    }


@app.get("/metrics")
async def metrics():
    return await metrics_endpoint()


@app.post("/analyze")
async def analyze_emotion(file: UploadFile = File(...)):
    """Analyze facial emotion from an uploaded image.

    Returns:
        {
          "emotion":   <mapped emotion, e.g. "happy">,
          "confidence": <float 0-1>,
          "scores":    {<raw_label>: <prob>, ...},
          "source":    "tflite" | "no-face" | "no-model"
        }
    """
    temp_path = None
    try:
        suffix = os.path.splitext(file.filename or "img.jpg")[1] or ".jpg"
        with tempfile.NamedTemporaryFile(delete=False, suffix=suffix) as tmp:
            content = await file.read()
            if not content:
                ANALYZE_TOTAL.labels(emotion="unknown", status="invalid").inc()
                raise HTTPException(status_code=400, detail="empty file")
            tmp.write(content)
            temp_path = tmp.name

        logger.info("processing image: filename=%s bytes=%d", file.filename, len(content))

        if not USE_TFLITE:
            ANALYZE_TOTAL.labels(emotion="neutral", status="no-model").inc()
            logger.warning("no model loaded; returning neutral")
            return {
                "emotion": "neutral",
                "confidence": 0.5,
                "scores": {},
                "source": "no-model",
            }

        import cv2 as _cv2
        import numpy as _np

        with MODEL_INFERENCE_DURATION.time():
            img = _cv2.imread(temp_path)
            if img is None:
                ANALYZE_TOTAL.labels(emotion="unknown", status="invalid").inc()
                raise HTTPException(status_code=400, detail="Invalid image")
            gray = _cv2.cvtColor(img, _cv2.COLOR_BGR2GRAY)
            faces = face_cascade.detectMultiScale(
                gray, scaleFactor=1.1, minNeighbors=5, minSize=(30, 30)
            )
            if len(faces) == 0:
                ANALYZE_TOTAL.labels(emotion="neutral", status="no-face").inc()
                return {
                    "emotion": "neutral",
                    "confidence": 0.5,
                    "scores": {},
                    "source": "no-face",
                }
            x, y, w, h = max(faces, key=lambda r: r[2] * r[3])
            roi = _cv2.resize(gray[y:y + h, x:x + w], (64, 64)).astype("float32") / 255.0
            roi = _np.expand_dims(_np.expand_dims(roi, axis=0), axis=-1)
            interpreter.set_tensor(input_details[0]["index"], roi)
            interpreter.invoke()
            preds = interpreter.get_tensor(output_details[0]["index"])[0]
            idx = int(_np.argmax(preds))
            raw = EMOTIONS[idx]
            mapped = EMOTION_MAPPING.get(raw, "neutral")
            scores = {EMOTIONS[i]: float(preds[i]) for i in range(len(EMOTIONS))}
            ANALYZE_TOTAL.labels(emotion=mapped, status="ok").inc()
            return {
                "emotion": mapped,
                "confidence": float(preds[idx]),
                "scores": scores,
                "source": "tflite",
            }

    except HTTPException:
        raise
    except Exception as e:
        ANALYZE_TOTAL.labels(emotion="unknown", status="error").inc()
        logger.error("error processing image: %s", e, exc_info=True)
        raise HTTPException(status_code=500, detail=str(e))
    finally:
        if temp_path and os.path.exists(temp_path):
            try:
                os.unlink(temp_path)
            except OSError:
                pass


# -------- Graceful shutdown (Stage 20-1 pattern) --------
def main() -> None:
    parser = argparse.ArgumentParser(description="FER-tflite Emotion Analysis Service")
    parser.add_argument("--host", type=str, default="0.0.0.0")
    parser.add_argument("--port", type=int, default=8004)
    args = parser.parse_args()

    logger.info(
        "starting FER-tflite service on %s:%d (backend=%s)",
        args.host, args.port,
        "tflite+haar" if USE_TFLITE else "neutral-fallback",
    )

    config = uvicorn.Config(
        app=app,
        host=args.host,
        port=args.port,
        log_config=None,
        access_log=False,
        timeout_graceful_shutdown=10,
    )
    server = uvicorn.Server(config)

    loop = asyncio.new_event_loop()
    asyncio.set_event_loop(loop)
    try:
        loop.run_until_complete(server.serve())
    finally:
        loop.close()


if __name__ == "__main__":
    main()