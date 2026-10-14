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
4. **已建立阶段 PR（draft）**，且链接登记在 `parallel-contract.md §三` 占位行（**2026-10-14 起强制：先开 PR 再开工**）

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
