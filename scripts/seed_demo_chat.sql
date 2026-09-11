-- scripts/seed_demo_chat.sql
--
-- Stage 68 · dev 模式 dashboard 数据填充（按 stage-65-dashboard-empty-root-cause.md 选项 A）
--
-- 目的：
--   dev 默认 compose 启动后，demo 用户（uid=1, echo/echo123）没有任何聊天记录，
--   导致 emotion_echo_chat.conversations/messages 两表 0 行，
--   → msg_summary_v 视图空 → daily/weekly/monthly/annual 4 dashboard
--     conversationCount=0 + messageCount=0 + emotionDistribution=[]
--   （KAFKA_ENABLED=false 时 ai-svc Kafka consumer 也不写 emotion_analysis，
--    distribution 维度也空；本 seed 覆盖 distribution 维度）
--
-- 设计原则：
--   - 幂等：ON CONFLICT DO NOTHING + 固定 id 范围（1000+）避开真实数据
--   - 不动生产路径：seed id 起始 1000，与 chat-svc 主键 nextval 序列无冲突
--     （PG 序列 nextval 不会读已用过的 id，所以新插入 id 仍走序列）
--   - 多样性：3 个会话覆盖不同状态（open / closed）+ 多模态（text）
--   - 时间分布：3 天前 / 1 天前 / 今天（覆盖 weekly/monthly/annual 聚合）
--
-- 与 emotion_analysis 联动：
--   - emotion_analysis.event_id 须 UNIQUE（PG 约束），用 'seed-msg-N' 固定前缀
--   - message_id 引用 emotion_echo_chat.messages.id → FK + ON CONFLICT DO NOTHING 不会触发
--     （FK 无 ON CONFLICT 机制，但 INSERT 用固定 id 后重复跑 OK：第二次撞 UNIQUE/PK 报错）
--
-- 注意：本 SQL 用 BEGIN/COMMIT + psql ON_ERROR_STOP，需要 psql -1 单事务执行
-- （脚本里 psql -1 已默认；docker exec 跑要加 -1）。
--
-- 用法：
--   bash scripts/seed_demo_chat.sh              # 自动 psql -1 + docker exec
--   bash scripts/test_seed_demo_chat.sh          # 契约测试：seed 后断言 4 dashboard 数据非空
--
-- 范围：仅 dev 默认 demo 用户（uid=1）。
-- 生产环境绝对不要跑（dev-only seed）。

BEGIN;

-- =====================================================
-- 1. conversations（3 条，id 起始 1000）
-- =====================================================
INSERT INTO emotion_echo_chat.conversations
    (id, user_id, title, context, message_count, last_message_at, status, created_at, updated_at, closed_at)
VALUES
    (1001, 1, '工作压力倾诉', '{}'::jsonb, 3, NOW() - INTERVAL '3 hours', 1,
     NOW() - INTERVAL '4 hours', NOW() - INTERVAL '3 hours', NULL),
    (1002, 1, '睡眠质量讨论', '{}'::jsonb, 2, NOW() - INTERVAL '2 days', 1,
     NOW() - INTERVAL '2 days 4 hours', NOW() - INTERVAL '2 days', NULL),
    (1003, 1, '情绪复盘', '{}'::jsonb, 2, NOW() - INTERVAL '5 hours', 2,
     NOW() - INTERVAL '5 hours 30 minutes', NOW() - INTERVAL '5 hours', NOW() - INTERVAL '5 hours'),
    -- 今天有 1 个 open 会话（让 dailyReport.conversationCount 也非 0）
    (1004, 1, '今天的对话', '{}'::jsonb, 1, NOW() - INTERVAL '1 hour', 1,
     NOW() - INTERVAL '2 hours', NOW() - INTERVAL '1 hour', NULL)
ON CONFLICT (id) DO NOTHING;

-- =====================================================
-- 2. messages（7 条，id 起始 1000）
-- =====================================================
INSERT INTO emotion_echo_chat.messages
    (id, conversation_id, user_id, role, content, content_type, metadata, tokens_used, client_msg_id, created_at)
