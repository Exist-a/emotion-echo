-- migrations/005_create_assessment_jobs.sql
--
-- Stage 30-A SQL 落地: assessment_jobs 表（analytics 自有 schema）。
--
-- 背景：POST /api/v1/mental-health/trigger 的 TriggerQueue worker 需要
-- 持久化异步评估任务的执行结果。analytics-svc 对跨 schema 只读（只读 role），
-- 唯一可写表在本 schema —— assessment_jobs 记录任务状态机
-- running → done(result JSONB) | failed(error)。

CREATE TABLE IF NOT EXISTS emotion_echo_analytics.assessment_jobs (
    id              BIGSERIAL PRIMARY KEY,
    task_id         TEXT UNIQUE NOT NULL,          -- uuid（与 trigger.Response.TaskID 对应）
    user_id         BIGINT NOT NULL,
    assessment_type VARCHAR(64) NOT NULL,          -- daily | weekly | comprehensive
    status          VARCHAR(16) NOT NULL DEFAULT 'running',  -- running | done | failed
    result          JSONB,                          -- MentalAssessment 序列化（done）
    error           TEXT,                           -- 失败原因（failed）
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at    TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_assessment_jobs_user
    ON emotion_echo_analytics.assessment_jobs(user_id, created_at DESC);
