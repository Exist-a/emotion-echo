---
status: planned
priority: high
owner: TBD
created: 2026-09-04
related-stages:
  - stage-38-system-status.md (§四 隐患 1，2026-09-04 升级为已发作)
  - stage-39-nacos-enablement.md
---

# Plan — 服务 migrations 的自动应用机制

## 一、问题（2026-09-04 实测）

dev 库里 **13 个服务迁移一个都没有被应用过**，且没有任何机制会去应用它们。

发现路径：合并 main 前跑 AGENTS.md §2.4 数据契约 smoke，
`POST /api/v1/conversations/{id}/messages` 返 500：

```
chat-svc: column "client_msg_id" of relation "messages" does not exist (SQLSTATE 42703)
```

手工补上该迁移后 smoke 继续暴露，一度只剩 **1/10 PASS**：

| smoke 契约 | 报错 | 缺的迁移 |
|---|---|---|
| §1/§2 user_behavior_events | `column "event_id" ... does not exist` | analytics 006 |
| §3 analytics_reader | `FATAL: role "analytics_reader" does not exist` | analytics 004 |
| §4 /reports/daily | `relation "emotion_echo_chat.msg_summary_v" does not exist` | analytics 001 |

按依赖顺序全部应用后恢复 10/10 PASS。

## 二、根因

`deploy/docker-compose.infra.yml:28-31` 只挂了 4 个初始化脚本：

```yaml
- ./db/01-create-schemas.sql:/docker-entrypoint-initdb.d/01-create-schemas.sql:ro
- ./db/02-create-tables-in-schemas.sql:/docker-entrypoint-initdb.d/02-create-tables-in-schemas.sql:ro
- ./db/03-seed-default-users.sql:/docker-entrypoint-initdb.d/03-seed-default-users.sql:ro
- ./db/04-create-views.sql:/docker-entrypoint-initdb.d/04-create-views.sql:ro
```

而实际的 schema 演进散在各服务目录下，**从不参与启动**：

```
emotion-echo-ai-svc/migrations/001..005          (event_id / face / voice / fused / modality view)
emotion-echo-analytics-svc/migrations/001..006   (views / events / mv / reader role / jobs / event_id)
emotion-echo-chat-svc/migrations/001..002        (outbox_events / client_msg_id)
```

这些文件只在自己的注释里写了"应用方式：docker exec -i ... psql < 本文件"，
即**依赖人工执行且无人记录执行过没有**。Postgres 的 `initdb.d` 又只在数据卷为空
时跑一次，所以任何在首次初始化之后新增的迁移都永远不会生效。

注：`docs/stages/stage-38-system-status.md` 原把此事记为"🟡 中（重建 dev 环境会丢表/列）"，
低估了两点——(1) 不是"重建才丢"，当时的库就已经缺；(2) 范围不是 Stage 34 的
004/005/006，而是全部 13 个。该文已于 2026-09-04 更正。

## 三、候选方案

| 方案 | 做法 | 优点 | 代价 |
|---|---|---|---|
| A. 挂进 initdb.d | 把 13 个迁移按序软链/复制进 `deploy/db/05..17` | 改动最小 | 仍只在空卷首跑；新增迁移对已有环境无效——**没解决根问题** |
| B. 启动期 migrate 容器 | compose 加一次性 `migrate` service，依赖 postgres healthy，跑完退出 | 每次 up 都对齐；幂等迁移可重复执行 | 需定义执行顺序（跨服务有依赖：ai 002/003 先于 ai 005；analytics 001 依赖 chat/ai 表） |
| C. 各 svc 启动时自迁移 | 每个 svc main.go 启动跑自己的 migrations | 与服务所有权一致 | 并发启动时跨服务依赖顺序不可控 |

倾向 **B**：唯一能同时满足"已有环境也能补齐"和"顺序可控"的。

## 四、验收

1. `docker compose down -v && up -d` 后，未经任何手工 psql，`smoke_data_layer.py` 直接 10/10 PASS
2. 对已存在且已迁移的库重复执行，全部幂等无报错
3. 新增一个迁移文件后重新 up，该迁移生效
4. 契约测试断言"每个 `*/migrations/*.sql` 都被执行清单覆盖"，防止再次出现加了文件但没人跑

## 五、注意事项

- 现有 13 个迁移里 `analytics/004_create_analytics_reader_role.sql` 无 `IF NOT EXISTS` 守卫
  （`CREATE ROLE` 重复执行会报错），方案 B 落地时需要先补幂等守卫
- 跨服务顺序实测可用：chat 001/002 → ai 001..005 → analytics 001..006

## 六、调研依据

- 读过：`deploy/docker-compose.infra.yml`、`emotion-echo-chat-svc/migrations/002_add_client_msg_id.sql`、
  `emotion-echo-analytics-svc/migrations/00{1,4,6}_*.sql`、`emotion-echo-ai-svc/migrations/00{2,3,5}_*.sql`
- 查过：`docs/stages/stage-38-system-status.md` §四、`docs/stages/stage-39-nacos-enablement.md`、`AGENTS.md` §2.4
- 跑过：`scripts/smoke_data_layer.py`（补迁移前 1/10 PASS → 补齐后 10/10 PASS）、
  `docker exec emotion-echo-postgres psql -c "\d emotion_echo_chat.messages"`
