---
status: landed
stage: 103
title: dev 模式首次启动 + 9 真 bug 修复 + 1 已知未修（聊天发消息无 AI 回复）
date: 2026-09-16
type: dev-mode-launch
source-plan: （无前置 plan；用户原话"启动 dev 模式 + 自己浏览器测一遍 + 记问题修复后我测试"）
phase: 测试找问题修复（从分块开发转为测试驱动阶段）
depends-on:
  - stage-101-multi-round-iteration-closure.md（plan §十四修订：23 项全部落地）
  - stage-102-round-4.4-closure.md
related-stages:
  - stage-74-tech-debt-closure-2026-09-12.md（web Dockerfile 锁版本原方案）
  - stage-92-kafka-sw8-propagation-2026-09-14.md
  - stage-93-analytics-svc-sw8-propagation-2026-09-14.md
related-adrs:
  - ADR-20（compose 分层 + env profile 策略）
  - 决策 18 §P2-R2-11（user_behavior_events 月分区）
  - 决策 18 §P0-R2-10（web-bff JWT secret 移除默认值）
  - Round 4.7 PR-3 §P2-R2-19（Dockerfile digest pin）
---

# Stage 103 — dev 模式首次启动 + 9 真 bug 修复

> **阶段切换声明**：本 stage 标志着项目从 **分块开发（Stage 1-102 多轮 sprint 收口）** 转入
> **测试找问题修复阶段**。后续 session 的核心工作流变为：
> 1. 启动 dev / prod 链路
> 2. 用 browser-use 或 curl / smoke 跑实际功能
> 3. 发现 bug → 写 RED 测试 → 修 → GREEN → commit push
> 4. 重复 2-3 直到该阶段验收清单全绿
>
> 这与 AGENTS.md §〇"先写测试再写代码"原则一致，只是现在测试驱动的方式是"实测端到端"而非"写单元测试"。

---

## 一、本 session 起点状态（用户原话）

> "启动 dev，你自己先在浏览器里测一遍，然后你记录下问题修复，随后我测试。"

**期望产出**：
1. dev 模式成功启动（PG/Redis/Kafka/Nacos/SkyWalking/APISIX/MinIO + 6 业务 svc + BFF + 前端 + AI profile 可选）
2. agent 用 browser-use 走完主要功能（登录 / 聊天 / 报表）
3. 记录所有发现的问题 + 修复后 commit push
4. 交付清单交付用户接手

---

## 二、启动链路真实状态（2026-09-16 14:23 实测）

### 2.1 infra 9 容器 + 业务 6 svc + BFF + llm-service + web 容器

| 服务 | 状态 | 备注 |
|---|---|---|
| postgres | healthy | 26/26 migration OK（含 a008 第 3 bug 修复） |
| redis | healthy | — |
| kafka | healthy | 6 partition chat-events（Round C 实证） |
| nacos | healthy | web-bff/chat-svc 等 6 业务 svc 已注册 |
| skywalking-{oap,ui} | Up | OAP 没 healthcheck（apm 双臂部署时再补） |
| minio | healthy | MinIOInit 已跑（avatars bucket 已建） |
| etcd | healthy | APISIX 配置后端 |
| apisix | Up | 13 routes 已注册 |
| `emotion-echo-user-svc` | healthy | — |
| `emotion-echo-chat-svc` | healthy | Kafka producer + outbox relay + grpc :8892 |
| `emotion-echo-analytics-svc` | healthy | — |
| `emotion-echo-assessment-svc` | healthy | — |
| `emotion-echo-ai-svc` | healthy | Kafka consumer chat-events |
| `emotion-echo-web-bff` | **healthy** | gRPC :8892 客户端 + JWT auth |
| `emotion-llm-service` | healthy | Python :8000 HTTP + :50051 gRPC |
| `emotion-echo-web` | healthy | 前端 v0.1.0（之前 Stage 74 build 成功的镜像） |
| `emotion-echo-db-migrate` | Exited (0) | 一次性 init 容器，幂等 OK |
| `emotion-echo-apisix-seed` | Exited (3) | 一次性 init 容器，重复 up 时 schema 校验拒（routes 已注册） |

### 2.2 浏览器端到端实测（browser-use）

