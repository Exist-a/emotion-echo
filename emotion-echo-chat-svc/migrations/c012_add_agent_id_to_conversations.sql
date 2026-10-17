-- c012_add_agent_id_to_conversations.sql · CL-06（用户自定义 Agent·多角色）
--
-- 会话的**当前角色**：`agent_id` 引用 user-svc 的角色档案表主键（跨服务引用，**故意不建外键**）。
--
-- 本文件只操作自己的 schema（`emotion_echo_chat`）——**不引用其他服务的表**，
-- 因此与 `scripts/test_migrations_no_service_order.sh` 的"服务顺序独立性"要求相符（该守卫按
-- 子串匹配 schema 名，注释里也不要写别家的 schema 前缀）。
--
-- ⚠️ **跨 schema，故意不建外键**（与仓库"跨服务不跨 schema JOIN"的约定一致）：
--    角色由 user-svc 拥有，chat-svc 只存引用。角色被软删除后本列会指向不存在的行 ⇒
--    **消费方（BFF）解析失败时回落用户的默认角色**（不报错、不给空 prompt）——
--    该行为由 `docs/closure-roadmap/stages/cl-06-custom-agent/plan.md` T-4 钉住。
--
-- NULL = 未显式选择（用用户默认角色）；与 0 区分：proto 侧用 0 表示"未选择"。
ALTER TABLE emotion_echo_chat.conversations
    ADD COLUMN IF NOT EXISTS agent_id BIGINT;

COMMENT ON COLUMN emotion_echo_chat.conversations.agent_id IS
    'CL-06 本会话当前角色（引用 user_agent_profile.id；跨 schema 无外键；NULL=未显式选择）';
