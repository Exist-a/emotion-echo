-- 初始数据库迁移（Up）
-- 创建于: 2026-04-19

-- 用户表
CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    username VARCHAR(64) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    nickname VARCHAR(64) DEFAULT '用户',
    avatar VARCHAR(500) DEFAULT '/imgs/default-avatar.webp',
    age INTEGER CHECK (age >= 0 AND age <= 150),
    wechat_open_id VARCHAR(64),
    wechat_union_id VARCHAR(64),
    config JSONB DEFAULT '{}',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP
);

CREATE INDEX idx_users_username ON users(username);
CREATE INDEX idx_users_wechat_open_id ON users(wechat_open_id);
CREATE INDEX idx_users_deleted_at ON users(deleted_at);

-- 会话表
CREATE TABLE IF NOT EXISTS conversations (
    id VARCHAR(32) PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title VARCHAR(200) NOT NULL,
    is_top BOOLEAN DEFAULT FALSE,
    last_message_content TEXT,
    last_message_time TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_conversations_user_id ON conversations(user_id);
CREATE INDEX idx_conversations_user_updated ON conversations(user_id, updated_at DESC);
CREATE INDEX idx_conversations_pinned ON conversations(user_id, is_top DESC, updated_at DESC);

-- 消息表
CREATE TABLE IF NOT EXISTS messages (
    id VARCHAR(32) PRIMARY KEY,
    conversation_id VARCHAR(32) NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    sender VARCHAR(10) NOT NULL CHECK (sender IN ('user', 'ai')),
    content TEXT,
    content_type VARCHAR(10) DEFAULT 'text' CHECK (content_type IN ('text', 'audio', 'img')),
    emotion_tag VARCHAR(10) CHECK (emotion_tag IN ('sad', 'angry', 'anxious')),
    send_time BIGINT NOT NULL,
    created_at BIGINT NOT NULL DEFAULT EXTRACT(EPOCH FROM CURRENT_TIMESTAMP)::BIGINT
);

CREATE INDEX idx_messages_conversation_id ON messages(conversation_id);
CREATE INDEX idx_messages_conversation_time ON messages(conversation_id, send_time DESC);

-- 刷新令牌表
CREATE TABLE IF NOT EXISTS refresh_tokens (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash VARCHAR(64) UNIQUE NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_refresh_tokens_hash ON refresh_tokens(token_hash);
CREATE INDEX idx_refresh_tokens_user_id ON refresh_tokens(user_id);

-- 心理测验表
CREATE TABLE IF NOT EXISTS surveys (
    id SERIAL PRIMARY KEY,
    title VARCHAR(200) NOT NULL,
    description TEXT,
    estimated_time VARCHAR(50),
    questions JSONB NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 测验记录表
CREATE TABLE IF NOT EXISTS survey_results (
    id VARCHAR(32) PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    survey_id INTEGER NOT NULL REFERENCES surveys(id),
    answers JSONB NOT NULL,
    total_score INTEGER,
    level VARCHAR(50),
    suggestion TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_survey_results_user_id ON survey_results(user_id);

-- 情绪分析表
CREATE TABLE IF NOT EXISTS emotion_analyses (
    id BIGSERIAL PRIMARY KEY,
    conversation_id VARCHAR(32) NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    analyzed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    emotion_scores JSONB,
    summary TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_emotion_analyses_conversation ON emotion_analyses(conversation_id);
CREATE INDEX idx_emotion_analyses_user_time ON emotion_analyses(user_id, analyzed_at);
