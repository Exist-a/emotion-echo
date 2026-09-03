-- =====================================================
--  Stage 36-D Bug 1: 默认测试用户种子
--  路径：deploy/db/03-seed-default-users.sql
--  挂载：deploy/docker-compose.infra.yml postgres volumes
--  说明：在 emotion_echo_user.users 表插入 13800138000 / abc123 测试账号
--  对应：QUICKSTART.md "测试账号" 章节
-- =====================================================

-- 默认测试账号：13800138000 / abc123
-- bcrypt(cost=10) hash of "abc123"
INSERT INTO emotion_echo_user.users (
    username, phone, password_hash, nickname, status, created_at, updated_at
) VALUES (
    '13800138000',
    '13800138000',
    '$2a$10$WXn7mPSA5M07r//og.ZxbuZPln7akYsevbrcNldF.piYk2anu5lUK',
    'Smoke User',
    1,
    NOW(),
    NOW()
)
ON CONFLICT (username) DO NOTHING;

-- 可选：smoke_user / abc123 (Stage 35 用过)
INSERT INTO emotion_echo_user.users (
    username, phone, password_hash, nickname, status, created_at, updated_at
) VALUES (
    'smoke_user',
    NULL,
    '$2a$10$WXn7mPSA5M07r//og.ZxbuZPln7akYsevbrcNldF.piYk2anu5lUK',
    'Smoke User 2',
    1,
    NOW(),
    NOW()
)
ON CONFLICT (username) DO NOTHING;