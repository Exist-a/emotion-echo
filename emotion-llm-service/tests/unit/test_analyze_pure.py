"""
test_analyze_pure.py · 纯函数测试 emotion 关键字分析算法

不依赖 FastAPI / Prometheus，可独立运行。
为绕开 metrics_setup 的可选 import，复制 EMOTION_KEYWORDS / SENTIMENT_WORDS
常量快照到本测试文件，保持与 main.py 实现同步。
"""
from __future__ import annotations

import sys
from pathlib import Path

import pytest

# 把 emotion-llm-service 加入 PYTHONPATH
SERVICE_DIR = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(SERVICE_DIR))


# 关键字/情感词典快照 — 与 main.py 保持一致
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


def analyze(text: str) -> dict:
    """简化版 analyze：返回 dict 而不是 Pydantic，可独立单元测试"""
    if not text or not text.strip():
        return {"primaryEmotion": "neutral", "sentimentScore": 0.0, "confidence": 0.0, "model": "keyword-v1"}

    score_sum = 0.0
    hits = 0
    for word, weight in SENTIMENT_WORDS.items():
        if word in text:
            score_sum += weight
            hits += 1
    sentiment = (score_sum / hits) if hits > 0 else 0.0

    max_hits = 0
    primary = "neutral"
    for emotion, keywords in EMOTION_KEYWORDS.items():
        cnt = sum(1 for kw in keywords if kw in text)
        if cnt > max_hits:
            max_hits = cnt
            primary = emotion

    total_words = max(len(text) // 2, 1)
    confidence = min(1.0, max_hits / total_words)
    return {
        "primaryEmotion": primary,
        "sentimentScore": round(sentiment, 3),
        "confidence": round(confidence, 3),
        "model": "keyword-v1",
    }


# ---------- 测试 ----------

def test_empty_text_returns_neutral():
    r = analyze("")
    assert r["primaryEmotion"] == "neutral"
    assert r["sentimentScore"] == 0.0
    assert r["confidence"] == 0.0


def test_whitespace_only_text_returns_neutral():
    r = analyze("   \n\t  ")
    assert r["primaryEmotion"] == "neutral"


@pytest.mark.parametrize("text,expected_emotion", [
    ("今天很开心", "happy"),
    ("我很高兴", "happy"),
    ("谢谢你的帮助", "happy"),
    ("真难过", "sad"),
    ("我感到失落", "sad"),
    ("太生气了", "angry"),
    ("很焦虑，睡不着", "anxious"),
    ("感觉很放松", "calm"),
])
def test_emotion_classification_table_driven(text, expected_emotion):
    r = analyze(text)
    assert r["primaryEmotion"] == expected_emotion, f"text={text!r} got={r['primaryEmotion']}"


@pytest.mark.parametrize("text", [
    "今天天气不错",
    "随便看看",
    "Hello world",
    "12345678",
])
def test_unrecognized_text_returns_neutral(text):
    r = analyze(text)
    assert r["primaryEmotion"] == "neutral"


def test_sentiment_score_positive_for_happy_words():
    r = analyze("开心")
    assert r["sentimentScore"] > 0


def test_sentiment_score_negative_for_sad_words():
    r = analyze("糟糕")
    assert r["sentimentScore"] < 0


def test_confidence_is_between_zero_and_one():
    r = analyze("今天很开心")
    assert 0.0 <= r["confidence"] <= 1.0


def test_model_field_is_stable():
    r = analyze("任何文本")
    assert r["model"] == "keyword-v1"


def test_response_keys_complete():
    r = analyze("anything")
    for k in ("primaryEmotion", "sentimentScore", "confidence", "model"):
        assert k in r
