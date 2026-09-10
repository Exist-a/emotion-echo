-- =====================================================
-- Emotion-Echo 微服务拆分 · 在 5 个新 schema 中建表
-- 使用 schema-qualified 表名以避免 search_path 问题
-- =====================================================

-- ===== emotion_echo_user =====
CREATE TABLE IF NOT EXISTS emotion_echo_user.users (
    id BIGSERIAL PRIMARY KEY,
    username VARCHAR(64) UNIQUE NOT NULL,
    phone VARCHAR(20) UNIQUE,
    email VARCHAR(128) UNIQUE,
    password_hash VARCHAR(255),
    nickname VARCHAR(64),
    avatar_url TEXT,
    gender SMALLINT DEFAULT 0,
    birthday DATE,
    status SMALLINT DEFAULT 1,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS emotion_echo_user.refresh_tokens (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    token VARCHAR(255) UNIQUE NOT NULL,
    device_info TEXT,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked BOOLEAN DEFAULT false,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user ON emotion_echo_user.refresh_tokens(user_id);

-- Stage 62 PR-5：emotion_echo_user.user_oauth 表已删（ADR 21，wechat-qq-login-and-upload.md
-- superseded-by Stage 38-A）。历史 DDL 块从本文件移除，避免新人误读 OAuth 已落地。
-- drop table 由 deploy/db/06-drop-user-oauth.sql 单独执行（migrate.sh 会按顺序应用）。

CREATE TABLE IF NOT EXISTS emotion_echo_user.upload_files (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    file_key VARCHAR(255) UNIQUE NOT NULL,
    original_name VARCHAR(255),
    mime_type VARCHAR(64),
    size_bytes BIGINT,
    bucket VARCHAR(64),
    purpose VARCHAR(32),
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_upload_files_user ON emotion_echo_user.upload_files(user_id);

CREATE TABLE IF NOT EXISTS emotion_echo_user.login_attempts (
    id BIGSERIAL PRIMARY KEY,
    identifier VARCHAR(128) NOT NULL,
    ip INET,
    user_agent TEXT,
    success BOOLEAN,
    attempted_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_login_attempts_id_at ON emotion_echo_user.login_attempts(identifier, attempted_at DESC);

-- ===== emotion_echo_chat =====
CREATE TABLE IF NOT EXISTS emotion_echo_chat.conversations (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    title VARCHAR(255),
    context JSONB DEFAULT '{}',
    message_count INT DEFAULT 0,
    last_message_at TIMESTAMPTZ,
    status SMALLINT DEFAULT 1,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    closed_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_conversations_user_at ON emotion_echo_chat.conversations(user_id, last_message_at DESC);

CREATE TABLE IF NOT EXISTS emotion_echo_chat.messages (
    id BIGSERIAL PRIMARY KEY,
    conversation_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    role VARCHAR(16) NOT NULL,
    content TEXT NOT NULL,
    content_type VARCHAR(16) DEFAULT 'text',
    metadata JSONB DEFAULT '{}',
    tokens_used INT DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_messages_conv_time ON emotion_echo_chat.messages(conversation_id, created_at);

-- ===== emotion_echo_ai =====
-- Stage 30-C A1: emotion_analysis 加 event_id 列 + UNIQUE 约束（消费幂等去重）。
-- 完整 migration 见 emotion-echo-ai-svc/migrations/001_add_event_id_to_emotion_analysis.sql。
-- 新环境部署时由该 migration 同步 schema；本集中式 DDL 同步更新避免漂移。
CREATE TABLE IF NOT EXISTS emotion_echo_ai.emotion_analysis (
    id BIGSERIAL PRIMARY KEY,
    event_id VARCHAR(64),
    message_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    conversation_id BIGINT NOT NULL,
    primary_emotion VARCHAR(32),
    emotion_scores JSONB DEFAULT '{}',
    sentiment_score REAL,
    confidence REAL,
    model VARCHAR(64),
    raw_response JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
-- Stage 36-D Bug 2 fix: CREATE UNIQUE INDEX 在 event_id 列不存在时会失败（旧版 cluster 漂移），
-- 让它单独容错，让 03/04 后续 SQL 仍能跑。ai-svc 的 migration 001 会在启动时补 event_id 列。
--
-- 2026-09-04 修正（两处）：
--
-- (a) 上述容错原本**包错了语句**：uq_emotion_analysis_event_id 裸写在 DO 块外，
--     DO 块里包的却是另一条 idx_emotion_user_time——注释描述的保护对象与实际
--     保护对象不一致，真正会失败的那条从未被保护。实测（down -v 后全新起）：
--       02-create-tables-in-schemas.sql:117: ERROR: column "event_id" does not exist
--     该错误中断整条 initdb 链，03-seed-default-users.sql / 04-create-views.sql
--     全部未执行 → users 表为空 → 登录 401。
--
-- (b) 本文件**不再创建** uq_emotion_analysis_event_id。该唯一性由
--     emotion-echo-ai-svc/migrations/001 以 **CONSTRAINT** 形式独占创建
--     （GORM 的 OnConflict Columns:[event_id] 需要匹配约束）。
--     此处若再建一个**同名索引**，会造成同名对象冲突：迁移 001 的守卫查
--     pg_constraint 查不到索引（索引在 pg_class），放行后 ADD CONSTRAINT 报
--       ERROR: relation "uq_emotion_analysis_event_id" already exists
--     迁移随即整体失败。既然 db-migrate 容器已保证迁移必然先于业务服务执行，
--     这里不需要抢先建，交给迁移单一来源即可。
DO $$ BEGIN
    BEGIN
        EXECUTE 'CREATE INDEX IF NOT EXISTS idx_emotion_user_time
            ON emotion_echo_ai.emotion_analysis(user_id, created_at DESC)';
    EXCEPTION WHEN OTHERS THEN
        RAISE NOTICE 'idx_emotion_user_time skipped: %', SQLERRM;
    END;
END $$;

CREATE TABLE IF NOT EXISTS emotion_echo_ai.voice_transcripts (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    upload_file_id BIGINT,
    duration_ms INT,
    transcript TEXT,
    language VARCHAR(16),
    model VARCHAR(64),
    confidence REAL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS emotion_echo_ai.face_detections (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    upload_file_id BIGINT,
    detections JSONB,
    model VARCHAR(64),
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- ===== emotion_echo_assessment =====
CREATE TABLE IF NOT EXISTS emotion_echo_assessment.surveys (
    id BIGSERIAL PRIMARY KEY,
    code VARCHAR(64) UNIQUE NOT NULL,
    title VARCHAR(255) NOT NULL,
    description TEXT,
    category VARCHAR(32),
    questions JSONB NOT NULL,
    scoring_rules JSONB,
    version INT DEFAULT 1,
    status SMALLINT DEFAULT 1,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS emotion_echo_assessment.survey_results (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    survey_id BIGINT NOT NULL,
    answers JSONB NOT NULL,
    total_score REAL,
    factor_scores JSONB DEFAULT '{}',
    risk_level VARCHAR(32),
    duration_seconds INT,
    submitted_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_survey_results_user ON emotion_echo_assessment.survey_results(user_id, submitted_at DESC);

CREATE TABLE IF NOT EXISTS emotion_echo_assessment.mental_health_assessments (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    assessment_type VARCHAR(64),
    period_start DATE,
    period_end DATE,
    overall_score REAL,
    dimensions JSONB DEFAULT '{}',
    summary TEXT,
    recommendations JSONB DEFAULT '[]',
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_mh_user_time ON emotion_echo_assessment.mental_health_assessments(user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS emotion_echo_assessment.reports (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    title VARCHAR(255),
    report_type VARCHAR(64),
    content JSONB DEFAULT '{}',
    file_url TEXT,
    generated_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_reports_user ON emotion_echo_assessment.reports(user_id, generated_at DESC);

-- ===== emotion_echo_analytics =====
CREATE TABLE IF NOT EXISTS emotion_echo_analytics.user_behavior_events (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    event_type VARCHAR(64) NOT NULL,
    target VARCHAR(255),
    properties JSONB DEFAULT '{}',
    session_id VARCHAR(64),
    ip INET,
    user_agent TEXT,
    occurred_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_events_user_time ON emotion_echo_analytics.user_behavior_events(user_id, occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_events_type_time ON emotion_echo_analytics.user_behavior_events(event_type, occurred_at DESC);

-- =====================================================
-- 验证：5 个 schema 各自的表数量
-- =====================================================
SELECT
    schemaname AS schema_name,
    COUNT(*) AS table_count
FROM pg_tables
WHERE schemaname LIKE 'emotion_echo_%'
GROUP BY schemaname
ORDER BY schemaname;