| 步骤 | 状态 | 备注 |
|---|---|---|
| 浏览器开 `http://localhost:3000` | ✅ 200 | 自动跳转 `/chat/conversation/new` |
| 登录页渲染 | ✅ OK | "情绪回音" 标题 + "用演示账号快速体验" 按钮提示 echo/echo123 |
| 演示账号一键登录 | ✅ OK | 跳转到 `/chat/conversation` 主页面 |
| 聊天页面渲染 | ✅ OK | 侧边栏（对话/心理测验/日报/周报/月报/年报/我的空间/设置）+ 主区域（输入框 + 发送按钮）|
| 发送消息触发 AI 回复 | ❌ **失败** | 13 秒无回复，会话列表仍显示"还没有对话"。BFF 日志只看到 Login 请求，无 ChatCompletion 调用。chat-svc 日志无新请求 |

---

## 三、本 session 修复的 9 个真 bug（按 commit 时间排序）

每个 bug 的修复策略均为 TDD：写 RED 测试 → 改实现 → GREEN → commit push。

### 3.1 a008 partition PK 不含分区键 + 列定义不完整

- **commit**：`b4f0ef0`
- **关联**：决策 18 §P2-R2-11（user_behavior_events 月分区）/ docs/architecture/roadmap.md §当前 open 清单
- **Bug A**：a008 用 `LIKE ... INCLUDING ALL` 复制原表结构，复制了原表 PK `(id BIGSERIAL)`，但 PG 强制要求分区表 PK 必须包含分区键 `occurred_at`（`unique constraint on partitioned table must include all partitioning columns`）
- **Bug B**：a008 显式列定义只列 7 列，漏了原表真实存在的 `properties (jsonb)` / `ip (inet)` / `user_agent (text)` —— 这 3 列由 `deploy/db/02-create-tables-in-schemas.sql:215` 初始化，a002 migration 没文档化（已知漂移）
- **修复**：放弃 `LIKE INCLUDING ALL`，用显式列定义（10 列对齐）+ `ADD PRIMARY KEY (id, occurred_at)` + `UNIQUE (event_id, occurred_at)`（DO $$ 守卫幂等）+ INSERT 显式列名列表
- **测试**：`a008_partition_pk_test.go` 2 个 case（PK 含分区键 / 10 列齐）— 4/4 绿

### 3.2 Compose v5.5.1 跨文件 depends_on 严格校验

- **commit**：`154981b`
- **关联**：ADR-20（compose 分层）/ deploy/configuration.md §1 文件结构
- **Bug**：Docker Compose v5.5.1 对跨文件 `depends_on` 严格校验，要求引用 service 在同一 project；infra + apps 分层架构下业务 svc 引用 infra 的 `nacos` / `postgres` / `kafka` 被拒：
  ```
  service "emotion-echo-assessment-svc" depends on undefined service "nacos": invalid compose project
  ```
- **修复**：apps.yml 业务 svc `depends_on` 仅保留 apps.yml 内服务（db-migrate / llm-service / 业务 svc 互依赖）；跨文件依赖改为运行时检查（NACOS_ENABLED env / POSTGRES_DSN 直连）—— infra 已先 up，dev 启动时 PG/Kafka/Nacos 必然 healthy
- **测试**：`test_compose_v55_cross_file.sh` 9 项断言（config --services 不 bail / 7 业务 svc 不引用 infra service key / dev 链路 config --quiet 退出码 0）

### 3.3 Dockerfile digest ARG 模式 buildkit 拒绝

- **commit**：`9fc9452`
- **关联**：Round D `ded2efc`（P2-R2-19 digest pin）/ docs/evidence/round-d-dockerfile-digest-pin.md
- **Bug**：Round D 把 8 个 Dockerfile 改为 `ARG XXX_DIGEST=""` + `FROM image@${XXX_DIGEST:-image:tag}` —— buildkit 静态分析时把 `@` 后内容当 digest 解析，必须 sha256 literal；`@${VAR:-tag}` 形式被拒，报 `failed to parse stage name "alpine@alpine:3.19"`
- **修复**：标准模式（buildkit 官方支持）：
  ```dockerfile
  ARG XXX_IMAGE               # 无默认
  ARG XXX_DIGEST              # 无默认
  FROM ${XXX_IMAGE:-image:tag}${XXX_DIGEST:+@${XXX_DIGEST}}
  ```
  dev 模式（不传 DIGEST）→ `FROM image:tag`；prod 模式（传 DIGEST=sha256:abc）→ `FROM image:tag@sha256:abc`
- **测试**：`test_dockerfile_arg_pattern.sh` 24/24 绿 + `check_docker_digests.sh` 17/17 仍绿

