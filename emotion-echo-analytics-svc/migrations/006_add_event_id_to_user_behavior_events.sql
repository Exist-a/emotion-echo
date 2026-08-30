-- 006_add_event_id_to_user_behavior_events.sql
--
-- Stage 30-C A1: 消费幂等去重 — analytics-svc 侧 migration。
--
-- 背景：
--   analytics-svc 消费 chat-svc 发布的 chat-events，写
--   emotion_echo_analytics.user_behavior_events。at-least-once 投递下
--   重复消费会重复落库（行为统计翻倍）。给 user_behavior_events 加
--   event_id 列 + UNIQUE 约束，配合 repo 层 ON CONFLICT (event_id)
--   DO NOTHING 兜底幂等。
--
-- 老数据处理：
--   user_behavior_events 表可能有历史重复行。直接 ADD CONSTRAINT UNIQUE
--   会因 NULL 重复而失败。处理顺序：
--     1) UPDATE 老行 event_id = 'legacy-' || id::text（保证唯一）
--     2) 加 UNIQUE 约束（完整约束，非 partial，GORM OnConflict 直接匹配）
--
-- 幂等性：使用 IF NOT EXISTS / DO block，可重复执行不报错。
-- 适用范围：emotion_echo_analytics schema（analytics-svc 独占）。

BEGIN;

-- 1) 加 event_id 列（可空，老行 NULL 不会阻塞本步）
ALTER TABLE emotion_echo_analytics.user_behavior_events
    ADD COLUMN IF NOT EXISTS event_id VARCHAR(64);

-- 2) 老数据回填：NULL → 'legacy-'||id
UPDATE emotion_echo_analytics.user_behavior_events
   SET event_id = 'legacy-' || id::text
 WHERE event_id IS NULL;

-- 3) 加 UNIQUE 约束。完整（非 partial）。
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'uq_user_behavior_events_event_id'
    ) THEN
        ALTER TABLE emotion_echo_analytics.user_behavior_events
            ADD CONSTRAINT uq_user_behavior_events_event_id UNIQUE (event_id);
    END IF;
END$$;

COMMIT;
