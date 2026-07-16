"""
emotion-llm-service · 文本情绪分析微服务

端口：8000
接口：
  GET  /health    - 健康检查
  GET  /metrics   - Prometheus metrics (Stage 20-P0-2)
  POST /analyze   - 文本情绪分析

输入：
  {"text": "今天很开心"}

输出：
  {
    "primaryEmotion": "happy",
    "sentimentScore": 0.65,
    "confidence": 0.8,
    "model": "keyword-v1"
  }

设计原则：
  - 接口与 ai-svc 的 analyzer.Analyzer 输出一致（兼容）
  - 关键词 + 情感词典实现（与 keyword-stub 等价）
  - 未来可换 jieba + SnowNLP / LLM API
"""
import logging
from typing import Optional

from fastapi import FastAPI, HTTPException
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel, Field

from logging_setup import setup_logging
from metrics_setup import (
    ANALYZE_TOTAL,
    HTTP_REQUESTS_TOTAL,
    HTTP_REQUEST_DURATION,
    MetricsMiddleware,
    metrics_endpoint,
)

# Stage 20-2: 结构化日志（默认 JSON，LOG_FORMAT=text 切换）
setup_logging()
logger = logging.getLogger(__name__)

app = FastAPI(title="Emotion LLM Service", version="0.1.0")

app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)
# Stage 20-P0-2: Prometheus metrics 中间件（ASGI level，记录 HTTPRequestsTotal + HTTPRequestDuration）
app.add_middleware(MetricsMiddleware)


# ============ Schemas ============

class AnalyzeRequest(BaseModel):
    text: str = Field(..., min_length=0, max_length=4096, description="待分析文本")


class AnalyzeResponse(BaseModel):
    primaryEmotion: str
    sentimentScore: float
    confidence: float
    model: str


# ============ Analyzer ============

EMOTION_KEYWORDS = {
    "happy":   ["开心", "高兴", "快乐", "哈哈", "棒", "喜欢", "感谢", "谢谢", "好极了"],
    "sad":     ["难过", "伤心", "哭", "失落", "沮丧", "抑郁", "糟糕", "唉"],
    "angry":   ["生气", "愤怒", "气死", "烦死", "受不了", "滚", "可恶", "火大"],
    "anxious": ["焦虑", "紧张", "担心", "害怕", "恐惧", "慌", "不安", "压力", "失眠"],
    "calm":    ["平静", "放松", "安心", "舒适", "冥想", "佛系", "无所谓"],
}

SENTIMENT_WORDS = {
    "好": 0.5, "棒": 0.5, "喜欢": 0.6, "感谢": 0.7, "谢谢": 0.5,
    "开心": 0.7, "高兴": 0.7, "快乐": 0.8, "好极了": 0.9,
    "难": -0.4, "糟": -0.5, "烂": -0.6, "糟糕": -0.7, "气": -0.5,
    "死": -0.6, "害怕": -0.5, "担心": -0.3, "压力": -0.4,
}


def analyze(text: str) -> AnalyzeResponse:
    """主分析函数。返回结构与 ai-svc 的 EmotionResult 一致。"""
    if not text or not text.strip():
        return AnalyzeResponse(
            primaryEmotion="neutral",
            sentimentScore=0.0,
            confidence=0.0,
            model="keyword-v1",
        )

    # sentiment_score：情感词命中均值
    score_sum = 0.0
    hits = 0
    for word, weight in SENTIMENT_WORDS.items():
        if word in text:
            score_sum += weight
            hits += 1
    sentiment = (score_sum / hits) if hits > 0 else 0.0

    # primary_emotion：命中最多的情绪类别
    max_hits = 0
    primary = "neutral"
    for emotion, keywords in EMOTION_KEYWORDS.items():
        cnt = sum(1 for kw in keywords if kw in text)
        if cnt > max_hits:
            max_hits = cnt
            primary = emotion

    # confidence：粗略 = 命中词数 / 总词数
    total_words = max(len(text) // 2, 1)
    confidence = min(1.0, max_hits / total_words)

    return AnalyzeResponse(
        primaryEmotion=primary,
        sentimentScore=round(sentiment, 3),
        confidence=round(confidence, 3),
        model="keyword-v1",
    )


# ============ Routes ============

@app.get("/health")
async def health():
    return {"status": "ok", "service": "emotion-llm", "version": "0.1.0"}


# Stage 20-P0-2: Prometheus metrics 端点（无 auth，无自循环）
@app.get("/metrics")
async def metrics():
    return metrics_endpoint()


@app.post("/analyze", response_model=AnalyzeResponse)
async def analyze_endpoint(req: AnalyzeRequest):
    try:
        result = analyze(req.text)
        # Stage 20-P0-2: 业务指标
        ANALYZE_TOTAL.labels(emotion=result.primaryEmotion, status="ok").inc()
        logger.info(
            f"analyzed: text_len={len(req.text)} emotion={result.primaryEmotion} score={result.sentimentScore}"
        )
        return result
    except Exception as e:
        ANALYZE_TOTAL.labels(emotion="unknown", status="err").inc()
        logger.error(f"analyze err: {e}", exc_info=True)
        raise HTTPException(status_code=500, detail=str(e))


# ============ Entry ============

if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=8000)