VALUES
    -- conv 1001（work stress）：3 条
    (1001, 1001, 1, 'user', '最近项目 deadline 太紧', 'text', '{}'::jsonb, 0, NULL,
     NOW() - INTERVAL '4 hours'),
    (1002, 1001, 1, 'assistant', '深呼吸，先聊聊哪部分压力最大？', 'text', '{}'::jsonb, 0, NULL,
     NOW() - INTERVAL '3 hours 50 minutes'),
    (1003, 1001, 1, 'user', '后端 API 集成卡了两天', 'text', '{}'::jsonb, 0, NULL,
     NOW() - INTERVAL '3 hours'),
    -- conv 1002（sleep）：2 条
    (1004, 1002, 1, 'user', '最近失眠', 'text', '{}'::jsonb, 0, NULL,
     NOW() - INTERVAL '2 days 3 hours'),
    (1005, 1002, 1, 'assistant', '试着睡前 30 分钟不看手机', 'text', '{}'::jsonb, 0, NULL,
     NOW() - INTERVAL '2 days'),
    -- conv 1003（emotion review）：2 条
    (1006, 1003, 1, 'user', '今天心情不错', 'text', '{}'::jsonb, 0, NULL,
     NOW() - INTERVAL '5 hours 20 minutes'),
    (1007, 1003, 1, 'assistant', '继续保持 😊', 'text', '{}'::jsonb, 0, NULL,
     NOW() - INTERVAL '5 hours'),
    -- conv 1004（today）：1 条
    (1008, 1004, 1, 'user', '今天有点焦虑', 'text', '{}'::jsonb, 0, NULL,
     NOW() - INTERVAL '1 hour')
ON CONFLICT (id) DO NOTHING;

-- =====================================================
-- 3. emotion_analysis（覆盖 daily_emotion_v 视图 → dashboard emotionDistribution）
--
-- 7 条 emotion_analysis 对应 7 条 user message（assistant 不分析）
-- event_id 用 'seed-evt-N' 前缀避免与 chat-svc outbox UUID 冲突
-- =====================================================
INSERT INTO emotion_echo_ai.emotion_analysis
    (event_id, message_id, user_id, conversation_id, primary_emotion, emotion_scores,
     sentiment_score, confidence, model, raw_response, created_at)
VALUES
    ('seed-evt-1001', 1001, 1, 1001, 'negative',
     '{"anxiety": 0.7, "stress": 0.6}'::jsonb, -0.5, 0.85,
     'keyword-stub-v1', '{}'::jsonb, NOW() - INTERVAL '3 hours 50 minutes'),
    ('seed-evt-1003', 1003, 1, 1001, 'negative',
     '{"frustration": 0.6, "stress": 0.5}'::jsonb, -0.4, 0.80,
     'keyword-stub-v1', '{}'::jsonb, NOW() - INTERVAL '2 hours 50 minutes'),
    ('seed-evt-1004', 1004, 1, 1002, 'negative',
     '{"insomnia": 0.7, "anxiety": 0.4}'::jsonb, -0.6, 0.82,
     'keyword-stub-v1', '{}'::jsonb, NOW() - INTERVAL '2 days 2 hours 50 minutes'),
    ('seed-evt-1005', 1005, 1, 1002, 'neutral',
     '{"calm": 0.5}'::jsonb, 0.0, 0.75,
     'keyword-stub-v1', '{}'::jsonb, NOW() - INTERVAL '1 day 23 hours 50 minutes'),
    ('seed-evt-1006', 1006, 1, 1003, 'positive',
     '{"joy": 0.7, "calm": 0.6}'::jsonb, 0.6, 0.88,
     'keyword-stub-v1', '{}'::jsonb, NOW() - INTERVAL '5 hours 10 minutes'),
    ('seed-evt-1007', 1007, 1, 1003, 'positive',
     '{"joy": 0.6}'::jsonb, 0.5, 0.85,
     'keyword-stub-v1', '{}'::jsonb, NOW() - INTERVAL '4 hours 50 minutes'),
    ('seed-evt-1008', 1008, 1, 1004, 'negative',
     '{"anxiety": 0.5}'::jsonb, -0.3, 0.78,
     'keyword-stub-v1', '{}'::jsonb, NOW() - INTERVAL '50 minutes')
ON CONFLICT (event_id) DO NOTHING;

COMMIT;