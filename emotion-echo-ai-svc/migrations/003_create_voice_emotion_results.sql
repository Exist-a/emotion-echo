-- 003_create_voice_emotion_results.sql
--
-- Stage 34 PR-4/15/16: SenseVoice（语音情绪识别）落库表。
--
-- 背景：
--   ai-svc 多模态路径中 SenseVoice 产物当前仅在 HTTP 响应里返回。
--   Stage 34 让 SenseVoice 结果持久化，作为 Fusion Worker 的"语音"模态输入。
--
-- 设计：
--   - upload_id UNIQUE：前端上传去重
--   - message_id 可空：语音上传可能不挂聊天消息（demo / 探索场景）
--   - transcript TEXT：SenseVoice ASR 转写文本（ASR 失败时为空）
--   - emotion_scores JSONB：SenseVoice 情绪 token 的完整概率
--   - duration_ms / language：产品侧"语速 / 多语种"分析（Stage 35+ 用）
--
-- 幂等性：使用 IF NOT EXISTS 类语法，可重复执行不报错。

BEGIN;

CREATE TABLE IF NOT EXISTS emotion_echo_ai.voice_emotion_results (
    id               BIGSERIAL PRIMARY KEY,
    upload_id        VARCHAR(64),
    message_id       BIGINT,
    user_id          BIGINT NOT NULL,
    conversation_id  BIGINT,
    transcript       TEXT,
    primary_emotion  VARCHAR(32),
    emotion_scores   JSONB DEFAULT '{}'::jsonb,
    confidence       REAL,
    model            VARCHAR(64),
    duration_ms      INT,
    language         VARCHAR(16),
    raw_response     JSONB,
    created_at       TIMESTAMPTZ DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_voice_emotion_upload_id
    ON emotion_echo_ai.voice_emotion_results(upload_id);

CREATE INDEX IF NOT EXISTS idx_voice_message
    ON emotion_echo_ai.voice_emotion_results(message_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_voice_user_time
    ON emotion_echo_ai.voice_emotion_results(user_id, created_at DESC);

COMMIT;