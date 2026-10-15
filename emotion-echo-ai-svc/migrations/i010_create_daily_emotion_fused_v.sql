-- migrations/i010_create_daily_emotion_fused_v.sql
--
-- CL-07（后端切片）Phase 1：融合情绪日聚合视图。
--
-- 背景（fusion-result-consumption.md §0.2 结论）：
--   fused_emotions（三模态收敛后的**综合情绪**）此前**只写不读** ——
--   唯一消费点是一个按消息查的端点，前端零调用；按模态视图
--   （i005 daily_emotion_by_modality_v）**注释明确排除**了它
--   （"fused 是收敛结果，不与单模态并列聚合"）。⇒ 最有价值的产物无人消费。
--
-- 设计：
--   - 按 user_id × day × primary_emotion × fusion_method 聚合
--   - fusion_method 维度是核心：能算出"多少比例真 LLM 融合 / 多少降级加权"
--   - face_ratio / voice_ratio：反映"多少消息真带上了面部/语音模态"
--   - 走现成索引 idx_fused_user_time(user_id, created_at DESC)
--
-- 关键纪律（与 i005 同）：
--   - **只读 fused_emotions**，不与单模态表 UNION（fused 是收敛结果，并列会双计）
--   - **必须过滤软删除**（i007 给 fused_emotions 加了 deleted_at）
--   - available_modalities 是 JSONB 字符串数组 ⇒ `?` 操作符前显式 ::jsonb
--
-- 权限：本视图由 analytics_reader 消费 ⇒ 授权见
--   emotion-echo-analytics-svc/migrations/a011_grant_daily_emotion_fused_v.sql
--   （不能写在 a004 里：a004 已应用，改它会让 migrate.sh 报 checksum 不一致而 die）
--
-- 幂等：CREATE OR REPLACE VIEW。

BEGIN;

CREATE OR REPLACE VIEW emotion_echo_ai.daily_emotion_fused_v AS
SELECT
    user_id,
    DATE_TRUNC('day', created_at)::date              AS day,
    primary_emotion,
    COALESCE(fusion_method, '')                      AS fusion_method,
    COUNT(*)                                          AS cnt,
    AVG(sentiment_score)                              AS avg_sentiment,
    AVG(confidence)                                   AS avg_confidence,
    -- available_modalities 为 JSONB 数组（如 ["text","face"]）；
    -- jsonb `?` 对数组元素与顶层 key 均可判定。
    AVG(CASE WHEN available_modalities::jsonb ? 'face'  THEN 1 ELSE 0 END) AS face_ratio,
    AVG(CASE WHEN available_modalities::jsonb ? 'voice' THEN 1 ELSE 0 END) AS voice_ratio
FROM emotion_echo_ai.fused_emotions
WHERE deleted_at IS NULL
GROUP BY user_id, DATE_TRUNC('day', created_at), primary_emotion, COALESCE(fusion_method, '');

COMMIT;
