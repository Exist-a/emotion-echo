-- migrations/a009_cursor_pagination_indexes.sql
--
-- P2-R2-9: analytics-svc raw SQL 游标分页加 (user_id, id) 复合索引
--
-- 背景：mentalhealth_repository 之外还有 event_repository / msg_repository
-- 走游标分页（WHERE user_id = $1 AND id < $3 ORDER BY id DESC LIMIT $4），
-- 但底层表没 (user_id, id DESC) 索引 → 全表扫描 + sort。
--
-- 适用范围：emotion_echo_analytics + emotion_echo_chat + emotion_echo_ai schemas

BEGIN;

-- analytics 自己的表
CREATE INDEX IF NOT EXISTS idx_ube_user_id_desc
    ON emotion_echo_analytics.user_behavior_events(user_id, id DESC);

-- ai-svc 表（analytics 也会查）
CREATE INDEX IF NOT EXISTS idx_ea_user_id_desc
    ON emotion_echo_ai.emotion_analysis(user_id, id DESC);
CREATE INDEX IF NOT EXISTS idx_fused_user_id_desc
    ON emotion_echo_ai.fused_emotions(user_id, id DESC);
CREATE INDEX IF NOT EXISTS idx_face_user_id_desc
    ON emotion_echo_ai.face_emotion_results(user_id, id DESC);
CREATE INDEX IF NOT EXISTS idx_voice_user_id_desc
    ON emotion_echo_ai.voice_emotion_results(user_id, id DESC);

-- chat 表（msg_repository 游标分页）
CREATE INDEX IF NOT EXISTS idx_messages_user_id_desc
    ON emotion_echo_chat.messages(user_id, id DESC);

COMMIT;
