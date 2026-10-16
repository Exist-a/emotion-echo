-- a011_dlq_replay_runs.sql
--
-- CL-03 Phase 4c（CL-D-18=B）：DLQ 回放运行记录表。
--
-- 变更原因：回放工具（scripts/replay_dlq）是**人工 CLI**，没有 /metrics 端点
--   ⇒ "这次回放了多少条、成功多少、谁跑的" 若只活在当次终端输出里，事后不可查、
--   也无法被 Grafana 消费。落一张表后：① 事后排查直接 SQL 查；② Grafana 用
--   PostgreSQL 数据源出"最近回放"面板（与 T-10 验收口径一致）。
--
-- 跨域影响：仅 analytics-svc schema（emotion_echo_analytics）新增一张**自表**；
--   无外键、无视图依赖、不改既有列、不参与 Kafka 事件 payload ⇒ 对 ai-svc / chat-svc /
--   BFF 零影响。
--
-- 可空/默认：operator 允许空（人工工具可能不填）；计数字段默认 0。
--
-- 回滚方法：
--   DROP TABLE IF EXISTS emotion_echo_analytics.dlq_replay_runs;
--   （纯审计表，删除只丢失历史回放记录，不影响任何业务数据）
--
-- 参考：docs/closure-roadmap/stages/cl-03-kafka-reliability/plan.md §3 Phase 4c

BEGIN;

CREATE TABLE IF NOT EXISTS emotion_echo_analytics.dlq_replay_runs (
    id           BIGSERIAL PRIMARY KEY,
    started_at   TIMESTAMPTZ NOT NULL,
    finished_at  TIMESTAMPTZ NOT NULL,
    dlq_topic    VARCHAR(128) NOT NULL,
    target_topic VARCHAR(128),
    replayed     INT NOT NULL DEFAULT 0,
    failed       INT NOT NULL DEFAULT 0,
    dry_run      BOOLEAN NOT NULL DEFAULT FALSE,
    operator     VARCHAR(64)
);

-- 面板/排查都按时间倒序取最近若干次
CREATE INDEX IF NOT EXISTS idx_dlq_replay_runs_started_at
    ON emotion_echo_analytics.dlq_replay_runs(started_at DESC);

COMMIT;
