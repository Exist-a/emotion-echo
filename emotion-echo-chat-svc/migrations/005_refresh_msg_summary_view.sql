-- 005_refresh_msg_summary_view.sql · Stage 82 PR-3b
--
-- msg_summary_v 视图补 intent 列（analytics 意图分布数据源）。
-- CREATE OR REPLACE 无法变更列集 → 必须 DROP + CREATE（升级安全）。
-- 集中式 DDL（deploy/db/04-create-views.sql）已同步，新环境直接建新视图。
DROP VIEW IF EXISTS emotion_echo_chat.msg_summary_v;
CREATE VIEW emotion_echo_chat.msg_summary_v AS
SELECT
    id,
    conversation_id,
    user_id,
    role,
    content_type,
    intent,
    tokens_used,
    LENGTH(content) AS content_len,
    created_at AS send_time
FROM emotion_echo_chat.messages;
