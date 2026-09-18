-- =====================================================
--  契约账号种子（initdb.d 自动执行）
--  路径：deploy/db/03-seed-default-users.sql
--  挂载：deploy/docker-compose.infra.yml postgres volumes
--
--  ⚠️ 本文件只创建"契约账号"（smoke_user），不可删除。
--  演示账号（echo）由 deploy/db/seed-demo-account.sh 负责创建，
--  可通过 deploy/db/cleanup-demo-account.sh 清理。
--
--  为什么分开：
--    - initdb.d 的 SQL 在每次空卷重建时自动执行
--    - 演示账号需要"可删除 + 可重建"，不应在 initdb.d 硬编码
--    - 契约账号（smoke_user）是 smoke 脚本依赖的基础设施，必须始终存在
-- =====================================================

-- 契约账号：smoke_user / echo123 (Stage 37 数据契约 smoke 用)
-- 注意：smoke 脚本（scripts/smoke_data_layer.py）依赖此账号存在
-- bcrypt(cost=10) hash of "echo123"
INSERT INTO emotion_echo_user.users (
    username, password_hash, nickname, created_at, updated_at
) VALUES (
    'smoke_user',
    '$2a$10$x/oarv7WP0HJBNTiJGJBSeBMCvqIS.jMndnYasMS.O2SLzm7pqQnC',
    'Smoke User',
    NOW(),
    NOW()
)
ON CONFLICT (username) DO NOTHING;
