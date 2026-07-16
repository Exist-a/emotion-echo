-- 回滚：将 last_message_time 从 BIGINT 改回 TIMESTAMP
-- 创建于: 2026-04-28

-- 1. 添加临时列
ALTER TABLE conversations ADD COLUMN last_message_time_new TIMESTAMP;

-- 2. 迁移数据（将毫秒时间戳转换为 TIMESTAMP）
UPDATE conversations 
SET last_message_time_new = to_timestamp(last_message_time / 1000.0)
WHERE last_message_time IS NOT NULL;

-- 3. 删除 bigint 列
ALTER TABLE conversations DROP COLUMN last_message_time;

-- 4. 重命名新列
ALTER TABLE conversations RENAME COLUMN last_message_time_new TO last_message_time;
