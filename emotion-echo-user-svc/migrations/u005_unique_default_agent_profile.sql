-- CL-06（2026-10-17）：每户**至多一条**默认角色（DB 层兜底）。
--
-- 为什么需要：默认角色物化（首次读列表时 create「小E」）在服务层已改为
-- "事务 + 每用户咨询锁"的原子路径（见 internal/repository/user_agent_profile_repository.go），
-- 但那只覆盖**本服务的这条代码路径**。DB 层再加一道，可同时挡住：
--   · 将来新增的其它写入路径（脚本 / 新 RPC / 手工 SQL）忘记复核；
--   · 服务层原子实现被回退时的静默回归（第二方核对（2026-10-17）读码确认原实现的 TOCTOU 真实存在）。
--
-- 语义：`is_default AND deleted_at IS NULL` 这个组合在**同一 user_id 上唯一**
-- —— 软删除的行不参与（所以"删掉默认角色再建一个默认角色"仍然合法）。
--
-- 幂等：CREATE UNIQUE INDEX IF NOT EXISTS（可重复执行）。
-- ⚠️ 前置：应用前库内不得已存在同户多条默认角色；本文件在 CL-06 Phase 8 的 dev 库上
--    实测每户仅 1 条（见 evidence/phase8-db-evidence.txt）。若在别的环境失败，
--    请先人工收敛重复行（保留 id 最小的一条，其余置 is_default=false）再重跑。
CREATE UNIQUE INDEX IF NOT EXISTS uq_user_agent_profile_one_default_per_user
    ON emotion_echo_user.user_agent_profile (user_id)
    WHERE deleted_at IS NULL AND is_default;
