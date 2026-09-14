-- migrations/i007_soft_delete_columns.sql
--
-- P2-R2-7: ai-svc 核心表加 deleted_at TIMESTAMPTZ 软删除字段
--
-- 背景：ai-svc 走 GORM 但当前模型无 gorm.DeletedAt 字段，
-- 删除是物理 DELETE → 误删后无法恢复 / 无审计轨迹。
--
-- 设计：
--   - 给 emotion_analysis / face_emotion_results / voice_emotion_results /
--     fused_emotions 加 deleted_at TIMESTAMPTZ NULL
--   - NULL = 未删除；非 NULL = 删除时间
--   - 现有查询需手动加 WHERE deleted_at IS NULL（migration 不自动改 Go 代码，
--     留作后续 PR 把 GORM 模型加 gorm.DeletedAt 后由 GORM 自动加 WHERE 条件）
--
-- 适用范围：emotion_echo_ai schema

BEGIN;

ALTER TABLE emotion_echo_ai.emotion_analysis ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
ALTER TABLE emotion_echo_ai.face_emotion_results ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
ALTER TABLE emotion_echo_ai.voice_emotion_results ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
ALTER TABLE emotion_echo_ai.fused_emotions ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

-- 软删除字段上的索引：加速 "where deleted_at IS NULL" 全表扫描
CREATE INDEX IF NOT EXISTS idx_emotion_deleted_at ON emotion_echo_ai.emotion_analysis(deleted_at) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_face_deleted_at ON emotion_echo_ai.face_emotion_results(deleted_at) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_voice_deleted_at ON emotion_echo_ai.voice_emotion_results(deleted_at) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_fused_deleted_at ON emotion_echo_ai.fused_emotions(deleted_at) WHERE deleted_at IS NULL;

COMMIT;
