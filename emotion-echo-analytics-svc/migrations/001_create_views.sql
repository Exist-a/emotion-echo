-- migrations/001_create_views.sql
--
-- Stage 30-A §六.6.1: 跨 schema 只读 VIEWs。
--
-- 设计：
--   - 每个 VIEW 跨 schema 暴露必要的字段给 analytics-svc
--   - 不暴露 content（仅元数据）
--   - 由 owning service 部署（chat-svc / ai-svc / assessment-svc）
--     应各自确认 VIEW 字段稳定后再 ALTER
--
-- 注意：执行本 SQL 前必须先 CREATE SCHEMA IF NOT EXISTS 各 schema。
-- deploy/init.sql 已经建好所有 4 个 schema；本 migration 假定 schema 存在。

-- emotion_echo_chat: 暴露 messages 给 analytics（不暴露 content，仅元数据）
CREATE OR REPLACE VIEW emotion_echo_chat.msg_summary_v AS
SELECT
    id,
    conversation_id,
    user_id,
    role,
    content_type,
    tokens_used,
    LENGTH(content) AS content_len,
    send_time
FROM emotion_echo_chat.messages;

-- emotion_echo_ai: 暴露 emotion_analysis 给 analytics
CREATE OR REPLACE VIEW emotion_echo_ai.daily_emotion_v AS
SELECT
    id,
    message_id,
    conversation_id,
    user_id,
    primary_emotion,
    sentiment_score,
    confidence,
    model,
    created_at
FROM emotion_echo_ai.emotion_analysis;

-- emotion_echo_assessment: 暴露 assessment 给 analytics
CREATE OR REPLACE VIEW emotion_echo_assessment.assessment_v AS
SELECT
    id,
    user_id,
    assessment_type,
    period_start,
    period_end,
    overall_score,
    risk_level,
    dimensions,
    created_at
FROM emotion_echo_assessment.mental_health_assessments;