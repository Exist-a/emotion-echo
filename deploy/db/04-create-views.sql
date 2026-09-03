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
-- 注意：新微服务 messages 表时间列是 created_at（send_time 仅存在于 legacy 单体）。
CREATE OR REPLACE VIEW emotion_echo_chat.msg_summary_v AS
SELECT
    id,
    conversation_id,
    user_id,
    role,
    content_type,
    tokens_used,
    LENGTH(content) AS content_len,
    created_at AS send_time
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
-- 注意：mental_health_assessments 无 risk_level 列（risk_level 在 survey_results 上）；
-- risk_level 由 analytics-svc 在 Go 侧从 overall_score 阈值推导。
CREATE OR REPLACE VIEW emotion_echo_assessment.assessment_v AS
SELECT
    id,
    user_id,
    assessment_type,
    period_start,
    period_end,
    overall_score,
    dimensions,
    created_at
FROM emotion_echo_assessment.mental_health_assessments;

-- migrations/004_create_analytics_reader_role.sql
--
-- Stage 30-A §六.6.4: analytics_reader 只读 role + grants。
--
-- 设计：
--   - analytics_reader: 仅 SELECT 权限，无 INSERT/UPDATE/DELETE
--   - search_path: emotion_echo_analytics first, 然后其他 schema
--     （让 SQL 引用 emotion_echo_analytics.X 时无需 schema 前缀）
--   - GRANT USAGE on schema：能 SELECT 但不能 CREATE TABLE
--   - GRANT SELECT on specific VIEWs + 本 schema 表：
--     msg_summary_v / daily_emotion_v / assessment_v /
--     user_behavior_events
--
-- 注意：deploy/init.sql 已经创建 schemas；本 migration 假定它们存在。
--
-- Stage 37-A 修订：
--   - 删除 mv_daily_emotion 引用（materialized view 从未创建）
--   - CREATE ROLE 用 DO block 包裹 + 兼容已存在情况（init 重跑不报错）
--   - GRANT SELECT 拆成单行（更易调试 + 部分失败不影响其它）

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname='analytics_reader') THEN
        CREATE ROLE analytics_reader LOGIN PASSWORD 'CHANGE_ME_AT_DEPLOY'
            NOSUPERUSER NOCREATEDB NOCREATEROLE;
    END IF;
END$$;

GRANT USAGE ON SCHEMA
    emotion_echo_chat,
    emotion_echo_ai,
    emotion_echo_assessment,
    emotion_echo_analytics
TO analytics_reader;

-- 只读 VIEW + 本 schema 表权限（拆成单行，部分失败不影响其它）
GRANT SELECT ON emotion_echo_chat.msg_summary_v TO analytics_reader;
GRANT SELECT ON emotion_echo_ai.daily_emotion_v TO analytics_reader;
GRANT SELECT ON emotion_echo_assessment.assessment_v TO analytics_reader;
GRANT SELECT ON emotion_echo_analytics.user_behavior_events TO analytics_reader;

ALTER ROLE analytics_reader SET search_path TO
    emotion_echo_analytics, emotion_echo_chat,
    emotion_echo_ai, emotion_echo_assessment;