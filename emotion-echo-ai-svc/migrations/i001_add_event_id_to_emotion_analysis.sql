-- 001_add_event_id_to_emotion_analysis.sql
--
-- Stage 30-C A1: 消费幂等去重 — ai-svc 侧 migration。
--
-- 背景：
--   ai-svc 消费 chat-svc 发布的 chat-events，写 emotion_echo_ai.emotion_analysis。
--   在 at-least-once 投递下，重复消费会重复落库（emotion counts 虚高）。
--   给 emotion_analysis 加 event_id 列 + UNIQUE 约束，配合 repo 层
--   ON CONFLICT (event_id) DO NOTHING 兜底幂等。
--
-- 老数据处理：
--   emotion_analysis 表可能有历史重复行。直接 ADD CONSTRAINT UNIQUE 会因
--   NULL 重复而失败。处理顺序：
--     1) UPDATE 老行 event_id = 'legacy-' || id::text（保证唯一）
--     2) 加 UNIQUE 约束（完整约束，非 partial，GORM OnConflict 直接匹配）
--
-- 幂等性：使用 IF NOT EXISTS 类语法 / DO block，可重复执行不报错。
-- 适用范围：仅 emotion_echo_ai schema（ai-svc 独占）。

BEGIN;

-- 1) 加 event_id 列（可空，老行 NULL 不会阻塞本步）
ALTER TABLE emotion_echo_ai.emotion_analysis
    ADD COLUMN IF NOT EXISTS event_id VARCHAR(64);

-- 2) 老数据回填：NULL → 'legacy-'||id（保证 UNIQUE 可加）
UPDATE emotion_echo_ai.emotion_analysis
   SET event_id = 'legacy-' || id::text
 WHERE event_id IS NULL;

-- 3) 加 UNIQUE 约束。完整（非 partial）— GORM OnConflict Columns: [event_id] 直接匹配。
DO $$
BEGIN
    -- 2026-09-04：守卫放宽到同时检查 pg_class。原守卫只查 pg_constraint，但同名
    -- 对象也可能以**索引**形式存在（旧版 deploy/db/02 曾建同名 UNIQUE INDEX，
    -- 索引记在 pg_class 而非 pg_constraint）。守卫查不到便放行，ADD CONSTRAINT
    -- 随即报 relation "uq_emotion_analysis_event_id" already exists，
    -- 令整个迁移失败、db-migrate 容器退出 1、所有业务服务被卡住启动。
    -- 02 侧已停止创建该同名索引；此处放宽是为兼容已有该索引的老环境。
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'uq_emotion_analysis_event_id'
    ) AND NOT EXISTS (
        SELECT 1 FROM pg_class
        WHERE relname = 'uq_emotion_analysis_event_id'
    ) THEN
        ALTER TABLE emotion_echo_ai.emotion_analysis
            ADD CONSTRAINT uq_emotion_analysis_event_id UNIQUE (event_id);
    END IF;
END$$;

COMMIT;
