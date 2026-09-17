---
status: active
priority: high
created: 2026-09-17
type: e2e-execution-protocol
---

# E2E 阶段执行协议（RUNBOOK）

> 本文件是**执行层的单一事实源**：大模型（或人）拿到某个阶段的 `plan.md` 后，按本协议的固定动作执行与验收，不依赖外部 skill 是否加载。
> 方法论（为什么这么做）见全局 skill `e2e-stage-testing`；本文件只讲**怎么做、做到什么算完成**。

---

## 0. 文件地图

| 文件 | 角色 | 何时读 | 何时写 |
|------|------|--------|--------|
| [roadmap.md](roadmap.md) | 阶段总表 + 状态 + 决策门 | 每次开工前 | 阶段状态变更时 |
| [RUNBOOK.md](RUNBOOK.md) | **执行协议（本文件）** | 每次开工前必读 | 协议演进时 |
| [decisions.md](decisions.md) | 改造项决议 | 阶段涉改造时 | 新决议产生时 |
| [discovered-unresolved.md](discovered-unresolved.md) | 发现账本 | 记录范围外发现时 | 每次有新发现 |
| `stages/<e2e-NN-slug>/plan.md` | 该阶段详档 | 开工前 | 阶段启动前 1 阶段时撰写 |
| `stages/<e2e-NN-slug>/report.md` | 该阶段执行记录 | 收口时 | 执行过程中与收口时 |
| `stages/<e2e-NN-slug>/screenshots/` | 截图证据 | 收口核对时 | 执行中 |

---

## 1. 状态机

阶段状态取值与转移（**唯一权威在 roadmap.md 的表格里**）：

```
pending ──(开工)──> in-progress ──(DoD 全过)──> done
                        │
                        └──(被决议阻塞)──> blocked ──(决策落定)──> pending
```

**开工前置检查（三项全过才允许把状态改成 in-progress）**：

1. `depends-on` 列出的阶段状态均为 `done`
2. 该阶段**不在决策门的阻塞列表中**（见 §9）
3. `plan.md` 存在且其"前置条件"表全部满足

**当前激活阶段指针**：roadmap.md 顶部「当前激活阶段」一节。一次只允许一个 `in-progress`。

---

## 2. 环境准备（固定动作）

### 2.1 启动 dev 模式

```bash
cd deploy && docker compose \
  -f docker-compose.infra.yml -f docker-compose.apps.yml -f compose.dev.yml \
  --env-file .env.local up -d
```

> 🔴 **`--env-file .env.local` 不可省略**。该文件是 LLM key 唯一存放点（gitignored），不带它容器会静默降级 mock。**严禁删除/清空/覆盖 `.env.local`**（AGENTS.md §四红线）。

### 2.2 健康检查（必须全绿才继续）

```bash
docker ps -a --filter "name=emotion-echo" --format "table {{.Names}}\t{{.Status}}"
docker inspect emotion-echo-db-migrate --format '{{.State.ExitCode}}'   # 期望 0
```

期望：6 个应用服务（user/chat/analytics/assessment/ai/web-bff）+ web **全部 healthy**；`db-migrate` / `apisix-seed` / `minio-init` 为 `Exited (0)`（一次性容器，退出码 0 即正常）。

> 容器数 < 14 或存在 `unhealthy` → 停止执行，先诊断环境（属环境问题而非被测 bug）。

### 2.3 前端与网关可达性

| 入口 | 地址 | 说明 |
|------|------|------|
| 应用 | `http://localhost:3000` | 若 Chrome 拒绝，改用 `http://127.0.0.1:3000`（CORS 已支持双 host） |
| 网关 | `http://localhost:19080/api/v1` | APISIX 宿主映射 |
| BFF 直连 | `http://localhost:8894` | dev 覆盖项开放 |

### 2.4 已知环境坑（执行时先对照）

