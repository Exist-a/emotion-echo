-- 007_widen_intent_to_32.sql · Stage 89 PR-6 e2e 揪出 + 修
--
-- 背景：Stage 82 PR-3b 时 DDL intent 列定义为 VARCHAR(16)，allowedIntents 白名单
-- 也按 ≤16 字符设计；Stage 87 引入 LLM 消歧后实际返回的 6 类标签最长 'emotional_support'
-- （18 字符），意图白名单校验放行但 INSERT 仍 22001 超长 → 500。
-- Stage 89 PR-6 文件理解 e2e 实测首例落库即触发，session 取证：
--   INSERT ... VALUES (... 'emotional_support' ...) → value too long for type character varying(16)
-- 修：DDL 列扩到 VARCHAR(32）（覆盖全部白名单标签 + 余量），与 §契约 5 DDL/白名单/
-- 写入端三方一致。同时 stage-89 报告登记本失真。
--
-- 实现要点：msg_summary_v 视图引用 intent 列，ALTER TYPE 必须先 DROP VIEW；
-- 重建视图内容与 migration 005 一致（DROP → 改列 → CREATE → 重新 GRANT）。
DROP VIEW IF EXISTS emotion_echo_chat.msg_summary_v;
ALTER TABLE emotion_echo_chat.messages ALTER COLUMN intent TYPE VARCHAR(32);
CREATE VIEW emotion_echo_chat.msg_summary_v AS
SELECT id, conversation_id, user_id, role, content_type, intent,
       tokens_used, LENGTH(content) AS content_len, created_at AS send_time
FROM emotion_echo_chat.messages;
GRANT SELECT ON emotion_echo_chat.msg_summary_v TO analytics_reader;