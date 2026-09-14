-- 004_add_intent_to_messages.sql · Stage 82 PR-3b
--
-- messages 加 intent 列（6 类消息意图：emotional_support/study_help/tech_help/
-- career_help/lifestyle/other）。写入端（chat-svc SendMessage logic）做白名单校验，
-- 非法值落 ''（未分类）。'' 兼容历史行。
--
-- §契约 5：DDL ↔ GORM model ↔ 写入端白名单三方一致（intent 允许集见
-- emotion-echo-chat-svc/internal/logic/sendmessagelogic.go allowedIntents）。
ALTER TABLE emotion_echo_chat.messages
    ADD COLUMN IF NOT EXISTS intent VARCHAR(16) NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_messages_intent ON emotion_echo_chat.messages(intent);
