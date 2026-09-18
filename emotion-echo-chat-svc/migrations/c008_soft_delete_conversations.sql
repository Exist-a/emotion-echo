-- =====================================================
--  E2E-06: conversations 表软删除改造
--  路径：emotion-echo-chat-svc/migrations/c008_soft_delete_conversations.sql
--
--  背景：原来 DeleteConversation 是物理 DELETE，与 users/ai 域的软删除不一致。
--  统一为 deleted_at 列 + 查询过滤。
--
--  幂等：IF NOT EXISTS 保证可重复执行
-- =====================================================

-- 添加 deleted_at 列
ALTER TABLE emotion_echo_chat.conversations
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

-- 添加索引（过滤未删除会话）
CREATE INDEX IF NOT EXISTS idx_conversations_deleted_at
    ON emotion_echo_chat.conversations(deleted_at)
    WHERE deleted_at IS NULL;