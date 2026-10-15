# CL-02 · Phase 2/6 运行时验证证据（🔴 需 dev 栈 · 2026-10-15）

> 采集时间：2026-10-15 06:36–07:40 · C 轨（Lane-CL-C）· 基线 = 本分支 HEAD（已 merge `main@ef4887f`）
> 栈锁：本轨 06:36 取锁（`.devmode-session` owner=`lane-cl-c`），**用毕释放**。
> 镜像策略（H5/条款10）：一律构建为 **`:cl02-verify`** 后缀 tag，**不覆盖公共 tag**；`up` 只指名自己的服务。

---

> 本文档步骤的**可复跑脚本**：[`cl02-up-verify.sh`](cl02-up-verify.sh)（起栈 → 等 healthy → 复核镜像归属 → 重跑 apisix-seed → Nacos 计数 → env 检查）。
> 镜像 tag 覆盖定义：[`deploy/docker-compose.cl02-verify.yml`](../../../../deploy/docker-compose.cl02-verify.yml)。
> 脚本路径**可用环境变量覆盖**：`CL02_WT`（默认由 `git rev-parse --show-toplevel` 推导）/ `CL02_SHARED`（默认取 `$CL02_WT/../Emotion-Echo`）。（第二方核对 F-7 指出原为硬编码绝对路径，已改为可覆盖；二轮 N-2 指出本警告当时未同步，已更正。）

## 1. 做了什么（H6 两步法）

```console
# ① worktree 构建镜像（context = worktree 根 ⇒ 验的是本轨代码）
$ cd <worktree> && docker compose -f deploy/docker-compose.infra.yml -f deploy/docker-compose.apps.yml \
    -f deploy/compose.dev.yml -f deploy/docker-compose.cl02-verify.yml \
    --env-file <共享目录>/deploy/.env.local build <7 个服务>
→ user-svc / chat-svc / analytics-svc / ai-svc / assessment-svc / web-bff / llm-service 均 Built

# ② 共享目录 up 容器（用本轨 tag 重建自己的服务）
$ cd <共享目录>/deploy && docker compose ... -f <worktree>/deploy/docker-compose.cl02-verify.yml \
    --env-file .env.local --profile dev --profile ai up -d --force-recreate --no-deps <同上>
```

**H5 复核（跑的是不是我的代码）**：

```console
$ docker ps --format '{{.Names}}|{{.Image}}' | grep -E "svc|bff|llm"
emotion-echo-ai-svc|emotion-echo/ai-svc:cl02-verify
emotion-echo-analytics-svc|emotion-echo/analytics-svc:cl02-verify
emotion-echo-assessment-svc|emotion-echo/assessment-svc:cl02-verify
emotion-echo-chat-svc|emotion-echo/chat-svc:cl02-verify
emotion-echo-user-svc|emotion-echo/user-svc:cl02-verify
emotion-echo-web-bff|emotion-echo/web-bff:cl02-verify
emotion-llm-service|emotion-echo/llm-service:cl02-verify
```

## 2. 过程中的 5 个真问题（含 2 个新发现）

| # | 现象 | 根因 | 处置 | 性质 |
|---|---|---|---|---|
| P1 | 首次 `up` 后服务**全部 DNS 失败**：`lookup emotion-echo-postgres on 127.0.0.11:53: no such host` | **基础设施容器已被整个移除**（`docker ps -a` 只剩 7 个；宿主盘 41G 可用，**不是盘满**）⇒ 我到达时栈其实已被拆掉 | 按 RUNBOOK §2 带 `--profile dev` 拉整栈 | 环境（**非本阶段引入**） |
| P2 | `ai-svc` 镜像构建失败：`"/workspace/deploy/tls/ai-client.key": not found` | **worktree 缺 gitignored 的 `deploy/tls/`**（Dockerfile 要 COPY 进镜像）⇒ **CL-F-07 的新实例（构建期 COPY 失败）** | 从共享目录 `cp -r deploy/tls` 到 worktree（gitignored，未污染版本库） | 🆕 新发现（CL-F-07 补充） |
| P3 | `chat-svc` 构建 `RUN go mod download` exit 1 | 瞬时网络（go.mod 未变；Dockerfile 把源码 COPY 放在 download 之前 ⇒ 每次构建都重新下载模块，天然易受网络抖动影响） | 重试即过（rc=0） | 环境 |
| P4 | 重建后 3 个服务 **readiness 恒 503**（`/health/ready` → 503） | 它们在 **postgres 尚未就绪时启动**：DB 连接是**单次尝试**，失败后 repo 保持 nil（`healthlogic.go` 注释自述该设计）⇒ 之后不会自愈 | 在 infra 就绪后 `--force-recreate` 这三个服务 ⇒ 6/6 healthy | 环境（已知设计） |
| P5 | `xtts` 不在默认 profile ⇒ 一度缺 `--profile ai` | xtts 由 `profiles: ["ai"]` 门控（E2E-F-210/D-50 语境） | 补 `--profile ai`（回到 19 容器基线） | 环境（已知） |

