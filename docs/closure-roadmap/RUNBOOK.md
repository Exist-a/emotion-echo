---
status: active
priority: high
created: 2026-10-13
type: execution-protocol
inherits: ../e2e-roadmap/RUNBOOK.md（纪律来源；本文件为精简自包含版）
---

# 「闭环」阶段执行协议（CL RUNBOOK）

> 本文件是**执行层的单一事实源**：拿到某阶段的 `plan.md` 后按这里的固定动作执行与验收。
> **继承** `docs/e2e-roadmap/RUNBOOK.md`（Lane E）的纪律；为防 E2E 目录归档后失效，关键条款在此复述。

---

## 0. 文件地图

| 文件 | 角色 | 何时读 | 何时写 |
|------|------|--------|--------|
| [roadmap.md](roadmap.md) | 阶段总表 + 排序 + 完成判据 | 每次开工前 | 阶段状态变更时 |
| [RUNBOOK.md](RUNBOOK.md) | 执行协议（本文件） | 每次开工前必读 | 协议演进时 |
| [parallel-contract.md](parallel-contract.md) | 并行执行契约（轨道 / 文件面隔离 / 占位登记） | **并行开工前必读** | 登记 / 注销占位时 |
| [decisions.md](decisions.md) | 本阶段决策（CL-D-xx） | 涉决策时 | 新决议产生时 |
| [discovered-unresolved.md](discovered-unresolved.md) | 发现账本（CL-F-xx） | 记录范围外发现时 | 每次有新发现 |
| `stages/<cl-NN-slug>/plan.md` | 该阶段详档 | 开工前 | 阶段启动前撰写（just-in-time） |
| `stages/<cl-NN-slug>/report.md` | 该阶段执行记录 | 收口时 | 执行中与收口时 |

---

## 1. 状态机

```
pending ──(开工)──> in-progress ──(判据全过 + 回归钉)──> done
                        │
                        └──(被决策门阻塞)──> blocked ──(决策落定)──> pending
```

**开工前置检查（四项全过才允许改 `in-progress`）**：

1. `depends-on` 列出的阶段状态均为 `done`
2. 该阶段不在决策门阻塞列表（roadmap §5）
3. `stages/<cl-NN>/plan.md` 存在且"前置条件"满足
4. **已建立阶段 PR（draft）**，且链接登记在 `parallel-contract.md §三` 占位行（**2026-10-14 起强制：先开 PR 再开工**；登记本身按 H2 走独立小 PR 先落 main）

> 端侧 CL-10 为 **deferred 独立轨**（本阶段先不做），不占用主线 `in-progress` 名额；主线一次只允许一个 `in-progress`。

---

## 2. 环境准备（固定动作）

```bash
cd deploy && docker compose \
  -f docker-compose.infra.yml -f docker-compose.apps.yml -f compose.dev.yml \
  --env-file .env.local --profile dev up -d
```

- 🔴 **`--env-file .env.local` 不可省略**（LLM key 唯一存放点，gitignored）；**严禁删除/清空/覆盖 `.env.local`**（AGENTS.md §四红线）。
- 🔴 **`--profile dev` 不可省略**（Nacos 在该 profile 下才起；缺它 6 应用服务注册失败）。
- ⚠️ **前端用本地 `pnpm dev`**（非容器预构建产物），改代码即时生效。
- ⚠️ **验收前必查服务注册齐全**（期望 count:6）：`curl -s "http://localhost:8848/nacos/v1/ns/service/list?pageNo=1&pageSize=50&namespaceId=emotion-echo-dev"`。缺服务 → `docker restart emotion-echo-<svc>`。
- ⚠️ **改完 Go 必须重建镜像再验收**（否则 curl 仍是旧行为）；改 proto 必须复现原生成方式（`protoc 32.1` + `protoc-gen-go v1.36.11` + `protoc-gen-go-grpc v1.6.2`，以文件名调用）。
- ⚠️ **改 seed/路由后须在 BFF 起后重跑 `apisix-seed`**（否则 upstream 节点 TCP 为空 → 502）。

