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
--     user_behavior_events / mv_daily_emotion
--
-- 注意：deploy/init.sql 已经创建 schemas；本 migration 假定它们存在。

CREATE ROLE analytics_reader LOGIN PASSWORD 'CHANGE_ME_AT_DEPLOY'
    NOSUPERUSER NOCREATEDB NOCREATEROLE;

GRANT USAGE ON SCHEMA
    emotion_echo_chat,
    emotion_echo_ai,
    emotion_echo_assessment,
    emotion_echo_analytics
TO analytics_reader;

-- 只读 VIEW + 本 schema 表权限
GRANT SELECT ON
    emotion_echo_chat.msg_summary_v,
    emotion_echo_ai.daily_emotion_v,
    emotion_echo_assessment.assessment_v,
    emotion_echo_analytics.user_behavior_events,
    emotion_echo_analytics.mv_daily_emotion
TO analytics_reader;

ALTER ROLE analytics_reader SET search_path TO
    emotion_echo_analytics, emotion_echo_chat,
    emotion_echo_ai, emotion_echo_assessment;