| 坑 | 表现 | 应对 |
|----|------|------|
| 未带 `--env-file .env.local` | AI 回复走 mock | 重新用带 env-file 的命令起 |
| ai profile 未启用 | FER/SenseVoice/XTTS 容器不存在（镜像约 18GB） | 多模态/数字人/TTS 阶段需显式启用 profile |
| Playwright 未设 `BASE_URL` | 自动起 `pnpm dev`，假设后端已起 | 前后端都要起；后端用 §2.1 |
| uv `workers: 1` | 用例串行，慢是正常的 | 不要为提速改成并行 |
| dev 覆盖项与 prod 不同 | `BFF_DEV_RETURN_CODE=1`、`BFF_TRUST_APISIX=true`、CORS localhost | **在 report.md 明确声明"本阶段验证的是 dev 配置"**，prod 差异归 E2E-25/29 |
| 验证码回显 | dev 下验证码直接出现在响应里 | 测试点若断言"验证码不可见"，须标注为"prod 语义，dev 无法验证" |

---

## 3. 执行循环（固定六步）

| 步 | 动作 | 产出 |
|----|------|------|
| 1 | 读 `plan.md`，核对 §1 前置检查 | 状态改 `in-progress` |
| 2 | 按 §2 起环境 + 健康检查 | 环境基线记录 |
| 3 | **IAB 实测**：逐条跑 `plan.md` 的测试点清单 | 每条一行结果 + 证据 |
| 4 | **发现分类**：范围内 → 修复队列；范围外 → 账本 | 分类清单 |
| 5 | **修复**：范围内 bug 走 TDD（Red→Green→Refactor），修后复测该点 | commit 序列 |
| 6 | **收口**：写回归钉 + 按 §7 更新文档 | report.md + 状态更新 |

### 步骤 3 的 IAB 实测细则

- 用内置浏览器（browser-use `control-browser`）**黑盒**操作：只模拟真实用户动作，**禁止注入 JS 改页面状态**
- 元素定位必须基于实际观测（DOM snapshot / 截图），**禁止猜 selector**
- 每个测试点必须**双重验证**：
  - **代码验证**：只读断言（DOM snapshot、`page.evaluate` 只读取值、网络响应）
  - **视觉验证**：截图且**截图必须被查看**
- 截图命名：`screenshots/<测试点编号>-<简述>.png`（如 `03-session-restored-after-reload.png`）

---

## 4. 测试点判定分级（验收的客观化）

`plan.md` 的每个测试点标注判定方式，**验收时必须按标注方式产出证据**：

| 标记 | 含义 | 判定依据 | 谁判定 |
|------|------|---------|--------|
| `[A]` | **自动可判** | 有布尔结果：HTTP 状态码、`context.cookies()` 字段、DB 查询结果、断言输出、退出码 | 执行者可自证 |
| `[V]` | **视觉判定** | 需要"人眼看待是否对"：布局正确、文案清晰、无破版 | 需截图 + 视觉审查 |
| `[M]` | **需人工/设计裁定** | 结果无客观对错，需产品/设计判断（如"体验是否流畅"） | 必须升级给用户 |

**每条测试点的结果只允许四种取值**，写入 report.md：

| 结果 | 含义 | 后续动作 |
|------|------|---------|
| `PASS` | 断言通过且证据齐备 | — |
| `FAIL` | 断言失败，确认是产品缺陷 | 进入分类（步骤 4） |
| `BLOCKED` | 因环境/前置/决策未定而无法执行 | 记明阻塞原因，按 §8 处理 |
| `N/A` | 该点在当前配置下不适用（如 dev 无法验证 prod 语义） | **必须写明理由**，不允许裸标 N/A |

> 禁止用 `N/A` 或 `BLOCKED` 掩盖未做的工作。`BLOCKED` 的测试点数 > 该阶段总数的 1/3 时，**不得判该阶段 done**。

---

## 5. 发现分类与账本写入契约

实测中每条 `FAIL` 必须被分类，**分类存疑时倾向于记账本而非顺手修**：

| 分类 | 判据 | 动作 |
|------|------|------|
| **范围内** | 与本阶段 `plan.md` 的"做"直接相关 | 进修复队列（步骤 5） |
| **范围外** | 属其他阶段的功能/支撑模块 | 只写账本，**不修** |

### 账本写入格式（追加到 `discovered-unresolved.md` 的表格）

```
| E2E-F-NN | <来源：阶段号 + 实测> | <现象：一句话，可复现> | <根因：有证据的推断，无证据写"待查"> | E2E-XX | 🔴 未解决 |
```

