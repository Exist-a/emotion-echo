-- migrations/i013_memory_graph_uuid.sql
--
-- CL-08d（记忆 · 删除闭环）：memory_episode 补 graph_uuid。
--
-- 背景：Phase 4 图摄取（graphiti add_episode）返回的 episode uuid 之前没落 PG；
-- 删除闭环（T-16 / ADR 决策 3「图随删」）需要它定位 Graphiti.remove_episode。
-- 旧行（Phase 4 之前摄取的）graph_uuid 为 NULL ⇒ 删除时回落「全图重建」路径。
--
-- 归属：emotion_echo_ai；幂等（compose up 每次都跑）。

BEGIN;

ALTER TABLE emotion_echo_ai.memory_episode
    ADD COLUMN IF NOT EXISTS graph_uuid VARCHAR(64);

COMMIT;
