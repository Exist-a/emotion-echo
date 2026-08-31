-- 002_add_client_msg_id.sql
--
-- Stage 33 PR-18 · 客户端幂等键。
--
-- 背景：A-1 P0 修复。前端每次发送消息时生成 UUID 作为 client_msg_id，
-- chat-svc 用 partial UNIQUE INDEX 拒绝重复入库（网络重试场景）。
--
-- 设计要点：
-- 1. 列允许 NULL（历史消息不补字段；新消息才用 client_msg_id）
-- 2. partial UNIQUE INDEX 仅在 client_msg_id IS NOT NULL 时生效
--    → 避免历史 NULL 数据触发冲突
-- 3. 用 IF NOT EXISTS 保证幂等（运维可重复执行）
--
-- 应用方式：
--   docker exec -i emotion-echo-postgres psql -U postgres -d emotion_echo \
--     < emotion-echo-chat-svc/migrations/002_add_client_msg_id.sql

BEGIN;

ALTER TABLE emotion_echo_chat.messages
  ADD COLUMN IF NOT EXISTS client_msg_id UUID;

CREATE UNIQUE INDEX IF NOT EXISTS uq_messages_client_msg_id
  ON emotion_echo_chat.messages(client_msg_id)
  WHERE client_msg_id IS NOT NULL;

COMMIT;
