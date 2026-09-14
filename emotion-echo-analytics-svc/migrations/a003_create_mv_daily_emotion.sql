-- migrations/003_create_mv_daily_emotion.sql
--
-- Stage 30-A §六.6.3: materialized view for daily emotion aggregation.
--
-- mv_daily_emotion 聚合 user_id × day × primary_emotion，
-- 加速 reports/daily endpoint 的 emotion counts 查询。
--
-- v1 阶段：main.go 启动时 REFRESH MATERIALIZED VIEW（无需 pg_cron）。
-- v2 阶段（Stage-2 trigger 满足时）：pg_cron '*/15 * * * *' REFRESH CONCURRENTLY。

CREATE MATERIALIZED VIEW IF NOT EXISTS emotion_echo_analytics.mv_daily_emotion AS
SELECT
    user_id,
    DATE_TRUNC('day', created_at)::date AS day,
    primary_emotion,
    COUNT(*)                            AS cnt,
    AVG(sentiment_score)                AS avg_sentiment,
    AVG(confidence)                     AS avg_confidence
FROM emotion_echo_ai.daily_emotion_v
WHERE created_at > NOW() - INTERVAL '90 days'
GROUP BY user_id, DATE_TRUNC('day', created_at), primary_emotion;

-- 唯一索引：支持 REFRESH MATERIALIZED VIEW CONCURRENTLY（v2 需要）
CREATE UNIQUE INDEX IF NOT EXISTS mv_daily_emotion_user_day_emotion_idx
    ON emotion_echo_analytics.mv_daily_emotion(user_id, day, primary_emotion);

-- 加速按 user + 时间窗口查询
CREATE INDEX IF NOT EXISTS mv_daily_emotion_user_day_idx
    ON emotion_echo_analytics.mv_daily_emotion(user_id, day DESC);