-- c013_add_agent_id_to_messages.sql · CL-06（用户自定义 Agent·多角色）
--
-- 消息级**角色快照**：`agent_id` 记录"这条 AI 消息由哪个角色产生"。
--
-- 语义：**快照**（写入时取该会话当时的当前角色），因此角色后来改名/被删都不影响历史可读性。
-- 只对 assistant 消息有意义；用户消息留 NULL。
--
-- ⚠️ 与 c012 同理**不建跨 schema 外键**（角色归 user-svc 所有）。
ALTER TABLE emotion_echo_chat.messages
    ADD COLUMN IF NOT EXISTS agent_id BIGINT;

COMMENT ON COLUMN emotion_echo_chat.messages.agent_id IS
    'CL-06 该 AI 消息由哪个角色产生（快照；仅 assistant 消息有值；跨 schema 无外键）';