### 3.4 tini v0.19.0 SHA256 抄写错误

- **commit**：`cc77ea6`
- **关联**：Round 4.7 PR-3（依赖链加固）/ emotion-llm-service/Dockerfile:67
- **Bug**：`TINI_SHA256=93dccd091001205fb6bd1edfaf66f50bc6b054b5b8212ddc6aa4633c12e4c0bb` 是字符级抄写错误（前 6 位 `93dccd` 匹配后续全错）。docker build 报 `FATAL: tini SHA256 mismatch`
- **修复**：用本地 `emotion-echo/llm-service:v0.1.2` 镜像（同样 Dockerfile 历史 build）`/usr/local/bin/tini` 真值 `93dcc18adc78c65a028a84799ecf8ad40c936fdfc5f2a57b1acda5a8117fa82c` 替换。CI 网络可达时应再 verify GitHub release 真值
- **测试**：`test_llm_tini_sha.py` 字面量断言 Dockerfile `ARG TINI_SHA256` 必须等于权威值

### 3.5 pip install tuna 大文件 read timeout

- **commit**：`a5f51e4`
- **关联**：emotion-llm-service/Dockerfile:24-30
- **Bug**：`pip install` 用清华 tuna 镜像，grpcio 7.2MB wheel 在 188s 处 read timeout（沙箱主机 curl PyPI / tuna / 阿里云均 200 OK，属镜像站限速非网络问题）
- **修复**：`pip install --no-cache-dir --default-timeout=120 --retries 5`，加 `PIP_INDEX_URL` / `PIP_TRUSTED_HOST` build-arg 默认清华 tuna，可覆盖为阿里云
- **测试**：本 session 未写专门测试（build 实测验证）

### 3.6 本地重新生成完整 package-lock.json

- **commit**：`de58755`
- **关联**：QUICKSTART §"5 秒必读 · 前端是独立进程" / Stage 97 a9888c4 P0-R2-9（npm ci 锁版本）
- **Bug**：Stage 97 改 `npm install` 为 `npm ci --omit=optional` 后没重新生成本地 lockfile，原 lockfile 缺 dev deps + linux-x64-musl optional binding（nuxt 必备）。`npm ci` 报 `package.json and package-lock.json out of sync` + `Cannot find native binding (npm/cli#4828)`
- **修复**：本地 `npm install`（3 分钟 / 828 packages）生成完整 9796 行 lockfile
- **范围**：仅 lockfile 改动。QUICKSTART 说明 dev 模式 web 容器不构建（本地 `pnpm dev` 跳过容器构建，迭代更快；prod 部署走 `emotion-echo-web` 容器 + APISIX）
- **测试**：未写专门测试（lockfile 完整性由 build 实测验证）

### 3.7 a008 partition 重跑自递归守卫

- **commit**：`9c7a5af`
- **关联**：3.1 a008 修复后
- **Bug**：a008 第一次跑按设计走（SELECT 原表 + RENAME 交换 + COMMIT）。第二次跑时 RENAME 已完成，`user_behavior_events` 指向 partitioned 表；`INSERT INTO _partitioned SELECT FROM user_behavior_events` 变成自递归，PG 报 `no partition of relation found for row`
- **修复**：a008 开头 DO $$ 守卫 + `RAISE EXCEPTION` + `\set ON_ERROR_STOP off` —— 检测到已 partitioned 时让事务回滚（避免后续 DDL 副作用），psql 继续到下一文件，migrate.sh 看到 OK 继续 a009
- **测试**：`a008_partition_idempotent_test.go` 1 个 case（守卫三要素：DO $$ + pg_partitioned_table + RAISE EXCEPTION；位置：BEGIN 之后；含 ON_ERROR_STOP off）

### 3.8 web-bff block 缺 BFF_JWT_SECRET env

- **commit**：`c64f690`
- **关联**：决策 18 §P0-R2-10（web-bff JWT secret 移除默认值 fail-fast）/ Stage 32 PR-14 注释（"由 APISIX jwt-auth 统一管理"误导）/ emotion-echo-web-bff/internal/auth/jwt.go:50
- **Bug**：Stage 94 PR-5 §P0-10 删除 web-bff `config.go` 中 `Auth.JWTSecret` 默认值，空字符串触发 `log.Fatal "auth: JWT secret must not be empty"` fail-fast。但 docker-compose.apps.yml web-bff block 历史上只设 `INTERNAL_API_KEY` 等 env，没设 `BFF_JWT_SECRET` —— Stage 32 PR-14 注释说"由 APISIX 统一管理"误导。实测 dev 模式启动 web-bff 反复重启
- **修复**：web-bff block 加 `BFF_JWT_SECRET: ${BFF_JWT_SECRET:-dev-bff-secret}`，与 apisix-seed block（line 699）一致；dev 默认 `dev-bff-secret` 让 APISIX jwt-auth consumer 与 BFF 用同一密钥验签，prod 由 `.env.local` 注入真随机 secret
- **测试**：`test_bff_jwt_secret.sh` 3 项断言（web-bff block 含 env / 默认值非空 / 与 apisix-seed 值一致）

