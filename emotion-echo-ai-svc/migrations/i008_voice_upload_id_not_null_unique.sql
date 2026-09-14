-- migrations/008_voice_upload_id_not_null_unique.sql
--
-- Round 1.1 / P1-R2-8: voice_emotion_results.upload_id 列允许多 NULL → UNIQUE 索引
-- 对 NULL 失效（PostgreSQL 普通 UNIQUE 索引默认把多个 NULL 视为不重复）。
-- 结果：同 voice 上传多次可落 N 行，幂等去重失效 → 报表翻倍 / 落库重复。
--
-- 修复策略（与 i006 face_emotion_results.upload_id 完全对称）：
--   1. upload_id 加 NOT NULL 约束（先回填已有 NULL 为 '__legacy__'）
--   2. 把普通 UNIQUE INDEX 改为 partial unique 索引
--      （PG 15+ 风格：partial `WHERE upload_id <> '__legacy__'`）
--   3. 新约束保证 upload_id = X 的非 legacy 行全局唯一，幂等恢复
--   4. 多个 legacy 行可并存（partial unique 排除）
--
-- 适用范围：emotion_echo_ai schema（ai-svc 独占）
-- 前置：i003 (voice_emotion_results) 已建表 + 普通 UNIQUE INDEX uq_voice_emotion_upload_id

BEGIN;

-- 1) voice_emotion_results.upload_id
-- 回填 NULL 为 '__legacy__' 以允许 NOT NULL 约束
UPDATE emotion_echo_ai.voice_emotion_results
SET upload_id = '__legacy__'
WHERE upload_id IS NULL;

ALTER TABLE emotion_echo_ai.voice_emotion_results
    ALTER COLUMN upload_id SET NOT NULL;

-- 删除原 UNIQUE INDEX（普通索引视 NULL 为不重复，幂等失效）
DROP INDEX IF EXISTS emotion_echo_ai.uq_voice_emotion_upload_id;

-- 重建：partial unique 索引（保证非 legacy 行全局唯一）
CREATE UNIQUE INDEX IF NOT EXISTS uq_voice_emotion_upload_id
    ON emotion_echo_ai.voice_emotion_results(upload_id)
    WHERE upload_id <> '__legacy__';

COMMIT;
