-- =====================================================
-- Emotion-Echo 微服务拆分 · 数据库 schema 分离
-- =====================================================
-- 原则：每张表归属唯一一个业务域 svc
-- 跨域查询 → 通过 RPC（每个 svc 暴露自己的查询接口），禁止跨库 JOIN
-- =====================================================

-- 1. 创建 5 个业务 schema
CREATE SCHEMA IF NOT EXISTS emotion_echo_user;       -- user-svc 拥有
CREATE SCHEMA IF NOT EXISTS emotion_echo_chat;       -- chat-svc 拥有
CREATE SCHEMA IF NOT EXISTS emotion_echo_ai;         -- ai-svc 拥有
CREATE SCHEMA IF NOT EXISTS emotion_echo_assessment; -- assessment-svc 拥有
CREATE SCHEMA IF NOT EXISTS emotion_echo_analytics;  -- analytics-svc 拥有

-- =====================================================
-- 2. emotion_echo_user（user-svc）
-- =====================================================
-- 用户主表、token、OAuth、上传文件元数据、登录尝试日志
SET search_path TO emotion_echo_user;

CREATE TABLE IF NOT EXISTS users (
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

CREATE TABLE IF NOT EXISTS refresh_tokens (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id),
    token VARCHAR(255) UNIQUE NOT NULL,
    device_info TEXT,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked BOOLEAN DEFAULT false,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user ON refresh_tokens(user_id);

CREATE TABLE IF NOT EXISTS user_oauth (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id),
    provider VARCHAR(32) NOT NULL,  -- wechat / apple / qq
    open_id VARCHAR(128) NOT NULL,
    union_id VARCHAR(128),
    access_token TEXT,
    refresh_token TEXT,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(provider, open_id)
);

CREATE TABLE IF NOT EXISTS upload_files (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id),
    file_key VARCHAR(255) UNIQUE NOT NULL,
    original_name VARCHAR(255),
    mime_type VARCHAR(64),
    size_bytes BIGINT,
    bucket VARCHAR(64),
    purpose VARCHAR(32),  -- avatar / voice / image
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_upload_files_user ON upload_files(user_id);

CREATE TABLE IF NOT EXISTS login_attempts (
    id BIGSERIAL PRIMARY KEY,
    identifier VARCHAR(128) NOT NULL,
    ip INET,
    user_agent TEXT,
    success BOOLEAN,
    attempted_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_login_attempts_id_at ON login_attempts(identifier, attempted_at DESC);

-- =====================================================
-- 3. emotion_echo_chat（chat-svc）
-- =====================================================
SET search_path TO emotion_echo_chat;

CREATE TABLE IF NOT EXISTS conversations (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,           -- 不加外键（user-svc 拥有）
    title VARCHAR(255),
    context JSONB DEFAULT '{}',
    message_count INT DEFAULT 0,
    last_message_at TIMESTAMPTZ,
    status SMALLINT DEFAULT 1,         -- 1 open / 2 closed
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    closed_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_conversations_user_at ON conversations(user_id, last_message_at DESC);

CREATE TABLE IF NOT EXISTS messages (
    id BIGSERIAL PRIMARY KEY,
    conversation_id BIGINT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL,
    role VARCHAR(16) NOT NULL,         -- user / assistant / system
    content TEXT NOT NULL,
    content_type VARCHAR(16) DEFAULT 'text', -- text / image / voice
    metadata JSONB DEFAULT '{}',
    tokens_used INT DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_messages_conv_time ON messages(conversation_id, created_at);

-- =====================================================
-- 4. emotion_echo_ai（ai-svc）
-- =====================================================
SET search_path TO emotion_echo_ai;

CREATE TABLE IF NOT EXISTS emotion_analysis (
    id BIGSERIAL PRIMARY KEY,
    message_id BIGINT NOT NULL,        -- 来自 chat-svc.messages
    user_id BIGINT NOT NULL,
    conversation_id BIGINT NOT NULL,
    primary_emotion VARCHAR(32),
    emotion_scores JSONB DEFAULT '{}', -- {"happy": 0.8, "sad": 0.1}
    sentiment_score REAL,
    confidence REAL,
    model VARCHAR(64),
    raw_response JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_emotion_user_time ON emotion_analysis(user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS voice_transcripts (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    upload_file_id BIGINT,             -- 来自 user-svc.upload_files
    duration_ms INT,
    transcript TEXT,
    language VARCHAR(16),
    model VARCHAR(64),
    confidence REAL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS face_detections (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    upload_file_id BIGINT,
    detections JSONB,                  -- [{box, emotion, ...}]
    model VARCHAR(64),
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- =====================================================
-- 5. emotion_echo_assessment（assessment-svc）
-- =====================================================
SET search_path TO emotion_echo_assessment;

CREATE TABLE IF NOT EXISTS surveys (
    id BIGSERIAL PRIMARY KEY,
    code VARCHAR(64) UNIQUE NOT NULL,  -- SCL-90 / SAS / SDS / PHQ-9
    title VARCHAR(255) NOT NULL,
    description TEXT,
    category VARCHAR(32),              -- anxiety / depression / personality
    questions JSONB NOT NULL,          -- [{id, text, type, options}]
    scoring_rules JSONB,
    version INT DEFAULT 1,
    status SMALLINT DEFAULT 1,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS survey_results (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    survey_id BIGINT NOT NULL REFERENCES surveys(id),
    answers JSONB NOT NULL,            -- {q1: 3, q2: 1, ...}
    total_score REAL,
    factor_scores JSONB DEFAULT '{}',  -- {somatization: 1.5, ...}
    risk_level VARCHAR(32),            -- low / moderate / high
    duration_seconds INT,
    submitted_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_survey_results_user ON survey_results(user_id, submitted_at DESC);

CREATE TABLE IF NOT EXISTS mental_health_assessments (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    assessment_type VARCHAR(64),        -- comprehensive / weekly / monthly
    period_start DATE,
    period_end DATE,
    overall_score REAL,
    dimensions JSONB DEFAULT '{}',     -- {emotion, sleep, social, ...}
    summary TEXT,
    recommendations JSONB DEFAULT '[]',
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_mh_user_time ON mental_health_assessments(user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS reports (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    title VARCHAR(255),
    report_type VARCHAR(64),
    content JSONB DEFAULT '{}',
    file_url TEXT,
    generated_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_reports_user ON reports(user_id, generated_at DESC);

-- =====================================================
-- 6. emotion_echo_analytics（analytics-svc）
-- =====================================================
SET search_path TO emotion_echo_analytics;

CREATE TABLE IF NOT EXISTS user_behavior_events (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    event_type VARCHAR(64) NOT NULL,    -- page_view / click / feature_use
    target VARCHAR(255),                -- 路由 / 功能名 / 按钮 id
    properties JSONB DEFAULT '{}',
    session_id VARCHAR(64),
    ip INET,
    user_agent TEXT,
    occurred_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_events_user_time ON user_behavior_events(user_id, occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_events_type_time ON user_behavior_events(event_type, occurred_at DESC);

-- =====================================================
-- 验证
-- =====================================================
SELECT schema_name FROM information_schema.schemata
WHERE schema_name LIKE 'emotion_echo_%'
ORDER BY schema_name;