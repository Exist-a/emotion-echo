-- migrations/a010_fix_partition_id_sequence.sql
--
-- E2E-F-175：a008 换表未推进 BIGSERIAL 序列 ⇒ 新行 id 与老行 id 空间重叠
--
-- 背景（2026-10-01 dev 库实测）：a008 建分区表 user_behavior_events_partitioned
-- 时 BIGSERIAL 序列从 0 起，INSERT...SELECT 显式带 id 复制老数据但不推进序列。
-- 换表后新写入拿 id=1,2,3…，而老行 id 已到 162 ⇒ id 失去唯一性（复合 PK
-- (id, occurred_at) 挡住硬冲突），下游 a009 的 id 游标分页与 max(id) 类
-- 查询结果错乱。
--
-- 修法：把序列推到 ≥ 当前 max(id)。GREATEST 兜底空表/新库（max=NULL→1）。
-- 幂等：setval 可重复执行；对已正确的库重复跑无副作用。
--
-- 适用范围：emotion_echo_analytics schema

SELECT setval(
    'emotion_echo_analytics.user_behavior_events_partitioned_id_seq',
    GREATEST(
        (SELECT COALESCE(MAX(id), 0) FROM emotion_echo_analytics.user_behavior_events),
        1
    ),
    true
);
