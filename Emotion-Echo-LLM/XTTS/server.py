"""XTTS voice-cloning TTS service.

Stage 22-A.3 upgrades:
- Structured JSON logging (Stage 20-2)
- Prometheus /metrics (Stage 20-P0-2)
- Graceful shutdown on SIGTERM (Stage 20-1)
- Standardized output for /tts, /tts_stream, /tts_with_phonemes
"""
import argparse
import asyncio
import base64
import io
import os
import signal
import sys
import time

import torch
import uvicorn
from fastapi import FastAPI, HTTPException
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import StreamingResponse
from pydantic import BaseModel

# add local TTS source tree (Coqui TTS vendored copy)
TTS_SRC = os.path.join(os.path.dirname(os.path.abspath(__file__)), "TTS")
sys.path.insert(0, TTS_SRC)

from logging_setup import setup_logging  # noqa: E402
from metrics_setup import (  # noqa: E402
    TTS_INFERENCE_DURATION,
    TTS_PHONEMES_TOTAL,
    TTS_STREAM_TOTAL,
    TTS_SYNTHESIS_TOTAL,
    MetricsMiddleware,
    metrics_endpoint,
)

logger = setup_logging("xtts")

# Model directory: prefer mounted model, else download on first run.
MODEL_DIR = os.path.dirname(os.path.abspath(__file__))
CACHE_DIR = os.path.join(MODEL_DIR, "AI-ModelScope", "XTTS-v2")

if not os.path.exists(CACHE_DIR):
    try:
        from modelscope import snapshot_download
        logger.info("XTTS model missing, downloading to %s …", CACHE_DIR)
        snapshot_download("AI-ModelScope/XTTS-v2",
                          cache_dir=os.path.join(MODEL_DIR, "AI-ModelScope"))
        logger.info("XTTS model downloaded")
    except Exception as e:
        logger.warning("model download failed: %s; will load lazily", e)

app = FastAPI(title="XTTS TTS Service")
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)
app.add_middleware(MetricsMiddleware)


# -------- Global model state --------
tts_model = None
xtts_config = None
xtts_speaker = None
gpt_cond_latent = None
speaker_embedding = None
SAMPLE_RATE = 24000


class TTSRequest(BaseModel):
    text: str
    language: str = "zh-cn"
    speed: float = 0.75
    volume: float = 2.0


def load_xtts_model(device: str = "cpu"):
    global tts_model, xtts_config, xtts_speaker, gpt_cond_latent, speaker_embedding
    logger.info("loading XTTS model device=%s cache_dir=%s", device, CACHE_DIR)
    try:
        from TTS.tts.configs.xtts_config import XttsConfig
        from TTS.tts.models.xtts import Xtts

        config = XttsConfig()
        config.load_json(os.path.join(CACHE_DIR, "config.json"))
        model = Xtts.init_from_config(config)
        model.load_checkpoint(config, checkpoint_dir=CACHE_DIR, eval=True)
        if device == "cuda" and torch.cuda.is_available():
            model.cuda()
            logger.info("XTTS using CUDA")
        else:
            model.cpu()
            logger.info("XTTS using CPU")

        tts_model = model
        xtts_config = config

        speaker_wav = os.path.join(CACHE_DIR, "samples", "zh-cn-sample.wav")
        if os.path.exists(speaker_wav):
            xtts_speaker = speaker_wav
            logger.info("reference audio: %s; precomputing conditioning latents…", speaker_wav)
            gpt_cond_latent, speaker_embedding = model.get_conditioning_latents(
                audio_path=speaker_wav
            )
            logger.info("conditioning latents ready")

        logger.info("XTTS model loaded")
    except Exception as e:
        logger.error("failed to load XTTS model: %s", e)
        raise


# -------- Routes --------
@app.get("/health")
async def health_check():
    return {
        "status": "ok",
        "model_loaded": tts_model is not None,
        "model_type": "XTTS-v2",
    }


@app.get("/metrics")
async def metrics():
    return await metrics_endpoint()


@app.post("/tts")
async def text_to_speech(req: TTSRequest):
    """Synthesize WAV audio + base64 encode it.

    Returns:
        {audio: <base64 WAV>, sample_rate: 24000, text: input, language: lang}
    """
    if tts_model is None:
        TTS_SYNTHESIS_TOTAL.labels(language=req.language, status="model-missing").inc()
        raise HTTPException(status_code=500, detail="Model not loaded")

    try:
        clipped = req.text[:100]
        logger.info("tts: lang=%s len=%d text=%r", req.language, len(req.text), clipped)

        with TTS_INFERENCE_DURATION.labels(endpoint="tts").time():
            outputs = tts_model.synthesize(
                clipped,
                xtts_config,
                speaker_wav=xtts_speaker,
                gpt_cond_len=3,
                language=req.language,
                speed=req.speed,
            )

        audio = outputs["wav"]
        buf = io.BytesIO()
        torchaudio.save(buf, torch.tensor(audio).unsqueeze(0), SAMPLE_RATE, format="wav")
        buf.seek(0)
        audio_b64 = base64.b64encode(buf.read()).decode("utf-8")

        TTS_SYNTHESIS_TOTAL.labels(language=req.language, status="ok").inc()
        return {
            "audio": audio_b64,
            "sample_rate": SAMPLE_RATE,
            "text": req.text,
            "language": req.language,
        }

    except Exception as e:
        TTS_SYNTHESIS_TOTAL.labels(language=req.language, status="error").inc()
        logger.error("tts error: %s", e, exc_info=True)
        raise HTTPException(status_code=500, detail=str(e))