规则：
- 编号**连续递增**，不跳号、不复用
- 现象必须**可复现**（写清操作步骤或接口）
- 根因未查明时写「待查」，**禁止臆断**（ADR-18 的失真分类之一就是"根因臆断"）
- 账本**只增不删**；已解决项改状态保留行

---

## 6. 修复纪律

1. **TDD 强制**（AGENTS.md 第一性原则）：先写会失败的测试（Red）→ 最小实现（Green）→ 重构（Refactor）
2. **禁止修改测试行为让其通过**；禁止 `t.Skip` 跳过写不出的测试
3. 每个修复独立 commit，前缀按 AGENTS.md：`fix:` / `feat:` / `refactor:` / `test:`
4. commit message **末尾必须写调研依据**（读了哪些文件/ADR/smoke 输出）
5. 修复后**必须用 IAB 复测同一测试点**，不能只跑单测

---

## 7. 收口契约（必更新的文件清单）

阶段判 `done` 前，**以下每一项都必须完成**：

| # | 动作 | 位置 | 检查方式 |
|---|------|------|---------|
| 1 | 写 `report.md` | `stages/<本阶段>/report.md` | 存在且按 §10 模板 |
| 2 | 截图归档 | `stages/<本阶段>/screenshots/` | 每个 `[V]` 测试点至少 1 张 |
| 3 | 回归钉 | `emotion-echo-web/e2e/<name>.spec.ts` | 新 spec 文件存在，且**跑过至少一次且绿** |
| 4 | 更新阶段状态 | `roadmap.md` 表格 + 顶部「当前激活阶段」 | 改为 `done`，激活阶段指向下一个 |
| 5 | 更新账本 | `discovered-unresolved.md` | 本次发现已登记（若有） |
| 6 | 更新决策（若涉及） | `decisions.md` | 已决议项改为已决议 |
| 7 | commit + push | `main` | 已推送 |
| 8 | 收口自检三连 | 见下 | 三条命令输出符合断言 |

**收口自检三连**（AGENTS.md §2.5）：

```bash
git status            # 工作树干净（或改动是有意保留的）
git status -sb        # main 与 origin/main 无 ahead/behind
git branch --merged main   # 除 main 外应为空
```

**回归钉的累积策略**：每阶段新增 spec 独立文件（不合并进别人的 spec，避免互相干扰）。阶段收口时跑**全量** `pnpm playwright test`；阶段执行中只需跑本阶段 spec。

---

## 8. 阻塞与升级协议

**允许自行决定并继续**（无需打断用户）：
- 环境重启、容器重建
- 测试数据准备（用 API 造数优先于 UI 造数）
- 范围内 bug 的修复方式选择（在多方案间择优）
- 补充测试点（可追加，不可删减 plan 中的既有测试点）

**必须停下并升级给用户**（写入 report.md 的「待决策」节 + 在回复中明确提出）：

| 场景 | 为什么必须停 |
|------|-------------|
| 测试点被判定为 `[M]` | 无客观对错，需产品裁定 |
| 修复需要**改变已决议的方向**（如 D-01/D-02/D-03） | 属范围变更 |
| 修复会触及**本阶段边界外**的模块 | 防范围蔓延，应记账本 |
| 发现的问题**动摇了后续阶段的依赖假设** | 需重排 roadmap |
| `BLOCKED` 测试点超过 1/3 | 阶段无法诚实收口 |
| 需要外部凭据/权限（如 GitHub token scope） | 执行者无权限 |
| 涉及**破坏性操作**（删数据/删列/删文件）且无回滚 | AGENTS.md 红线 |

> 升级时不要问"要不要继续"，而是**给出结论 + 建议方案 + 需要用户做的具体决定**。

---

## 9. 决策门（哪些阶段当前被阻塞）

**开工前必须核对**：若该阶段在下方列表且决策未落定 → 状态标 `blocked`，不得开工。

| 阻塞项 | 阻塞阶段 | 需谁决定 | 现状 |
|--------|---------|---------|------|
| D-04 i18n 是否立项 | 不阻塞任何阶段 | 用户 | 🟡 候选 |

**已解除的决策门（保留记录）**：

