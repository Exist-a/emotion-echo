-- 005_create_daily_emotion_by_modality_v.sql
--
-- Stage 34 PR-15/16: 跨模态日聚合 VIEW（给 analytics-svc 用）。
--
-- 背景：
--   analytics-svc 报表原数据源 emotion_echo_ai.daily_emotion_v 只聚合
--   emotion_analysis（纯文本路径）。Stage 34 增加 face / voice / fused 三路
--   数据源后，需要按模态维度重新聚合。
--
-- 设计：
--   - UNION ALL 三张表（emotion_analysis / face_emotion_results / voice_emotion_results）
--   - 按 user_id × day × primary_emotion × modality 聚合
--   - daily_emotion_v 保留（analytics-svc 老字段 emotionCounts 仍可用，向后兼容）
--   - 本 VIEW 仅补"by modality"维度，前端可选消费
--
-- 注意：
--   - fused_emotions 不在此 VIEW（fused 是收敛结果，不与单模态并列聚合）
--   - sentiment_score 仅文本模态有；face / voice 的 avg_sentiment 为 NULL

CREATE OR REPLACE VIEW emotion_echo_ai.daily_emotion_by_modality_v AS
-- 文本：emotion_analysis（Kafka 异步链路写入）
SELECT
    user_id,
    DATE_TRUNC('day', created_at)::date AS day,
    primary_emotion,
    'text'::text AS modality,
    COUNT(*) AS cnt,
    AVG(sentiment_score) AS avg_sentiment,
    AVG(confidence) AS avg_confidence
FROM emotion_echo_ai.emotion_analysis
GROUP BY user_id, DATE_TRUNC('day', created_at), primary_emotion

UNION ALL

-- 人脸：face_emotion_results（multimodal 端点 persist=true 写入）
SELECT
    user_id,
    DATE_TRUNC('day', created_at)::date,
    primary_emotion,
    'face'::text,
    COUNT(*),
    NULL::float8,
    AVG(confidence)
FROM emotion_echo_ai.face_emotion_results
GROUP BY user_id, DATE_TRUNC('day', created_at), primary_emotion

UNION ALL

-- 语音：voice_emotion_results（multimodal 端点 persist=true 写入）
SELECT
    user_id,
    DATE_TRUNC('day', created_at)::date,
    primary_emotion,
    'voice'::text,
    COUNT(*),
    NULL::float8,
    AVG(confidence)
FROM emotion_echo_ai.voice_emotion_results
GROUP BY user_id, DATE_TRUNC('day', created_at), primary_emotion;