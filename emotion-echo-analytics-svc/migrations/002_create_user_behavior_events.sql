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


-- ============================================================================
-- ADR-19 Sprint A 收口（PR-A1.3 v2）: 落库格式统一为 chat-svc DevEventPublisher 风格
-- ----------------------------------------------------------------------------
-- 之前 analytics-svc Kafka consumer 的 target / session_id 落库格式:
--   target:     '100' / '42'                              (纯数字)
--   session_id: 'chat-events'                              (Kafka topic 名)
-- chat-svc DevEventPublisher (KAFKA_ENABLED=false dev 路径) 的格式:
--   target:     'msg:100' / 'conv:42'                      (语义化前缀)
--   session_id: 'conv:42'                                  (语义化前缀)
--
-- Sprint A 收口后,两路径都走 shared/pkg/eventrow.MapEventToUserBehaviorRow,
-- 统一落库格式:
--   target:     'msg:N'         (message.created 类)
--                'conv:N'        (conversation.created/closed 类)
--   session_id: 'conv:N'        (全事件统一,会话聚合语义清晰)
--
-- event_type 字段仍由 normalizeEventType 后值落库(不带点):
--   'message.created'             → 'message'
--   'conversation.created'        → 'conversation_created'
--   'conversation.closed'         → 'conversation_closed'
-- chat-svc DevEventPublisher 路径 event_type 落 ev.Type 原值(带点)。
-- 两种 event_type 落库值并存(后续数据迁移 PR 决定是否统一)。
--
-- 数据迁移参考 SQL（仅在新表/新集群需要时执行,历史数据可保留）:
--
--   -- 1. target: 纯数字 '100' → 'msg:100'（仅 message 类）
--   UPDATE emotion_echo_analytics.user_behavior_events
--   SET target = 'msg:' || target
--   WHERE event_type = 'message'
--     AND target ~ '^[0-9]+$';
--
--   -- 2. target: 纯数字 '42' → 'conv:42'（conversation 类）
--   UPDATE emotion_echo_analytics.user_behavior_events
--   SET target = 'conv:' || target
--   WHERE event_type IN ('conversation_created', 'conversation_closed')
--     AND target ~ '^[0-9]+$';
--
--   -- 3. session_id: 'chat-events' → 'conv:' || <conv_id from data>
--   --    （需从原始 Kafka payload 查 conversation_id;纯 SQL 难做,留应用层工具）
--
--   -- 4. event_type: 'message' → 'message.created'（仅历史 normalize 后值）
--   UPDATE emotion_echo_analytics.user_behavior_events
--   SET event_type = 'message.created'
--   WHERE event_type = 'message';
--   UPDATE emotion_echo_analytics.user_behavior_events
--   SET event_type = 'conversation.created'
--   WHERE event_type = 'conversation_created';
--   UPDATE emotion_echo_analytics.user_behavior_events
--   SET event_type = 'conversation.closed'
--   WHERE event_type = 'conversation_closed';
-- ============================================================================