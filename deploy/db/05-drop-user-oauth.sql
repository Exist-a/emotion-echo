-- =====================================================
-- Stage 62 PR-5 · drop emotion_echo_user.user_oauth 表
-- 时间：2026-09-10
-- 决策：ADR 21（OAuth DDL 清理）
--
-- 背景（详见 stage-62-cleanup-and-grpc-plan.md §二.5）：
--   - docs/plans/wechat-qq-login-and-upload.md 已被 superseded-by Stage 38-A
--     （决策记录：改用 username + password 登录，去掉微信/QQ OAuth 路径）
--   - user-svc 当前 model 无 OAuth 字段（实测 grep WechatOpenID/UnionID/provider 0 命中）
--   - 代码侧 Go / 前端 / Python 三层 grep user_oauth 全部 0 引用
--     （契约测试 scripts/test_user_oauth_zero_ref.sh 5/5 PASS）
--   - 决策 19 规范：legacy/ 已归档 oauth_handler.go 不动
--
-- 恢复条件（ADR 21 §C，未来如需重新接入 OAuth）：
--   1. 用户明确决策 '重新启用 OAuth 登录'
--   2. 写新 ADR 撤销本决策
--   3. 新建 migration 重建表 + user-svc model 加 OAuth 字段
-- =====================================================

DROP TABLE IF EXISTS emotion_echo_user.user_oauth;

-- 验证表已删（开发期肉眼检查；不阻塞脚本退出）
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM pg_tables
        WHERE schemaname = 'emotion_echo_user' AND tablename = 'user_oauth'
    ) THEN
        RAISE EXCEPTION 'Stage 62 PR-5: user_oauth 表删除失败';
    END IF;
    RAISE NOTICE 'Stage 62 PR-5: user_oauth 表已删除';
END
$$;