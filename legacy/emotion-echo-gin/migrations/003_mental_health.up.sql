-- 心理健康评估系统迁移（MVP v1.0）
-- 创建于: 2026-04-26

-- 心理健康评估表
CREATE TABLE IF NOT EXISTS mental_health_assessments (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    
    -- 评估类型和周期
    assessment_type VARCHAR(20) NOT NULL CHECK (assessment_type IN ('daily', 'weekly', 'comprehensive')),
    period_start TIMESTAMP NOT NULL,
    period_end TIMESTAMP NOT NULL,
    
    -- 六维评分 (0-100，越高越健康)
    emotion_score INT CHECK (emotion_score BETWEEN 0 AND 100),
    depression_score INT CHECK (depression_score BETWEEN 0 AND 100),
    anxiety_score INT CHECK (anxiety_score BETWEEN 0 AND 100),
    stress_score INT CHECK (stress_score BETWEEN 0 AND 100),
    sleep_score INT CHECK (sleep_score BETWEEN 0 AND 100),
    social_score INT CHECK (social_score BETWEEN 0 AND 100),
    
    -- 综合评估
    overall_score INT CHECK (overall_score BETWEEN 0 AND 100),
    risk_level VARCHAR(20) NOT NULL CHECK (risk_level IN ('low', 'medium', 'high', 'critical')),
    risk_factors JSONB DEFAULT '[]',
    warning_flags JSONB DEFAULT '[]',
    
    -- 报告内容
    summary TEXT,
    suggestions JSONB DEFAULT '[]',
    
    -- 关联数据
    emotion_analysis_ids JSONB DEFAULT '[]',
    survey_result_ids JSONB DEFAULT '[]',
    
    -- 元数据
    is_notified BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    
    CONSTRAINT valid_period CHECK (period_end > period_start)
);

-- 索引
CREATE INDEX IF NOT EXISTS idx_mha_user_id ON mental_health_assessments(user_id);
CREATE INDEX IF NOT EXISTS idx_mha_user_type_created ON mental_health_assessments(user_id, assessment_type, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_mha_risk_level ON mental_health_assessments(risk_level) WHERE risk_level IN ('high', 'critical');
CREATE INDEX IF NOT EXISTS idx_mha_period ON mental_health_assessments(user_id, period_start, period_end);
