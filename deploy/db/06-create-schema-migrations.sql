-- =====================================================
--  E2E-06: 迁移版本追踪表
--  路径：deploy/db/06-create-schema-migrations.sql
--
--  目的：记录哪些迁移已被应用，支持 checksum 校验，
--  让 migrate.sh 可以跳过已应用的迁移并检测变更。
--
--  幂等：IF NOT EXISTS 保证可重复执行
-- =====================================================

CREATE TABLE IF NOT EXISTS emotion_echo_user.schema_migrations (
    version VARCHAR(255) PRIMARY KEY,
    checksum VARCHAR(64) NOT NULL,
    applied_at TIMESTAMPTZ DEFAULT NOW(),
    execution_ms INTEGER
);

COMMENT ON TABLE emotion_echo_user.schema_migrations IS '迁移版本追踪：记录已应用的迁移文件及其 checksum';
COMMENT ON COLUMN emotion_echo_user.schema_migrations.version IS '迁移文件名（不含路径），如 u001_drop_dead_fields.sql';
COMMENT ON COLUMN emotion_echo_user.schema_migrations.checksum IS '文件内容 SHA-256 前 64 位，用于检测迁移文件被修改';
COMMENT ON COLUMN emotion_echo_user.schema_migrations.execution_ms IS '迁移执行耗时（毫秒），用于性能监控';