-- migrations/i011_create_memory_facts.sql
--
-- CL-08a（记忆 · 用户事实切片）A1：跨会话记忆的 PG 系统记录。
--
-- 设计依据（ADR adr-2026-10-agent-memory-graph.md · 决策 45）：
--   - PG 为**系统记录**，图（CL-08c）是可重建索引 ⇒ 事实先落这里。
--   - 双时间轴：valid_from / valid_to；旧值**不删除**，只关区间（T-3 / T-15）。
--   - 软删除：status='deleted' + deleted_at（用户裁定 D3「PG 软删 + 图物理删」）。
--   - 两个来源（用户裁定 D6）：正常会话消息（message）/「我的记忆」自述条（note）。
--   - 幂等：UNIQUE(source_kind, source_id, subject, predicate, object) 防同一来源重复落库；
--           partial UNIQUE(user_id, subject, predicate) WHERE status='active'
--           防同一键出现两条 active（并发兜底；planner 已保证顺序）。
--
-- 归属：emotion_echo_ai（ai-svc 拥有；迁移守卫禁止跨 schema）。
-- 可重跑：全部 IF NOT EXISTS（migrate.sh 另有 schema_migrations checksum 追踪）。

BEGIN;

CREATE TABLE IF NOT EXISTS emotion_echo_ai.memory_fact (
    id                 BIGSERIAL PRIMARY KEY,
    user_id            BIGINT       NOT NULL,
    conversation_id    BIGINT       NOT NULL,
    source_kind        VARCHAR(16)  NOT NULL,
    source_id          BIGINT       NOT NULL,
    subject            TEXT         NOT NULL,
    predicate          TEXT         NOT NULL,
    object             TEXT         NOT NULL,
    fact_text          TEXT         NOT NULL,
    confidence         REAL,
    valid_from         TIMESTAMPTZ  NOT NULL,
    valid_to           TIMESTAMPTZ,
    status             VARCHAR(16)  NOT NULL DEFAULT 'active',
    deleted_at         TIMESTAMPTZ,
    created_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_memory_fact_status
        CHECK (status IN ('active', 'superseded', 'deleted')),
    CONSTRAINT chk_memory_fact_source_kind
        CHECK (source_kind IN ('message', 'note'))
);

-- 幂等：同一条来源重复消费不得重复落库
CREATE UNIQUE INDEX IF NOT EXISTS uq_memory_fact_source_triple
    ON emotion_echo_ai.memory_fact (source_kind, source_id, subject, predicate, object);

-- 并发兜底：同一 (user, subject, predicate) 至多一条 active
CREATE UNIQUE INDEX IF NOT EXISTS uq_memory_fact_active_key
    ON emotion_echo_ai.memory_fact (user_id, subject, predicate)
    WHERE status = 'active' AND deleted_at IS NULL;

-- 注入/摘要主路径：按用户取 active，按时间倒序
CREATE INDEX IF NOT EXISTS idx_memory_fact_user_active
    ON emotion_echo_ai.memory_fact (user_id, status, valid_to);

-- 矛盾判定路径：找同 subject+predicate 的 active 行
CREATE INDEX IF NOT EXISTS idx_memory_fact_user_key
    ON emotion_echo_ai.memory_fact (user_id, subject, predicate, valid_to);

-- 「我的记忆」自述条（独立于 conversations/messages）
CREATE TABLE IF NOT EXISTS emotion_echo_ai.memory_note (
    id             BIGSERIAL PRIMARY KEY,
    user_id        BIGINT       NOT NULL,
    content        TEXT         NOT NULL,
    extract_status VARCHAR(16)  NOT NULL DEFAULT 'pending',
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at     TIMESTAMPTZ,
    CONSTRAINT chk_memory_note_extract_status
        CHECK (extract_status IN ('pending', 'done', 'failed'))
);

-- 页面列表：按用户取最新
CREATE INDEX IF NOT EXISTS idx_memory_note_user_created
    ON emotion_echo_ai.memory_note (user_id, created_at DESC);

-- 抽取审计 + 幂等登记（复合主键 = 来源身份）
CREATE TABLE IF NOT EXISTS emotion_echo_ai.memory_extract_run (
    source_kind     VARCHAR(16)  NOT NULL,
    source_id       BIGINT       NOT NULL,
    user_id         BIGINT       NOT NULL,
    conversation_id BIGINT       NOT NULL,
    status          VARCHAR(16)  NOT NULL DEFAULT 'pending',
    facts_count     INTEGER      NOT NULL DEFAULT 0,
    error           TEXT         NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    PRIMARY KEY (source_kind, source_id),
    CONSTRAINT chk_memory_extract_status
        CHECK (status IN ('pending', 'done', 'failed'))
);

-- 会话收尾合并：按会话取已抽取的运行记录
CREATE INDEX IF NOT EXISTS idx_memory_extract_run_conversation
    ON emotion_echo_ai.memory_extract_run (conversation_id);

-- 启动补抽：按状态扫未完成/失败的自述条
CREATE INDEX IF NOT EXISTS idx_memory_extract_run_status
    ON emotion_echo_ai.memory_extract_run (status);

COMMIT;
