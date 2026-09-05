"""FER (Facial Expression Recognition) emotion-analysis service.

Stage 22-A.1 upgrades:
- Structured JSON logger (Stage 20-2 pattern)
- Prometheus /metrics (Stage 20-P0-2 pattern)
- Graceful shutdown on SIGTERM (Stage 20-1 pattern)
- Standardized output: {emotion, confidence, scores, source}

Primary backend: fer library (mtcnn=True)
Fallback backend: OpenCV DNN with `deploy.prototxt.txt` + `emotion_net.caffemodel`
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

app = FastAPI(title="FER Emotion Analysis Service")

app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)
app.add_middleware(MetricsMiddleware)


# -------- Emotion taxonomy (aligned with emotion-llm-service) --------
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
USE_FER_LIB = False
fer_detector = None
net = None

try:
    logger.info("Trying to import fer library…")
    from fer.fer import FER  # fer==25.10.3 import path

    fer_detector = FER(mtcnn=True)
    USE_FER_LIB = True
    logger.info("FER detector initialised (library backend)")
except Exception as e:
    logger.warning("fer library unavailable, falling back to OpenCV DNN: %s", e)
    MODEL_CONFIG = "deploy.prototxt.txt"
    MODEL_WEIGHTS = "emotion_net.caffemodel"
    try:
        import cv2
        import numpy as np

        if os.path.exists(MODEL_CONFIG) and os.path.exists(MODEL_WEIGHTS):
            net = cv2.dnn.readNetFromCaffe(MODEL_CONFIG, MODEL_WEIGHTS)
            logger.info("OpenCV DNN FER model loaded (fallback)")
        else:
            logger.warning(
                "Pre-trained model files not found in %s; service will return neutral emotion",
                os.getcwd(),
            )
            net = None
    except Exception as cv_err:
        logger.warning("Failed to load OpenCV DNN fallback: %s", cv_err)
        net = None


# -------- Routes --------
@app.get("/health")
async def health_check():
    """Liveness probe + backend indicator."""
    model_loaded = USE_FER_LIB or (net is not None)
    return {
        "status": "ok",
        "model_loaded": model_loaded,
        "backend": "fer" if USE_FER_LIB else ("opencv-dnn" if net is not None else "neutral-fallback"),
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
          "source":    "fer" | "opencv-dnn" | "neutral-fallback"
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

        with MODEL_INFERENCE_DURATION.time():
            # backend: fer library
            if USE_FER_LIB:
                import cv2 as _cv2

                img = _cv2.imread(temp_path)
                if img is None:
                    ANALYZE_TOTAL.labels(emotion="unknown", status="invalid").inc()
                    raise HTTPException(status_code=400, detail="Invalid image")
                result = fer_detector.detect_emotions(img)
                if not result:
                    ANALYZE_TOTAL.labels(emotion="neutral", status="no-face").inc()
                    return {
                        "emotion": "neutral",
                        "confidence": 0.5,
                        "scores": {},
                        "source": "fer",
                    }
                emotions = result[0]["emotions"]
                top = max(emotions, key=emotions.get)
                mapped = EMOTION_MAPPING.get(top, "neutral")
                ANALYZE_TOTAL.labels(emotion=mapped, status="ok").inc()
                return {
                    "emotion": mapped,
                    "confidence": float(emotions[top]),
                    "scores": {k: float(v) for k, v in emotions.items()},
                    "source": "fer",
                }

            # backend: opencv-dnn fallback
            if net is not None:
                import cv2 as _cv2
                import numpy as _np

                img = _cv2.imread(temp_path)
                if img is None:
                    ANALYZE_TOTAL.labels(emotion="unknown", status="invalid").inc()
                    raise HTTPException(status_code=400, detail="Invalid image")
                gray = _cv2.cvtColor(img, _cv2.COLOR_BGR2GRAY)
                cascade = _cv2.CascadeClassifier(
                    _cv2.data.haarcascades + "haarcascade_frontalface_default.xml"
                )
                faces = cascade.detectMultiScale(
                    gray, scaleFactor=1.1, minNeighbors=5, minSize=(30, 30)
                )
                if len(faces) == 0:
                    ANALYZE_TOTAL.labels(emotion="neutral", status="no-face").inc()
                    return {
                        "emotion": "neutral",
                        "confidence": 0.5,
                        "scores": {},
                        "source": "opencv-dnn",
                    }
                (x, y, w, h) = max(faces, key=lambda r: r[2] * r[3])
                roi = _cv2.resize(gray[y:y + h, x:x + w], (48, 48)).astype("float") / 255.0
                roi = _np.expand_dims(_np.expand_dims(roi, axis=0), axis=-1)
                net.setInput(roi)
                preds = net.forward()[0]
                idx = int(_np.argmax(preds))
                raw = EMOTIONS[idx]
                mapped = EMOTION_MAPPING.get(raw, "neutral")
                scores = {EMOTIONS[i]: float(preds[i]) for i in range(len(EMOTIONS))}
                ANALYZE_TOTAL.labels(emotion=mapped, status="ok").inc()
                return {
                    "emotion": mapped,
                    "confidence": float(preds[idx]),
                    "scores": scores,
                    "source": "opencv-dnn",
                }

            # no model loaded
            ANALYZE_TOTAL.labels(emotion="neutral", status="no-model").inc()
            logger.warning("no model loaded; returning neutral")
            return {
                "emotion": "neutral",
                "confidence": 0.5,
                "scores": {},
                "source": "neutral-fallback",
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
async def _shutdown_event(loop):
    """Trigger uvicorn exit when receiving SIGTERM/SIGINT."""
    logger.info("received shutdown signal; stopping server…")
    for sig in (signal.SIGTERM, signal.SIGINT):
        loop.remove_signal_handler(sig)


def _install_signal_handlers(loop: asyncio.AbstractEventLoop, server: uvicorn.Server) -> None:
    """Wire SIGTERM/SIGINT to a graceful uvicorn shutdown."""
    for sig in (signal.SIGINT, signal.SIGTERM):
        loop.add_signal_handler(
            sig,
            lambda s=sig: (logger.info("got signal %s; shutting down", s), asyncio.create_task(_shutdown_event(loop))),
        )

    # Use uvicorn's should_exit flag instead of forcibly killing.
    original_install = server.install_signal_handlers
    server.install_signal_handlers = lambda: None  # we handle ourselves

    async def _on_signal():
        server.should_exit = True

    for sig in (signal.SIGINT, signal.SIGTERM):
        loop.add_signal_handler(sig, lambda: asyncio.create_task(_on_signal()))


def main() -> None:
    parser = argparse.ArgumentParser(description="FER Emotion Analysis Service")
    parser.add_argument("--host", type=str, default="0.0.0.0")
    parser.add_argument("--port", type=int, default=8004)
    args = parser.parse_args()

    logger.info("starting FER service on %s:%d (backend=%s)", args.host, args.port,
                "fer" if USE_FER_LIB else ("opencv-dnn" if net is not None else "neutral-fallback"))

    config = uvicorn.Config(
        app=app,
        host=args.host,
        port=args.port,
        log_config=None,                # use our structured logger
        access_log=False,
        timeout_graceful_shutdown=10,
    )
    server = uvicorn.Server(config)

    loop = asyncio.new_event_loop()
    asyncio.set_event_loop(loop)

    # graceful shutdown wiring
    for sig in (signal.SIGINT, signal.SIGTERM):
        loop.add_signal_handler(sig, server.handle_exit, sig, None)

    try:
        loop.run_until_complete(server.serve())
    finally:
        loop.close()


if __name__ == "__main__":
    main()