### 3.9 web depends_on apisix-seed 改 service_started

- **commit**：`881e674`
- **关联**：apisix-seed 一次性 init 容器语义
- **Bug**：web 容器 `depends_on apisix-seed: condition: service_completed_successfully`，但 apisix-seed 第二次跑（再次 `docker compose up`）时已注册的 13 routes 重新 PUT 触发 APISIX schema 校验拒绝，seed `exit 3` → web 永远 'Waiting' → frontend 不起
- **修复**：web `depends_on` 改 `service_started`（apisix-seed 启动 = 网络可达 + admin API 可达；routes 状态由 APISIX admin API 决定，不需要 seed 跑完）
- **测试**：未写专门测试（实测验证 + 13 routes curl 确认）

---

## 四、1 个已知未修 bug（用户接手排查）

### 4.1 聊天发消息无 AI 回复

- **复现步骤**：
  1. 浏览器开 `http://localhost:3000`
  2. 演示账号一键登录（echo / echo123）
  3. 进入 `/chat/conversation/new`
  4. 输入"今天心情不太好"，点击发送
  5. 等 15 秒 —— 无 AI 回复，会话列表仍显示"还没有对话"
- **日志证据**：
  - `docker logs emotion-echo-web-bff --tail 20` —— 只看到 `Login` 请求（`emotion_user.v1.UserService/Login`），无 `ChatCompletion` / `ChatService/SendMessage`
  - `docker logs emotion-echo-chat-svc --tail 20` —— 无新请求进入
- **可能根因（session 超时未深入排查）**：
  1. 前端用 localStorage 还是 cookie 存 token 需验证（session 中 evaluate localStorage 读出来空）
  2. BFF `/api/v1/chat/send` 端点路径 / header 可能不对（前端请求没到 BFF 也没到 chat-svc）
  3. 前端 WebSocket vs HTTP SSE 模式可能需要等连接建立
  4. 浏览器 console 错误没捕获（截图能直接看出来）

### 4.2 建议排查路径

```bash
# 1. 浏览器 DevTools Network tab 抓 /api/v1/chat/* 请求
#    看请求是否发出 + 状态码 + request body / headers

# 2. BFF 日志：docker logs emotion-echo-web-bff --tail 50
#    期望看到 [grpc-client] method=/emotion_chat.v1.ChatService/SendMessage

# 3. chat-svc 日志：docker logs emotion-echo-chat-svc --tail 50

# 4. 前端代码：emotion-echo-web/app/pages/chat/conversation/*.vue
#    找发送函数 (submit / sendMessage) + store / composable 调用

# 5. 前端 store：emotion-echo-web/app/stores/chat.ts 或 composables/useAIStreamHandler.ts
#    看 token 是否正确传递 + 请求路径是否对
```

---

## 五、dev 模式启动命令（已固化）

### 5.1 启动

```bash
cd D:/源码/Emotion-Echo/deploy
docker compose --env-file .env.local -f docker-compose.infra.yml -f docker-compose.apps.yml -f compose.dev.yml up -d --no-build
```

### 5.2 验证

```bash
# 1. 容器状态
docker ps --filter "name=emotion-echo" --format "table {{.Names}}\t{{.Status}}"

# 2. APISIX 健康
curl -H "X-API-KEY: WhZEPlrGviCSXlKFfALZlQWinluoGAbj" http://localhost:9180/apisix/admin/routes | python -c "import sys,json; print('routes:', len(json.load(sys.stdin)['list']))"

# 3. BFF 登录
curl -s http://localhost:19080/api/v1/auth/login -X POST -H "Content-Type: application/json" -d '{"username":"echo","password":"echo123"}' | python -c "import sys,json; print('login OK:', 'accessToken' in json.load(sys.stdin).get('data',{}))"

# 4. 前端
curl -s -o /dev/null -w "HTTP %{http_code}\n" http://localhost:3000/
```

