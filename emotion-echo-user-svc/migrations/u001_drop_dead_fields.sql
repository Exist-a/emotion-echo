-- =====================================================
--  E2E-06: 删除 users 表死字段
--  路径：emotion-echo-user-svc/migrations/u001_drop_dead_fields.sql
--
--  背景：phone/email/status 三个字段在业务代码中零读写或读写链路断裂
--  - phone: proto 定义了，Register 写入但从未被业务使用，GetMe/GetById 仅回显
--  - email: proto 定义了，model 有，但 logic 从未读写（幽灵字段）
--  - status: 仅 model 定义，proto 没有，logic 从未读写（纯死字段）
--
--  幂等：IF EXISTS 保证可重复执行
-- =====================================================

-- 删除 phone 列（UNIQUE 约束会自动随列删除）
ALTER TABLE emotion_echo_user.users DROP COLUMN IF EXISTS phone;

-- 删除 email 列
ALTER TABLE emotion_echo_user.users DROP COLUMN IF EXISTS email;

-- 删除 status 列
ALTER TABLE emotion_echo_user.users DROP COLUMN IF EXISTS status;