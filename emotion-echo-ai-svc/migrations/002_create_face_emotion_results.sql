-- 002_create_face_emotion_results.sql
--
-- Stage 34 PR-2/15/16: FER（人脸情绪）落库表。
--
-- 背景：
--   ai-svc 多模态路径中 FER 产物当前仅在 HTTP 响应里返回，不落库。
--   Stage 34 让 FER 结果持久化，作为 Fusion Worker 的"人脸"模态输入。
--
-- 设计：
--   - upload_id UNIQUE：前端上传去重 nonce（同帧多次上传只保留一条）
--   - message_id 可空：用户可能上传无人脸帧（系统返回 neutral），无关联聊天消息
--   - emotion_scores JSONB：FER 7 类（happy/sad/angry/neutral/calm/anxious/...）的完整概率分布
--   - raw_response JSONB：FER 服务的原始响应，便于审计
--
-- 幂等性：使用 IF NOT EXISTS 类语法，可重复执行不报错。
-- 适用范围：仅 emotion_echo_ai schema（ai-svc 独占）。

BEGIN;

CREATE TABLE IF NOT EXISTS emotion_echo_ai.face_emotion_results (
    id               BIGSERIAL PRIMARY KEY,
    upload_id        VARCHAR(64),
    message_id       BIGINT,
    user_id          BIGINT NOT NULL,
    conversation_id  BIGINT,
    primary_emotion  VARCHAR(32),
    emotion_scores   JSONB DEFAULT '{}'::jsonb,
    confidence       REAL,
    model            VARCHAR(64),
    raw_response     JSONB,
    created_at       TIMESTAMPTZ DEFAULT NOW()
);

-- 上传去重：同 upload_id 第二次 Create 走 ON CONFLICT DO NOTHING 幂等
CREATE UNIQUE INDEX IF NOT EXISTS uq_face_emotion_upload_id
    ON emotion_echo_ai.face_emotion_results(upload_id);

-- message_id 索引：Fusion Worker GetLatestByMessageID
CREATE INDEX IF NOT EXISTS idx_face_message
    ON emotion_echo_ai.face_emotion_results(message_id, created_at DESC);

-- user_id 索引：用户行为分析
CREATE INDEX IF NOT EXISTS idx_face_user_time
    ON emotion_echo_ai.face_emotion_results(user_id, created_at DESC);

COMMIT;