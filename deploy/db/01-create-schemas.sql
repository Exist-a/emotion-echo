-- =====================================================
-- Emotion-Echo 微服务拆分 · 数据库 schema 分离
-- =====================================================
-- 原则：每张表归属唯一一个业务域 svc
-- 跨域查询 → 通过 RPC（每个 svc 暴露自己的查询接口），禁止跨库 JOIN
--
-- P0-R2-7: 本文件仅创建 schema，表定义统一在 02-create-tables-in-schemas.sql。
-- 原因：01 和 02 都用 CREATE TABLE IF NOT EXISTS 创建同名表，01 先跑导致
-- 02 的 richer 定义（pinned 列、intent 列、FK 约束）被静默跳过。
-- =====================================================

-- 1. 创建 5 个业务 schema
CREATE SCHEMA IF NOT EXISTS emotion_echo_user;       -- user-svc 拥有
CREATE SCHEMA IF NOT EXISTS emotion_echo_chat;       -- chat-svc 拥有
CREATE SCHEMA IF NOT EXISTS emotion_echo_ai;         -- ai-svc 拥有
CREATE SCHEMA IF NOT EXISTS emotion_echo_assessment; -- assessment-svc 拥有
CREATE SCHEMA IF NOT EXISTS emotion_echo_analytics;  -- analytics-svc 拥有