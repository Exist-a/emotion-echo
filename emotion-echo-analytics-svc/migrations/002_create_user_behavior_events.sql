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
-- ADR-19 Sprint A 全收口（PR-A1.3 v2 + PR-A1.4）: 落库格式统一为 chat-svc 风格
-- ----------------------------------------------------------------------------
-- 之前 analytics-svc Kafka consumer 的 target / session_id / event_type 落库格式:
--   target:     '100' / '42'                              (纯数字)
--   session_id: 'chat-events'                              (Kafka topic 名)
--   event_type: 'message' / 'conversation_created' / 'conversation_closed'
--                (normalizeEventType 后值,不带点)
-- chat-svc DevEventPublisher (KAFKA_ENABLED=false dev 路径) 的格式:
--   target:     'msg:100' / 'conv:42'                      (语义化前缀)
--   session_id: 'conv:42'                                  (语义化前缀)
--   event_type: 'message.created' / 'conversation.created' / 'conversation.closed'
--                (ev.Type 原值,带点)
--
-- Sprint A 全收口后,两路径都走 shared/pkg/eventrow.MapEventToUserBehaviorRow
-- (PR-A1.3 v2) + analytics-svc consumer.handleOne 不再 normalizeEventType
-- (PR-A1.4)。统一落库格式:
--   target:     'msg:N'         (message 类)
--                'conv:N'        (conversation 类)
--   session_id: 'conv:N'        (全事件统一)
--   event_type: 'message.created' / 'conversation.created' / 'conversation.closed'
--                (带点原值,与 chat-svc 完全一致 → 真正消灭两份映射)
--
-- 数据迁移 SQL（dev 集群部署时执行一次,生产环境执行前需人工 review）:
--
--   -- 1. event_type: normalize 后值 → 带点原值
--   --    影响范围: 仅历史 normalizeEventType 写入的行
--   UPDATE emotion_echo_analytics.user_behavior_events
--   SET event_type = 'message.created'
--   WHERE event_type = 'message';
--   UPDATE emotion_echo_analytics.user_behavior_events
--   SET event_type = 'conversation.created'
--   WHERE event_type = 'conversation_created';
--   UPDATE emotion_echo_analytics.user_behavior_events
--   SET event_type = 'conversation.closed'
--   WHERE event_type = 'conversation_closed';
--
--   -- 2. target: 纯数字 → 'msg:N' / 'conv:N'
--   --    影响范围: 仅 PR-A1.3 v2 之前 consumer 写入的行(无前缀数字)
--   UPDATE emotion_echo_analytics.user_behavior_events
--   SET target = 'msg:' || target
--   WHERE event_type = 'message.created'
--     AND target ~ '^[0-9]+$';
--   UPDATE emotion_echo_analytics.user_behavior_events
--   SET target = 'conv:' || target
--   WHERE event_type IN ('conversation.created', 'conversation.closed')
--     AND target ~ '^[0-9]+$';
--
--   -- 3. session_id: 'chat-events' → 'conv:' || <conv_id from target>
--   --    启发式: 从 target(已迁移为 'conv:N') 抽 conv_id 重写 session_id
--   --    边界: 若 target 已为 'conv:N' 但 session_id 仍为 'chat-events',
--   --          此 UPDATE 会从 target 抽 N 重建 session_id
--   UPDATE emotion_echo_analytics.user_behavior_events
--   SET session_id = target
--   WHERE session_id = 'chat-events'
--     AND target LIKE 'conv:%';
--
--   -- 4. 验证: 应 0 行历史 normalize 后值 / 纯数字 target / 'chat-events' session_id
--   SELECT COUNT(*) FROM emotion_echo_analytics.user_behavior_events
--   WHERE event_type IN ('message', 'conversation_created', 'conversation_closed')
--      OR target ~ '^[0-9]+$'
--      OR session_id = 'chat-events';
--   -- 期望: 0
--
-- 历史 schema 中 normalizeEventType 已删除(PR-A1.4);若需查询老 enum
-- (dev/测试环境),用以下 SQL 取数:
--   SELECT * FROM emotion_echo_analytics.user_behavior_events
--   WHERE event_type IN ('message', 'conversation_created', 'conversation_closed');
-- ============================================================================