### 2.1 在 `git worktree` 里起栈的安全做法（2026-10-16 收口 `CL-F-07`）

> `git worktree` **只物化 tracked 文件** ⇒ `deploy/` 下**全部 gitignored 运行时文件都不在** worktree 里。
> 直接照 §2 起栈会**当场报错中断**（loud），但**绕错的解法会把 loud 失败换成 silent 降级**（更危险）。

| # | 缺什么 | 症状 | **正确**做法 | ⛔ 错误做法（会造成未被发现的危害） |
|---|---|---|---|---|
| W1 | `deploy/.env.local` | 服务级锚点 `env_file: [".env.local"]` 起栈**直接报错中断** | 从**共享主工作区**把 `deploy/` 下 gitignored 运行态文件**链/拷**进 worktree（gitignored，不污染版本库） | 用 `--env-file` 指向别处来"绕开报错" —— **服务级 `env_file` 仍然缺**，而 `LLM_API_KEY` 是"只从 `env_file` 读、不从 host 覆盖"（`deploy/docker-compose.apps.yml:329/:405`）⇒ **静默降级 mock**，"聊天链路看着通了其实在跟桩说话" |
| W2 | `deploy/tls/`（`ca.crt` / `ai-client.crt` / `ai-client.key`） | `docker build ai-svc` **直接失败**（Dockerfile `COPY deploy/tls/ai-client.key` → `not found`） | 同上，从共享目录 `cp -r deploy/tls` 到 worktree | 改 Dockerfile 取消 COPY（**改坏生产构建**） |
| W3 | `deploy/.devmode-session` | dev 栈归属锁**看不见** ⇒ 各会话 cwd 不同、互不可见 ⇒ **可静默双占** | 取锁/释锁**以共享目录为准**（`<共享目录>/deploy/.devmode-session`），并在契约 §3.1 播报 | 各自在自己 worktree 建锁文件（等于没锁） |
| W4 | 构建上下文 | `docker compose build` 的上下文**按 compose 文件位置解析** ⇒ **worktree 里改的源码无法直接构建成镜像**（会构建到共享目录的旧代码） | **两步**：① 在 worktree 里 `build`（用带轨后缀 tag，见契约 §1.3 R1）；② 到**共享目录** `up` | 在 worktree 里 `up --build` 就验收（跑的是共享目录的旧代码 —— E2E-F-70 同型陷阱） |

> 🔴 **判真"起栈成功"的门槛**：Nacos `count:6`（§2）+ 镜像新鲜度（`scripts/check_image_freshness.sh`，见其 §8 判据限制）
> **+ 一次真实的 LLM 往返**（不是 mock 回显）。

### 2.2 依赖后起 / 服务恒 503 时的自愈（2026-10-16 收口 `CL-F-16`）

**问题（CL-F-16 控制实验实测，证据 `stages/cl-09-backlog-paydown/evidence/clf16-readiness-selfheal-ab.txt`）**：
服务若在 **Postgres 就绪之前**启动，会带 nil repo **降级启动**；此后 **不会自愈** ——
postgres 变 healthy 后继续观察 **300s，`/health/ready` 恒 503**（只有 `docker restart` / `--force-recreate` 才好）。

**修复（2026-10-16，Lane-CL-P）**：三条线同时收：

| # | 线 | 行为 |
|---|---|---|
| ① | **启动期有限次退避重试** | user-svc 也接上共享 `dbconnect.ConnectWithRetry`（原本是裸单次连接）；与 ai / analytics / assessment 同范式。盖过"依赖稍后就绪"的启动竞态 |
| ② | **strict 模式 fail-fast** | 重试耗尽后 `ShouldFailFast() && IsRequired("postgres")` ⇒ **退出启动**，交给 compose 的 `restart: on-failure` 重试 ⇒ 依赖就绪后**自动恢复**（实测：postgres 起来后 ~30s 内 `/health/ready=200`） |
| ③ | **dev（默认非 strict）保持降级启动** | 仍有 `/health/ready` 如实报 `degraded`（D-29 契约），但**不自愈** ⇒ 需运维在依赖就绪后重启本服务 |

