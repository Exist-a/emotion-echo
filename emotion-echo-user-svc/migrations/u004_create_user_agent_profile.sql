-- =====================================================
--  CL-06（用户自定义 Agent·多角色）：角色档案表
--  路径：emotion-echo-user-svc/migrations/u004_create_user_agent_profile.sql
--
--  设计（见 docs/plans/user-custom-agent-persona.md §3.1 / CL-D-21）：
--  - 每个用户最多 5 个角色（含默认「小E」）——**上限在服务层强制**（本表不设 CHECK，
--    因为软删除后计数会变化，用 CHECK 会造成"删了也加不回来"的死锁）；
--  - 软删除：`deleted_at`（与 c008/c009 软删除纪律一致；NULL = 未删除）；
--  - `boundaries` = 负向约束清单（"不要…"），JSONB 字符串数组；
--  - `voice` = CosyVoice2 预置音色（带模型名前缀，如 FunAudioLLM/CosyVoice2-0.5B:anna）；
--  - `is_default` 标记默认角色「小E」（**可改可删**，不是系统行）；
--  - `template_id` = 预设模板来源（可空；v1 允许"纯自定义"）。
--
--  幂等：IF NOT EXISTS 保证可重复执行（迁移工具会重跑）。
-- =====================================================

CREATE TABLE IF NOT EXISTS emotion_echo_user.user_agent_profile (
    id          BIGSERIAL PRIMARY KEY,
    user_id     BIGINT       NOT NULL,
    name        VARCHAR(32)  NOT NULL,
    gender      VARCHAR(8),
    persona     TEXT,
    tone        VARCHAR(128),
    boundaries  JSONB,
    voice       VARCHAR(64),
    is_default  BOOLEAN      NOT NULL DEFAULT FALSE,
    template_id VARCHAR(32),
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ,
    FOREIGN KEY (user_id) REFERENCES emotion_echo_user.users(id) ON DELETE CASCADE
);

-- 索引：按属主列活跃角色（列表 / ≤５ 计数）
CREATE INDEX IF NOT EXISTS idx_user_agent_profile_user
    ON emotion_echo_user.user_agent_profile(user_id)
    WHERE deleted_at IS NULL;

COMMENT ON TABLE  emotion_echo_user.user_agent_profile IS 'CL-06 用户自定义角色档案（每户 ≤5，软删除；上限在服务层强制）';
COMMENT ON COLUMN emotion_echo_user.user_agent_profile.name        IS '角色名（≤20 字，保存时校验）';
COMMENT ON COLUMN emotion_echo_user.user_agent_profile.gender      IS '性别（展示用，不参与安全判定）';
COMMENT ON COLUMN emotion_echo_user.user_agent_profile.persona     IS '背景人设（≤500 字；核心字段，受安全过滤约束）';
COMMENT ON COLUMN emotion_echo_user.user_agent_profile.tone        IS '语气风格（≤100 字）';
COMMENT ON COLUMN emotion_echo_user.user_agent_profile.boundaries  IS '负向约束清单 JSONB 字符串数组（≤5 条，每条 ≤50 字）';
COMMENT ON COLUMN emotion_echo_user.user_agent_profile.voice       IS 'TTS 音色（CosyVoice2 预置，带模型名前缀）';
COMMENT ON COLUMN emotion_echo_user.user_agent_profile.is_default  IS '是否默认角色「小E」（可改可删）';
COMMENT ON COLUMN emotion_echo_user.user_agent_profile.template_id IS '预设模板来源（可空）';
COMMENT ON COLUMN emotion_echo_user.user_agent_profile.deleted_at  IS '软删除时间（NULL = 未删除）';
