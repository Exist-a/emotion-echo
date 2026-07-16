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

CREATE TABLE IF NOT EXISTS emotion_echo_user.user_oauth (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    provider VARCHAR(32) NOT NULL,
    open_id VARCHAR(128) NOT NULL,
    union_id VARCHAR(128),
    access_token TEXT,
    refresh_token TEXT,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(provider, open_id)
);

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
CREATE TABLE IF NOT EXISTS emotion_echo_ai.emotion_analysis (
    id BIGSERIAL PRIMARY KEY,
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
CREATE INDEX IF NOT EXISTS idx_emotion_user_time ON emotion_echo_ai.emotion_analysis(user_id, created_at DESC);

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