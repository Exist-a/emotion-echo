-- migrations/a011_grant_daily_emotion_fused_v.sql
--
-- CL-07（后端切片）Phase 1 配套：把新视图 daily_emotion_fused_v 授权给只读 role。
--
-- 为什么不加进 a004：
--   a004 已应用；改它会让 deploy/db/migrate.sh 的 checksum 校验返回 rc=2 并 die
--   （见 migrate.sh check_migration → run_tracked_sql_file）。故新增本迁移文件。
--
-- 顺序安全：
--   migrate.sh 按服务字典序应用（chat < ai < analytics）；role 定义（a004）与本文件
--   同在 analytics-svc/migrations/、字典序 a004 < a011 ⇒ 本文件执行时
--   analytics_reader 必已存在；ai-svc 的 i010（建视图）在 ai 目录先于 analytics 应用
--   ⇒ 视图也已存在。GRANT 幂等，可重复执行。

GRANT SELECT ON emotion_echo_ai.daily_emotion_fused_v TO analytics_reader;