| 阻塞项 | 阶段 | 结论 |
|--------|------|------|
| `users.status` 字段去留 | E2E-06 | ✅ **删**（用户 2026-09-17 确认） |
| 密保方案下注册验证码步骤去留 | E2E-09 | ✅ **删除**（无投递渠道，已用不到） |
| 密保可否跳过 | E2E-07 | ✅ **不可跳过**——唯一门禁；注册时必须设定 |
| GitHub token 缺 `workflow` scope | E2E-03 | ✅ **已具备**（2026-09-17 实测：`x-oauth-scopes` 含 `workflow`；临时分支真实 push `.github/workflows/` 成功） |
| 存量未设密保用户的处理 | E2E-06 | ✅ **选 C 不处理**——数据库现有数据全是无用信息、未上线、测试阶段。改为：更新字段后**新建演示账号**（带密保），并要求该账号**可重跑、可删除** |
| main 分支保护 | E2E-03 阶段 2 | ✅ **安全子集已开**（2026-09-17，经 API）：防强推 + 防删除 + `enforce_admins=true`，实测强推被拒 `GH006`。**status checks / PR 要求仍待 CI 落地后开**（先开会因 check 不上报而卡死） |

> 新增决策门时同步更新本表与 roadmap.md 的对应阶段「边界」列。

---

## 10. report.md 模板

```markdown
---
stage: e2e-NN
title: <阶段名>
executed: YYYY-MM-DD
status: done | partial | blocked
environment: dev 模式（<容器数> 容器 healthy，compose.dev.yml + .env.local）
---

# E2E-NN 执行记录

## 1. 环境基线
- 启动命令：<原样记录>
- 容器状态：<healthy 数 / 异常项>
- 声明的配置差异：<如 BFF_DEV_RETURN_CODE=1 等>

## 2. 测试点结果

| # | 测试点 | 判定 | 结果 | 证据 | 备注 |
|---|--------|------|------|------|------|
| 1 | | [A] | PASS | 断言输出 | |

汇总：PASS x / FAIL y / BLOCKED z / N/A w

## 3. 发现与分类

| 发现 | 分类 | 处理 |
|------|------|------|
| | 范围内 / 范围外 | 修复 commit <sha> / 账本 E2E-F-NN |

## 4. 修复清单（TDD 记录）

| commit | 内容 | 先行的失败测试 |
|--------|------|---------------|
| | | |

## 5. 回归钉
- 新增 spec：`emotion-echo-web/e2e/<name>.spec.ts`（用例数 n，首次运行结果）

## 6. 待决策 / 升级项
<无则写"无">

## 7. 收口自检
- [ ] git status 干净
- [ ] main 与 origin 无 ahead/behind
- [ ] 无残留已合并分支
```

---

## 11. 迭代上限与护栏

| 护栏 | 规则 |
|------|------|
| 单测试点修复迭代 | 最多 3 轮（改 → 复测 → 改）；3 轮未过 → 标 `BLOCKED` 并升级 |
| 单阶段范围蔓延 | 修复项一旦触碰边界外模块 → 立即转账本，不继续 |
| 回归钉失败 | 若新写的 spec 在收口时红 → 不得判 done |
| 破坏性操作 | 删数据/删列/删文件前必须确认有回滚路径；无则先升级 |
| 密钥 | 任何情况下不得把 `.env.local` 内容写入仓库、日志或 commit message |

---

## 12. 命令速查

```bash
# 起环境（必带 env-file）
cd deploy && docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml \
  -f compose.dev.yml --env-file .env.local up -d

# 健康检查
docker ps -a --filter "name=emotion-echo" --format "table {{.Names}}\t{{.Status}}"

# 前端单测
cd emotion-echo-web && pnpm test

# E2E（本阶段 spec）
cd emotion-echo-web && pnpm playwright test e2e/<name>.spec.ts

# E2E（全量，收口时）
cd emotion-echo-web && pnpm playwright test

# Go（某服务）
cd emotion-echo-<svc> && go test ./...

# 数据契约 smoke（E2E-30 用）
python scripts/smoke_data_layer.py

# 文档/配置一致性校验（E2E-05 用）
bash scripts/check_routes_alignment.sh && python scripts/check_view_consistency.py
```
