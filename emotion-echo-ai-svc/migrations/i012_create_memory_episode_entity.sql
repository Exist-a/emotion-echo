-- migrations/i012_create_memory_episode_entity.sql
--
-- CL-08c（记忆 · 情节 + 图记忆检索）Phase 1：情节与实体表。
--
-- 设计依据（ADR adr-2026-10-agent-memory-graph.md · 决策 45 / 决策 8）：
--   - PG 为**系统记录**，图（falkordblite，Phase 4）是可重建索引；
--     episode 是"从 PG 重建图"的重放原料，实体注册表是图节点在 PG 的镜像。
--   - memory_fact / memory_note / memory_extract_run 已由 CL-08a 落地（i011）；
--     本迁移只补情节与实体两块，不重复建表。
--   - 双时间轴：valid_at（事件发生时间）+ created_at（摄入时间）——
--     Graphiti 双时间轴的 PG 侧承载（与 fact 的 valid_from/valid_to 同族语义）。
--   - 软删除：status='deleted' + deleted_at（删除闭环归 Phase 5 / CL-08d，
--     以 PG 为准、图随删）。episode/entity 无 superseded —— 情节不可变、实体只刷新摘要。
--   - 幂等：UNIQUE(source_kind, source_id) —— 一条来源至多一个情节
--           （与 memory_extract_run 的复合主键同键，Phase 2 抽取 worker 直接复用）；
--           UNIQUE(user_id, name) —— 同一用户内同名实体只一行（消解锚点；
--           软删后再次抽取由仓储层"复活"该行，不新建）。
--
-- 归属：emotion_echo_ai（ai-svc 拥有；迁移守卫禁止跨 schema）。
-- 可重跑：全部 IF NOT EXISTS（migrate.sh 另有 schema_migrations checksum 追踪）。

BEGIN;

-- 情节：对来源（message / note）的一次可重建存档
CREATE TABLE IF NOT EXISTS emotion_echo_ai.memory_episode (
    id              BIGSERIAL    PRIMARY KEY,
    user_id         BIGINT       NOT NULL,
    conversation_id BIGINT       NOT NULL,
    source_kind     VARCHAR(16)  NOT NULL,
    source_id       BIGINT       NOT NULL,
    name            TEXT         NOT NULL,
    content         TEXT         NOT NULL,
    valid_at        TIMESTAMPTZ  NOT NULL,
    status          VARCHAR(16)  NOT NULL DEFAULT 'active',
    deleted_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_memory_episode_status
        CHECK (status IN ('active', 'deleted')),
    CONSTRAINT chk_memory_episode_source_kind
        CHECK (source_kind IN ('message', 'note'))
);

-- 幂等：一条来源至多一个情节（at-least-once 投递下不重复）
CREATE UNIQUE INDEX IF NOT EXISTS uq_memory_episode_source
    ON emotion_echo_ai.memory_episode (source_kind, source_id);

-- 用户隔离查询路径（管理页列表 / 删除）
CREATE INDEX IF NOT EXISTS idx_memory_episode_user_status
    ON emotion_echo_ai.memory_episode (user_id, status);

-- 会话维度查询路径（会话收尾 / 会话级删除）
CREATE INDEX IF NOT EXISTS idx_memory_episode_conversation
    ON emotion_echo_ai.memory_episode (conversation_id);

-- 实体注册表：图节点在 PG 的镜像（管理页读取 / 图重建依据）
CREATE TABLE IF NOT EXISTS emotion_echo_ai.memory_entity (
    id         BIGSERIAL    PRIMARY KEY,
    user_id    BIGINT       NOT NULL,
    name       TEXT         NOT NULL,
    summary    TEXT         NOT NULL DEFAULT '',
    status     VARCHAR(16)  NOT NULL DEFAULT 'active',
    deleted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_memory_entity_status
        CHECK (status IN ('active', 'deleted'))
);

-- 实体唯一锚点：同一用户内同名实体只一行（"我妈"/"母亲"的合并由 LLM 消解负责）
CREATE UNIQUE INDEX IF NOT EXISTS uq_memory_entity_user_name
    ON emotion_echo_ai.memory_entity (user_id, name);

-- 用户隔离查询路径
CREATE INDEX IF NOT EXISTS idx_memory_entity_user_status
    ON emotion_echo_ai.memory_entity (user_id, status);

COMMIT;
