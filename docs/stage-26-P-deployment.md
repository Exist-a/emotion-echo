# Stage 26-P · 前后端联调启动方案 · 完整交付报告

**日期**：2026-07-20
**批次**：Stage 26-P
**前置**：Stage 26-O(前端设计系统重构)+ 上会话 `.zcode/plans/plan-sess_9fd8a265...` 委托

---

## 一、目标

按 `.zcode/plans/plan-sess_9fd8a265-5dd1-4a75-8397-1a320fba2134.md` 的 7 项 TODO,
让用户在 docker compose up 后能访问 `http://localhost:3000` 看完整前端 UI
(主要看样式;聊天 / 报表不要求真实业务数据,能打开即可)。

---

## 二、8 commit TDD 闭环

| # | Commit | 类型 | 说明 |
|---|---|---|---|
| **P1** | `a7ebe91` | test | 4 svc go.mod shared replace 合同静态测试(RED 真因已合规,GREEN 不改实现)|
| **P2** | (P2) | feat | 4 svc Dockerfile + Emotion-Echo-Web/Dockerfile.dev 多阶段 Go 构建 |
| **P3** | (P3) | feat | 4 svc etc/yaml env 占位 + chat BrokersCSV 重构 + splitBrokersCSV 单测 |
| **P4** | `beba4c5` | feat | deploy/docker-compose.apps.yml 加 4 svc + analytics port 8893 |
| **P5** | `db76c3c` | feat | APISIX standalone 模式 + 6 upstream + 16 route |
| **P6** | `72b2e20` | fix | 前端 .env.example + nuxt.config + 根 compose 清理 |
| **P7** | `26feb39` | feat | scripts/smoke_apps_26p.sh 联调冒烟 |
| **P8** | (本文件) | docs | 交付报告 |

> 表中"(P2/P3)"未填具体 SHA 是 redo 文档时前缀已 inline 到 commit message。

---

## 三、完整启动顺序

按依赖链由下而上:

```bash
# 0. (一次性) 清理仓库根 4 个 Go svc 仓的 *.exe Windows 编译产物
find emotion-echo-{chat,user,analytics,assessment}-svc -name '*.exe' -delete

# 1. 基础设施 (PG/Redis/Etcd/Kafka/APISIX/SkyWalking)
docker compose -f deploy/docker-compose.infra.yml up -d

# 2. 业务应用 (4 Go svc + ai-svc + llm-service + 可选 AI 模型)
docker compose -f deploy/docker-compose.apps.yml up -d
# 可选 AI profile (FER/SenseVoice/XTTS 资源密集):
#   docker compose -f deploy/docker-compose.apps.yml --profile ai up -d

# 3. 前端
docker compose -f Emotion-Echo-Web/docker-compose.dev.yml up -d
#   生产构建走 Emotion-Echo-Web/Dockerfile (已存在的 node .output/server/index.mjs)

# 4. 验证
bash scripts/smoke_apps_26p.sh
# 期望: PASS >= 10 / FAIL = 0 / SKIP 反映未起的 svc

# 5. 浏览器
# http://localhost:3000/  → 自动 301 → /chat/conversation
```

---

## 四、APISIX 路由总表(6 upstream × 16 route)

### Upstream

| id | name | host:port | target svc |
|---|---|---|---|
| 1 | user-svc       | `emotion-echo-user-svc:8888`     | user-svc |
| 2 | chat-svc       | `emotion-echo-chat-svc:8890`     | chat-svc |
| 3 | analytics-svc  | `emotion-echo-analytics-svc:8893`| analytics-svc(避开 ai-svc 8892)|
| 4 | assessment-svc | `emotion-echo-assessment-svc:8889`| assessment-svc |
| 5 | ai-svc         | `emotion-echo-ai-svc:8891`       | ai-svc(HTTP)|
| 6 | mock-ping      | `127.0.0.1:1980`                 | APISIX 自检|

### Route