> **P4 值得记一笔**：`/health/ready` 是**启动期快照**语义（DB 连不上 ⇒ 永久 degraded），不是持续探测。
> 因此"重建服务"必须**在依赖就绪之后**做，否则会得到一串假 unhealthy。

## 3. 测试点结果

### T-21 清除/归档后全栈起得来 + 服务注册齐全 —— ✅ PASS

```console
$ docker ps --format '{{.Names}}|{{.Status}}' | grep -E "svc|bff|llm"
emotion-echo-ai-svc|Up (healthy)          emotion-echo-assessment-svc|Up (healthy)
emotion-echo-analytics-svc|Up (healthy)   emotion-echo-chat-svc|Up (healthy)
emotion-echo-user-svc|Up (healthy)        emotion-echo-web-bff|Up (healthy)
emotion-llm-service|Up (healthy)                        # 7/7 healthy

$ curl -s "http://localhost:8848/nacos/v1/ns/service/list?...&namespaceId=emotion-echo-dev"
{"count":6,"doms":["emotion-echo-chat-svc","emotion-echo-ai-svc","emotion-echo-assessment-svc",
                   "emotion-echo-analytics-svc","emotion-echo-web-bff","emotion-echo-user-svc"]}

$ docker logs emotion-echo-apisix-seed | tail -1
[seed] seed complete: 6 upstreams + 16 routes (1 catch-all + 5 health + 9 auth-whitelist + 1 self-health)

$ curl -o /dev/null -w "%{http_code}" -X POST http://localhost:19080/api/v1/auth/login -d '{"username":"echo","password":"echo123"}'
200
```

**关键含义**：清除 3 条 RPC + 重生成 Go/Python 桩之后，**整栈仍能起来、网关仍能路由、登录仍可用** ⇒ "删除没有破坏装配面"。

### T-23 §2.4 数据契约 smoke —— ✅ PASS（24/24，exit 0）

```console
$ python scripts/smoke_data_layer.py ; echo rc=$?
汇总: 24/24 PASS, 0 FAIL
rc=0
```

**冷启动 504 已排除为回归**（首跑 22/24，唯一功能性失败是 §4 `/reports/daily` 504
`downstream: analytics dailyReport: timeout: context deadline exceeded`；**重跑即 PASS** ⇒
符合账本 **E2E-F-115**「重启后首拍 5s DeadlineExceeded，重试即可」的已知形态）。

### T-15 运行时 `*_TRANSPORT` env —— ✅ PASS（静态 + 运行时两半齐了）

```console
$ for c in user chat analytics ai web-bff; do docker inspect emotion-echo-$c-svc ... grep _TRANSPORT; done
（5 个服务全部**未设置** *_TRANSPORT ⇒ 走 config.go 默认分支 = grpc）
```

### 功能点检：本阶段删/归档的 RPC 所在链路（额外取证，非测试点）

```console
$ curl -X POST http://localhost:19080/api/v1/auth/logout
200                                  # Logout RPC 已清除，登出仍由 cookie 清理承担（语义未变）

$ curl -X POST .../api/v1/auth/security-questions -d '{"username":"echo"}'
200 {"code":0,"data":{"questions":[]},"message":"ok"}      # 找回密码链路（在用路径）

$ curl -X POST .../api/v1/auth/verify-security-answer -d '{"username":"echo","questionOrder":1,"answer":"wrong"}'
401 "security answer verification failed"                  # 预期拒绝（echo 无密保答案）
```

⇒ **D4 裁定的前提得到运行时佐证**：找回密码实际走 `…ByUsername`（保留），被我清除的按-userID 版确属族内冗余。

## 4. 顺带修掉一处"栈与代码不一致"

`smoke §8 镜像新鲜度` 报 **assessment-svc STALE**：B 轨合并（`b1f13dc`）**确实改了** `assessment-svc/main.go`（+6 行），
而其运行镜像还是 2026-10-12 的 ⇒ 这不是 CL-F-08 类假红，是**真陈旧**。
本轨虽未改 assessment-svc，仍把它一并重建为 `:cl02-verify` 并重跑 smoke ⇒ 24/24。

## 5. 未做 / 边界（诚实声明）

- `emotion-echo-web`（前端）容器状态 `created`（未启动）⇒ smoke §8 对它的新鲜度检查 `SKIP`。前端按 RUNBOOK 用 `pnpm dev` 或另起，与本阶段无关。
- 本次验证**未覆盖** obs profile（prometheus/grafana/loki/sw-ui 等）：本阶段不涉观测面（B 轨已验）。
- 归档 4 条 RPC **不因本次验证变为可用**：`StreamMessages` 仍返 `Unimplemented`，三条 mental-health 的 HTTP 路由仍未接出（D1/D2 语义就是"明确不做"）。
- 栈用毕**已释放锁**（`deploy/.devmode-session` 删除），队列下一位 = D（CL-05）。
