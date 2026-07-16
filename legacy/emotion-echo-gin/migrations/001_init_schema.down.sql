-- 回滚初始数据库迁移（Down）

DROP TABLE IF EXISTS emotion_analyses;
DROP TABLE IF EXISTS survey_results;
DROP TABLE IF EXISTS surveys;
DROP TABLE IF EXISTS refresh_tokens;
DROP TABLE IF EXISTS messages;
DROP TABLE IF EXISTS conversations;
DROP TABLE IF EXISTS users;
