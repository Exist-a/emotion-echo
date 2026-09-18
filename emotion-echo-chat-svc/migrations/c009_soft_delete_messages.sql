-- =====================================================
--  E2E-06: messages 表软删除改造
--  路径：emotion-echo-chat-svc/migrations/c009_soft_delete_messages.sql
--
--  背景：c008 只给 conversations 添加了 deleted_at，遗漏了 messages。
--  删除会话时需要级联软删除消息。
--
--  幂等：IF NOT EXISTS 保证可重复执行
-- =====================================================

-- 添加 deleted_at 列
ALTER TABLE emotion_echo_chat.messages
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

-- 添加索引（过滤未删除消息）
CREATE INDEX IF NOT EXISTS idx_messages_deleted_at
    ON emotion_echo_chat.messages(deleted_at)
    WHERE deleted_at IS NULL;