async def stream_audio_generator(text: str, language: str = "zh-cn",
                                 stream_chunk_size: int = 20, speed: float = 0.9,
                                 volume: float = 2.0):
    global gpt_cond_latent, speaker_embedding, tts_model, xtts_speaker
    import numpy as np
    import torchaudio

    start_time = time.time()
    last_log = start_time
    try:
        if gpt_cond_latent is None or speaker_embedding is None:
            logger.warning("conditioning latents missing; computing now…")
            gpt_cond_latent, speaker_embedding = tts_model.get_conditioning_latents(
                audio_path=xtts_speaker
            )

        streamer = tts_model.inference_stream(
            text,
            language,
            gpt_cond_latent=gpt_cond_latent,
            speaker_embedding=speaker_embedding,
            stream_chunk_size=stream_chunk_size,
            enable_text_splitting=True,
            speed=speed,
        )

        chunk_count = 0
        total_bytes = 0
        for chunk in streamer:
            if isinstance(chunk, torch.Tensor):
                chunk = chunk.cpu().numpy()
            chunk = chunk * volume
            chunk = np.clip(chunk, -1.0, 1.0)
            if chunk.dtype != np.int16:
                chunk = (chunk * 32767).astype(np.int16)
            data = chunk.tobytes()
            yield data
            chunk_count += 1
            total_bytes += len(data)

            now = time.time()
            if now - last_log >= 0.5:
                logger.info(
                    "[stream] chunk=%d bytes=%d total=%d elapsed=%.2fs",
                    chunk_count, len(data), total_bytes, now - start_time,
                )
                last_log = now

        logger.info(
            "[stream] complete chunks=%d total_bytes=%d total_time=%.2fs",
            chunk_count, total_bytes, time.time() - start_time,
        )

    except Exception as e:
        logger.error("[stream] error: %s", e, exc_info=True)
        raise


@app.post("/tts_stream")
async def tts_stream(req: TTSRequest):
    if tts_model is None:
        TTS_STREAM_TOTAL.labels(language=req.language, status="model-missing").inc()
        raise HTTPException(status_code=500, detail="Model not loaded")
    if not req.text:
        TTS_STREAM_TOTAL.labels(language=req.language, status="invalid").inc()
        raise HTTPException(status_code=400, detail="Text is required")

    TTS_STREAM_TOTAL.labels(language=req.language, status="ok").inc()
    with TTS_INFERENCE_DURATION.labels(endpoint="tts_stream").time():
        return StreamingResponse(
            stream_audio_generator(req.text, req.language,
                                   speed=req.speed, volume=req.volume),
            media_type="audio/wav",
        )


@app.post("/tts_with_phonemes")
async def tts_with_phonemes(req: TTSRequest):
    if tts_model is None:
        TTS_PHONEMES_TOTAL.labels(language=req.language, status="model-missing").inc()
        raise HTTPException(status_code=500, detail="Model not loaded")

    try:
        clipped = req.text[:200]
        logger.info("tts_phonemes: lang=%s text=%r", req.language, clipped[:40])

        with TTS_INFERENCE_DURATION.labels(endpoint="tts_with_phonemes").time():
            outputs = tts_model.synthesize(
                clipped,
                xtts_config,
                speaker_wav=xtts_speaker,
                gpt_cond_len=3,
                language=req.language,
                speed=req.speed,
            )

        import numpy as np
        audio = outputs["wav"]
        buf = io.BytesIO()
        torchaudio.save(buf, torch.tensor(audio).unsqueeze(0), SAMPLE_RATE, format="wav")
        buf.seek(0)
        audio_b64 = base64.b64encode(buf.read()).decode("utf-8")

        chars = list(req.text)
        total_duration = len(audio) / SAMPLE_RATE
        char_duration = total_duration / len(chars) if chars else 0
        phonemes = [
            {
                "char": c,
                "start": round(i * char_duration, 3),
                "duration": round(char_duration, 3),
            }
            for i, c in enumerate(chars)
        ]

        TTS_PHONEMES_TOTAL.labels(language=req.language, status="ok").inc()
        return {
            "audio": audio_b64,
            "sample_rate": SAMPLE_RATE,
            "text": req.text,
            "language": req.language,
            "phonemes": phonemes,
            "duration": round(total_duration, 3),
        }

    except Exception as e:
        TTS_PHONEMES_TOTAL.labels(language=req.language, status="error").inc()
        logger.error("tts_phonemes error: %s", e, exc_info=True)
        raise HTTPException(status_code=500, detail=str(e))


# -------- Graceful shutdown --------
def main() -> None:
    parser = argparse.ArgumentParser(description="XTTS TTS Service")
    parser.add_argument("--host", type=str, default="0.0.0.0")
    parser.add_argument("--port", type=int, default=8003)
    parser.add_argument("--device", type=str, default="cpu")
    args = parser.parse_args()

    if args.device == "cuda" and not torch.cuda.is_available():
        logger.warning("CUDA unavailable; falling back to CPU")
        args.device = "cpu"

    logger.info("starting XTTS service on %s:%d device=%s",
                args.host, args.port, args.device)

    # load model once before serving
    load_xtts_model(device=args.device)

    config = uvicorn.Config(
        app=app,
        host=args.host,
        port=args.port,
        log_config=None,
        access_log=False,
        timeout_graceful_shutdown=20,
    )
    server = uvicorn.Server(config)

    loop = asyncio.new_event_loop()
    asyncio.set_event_loop(loop)

    for sig in (signal.SIGINT, signal.SIGTERM):
        loop.add_signal_handler(sig, server.handle_exit, sig, None)

    try:
        loop.run_until_complete(server.serve())
    finally:
        loop.close()


if __name__ == "__main__":
    main()
