-- =====================================================
--  E2E-06: 新建密保问题表（供 D-01=C 找回密码）
--  路径：emotion-echo-user-svc/migrations/u002_create_security_answers.sql
--
--  设计：
--  - 支持 1~2 个密保问题（question_order = 1 或 2）
--  - 答案用 bcrypt 哈希（与 password_hash 同套）
--  - 联合主键 (user_id, question_order) 保证唯一
--
--  幂等：IF NOT EXISTS 保证可重复执行
-- =====================================================

CREATE TABLE IF NOT EXISTS emotion_echo_user.user_security_answers (
    user_id BIGINT NOT NULL,
    question_order SMALLINT NOT NULL CHECK (question_order IN (1, 2)),
    question VARCHAR(255) NOT NULL,
    answer_hash VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (user_id, question_order),
    FOREIGN KEY (user_id) REFERENCES emotion_echo_user.users(id) ON DELETE CASCADE
);

-- 索引：按 user_id 查询密保问题（找回密码场景）
CREATE INDEX IF NOT EXISTS idx_security_answers_user
    ON emotion_echo_user.user_security_answers(user_id);

COMMENT ON TABLE emotion_echo_user.user_security_answers IS '用户密保问题（1~2 个），用于找回密码';
COMMENT ON COLUMN emotion_echo_user.user_security_answers.question_order IS '问题序号（1 或 2）';
COMMENT ON COLUMN emotion_echo_user.user_security_answers.question IS '密保问题文本';
COMMENT ON COLUMN emotion_echo_user.user_security_answers.answer_hash IS '答案的 bcrypt 哈希';