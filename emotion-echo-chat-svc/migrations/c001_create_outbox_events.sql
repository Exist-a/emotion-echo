-- 001_create_outbox_events.sql
--
-- Stage 30-C A3: chat-svc 事务性 Outbox 表。
--
-- 背景：
--   chat-svc 三个 logic（create / send / delete）原是「DB 落库 → Publish」，
--   Publish 失败仅写日志，事件静默丢失。本次改造为「同事务写业务 + outbox 行」，
--   再由 relay goroutine 异步发送。
--
-- Schema：
--   id          BIGSERIAL PK
--   event_id    VARCHAR(64) UNIQUE — 业务事件 ID（UUID），与 chat-svc Publish 的 Event.ID 一致
--                                同时是 A1 消费者幂等键（event_id UNIQUE）
--   event_type  VARCHAR(64) — conversation.created / message.created / conversation.closed
--   topic       VARCHAR(64) — chat-events（未来可扩展多 topic）
--   payload     JSONB NOT NULL — 已序列化的事件 JSON
--   status      VARCHAR(16) NOT NULL DEFAULT 'pending' — pending / sent / failed
--   attempts    INT NOT NULL DEFAULT 0 — Publish 失败次数（>0 时发告警）
--   last_error  TEXT — 最近一次 Publish 失败的错误信息（debug 用）
--   created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW() — 入库时间（relay 按此排序）
--   sent_at     TIMESTAMPTZ NULL — 成功发送时间
--
-- 索引：
--   - 业务事件 ID UNIQUE（A1 幂等去重）
--   - (status, created_at) 部分索引 — relay 拉 pending 高频路径

BEGIN;

CREATE SCHEMA IF NOT EXISTS emotion_echo_chat;

CREATE TABLE IF NOT EXISTS emotion_echo_chat.outbox_events (
    id          BIGSERIAL PRIMARY KEY,
    event_id    VARCHAR(64) NOT NULL UNIQUE,
    event_type  VARCHAR(64) NOT NULL,
    topic       VARCHAR(64) NOT NULL,
    payload     JSONB NOT NULL,
    status      VARCHAR(16) NOT NULL DEFAULT 'pending',
    attempts    INT NOT NULL DEFAULT 0,
    last_error  TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    sent_at     TIMESTAMPTZ
);

-- relay 拉 pending 时按 created_at ASC；partial index 减少索引体积
CREATE INDEX IF NOT EXISTS idx_outbox_pending
    ON emotion_echo_chat.outbox_events(created_at)
    WHERE status = 'pending';

-- attempts > 0 的行（失败过）便于运维查询
CREATE INDEX IF NOT EXISTS idx_outbox_attempts
    ON emotion_echo_chat.outbox_events(attempts)
    WHERE attempts > 0;

COMMIT;
