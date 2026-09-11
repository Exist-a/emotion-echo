-- 003_add_pinned_to_conversations.sql
--
-- Stage 72：PinConversation RPC 落地（决策 4 ADR §八 backlog 收口）
--
-- conversations 加 pinned 列（置顶标记）。
-- 与 deploy/db/02-create-tables-in-schemas.sql（全新安装 DDL）同步维护；
-- 既有库由 db-migrate 容器自动应用本文件。
--
-- 契约（AGENTS.md §2.4 §契约 5）：布尔列，不涉 VARCHAR enum；写入端
-- model.Conversation.Pinned gorm column:pinned 与本列名一致。

ALTER TABLE emotion_echo_chat.conversations
    ADD COLUMN IF NOT EXISTS pinned BOOLEAN NOT NULL DEFAULT FALSE;
