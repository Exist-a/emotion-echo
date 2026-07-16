-- 将 last_message_time 从 TIMESTAMP 改为 BIGINT（毫秒时间戳）
-- 创建于: 2026-04-28

-- 1. 添加临时列
ALTER TABLE conversations ADD COLUMN last_message_time_new BIGINT;

-- 2. 迁移数据（将现有 TIMESTAMP 转换为毫秒时间戳）
UPDATE conversations 
SET last_message_time_new = EXTRACT(EPOCH FROM last_message_time) * 1000
WHERE last_message_time IS NOT NULL;

-- 3. 删除旧列
ALTER TABLE conversations DROP COLUMN last_message_time;

-- 4. 重命名新列
ALTER TABLE conversations RENAME COLUMN last_message_time_new TO last_message_time;

-- 5. 添加索引（如果原来有的话）
-- 注意：根据 001_init_schema.up.sql，原表没有单独为 last_message_time 创建索引
