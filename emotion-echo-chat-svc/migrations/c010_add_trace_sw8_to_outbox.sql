-- c010_add_trace_sw8_to_outbox.sql
--
-- CL-03 Phase 1：outbox_events 增加 trace_sw8 列 —— 让异步链路
-- （入站 → outbox → relay → Kafka → 消费端）的 trace 连成一条。
--
-- 变更原因：
--   relay 跑在**后台根 ctx** 上（chat-svc/main.go 的 relayCtx =
--   context.WithCancel(context.Background())）⇒ events/kafka_publisher.go 的
--   CreateExitSpan 建出来的是「根」span，一条消息的 SkyWalking trace 在
--   outbox → relay 处断掉（消费侧从 Kafka header 往后那一段本来是连的）。
--   写入 outbox 行时身处**请求 ctx**（有 gin 入口 span）⇒ 把当时的 sw8 存进本列，
--   relay 发布前用 extractor 恢复父 ctx。
--
-- 跨域影响：**无**。本列只被 chat-svc 自身读写（写侧三条 logic + 读侧 relay）；
--   不进 Kafka 事件 payload、不进 proto；analytics-svc 视图 `msg_summary_v` 与
--   ai-svc consumer 均不依赖它。
--   ⇒ 无需同步 `deploy/db/02-create-tables-in-schemas.sql`：outbox_events 本就是
--     chat-svc 自治表（不在集中 initdb 内，见 ADR-2026-09-chat-svc-table-deps §2.3）。
--
-- 可空：历史行没有该值；tracer=nil（dev / 单测）与非请求路径也不写 ⇒ 允许 NULL。
--
-- 回滚方法：
--   1) 先回滚代码（去掉 relay 按 sw8 恢复父 ctx 的路径），再执行：
--      ALTER TABLE emotion_echo_chat.outbox_events DROP COLUMN IF EXISTS trace_sw8;
--   2) 顺序不能反：先 DROP 列会让 GORM 查询（SELECT *）因缺列报错。
--
-- 参考：docs/closure-roadmap/stages/cl-03-kafka-reliability/plan.md §3 Phase 1

BEGIN;

ALTER TABLE emotion_echo_chat.outbox_events
    ADD COLUMN IF NOT EXISTS trace_sw8 VARCHAR(512);

COMMIT;
