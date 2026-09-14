-- =====================================================
--  Stage 38-A: 默认测试用户种子（username+password 登录）
--  路径：deploy/db/03-seed-default-users.sql
--  挂载：deploy/docker-compose.infra.yml postgres volumes
--
-- P1-R2-11: "echo123" 在 .sql 注释中是明文，QUICKSTART.md 也明文出现。
-- 真实凭据是 **bcrypt hash**（下方 $2a$10$... 字符串），明文仅作注释说明用途。
-- 哈希验证：python -c "import bcrypt; bcrypt.checkpw(b'echo123', b'<hash>')"
--
-- dev / staging 混淆风险：
--   若 staging 直接 cp 这份 seed 而忘记改 hash，会留下 dev 弱密码。
--   Stage 36-D Bug 4 教训：dev 数据"原样上 prod"是反复栽跟头的源头。
--   建议：staging 改用随机生成密码 + 启动期注入，不复用 dev seed。
-- =====================================================

-- 默认测试账号（dev only）：echo / echo123
-- bcrypt(cost=10) hash of "echo123"
INSERT INTO emotion_echo_user.users (
    username, password_hash, nickname, status, created_at, updated_at
) VALUES (
    'echo',
    '$2a$10$x/oarv7WP0HJBNTiJGJBSeBMCvqIS.jMndnYasMS.O2SLzm7pqQnC',
    'Echo User',
    1,
    NOW(),
    NOW()
)
ON CONFLICT (username) DO NOTHING;

-- 可选：smoke_user / echo123 (Stage 37 数据契约 smoke 用)
-- 注意：smoke 脚本（scripts/smoke_data_layer.py）依赖此账号存在
INSERT INTO emotion_echo_user.users (
    username, password_hash, nickname, status, created_at, updated_at
) VALUES (
    'smoke_user',
    '$2a$10$x/oarv7WP0HJBNTiJGJBSeBMCvqIS.jMndnYasMS.O2SLzm7pqQnC',
    'Smoke User',
    1,
    NOW(),
    NOW()
)
ON CONFLICT (username) DO NOTHING;
