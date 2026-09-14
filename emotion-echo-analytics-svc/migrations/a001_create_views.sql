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

-- emotion_echo_chat.msg_summary_v:
--   本文件原有一份旧 8 列定义（无 intent），Stage 86 发现幂等性 bug：
--   chat-svc 005（Stage 82 PR-3b）已把该视图升级为带 intent 列的 9 列版
--   （DROP+CREATE + 重新 GRANT），本文件的 CREATE OR REPLACE 旧定义在
--   migrate.sh 全量重跑时会把 intent 列挤掉 → "cannot drop columns from view"。
--   视图所有权收敛到 chat-svc 005 + 集中式 deploy/db/04-create-views.sql，
--   此处不再重复定义（消除双 owner 漂移点）。

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