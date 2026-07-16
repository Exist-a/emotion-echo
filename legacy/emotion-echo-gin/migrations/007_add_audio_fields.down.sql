-- 删除语音消息相关字段
ALTER TABLE messages DROP COLUMN IF EXISTS audio_url;
ALTER TABLE messages DROP COLUMN IF EXISTS audio_duration;
