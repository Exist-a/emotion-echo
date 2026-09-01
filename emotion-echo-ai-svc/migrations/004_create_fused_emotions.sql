-- 004_create_fused_emotions.sql
--
-- Stage 34 PR-6/13/14: 多模态情绪融合产物表。
--
-- 背景：
--   Fusion Worker 每 5s tick 一次，对"有 text 但 fused_emotions 还没收敛"的消息
--   调 LLM-as-Fusion（或 weighted late fusion 兜底），结果写入本表。
--
-- 设计：
--   - message_id UNIQUE NOT NULL：每条消息至多一个融合结果（Worker 重试覆盖）
--   - modality_contrib JSONB：每路贡献度（如 {"text":0.4,"voice":0.3,"face":0.3}）
--   - reasoning TEXT：LLM 输出（走 late_fusion_weighted 时为空）
--   - fusion_method VARCHAR(32)："llm" | "late_fusion_weighted"
--   - available_modalities JSONB：本次融合实际用到的模态（如 ["text","voice","face"]）
--     注：原计划 TEXT[]，因 GORM []string → PG TEXT[] 转换有类型不匹配坑，
--     改为 JSONB 字符串数组（与 modality_contrib 同模式），代码侧用
--     model.AvailableModalitiesFromSlice() 序列化。
--   - sentiment_score REAL：综合情感极性 [-1, 1]
--   - confidence REAL：综合可信度 [0, 1]
--
-- 关键差异（vs emotion_analysis）：
--   - emotion_analysis 由 Kafka 异步链路写，一条 message 一行
--   - fused_emotions 由 Worker 同步覆盖写（同 message_id ON CONFLICT DO UPDATE）
--   两条路径独立，前端先消费 fused_emotions（fallback 到 emotion_analysis）
--
-- 幂等性：使用 IF NOT EXISTS 类语法。

BEGIN;

CREATE TABLE IF NOT EXISTS emotion_echo_ai.fused_emotions (
    id                    BIGSERIAL PRIMARY KEY,
    message_id            BIGINT NOT NULL,
    user_id               BIGINT NOT NULL,
    conversation_id       BIGINT NOT NULL,
    primary_emotion       VARCHAR(32) NOT NULL,
    sentiment_score       REAL,
    confidence            REAL,
    modality_contrib      JSONB DEFAULT '{}'::jsonb,
    reasoning             TEXT,
    fusion_method         VARCHAR(32),
    available_modalities  JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at            TIMESTAMPTZ DEFAULT NOW()
);

-- 核心约束：每条消息至多一个融合结果
CREATE UNIQUE INDEX IF NOT EXISTS uq_fused_emotions_message_id
    ON emotion_echo_ai.fused_emotions(message_id);

-- Fusion Worker 找 candidate 用
CREATE INDEX IF NOT EXISTS idx_fused_user_time
    ON emotion_echo_ai.fused_emotions(user_id, created_at DESC);

-- 按 conversation 查
CREATE INDEX IF NOT EXISTS idx_fused_conv_time
    ON emotion_echo_ai.fused_emotions(conversation_id, created_at DESC);

COMMIT;