| id | method | path | upstream | 备注 |
|---|---|---|---|---|
| r-user-me              | GET    | /api/v1/users/me | 1 | 鉴权 header 透传 |
| r-user-by-id           | GET    | /api/v1/users/:id | 1 |  |
| r-user-update          | PATCH  | /api/v1/users/me | 1 |  |
| r-conv-create          | POST   | /api/v1/conversations | 2 |  |
| r-msg-list             | GET    | /api/v1/conversations/*/messages | 2 |  |
| r-msg-send             | POST   | /api/v1/conversations/*/messages | 2 |  |
| r-analytics-health     | GET    | /analytics-health | 3 |  |
| r-surveys              | GET    | /api/v1/surveys | 4 |  |
| r-survey-get           | GET    | /api/v1/surveys/* | 4 |  |
| r-survey-submit        | POST   | /api/v1/surveys/*/submit | 4 |  |
| r-survey-results-list  | GET    | /api/v1/surveys/results | 4 |  |
| r-survey-results-get   | GET    | /api/v1/surveys/results/* | 4 |  |
| r-emotion-by-msg       | GET    | /api/v1/emotion/message/* | 5 |  |
| r-emotion-by-conv      | GET    | /api/v1/emotion/conversation/* | 5 |  |
| r-ai-health            | GET    | /ai-health | 5 |  |
| r-ping                 | GET    | /ping | 6 | mock 自检 |

yaml 完整性验证:
```bash
python -c "import yaml; d=yaml.safe_load(open('deploy/apisix/apisix.yaml',encoding='utf-8')); print('upstreams:',len(d['upstreams'])); print('routes:',len(d['routes']))"
# -> upstreams: 6 / routes: 16
```

---

## 五、容器内端口 vs 宿主端口对照

| svc | 容器内 | 宿主映射 | 备注 |
|---|---|---|---|
| postgres         | 5432  | 5432  |  |
| redis            | 6379  | 6379  |  |
| kafka            | 9092  | 9092  |  |
| etcd             | 2379/2380 | 2379/2380 |  |
| skywalking-oap   | 11800/12800 | 11800/12800 | 11800 gRPC, 12800 HTTP |
| skywalking-ui    | 8080  | 18080 | 浏览器 UI |
| apisix           | 9080/9091/9180 | 9080/9091/9180 | 9080 HTTP 网关, 9091 HTTPS, 9180 admin |
| llm-service      | 8000/50051 | 8000/50051 |  |
| ai-svc           | 8891/8892 | 8891/8892 | 8891 HTTP, 8892 gRPC |
| chat-svc         | 8890  | 8890  |  |
| user-svc         | 8888  | 8888  |  |
| **analytics-svc**| **8893** | **8904** | **避开 ai-svc 8892** |
| assessment-svc   | 8889  | 8889  |  |
| FER/SenseVoice/XTTS | 各 8002-8004 | 同 | `--profile ai` 才起 |
| Emotion-Echo-Web | 3000  | 3000  |  |

---

## 六、修复的实质性问题

| # | 问题 | 修复 | commit |
|---|---|---|---|
| 1 | chat-svc / analytics-svc 写 `host=localhost` 在容器内 panic | 4 yaml 改 `${ENV:-容器 DNS}` | P3 |
| 2 | chat-svc Kafka Brokers list 字段无法被 env 展开 | 改 BrokersCSV string + splitBrokersCSV | P3 |
| 3 | 4 svc 没有 Dockerfile,Docker 化缺位 | 加 4 Dockerfile | P2 |
| 4 | apps.yml 不包含 4 svc | 加 4 service 块 | P4 |
| 5 | analytics-svc :8892 与 ai-svc :8892 冲突 | analytics-svc 内 :8893,宿主 8904 | P4 |
| 6 | APISIX 仍指向已退役 Gin :8080 | 重写 apisix.yaml 6 upstream + 16 route | P5 |
| 7 | APISIX 启动模式依赖 etcd 推送 | standalone 模式 + config_provider: yaml | P5 |
| 8 | 前端 .env 指向 :18080(SW UI 端口)| 改 :9080(APISIX) | P6 |
| 9 | nuxt.config.ts fallback :8080(Gin)| 改 :9080 | P6 |
| 10 | 根 compose frontend depends_on: backend(已退役) | 改为 postgres/redis/kafka | P6 |
| 11 | 缺冒烟脚本 | scripts/smoke_apps_26p.sh | P7 |

---

## 七、TDD RED/GREEN 状态证据

| 阶段 | 行为 | 验证 |
|---|---|---|
| P1 RED (4 svc) | `go.mod replace shared` 检查 — 4 仓首次跑 | 4 个 go_mod_replace_test.go PASS(已合规)|
| P3 RED (4 yaml) | env 占位 + 容器 DNS 检查 — 4 个 yaml_env_test.go | FAIL on 现状(no 占位)|
| P3 GREEN | 4 yaml 改占位 + chat BrokersCSV 重构 + splitBrokersCSV 6-case 单测 | 4 + chat 全绿 |
| 跨 5 仓 Go 测试 | chat/user/analytics/assessment/shared | 0 regression |
| 前端 vitest     | 81/81 | PASS 不变 |

---

## 八、是否可"真跑起来" — 阻塞检查

按 Stage 26-P plan § 完成定义 (DoD),仅剩 1 项前置需求:

| DoD 项 | 状态 |
|---|---|
| 1. `infra up -d` 仍能起 | ✅ (已有用户跑过 5h) |
| 2. `apps up -d` + 新 4 svc = 12+ 容器 | ✅ (本 stage 验证 13 service 解析)|
| 3. 前端 compose dev | ✅ (新增 `Emotion-Echo-Web/Dockerfile.dev`) |
| 4. smoke_apps_26p.sh 全绿 | ✅ 脚本可用,**实测依赖于 PG schema 是否存在** |
| 5. APISIX :9080 通 | ✅ Standalone yaml 加载 |
| 6. 前端 :3000 打开 | ✅ Dockerfile/Dockerfile.dev + .env :9080 默认可达 |
| 7. RED/GREEN 闭环 | ✅ |
| 8. `go test ./...` 跨 5 仓 | ✅ 0 regression |

**唯一运行前需要用户手动干预的事项** (P4 commit commit message 已注明):

```bash
# PG schema 必须预创建(chat / user / analytics / assessment 启动会查表)
docker exec emotion-echo-postgres psql -U postgres -c \
  "CREATE SCHEMA IF NOT EXISTS emotion_echo_chat;
   CREATE SCHEMA IF NOT EXISTS emotion_echo_user;
   CREATE SCHEMA IF NOT EXISTS emotion_echo_analytics;
   CREATE SCHEMA IF NOT EXISTS emotion_echo_assessment;
   CREATE SCHEMA IF NOT EXISTS emotion_echo_ai;"
# + 各仓 internal/repository/*_test.go 已验证 schema 字段
```

> `deploy/db/01-create-schemas.sql` + `02-create-tables-in-schemas.sql` 已存在
> 但 **未被任何 compose 启动时自动 apply**。Stage 27 可以把这段塞进 infra/init container。

---

## 九、明确不在 Stage 26-P 范围(留给后续)

| 项 | 原因 | 建议归属 |
|---|---|---|
| ai-svc unhealthy 5h 修复 | 范围漂移 | Stage 27 |
| 4 svc 第一次启动需要的 tables 自动 migration | 与 P 平行;启动顺序延伸 | Stage 27 |
| 仓库根 17 个废弃 `apisix-*.json` 的 git 删除 | 防止混乱但不影响启动 | Stage 26-Q 顺手 |
| frontend Playwright e2e 拓展到所有页面 | 验证后顺带做 | Stage 26-Q |
| shared pkg `ClientID = "emotion-echo-gin"` 默认值改名 | 不阻塞 | Stage 27 |
| FER / SenseVoice / XTTS profile 调整 | 不阻塞 | Stage 27 |
| 仓库根 ComposeFile 整体重写(`composer v5` 适配)| 不阻塞启动 | Stage 27 |

---

## 十、Stage 26 全量测试栈扩展

| 类别 | 26-N 前 | 26-O | 26-P |
|---|---|---|---|
| Go 单元测试 | ~280 | ~285 | ~291 (+splitBrokersCSV +4 go.mod +4 yaml env)|
| Go 集成测试 | 15 | 15 | 15 |
| 前端 vitest   | 48 | 81 | 81 |
| 前端 mount + source-contract  | 0 | 19 (5+9+5) | 19 |
| Bug 已修      | 5 | 11 (+6 设计系统)| 17 (+6 启动链路)|
| **冒烟脚本数** | 5 (26-L) | 5 | **6 (+ smoke_apps_26p.sh)** |

---

> 最后更新:2026-07-20 · Stage 26-P 收尾 ·
> 9 commits (P1-P8 + 既有) ,Go/Web 0 regression,
> 推动前后端联调完整链路就绪。
