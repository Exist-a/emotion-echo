-- migrations/007_assessment_v_user_id_index.sql
--
-- P1-R2-12: mentalhealth_repository.ListAssessmentHistory 走 cursor 分页
-- WHERE user_id = $1 AND id < $3 ORDER BY id DESC。
-- 当前 assessment_v 没有 (user_id, id) 复合索引 → 全表扫描 + 排序，
-- 上线后随用户量增长会慢到秒级，接口 504。
--
-- 修复：建 (user_id, id DESC) 复合 B-tree 索引。
-- 注意：assessment_v 是 view（UNION ALL），所以索引建在底层表上：
--   emotion_echo_assessment.survey_results
--   emotion_echo_assessment.mental_health_assessments
-- ListAssessmentHistory 实际查的是 mental_health_assessments。
--
-- 适用范围：emotion_echo_assessment schema

BEGIN;

-- P1-R2-12: cursor 分页 (user_id, id DESC) 复合索引
CREATE INDEX IF NOT EXISTS idx_mha_user_id_desc
    ON emotion_echo_assessment.mental_health_assessments(user_id, id DESC);

-- 同理 survey_results 也加（虽然 ListAssessmentHistory 不直接查，但 future-proof）
CREATE INDEX IF NOT EXISTS idx_sr_user_id_desc
    ON emotion_echo_assessment.survey_results(user_id, id DESC);

COMMIT;