### 5.3 收口自检三连

```bash
cd D:/源码/Emotion-Echo
bash scripts/test_compose_v55_cross_file.sh   # 9/9
bash scripts/test_compose_override.sh       # 10/10
bash scripts/test_round4_env_split.sh        # 4/4
bash scripts/test_bff_jwt_secret.sh          # 3/3
bash scripts/test_dockerfile_arg_pattern.sh  # 24/24
python scripts/test_llm_tini_sha.py          # OK
bash scripts/check_docker_digests.sh         # 17/17
cd emotion-echo-analytics-svc/migrations && go test ./...  # 4/4
```

---

## 六、roadmap 阶段切换

### 6.1 之前（stage 101-102 收口）

> **修订后真正 open 总数**：**0 项本轮可启动**（plan §十四修订后的 23 项全部落地）。

### 6.2 现在（stage 103）

> **当前状态**：**测试找问题修复阶段**。9 个真 bug 已修 + commit push，dev 模式启动链路全 healthy，仅剩 1 个聊天发消息未修 bug 需用户接管排查。
>
> **后续 session 工作模式**：
> 1. 重启 dev 模式（`docker compose ... up -d --no-build`）
> 2. browser-use 或 curl 跑实际功能
> 3. 发现 bug → 写 RED 测试 → 修 → GREEN → commit push
> 4. 每轮 session 结束按 AGENTS.md §2.5 三连（git status / status -sb / branch --merged main）+ push

### 6.3 工作流纪律（参考 multi-pr-commit-discipline.md）

- 每个新发现 bug 一个 commit（commit message 含 git log --oneline 实证）
- 修复策略遵循 TDD：写 RED → 写最少实现 → GREEN → REFACTOR
- 文档化所有跨 stage 现象（不只"修完"还要"为什么修、修后影响"）
- 文档偏移立即修（决策 18 §四"结论须带可复现证据"）

---

## 七、附录：commit 完整列表（origin/main）

按时间顺序：

| # | Commit | 类型 | 一句话 |
|---|---|---|---|
| 1 | `133ad1b` | docs(xtts) | v1 retire + v2 单轨化 |
| 2 | `3501686` | test(compose) | Round 4 env split RED |
| 3 | `5932fa1` | feat(compose) | Round 4 GREEN（dev LOG_LEVEL=DEBUG 等） |
| 4 | `b4f0ef0` | fix(migration) | **a008 分区 PK 含 occurred_at + 10 列对齐** |
| 5 | `154981b` | fix(compose) | **跨文件 depends_on 移除（v5.5.1 回归）** |
| 6 | `9fc9452` | fix(dockerfile) | **Round D ARG 模式修复** |
| 7 | `cc77ea6` | fix(llm-svc) | **tini SHA256 抄写错误** |
| 8 | `a5f51e4` | fix(llm-svc) | pip install timeout/retries + PIP_INDEX_URL build-arg |
| 9 | `de58755` | fix(web) | 本地重新生成完整 package-lock.json |
| 10 | `9c7a5af` | fix(migration) | **a008 幂等守卫** |
| 11 | `c64f690` | fix(bff) | **web-bff 加 BFF_JWT_SECRET env** |
| 12 | `881e674` | fix(compose) | **web depends_on apisix-seed 改 service_started** |

---

## 八、引用与后续

- **新加测试脚本**：
  - `scripts/test_compose_v55_cross_file.sh`（commit `154981b`）
  - `scripts/test_dockerfile_arg_pattern.sh`（commit `9fc9452`）
  - `scripts/test_bff_jwt_secret.sh`（commit `c64f690`）
  - `scripts/test_llm_tini_sha.py`（commit `cc77ea6`）
- **新加 Go 测试**：
  - `emotion-echo-analytics-svc/migrations/a008_partition_pk_test.go`（commit `b4f0ef0`，2 case）
  - `emotion-echo-analytics-svc/migrations/a008_partition_idempotent_test.go`（commit `9c7a5af`，1 case）
- **关联 ADR/决策**：
  - ADR-20 compose 分层（3.2 / 3.5 / 3.9）
  - 决策 18 §P2-R2-11（3.1 / 3.7）
  - 决策 18 §P0-R2-10（3.8）
  - Round 4.7 PR-3 P2-R2-19 digest pin（3.3 / 3.6）
- **后续 session 接力清单**：4.1 聊天发消息 bug 排查 + 修复
