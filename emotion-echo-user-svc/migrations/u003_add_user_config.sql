-- u003: 新增 users.config JSONB 列（E2E-12 设置页持久化，D-09）
--
-- 幂等：IF NOT EXISTS 保证重复执行不报错。
-- 语义：NULL = 从未设置（区别于 {} 空对象 = 用户主动清空）。
-- 下游：user-svc model → proto → BFF → 前端 store 全链路透传。

ALTER TABLE emotion_echo_user.users
    ADD COLUMN IF NOT EXISTS config JSONB;

COMMENT ON COLUMN emotion_echo_user.users.config IS '用户个性化配置（fontSize/theme 等），JSONB 格式。NULL=未设置。';