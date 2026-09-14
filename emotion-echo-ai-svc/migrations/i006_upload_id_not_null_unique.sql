-- migrations/006_upload_id_not_null_unique.sql
--
-- P1-R2-8 / P1-R2-9: upload_id / event_id 列允许多 NULL → UNIQUE 索引对 NULL 失效
-- （PostgreSQL UNIQUE 索引默认把多个 NULL 视为不重复）。
-- 结果：同消息多次分析会插入多行，幂等去重失效 → 报表翻倍 / 落库重复。
--
-- 修复策略：
--   1. upload_id / event_id 加 NOT NULL 约束（先回填已有 NULL 为 '__legacy__'）
--   2. 把普通 UNIQUE INDEX 改为 NULLS NOT DISTINCT 风格的 partial unique 索引
--      （PG 15+ 支持 NULLS NOT DISTINCT，dev 环境 PG 15）
--   3. 新约束保证 upload_id = X 的行全局唯一，幂等恢复
--
-- 适用范围：emotion_echo_ai schema（ai-svc 独占）

BEGIN;

-- 1) face_emotion_results.upload_id
-- 回填 NULL 为 '__legacy__' 以允许 NOT NULL 约束
UPDATE emotion_echo_ai.face_emotion_results
SET upload_id = '__legacy__'
WHERE upload_id IS NULL;

ALTER TABLE emotion_echo_ai.face_emotion_results
    ALTER COLUMN upload_id SET NOT NULL;

-- 删除原 UNIQUE INDEX（普通索引视 NULL 为不重复，幂等失效）
DROP INDEX IF EXISTS emotion_echo_ai.uq_face_emotion_upload_id;

-- 重建：partial unique 索引（保证非空值全局唯一）
CREATE UNIQUE INDEX IF NOT EXISTS uq_face_emotion_upload_id
    ON emotion_echo_ai.face_emotion_results(upload_id)
    WHERE upload_id <> '__legacy__';

-- 2) emotion_analysis.event_id（同 P1-R2-9）
UPDATE emotion_echo_ai.emotion_analysis
SET event_id = '__legacy__'
WHERE event_id IS NULL;

ALTER TABLE emotion_echo_ai.emotion_analysis
    ALTER COLUMN event_id SET NOT NULL;

-- 注意：emotion_analysis 的 event_id 唯一约束已由 migrations/001_add_event_id_to_emotion_analysis.sql
-- 通过 ADD CONSTRAINT 创建（不是 INDEX）。检查约束名后重建。
DO $$
DECLARE
    cname text;
BEGIN
    SELECT conname INTO cname
    FROM pg_constraint
    WHERE conrelid = 'emotion_echo_ai.emotion_analysis'::regclass
      AND contype = 'u'
      AND pg_get_constraintdef(oid) ILIKE '%event_id%';
    IF cname IS NOT NULL THEN
        EXECUTE format('ALTER TABLE emotion_echo_ai.emotion_analysis DROP CONSTRAINT %I', cname);
    END IF;
END $$;

-- 重建约束：partial unique（排除 legacy 行）
CREATE UNIQUE INDEX IF NOT EXISTS uq_emotion_analysis_event_id
    ON emotion_echo_ai.emotion_analysis(event_id)
    WHERE event_id <> '__legacy__';

COMMIT;
