-- 为 refresh_tokens 表添加 remember_me 字段
ALTER TABLE refresh_tokens
ADD COLUMN IF NOT EXISTS remember_me BOOLEAN NOT NULL DEFAULT FALSE;