**怎么开 strict（生产/想自愈时）**：给容器设 `STARTUP_STRICT=true` + `STARTUP_STRICT_DEPS=postgres`
（与既有 `ShouldFailFast`/`IsRequired` 机制同一套 env）。**⚠️ 目前 compose 未把这些 env 传给任何容器** ⇒
机制默认休眠；生产起栈时应显式下发（`deploy/compose.prod.yml` 侧）。

**运维速查**：
```bash
# 谁在降级？
docker ps --filter health=unhealthy --format '{{.Names}}	{{.Status}}'
docker exec emotion-echo-<svc> wget -qO- http://localhost:<port>/health/ready   # 看 status/dbOk
# 依赖已就绪后仍 503 ⇒ 重启该服务（服务侧无自愈时这是唯一正解）
docker restart emotion-echo-<svc>
```
> + **一次真实的 LLM 往返**（不是 mock 回显）。

---

## 3. 执行循环（六步）

1. **实测**：用真实浏览器（IAB / 用户机器）走目标功能面，不预设"有没有问题"。
2. **记账**：范围外发现 → `discovered-unresolved.md`（CL-F-xx，6 列：编号/来源/现象/根因/归属/状态）。
3. **写 RED**：为要修的行为写**会失败**的回归钉（Playwright / vitest / Go）。
4. **GREEN**：最小实现让钉子转绿（TDD）。
5. **取证据**：命令+输出/退出码/截图（**"已创建"不算证据**）。
6. **收口**：过 §收口契约 + 状态改 `done`（由审计/第二方判定）。

---

## 4. 判定分级 + 证据有效性

| 标记 | 含义 | 谁来判 |
|---|---|---|
| `[A]` | 自动可判（curl/DB/断言） | 执行者 |
| `[V]` | 视觉（需人看截图） | 执行者截图 + 人核 |
| `[M]` | 需用户裁定 | **仅用户** |

- 结果四值：`PASS / FAIL / BLOCKED / N/A`，各附证据。
- **证据 = 执行输出**（命令 + 输出/退出码/截图）。以下**不算证据**：只说"已创建/已新增/已配置"、只给文件路径、只给 commit hash。
- `BLOCKED` 超 1/3 不得判 `done`。禁止用 `N/A` 掩盖未做。

---

## 5. 账本契约（CL-F-xx）

- 只记**范围外**发现（属当前阶段的就地修，不入账）。
- 6 列硬契约：`编号 | 来源 | 现象 | 根因 | 归属 | 状态`；编号连续、只增不删（已解决标 ✅ 保留）。
- 归属写**未解决的**主责阶段/专项；写"待定"须附触发条件。

---

## 6. 收口契约（每阶段 done 前逐条过）

1. plan.md 的每个测试点都给四值之一 + 证据
2. 回归钉已落地且绿
3. `report.md` 用模板（起点 / 实施 / 证据 / 结论 / 经验）
4. 账本对账：属本阶段未解决的 `CL-F-xx` 存在 ⇒ 只能 `partial`，不得 `done`
5. 落地计划迁 `docs/legacy-plans/landed/`（同步修链接）
6. roadmap.md 状态同步；分支/工作区收口（AGENTS §2.5）
7. 第二方核对（过渡期）通过

---

## 7. 反模式底线（详见 `../e2e-roadmap/anti-patterns.md`）

- ❌ 假 PASS（无证据 / 拿"已创建"当证据）
- ❌ 报告与代码相反；账本与状态脱钩
- ❌ 用 `N/A` / `BLOCKED` 掩盖未做
- ❌ 根因臆断（未做对照实验就下结论）
- ❌ 架构改动无 ADR；TDD 倒置；孤儿产出物
- ❌ 执行者自行宣布 done
