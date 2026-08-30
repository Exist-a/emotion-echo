-- migrations/002_create_user_behavior_events.sql
--
-- Stage 30-A §六.6.2: user_behavior_events 表。
--
-- 这是 analytics-svc 自己拥有的表（Kafka consumer 写入）。
-- 与跨 schema VIEW 不同——这里 analytics-svc 是 owning service。
--
-- 表结构与 Go 侧 model.UserBehaviorEvent 字段一一对应。

CREATE TABLE IF NOT EXISTS emotion_echo_analytics.user_behavior_events (
    id          BIGSERIAL PRIMARY KEY,
    user_id     BIGINT NOT NULL,
    event_type  VARCHAR(64) NOT NULL,            -- 'message' / 'conversation_created' / 'conversation_closed'
    target      VARCHAR(255),                     -- Event.ID 标识
    session_id  VARCHAR(64),                      -- Kafka topic 占位
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 加速按 user + 时间窗口聚合（Round 2 GREEN 的 GET day-night / depth / frequency）
CREATE INDEX IF NOT EXISTS idx_events_user_time
    ON emotion_echo_analytics.user_behavior_events(user_id, occurred_at DESC);

-- 加速按 type 过滤
CREATE INDEX IF NOT EXISTS idx_events_type_time
    ON emotion_echo_analytics.user_behavior_events(event_type, occurred_at DESC);