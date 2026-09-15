-- migrations/i008_voice_transcripts_soft_delete.sql
--
-- Round 1 follow-up：voice_transcripts 加 deleted_at TIMESTAMPTZ 软删除字段
--
-- 背景：
--   i007 已给 emotion_analysis / face_emotion_results / voice_emotion_results /
--   fused_emotions 加 deleted_at 列。第 5 张表 voice_transcripts 在
--   deploy/db/02-create-tables-in-schemas.sql:140-150 建表时未含 deleted_at 列。
--
-- 设计：
--   - voice_transcripts 表加 deleted_at TIMESTAMPTZ NULL
--   - NULL = 未删除；非 NULL = 删除时间
--   - 现有查询需手动加 WHERE deleted_at IS NULL（migration 不自动改 Go 代码，
--     留作后续 PR 把 GORM 模型加 gorm.DeletedAt 后由 GORM 自动加 WHERE 条件）
--   - 索引加速 "where deleted_at IS NULL" 全表扫描
--
-- 适用范围：emotion_echo_ai schema
-- 幂等性：IF NOT EXISTS 类语法，可重复执行不报错。

BEGIN;

ALTER TABLE emotion_echo_ai.voice_transcripts
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

-- 软删除字段上的索引
CREATE INDEX IF NOT EXISTS idx_voice_transcripts_deleted_at
    ON emotion_echo_ai.voice_transcripts(deleted_at) WHERE deleted_at IS NULL;

COMMIT;
