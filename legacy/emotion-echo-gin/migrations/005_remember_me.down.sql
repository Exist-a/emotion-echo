-- 回滚：删除 remember_me 字段
ALTER TABLE refresh_tokens
DROP COLUMN IF EXISTS remember_me;
