import uvicorn
from fastapi import FastAPI, HTTPException
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel
import io
import base64
import logging

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

app = FastAPI(title="TTS Service (gTTS)")

app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)


class TTSRequest(BaseModel):
    text: str
    language: str = "zh-cn"


@app.get("/health")
async def health_check():
    return {"status": "ok", "model_type": "gTTS"}


@app.post("/tts")
async def text_to_speech(request: TTSRequest):
    """使用 Google TTS 将文字转换为语音"""
    try:
        from gtts import gTTS
        
        logger.info(f"Synthesizing text: {request.text[:50]}...")
        
        tts = gTTS(text=request.text, lang=request.language, slow=False)
        
        buffer = io.BytesIO()
        tts.write_to_fp(buffer)
        buffer.seek(0)
        
        audio_base64 = base64.b64encode(buffer.read()).decode("utf-8")
        
        return {
            "audio": audio_base64,
            "sample_rate": 24000,
            "text": request.text
        }
        
    except Exception as e:
        logger.error(f"Error synthesizing speech: {e}", exc_info=True)
        raise HTTPException(status_code=500, detail=str(e))


@app.post("/tts_with_phonemes")
async def tts_with_phonemes(request: TTSRequest):
    """文字转语音，并估算音素时间戳"""
    try:
        from gtts import gTTS
        
        logger.info(f"Synthesizing text with phonemes: {request.text[:50]}...")
        
        tts = gTTS(text=request.text, lang=request.language, slow=False)
        
        buffer = io.BytesIO()
        tts.write_to_fp(buffer)
        buffer.seek(0)
        audio_base64 = base64.b64encode(buffer.read()).decode("utf-8")
        
        # 估算时间戳
        chars = list(request.text)
        avg_char_duration = 0.15  # 平均每个字符约0.15秒
        total_duration = len(chars) * avg_char_duration
        
        phonemes = []
        current_time = 0
        for char in chars:
            phonemes.append({
                "char": char,
                "start": round(current_time, 3),
                "duration": round(avg_char_duration, 3)
            })
            current_time += avg_char_duration
        
        return {
            "audio": audio_base64,
            "sample_rate": 24000,
            "text": request.text,
            "phonemes": phonemes,
            "duration": round(total_duration, 3)
        }
        
    except Exception as e:
        logger.error(f"Error synthesizing speech: {e}", exc_info=True)
        raise HTTPException(status_code=500, detail=str(e))


if __name__ == "__main__":
    import argparse
    
    parser = argparse.ArgumentParser(description="TTS Service (gTTS)")
    parser.add_argument("--host", type=str, default="0.0.0.0", help="Host to listen on")
    parser.add_argument("--port", type=int, default=8003, help="Port to listen on")
    
    args = parser.parse_args()
    
    logger.info(f"Starting TTS server on {args.host}:{args.port}")
    
    uvicorn.run(app, host=args.host, port=args.port)