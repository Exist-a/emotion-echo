-- migrations/009_emotion_analysis_event_id_complete_unique.sql
--
-- Round 1.1 修复：i006 引入的 partial unique index 破坏 GORM repo 的
-- `clause.OnConflict{Columns: [event_id]}` 路径。
--
-- 根因（PG 行为）：
--   i006 把 emotion_analysis.event_id 从"完整 UNIQUE CONSTRAINT"改为
--   "partial UNIQUE INDEX WHERE event_id <> '__legacy__'"。
--   GORM repo (emotion_repository.go:200) 走 `ON CONFLICT (event_id) DO NOTHING`
--   必须匹配一个**完整** UNIQUE 索引（不带 WHERE 谓词）— 否则报
--   `SQLSTATE 42P10: there is no unique or exclusion constraint matching
--   the ON CONFLICT specification`。
--
-- 修复策略：
--   1) 把现有 '__legacy__' 占位行回填为 '__legacy_<id>'（每行唯一）
--   2) 删除 partial UNIQUE INDEX（WHERE event_id <> '__legacy__'）
--   3) 重建为完整 UNIQUE INDEX（不带 WHERE，event_id NOT NULL 已由 i006 保证）
--   4) 加测试用例 `TestEmotionRepo_After009_CompleteUnique_OnConflict` 锁死契约
--
-- 适用范围：emotion_echo_ai schema（ai-svc 独占）
-- 前置：i001 (event_id 列) + i006 (NOT NULL + partial unique)
-- 影响：仅 emotion_analysis，voice_emotion_results 不变（i008 partial unique
--       不会被 ON CONFLICT 触碰 — voice_emotion_results repo 不走 OnConflict）

BEGIN;

-- 1) 把 i006 留下的 '__legacy__' 占位行改为 '__legacy_<id>'
-- （i001 已用 'legacy-<id>' 模式；这里 i006 的占位与 i001 不同，所以二次回填）
UPDATE emotion_echo_ai.emotion_analysis
SET event_id = '__legacy_' || id::text
WHERE event_id = '__legacy__';

-- 2) 删除 i006 引入的 partial UNIQUE INDEX
DROP INDEX IF EXISTS emotion_echo_ai.uq_emotion_analysis_event_id;

-- 3) 重建为完整 UNIQUE INDEX（不带 WHERE 谓词，匹配 ON CONFLICT (event_id)）
CREATE UNIQUE INDEX IF NOT EXISTS uq_emotion_analysis_event_id
    ON emotion_echo_ai.emotion_analysis(event_id);

COMMIT;
