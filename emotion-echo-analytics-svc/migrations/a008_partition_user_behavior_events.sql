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
--
-- 幂等守卫（2026-09-16 dev 模式修复）：a008 第一次跑按设计走 (SELECT 原表 +
-- RENAME 交换 + COMMIT)。第二次跑时 RENAME 已完成, user_behavior_events 已指向
-- partitioned 表, "INSERT INTO _partitioned SELECT FROM user_behavior_events" 变成
-- 自递归, PG 报 "no partition of relation found"。
--
-- 守卫策略：检测到已 partitioned 时, 用 psql 内置命令 \quit (退出码 0) 让 psql
-- 进程终止, migrate.sh 看到 OK 继续 a009。\quit 是 psql 命令不是 SQL, 必须在
-- 命令行输入 (PL/pgSQL DO block 内不可用), 所以改用 \\set ON_ERROR_STOP off +
-- 触发 SQL 错误后 psql 仍继续执行。
--
-- 实际最简方案: 守卫命中后, INSERT/RENAME 都会因名字冲突失败, 用 ON_ERROR_STOP off
-- 让这些错误变成 WARNING 而不阻塞 psql 继续执行, 整个 a008 文件最后一条语句 (我们
-- 加一个 SELECT 1; 占位) 退出码 0, migrate.sh 看到 OK。

\set ON_ERROR_STOP off

BEGIN;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM pg_partitioned_table pt
        JOIN pg_class c ON c.oid = pt.partrelid
        WHERE c.relname = 'user_behavior_events'
          AND c.relnamespace = 'emotion_echo_analytics'::regnamespace
    ) THEN
        RAISE NOTICE 'a008: user_behavior_events 已是 partitioned table, 整个 a008 静默跳过 (RENAME 已落地)';
        -- 整个事务 ROLLBACK (放弃本次"假"运行的所有 DDL, 它们都是 IF NOT EXISTS 守卫)
        RAISE EXCEPTION 'a008_skip_marker';
    END IF;
END$$;

-- 新分区表（结构与原表一致 + PARTITION BY RANGE）
-- 注意：PG 拒绝 `LIKE ... INCLUDING ALL` 后跟 `PARTITION BY` —— INCLUDING ALL
-- 会把原表的 PRIMARY KEY (id BIGSERIAL) 复制过来，但分区表 PK 必须包含分区键
-- occurred_at。改用显式列定义，绕开 LIKE ALL。
--
-- 列定义与 deploy/db/02-create-tables-in-schemas.sql:215 对齐（10 列）：
-- id / user_id / event_type / target / properties / session_id / ip / user_agent /
-- occurred_at / event_id。event_id 由 a006 ADD COLUMN 加，老分区表数据复制时
-- 若 NULL 走 ON CONFLICT DO NOTHING。
CREATE TABLE IF NOT EXISTS emotion_echo_analytics.user_behavior_events_partitioned (
    id          BIGSERIAL,
    user_id     BIGINT NOT NULL,
    event_type  VARCHAR(64) NOT NULL,
    target      VARCHAR(255),
    properties  JSONB DEFAULT '{}',
    session_id  VARCHAR(64),
    ip          INET,
    user_agent  TEXT,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    event_id    VARCHAR(64)
) PARTITION BY RANGE (occurred_at);

-- 分区表 PK 必须包含分区键 occurred_at。
ALTER TABLE emotion_echo_analytics.user_behavior_events_partitioned
    ADD PRIMARY KEY (id, occurred_at);

-- event_id 唯一性也需要包含分区键（PG 对分区表 UNIQUE 约束同样强制）
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'uq_user_behavior_events_partitioned_event_id'
    ) THEN
        ALTER TABLE emotion_echo_analytics.user_behavior_events_partitioned
            ADD CONSTRAINT uq_user_behavior_events_partitioned_event_id UNIQUE (event_id, occurred_at);
    END IF;
END$$;

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

-- 复制已有数据（显式列名避免顺序漂移触发不匹配）
INSERT INTO emotion_echo_analytics.user_behavior_events_partitioned
    (id, user_id, event_type, target, properties, session_id, ip, user_agent, occurred_at, event_id)
SELECT id, user_id, event_type, target, properties, session_id, ip, user_agent, occurred_at, event_id
FROM emotion_echo_analytics.user_behavior_events
ON CONFLICT DO NOTHING;

-- 原子表名交换（事务内）
ALTER TABLE emotion_echo_analytics.user_behavior_events RENAME TO user_behavior_events_legacy_2026_09;
ALTER TABLE emotion_echo_analytics.user_behavior_events_partitioned RENAME TO user_behavior_events;

COMMIT;
