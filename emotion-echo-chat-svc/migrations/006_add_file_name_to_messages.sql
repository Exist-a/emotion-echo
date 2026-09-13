-- 006_add_file_name_to_messages.sql · Stage 89 PR-2
--
-- messages 加 file_name 列（文件消息的原始文件名）。
-- 背景：对象 key = uploads/<uid>-<sha256(filename)[:8]><ext>，原始名此前只存在于
-- 上传瞬间的浏览器内存中——LLM prompt 与前端 ChatFile 展示都需要真实文件名。
-- '' 兼容历史行与非文件消息。
--
-- §契约 5：DDL ↔ GORM model（model.Message.FileName）↔ 写入端（SendMessage logic
-- 超 255 截断）三方一致。
ALTER TABLE emotion_echo_chat.messages
    ADD COLUMN IF NOT EXISTS file_name VARCHAR(255) NOT NULL DEFAULT '';
