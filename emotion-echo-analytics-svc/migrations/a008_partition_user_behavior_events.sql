-- migrations/008_partition_user_behavior_events.sql
--
-- P2-R2-11: user_behavior_events 按月分区（prod 几月后慢查询防护）
--
-- 背景：user_behavior_events 单表按 user_id 索引，全表扫描 + 排序在
-- 1M+ 行后会慢到秒级。按 occurred_at 月分区后：
--   - 单月查询只需扫当月分区（小表）
--   - 老数据可直接 detach / drop（无需 DELETE 全表）
--   - 索引体积也按月分割，PG planner 更智能
--
-- 设计：
--   - RANGE PARTITION BY (occurred_at)：月度分区
--   - 默认分区 p_default：承接超出现有分区范围的数据（防 "no partition of relation found"）
--   - 起始 2026-01-01：足够覆盖 dev + 初期 prod 数据
--   - 自动创建未来 3 个月分区（手动维护 cron 替代自动扩展，PG 14+ pg_partman 暂不引入）
--
-- 迁移策略：
--   1. CREATE TABLE IF NOT EXISTS 新分区表 _partitioned
--   2. INSERT INTO _partitioned SELECT * FROM user_behavior_events（数据复制）
--   3. 交换表名（ALTER TABLE RENAME）—— 单事务内完成
--   4. 旧表保留 30 天后人工 DROP（回滚窗口）
--
-- 适用范围：emotion_echo_analytics schema

BEGIN;

-- 新分区表（结构与原表一致 + PARTITION BY RANGE）
CREATE TABLE IF NOT EXISTS emotion_echo_analytics.user_behavior_events_partitioned (
    LIKE emotion_echo_analytics.user_behavior_events INCLUDING ALL
) PARTITION BY RANGE (occurred_at);

-- 创建月度分区（2026-01 到 2026-06 + 默认分区）
CREATE TABLE IF NOT EXISTS emotion_echo_analytics.ube_2026_01 PARTITION OF emotion_echo_analytics.user_behavior_events_partitioned
    FOR VALUES FROM ('2026-01-01') TO ('2026-02-01');
CREATE TABLE IF NOT EXISTS emotion_echo_analytics.ube_2026_02 PARTITION OF emotion_echo_analytics.user_behavior_events_partitioned
    FOR VALUES FROM ('2026-02-01') TO ('2026-03-01');
CREATE TABLE IF NOT EXISTS emotion_echo_analytics.ube_2026_03 PARTITION OF emotion_echo_analytics.user_behavior_events_partitioned
    FOR VALUES FROM ('2026-03-01') TO ('2026-04-01');
CREATE TABLE IF NOT EXISTS emotion_echo_analytics.ube_2026_04 PARTITION OF emotion_echo_analytics.user_behavior_events_partitioned
    FOR VALUES FROM ('2026-04-01') TO ('2026-05-01');
CREATE TABLE IF NOT EXISTS emotion_echo_analytics.ube_2026_05 PARTITION OF emotion_echo_analytics.user_behavior_events_partitioned
    FOR VALUES FROM ('2026-05-01') TO ('2026-06-01');
CREATE TABLE IF NOT EXISTS emotion_echo_analytics.ube_2026_06 PARTITION OF emotion_echo_analytics.user_behavior_events_partitioned
    FOR VALUES FROM ('2026-06-01') TO ('2026-07-01');
CREATE TABLE IF NOT EXISTS emotion_echo_analytics.ube_default PARTITION OF emotion_echo_analytics.user_behavior_events_partitioned DEFAULT;

-- 复制已有数据
INSERT INTO emotion_echo_analytics.user_behavior_events_partitioned
SELECT * FROM emotion_echo_analytics.user_behavior_events
ON CONFLICT DO NOTHING;

-- 原子表名交换（事务内）
ALTER TABLE emotion_echo_analytics.user_behavior_events RENAME TO user_behavior_events_legacy_2026_09;
ALTER TABLE emotion_echo_analytics.user_behavior_events_partitioned RENAME TO user_behavior_events;

COMMIT;
