-- =====================================================
--  Stage 38-A: 默认测试用户种子（username+password 登录）
--  路径：deploy/db/03-seed-default-users.sql
--  挂载：deploy/docker-compose.infra.yml postgres volumes
--  说明：dev compose up 后 users 表插入 echo / echo123 账号
--  对应：QUICKSTART.md "测试账号" 章节
--
-- 演进：
--   Stage 36-D：账号 = 13800138000 / abc123（手机号+密码语义）
--   Stage 38-A：账号 = echo / echo123（用户名+密码语义，纯 dev 演示）
-- =====================================================

-- 默认测试账号：echo / echo123
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
