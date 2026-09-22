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
| [remediation.md](remediation.md) | R 系列补救排期 + 「判定记录：哪些不补」 | 涉历史欠账时 | 判定变化时 |
| [debt-paydown-plan.md](debt-paydown-plan.md) | **旧账清偿计划**（波次 / 出口断言 / 明确不消） | 消历史欠账前 | 波次推进时 |
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

### 2.1 启动后端容器

```bash
cd deploy && docker compose \
  -f docker-compose.infra.yml -f docker-compose.apps.yml -f compose.dev.yml \
  --env-file .env.local --profile dev up -d
```

> 🔴 **`--env-file .env.local` 不可省略**。该文件是 LLM key 唯一存放点（gitignored），不带它容器会静默降级 mock。**严禁删除/清空/覆盖 `.env.local`**（AGENTS.md §四红线）。

> 🔴 **`--profile dev` 同样不可省略**（2026-09-22 E2E-16 开工实测，E2E-F-108）：Nacos 在 `docker-compose.infra.yml:203` 声明为 `profiles: ["dev"]`，不带该 profile 时 **Nacos 容器根本不会启动** ⇒ 6 个应用服务全部注册失败（analytics-svc 直接 `[nacos] boot failed (fatal)` 崩溃循环，`up -d` 中止）。本文件此前记录的「固定动作」缺此参数，属文档漂移，已按实测更正。

> ⚠️ **启动顺序陷阱**（同上实测 + E2E-F-107）：即便带上 `--profile dev`，若 BFF 先于 Nacos 就绪启动，BFF 日志出现 `[nacos] boot failed (continuing)` 后**永不重试** ⇒ Nacos `emotion-echo-dev` 命名空间里**没有 `emotion-echo-web-bff`** ⇒ APISIX upstream 6（`discovery_type: nacos`）无节点 ⇒ **网关全部 `/api/v1/*` 返回 503**（而 BFF 直连 `:8894` 正常）。
> **验收/测试前必查**（期望 `count:6`）：
> ```bash
> curl -s "http://localhost:8848/nacos/v1/ns/service/list?pageNo=1&pageSize=50&namespaceId=emotion-echo-dev"
> ```
> 缺任何服务 → `docker restart emotion-echo-<svc>` 后复查（重启即重新注册）。**测试点结论以「注册齐全」为前提**，否则会把 503 误判成被测功能缺陷。

### 2.1b 启动前端 dev server（本地优先）

**E2E 测试阶段使用本地 `pnpm dev` 而非 Docker 容器内的预构建产物。** 原因：
- 改代码后无需 rebuild 镜像（`npm run build` 耗时 2-3 分钟）
- dev server 支持 HMR，改完即生效
- SSR 配置变更等需要重新构建的改动可即时验证

```bash
# 1. 停止 Docker 内的前端容器（释放 3000 端口）
docker stop emotion-echo-web 2>/dev/null || true

# 2. 启动本地 dev server
cd emotion-echo-web && pnpm dev --port 3000
```

> ⚠️ **端口冲突**：如果 Docker 的 `emotion-echo-web` 容器仍在运行，本地 dev server 会因 3000 端口被占用而启动失败。必须先 `docker stop emotion-echo-web`。
>
> ⚠️ **API 地址**：本地 dev server 默认读 `nuxt.config.ts` 中的 `NUXT_PUBLIC_API_BASE_URL`（`http://localhost:19080/api/v1`），与 Docker 容器内一致，无需额外配置。

### 2.2 健康检查（必须全绿才继续）

```bash
# 后端容器健康检查
docker ps -a --filter "name=emotion-echo" --format "table {{.Names}}\t{{.Status}}"
docker inspect emotion-echo-db-migrate --format '{{.State.ExitCode}}'   # 期望 0

# 前端可达性检查（本地 dev server 或 Docker 容器均可）
curl -s -o /dev/null -w "%{http_code}" http://localhost:3000/   # 期望 200 或 301
```

期望：后端 6 个应用服务（user/chat/analytics/assessment/ai/web-bff）全部 healthy；`db-migrate` / `apisix-seed` / `minio-init` 为 `Exited (0)`。前端 `http://localhost:3000` 可达。

> 后端容器数不足或存在 `unhealthy` → 停止执行，先诊断环境。前端不可达 → 检查 dev server 是否启动或 Docker web 容器是否运行。

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
| **Docker 重启后端口转发不重注册**（2026-09-22 E2E-16 实测） | Docker Desktop 重启后 `localhost:3000 / 19080 / 8848` 全返 `000`，但容器内部 healthy、容器网络互访正常 | **不必重启 Docker Desktop** —— `docker restart emotion-echo-web emotion-echo-apisix emotion-echo-nacos` 即可恢复宿主端口 |
| **重建任何服务后必须重跑 `apisix-seed` + 重启 BFF**（E2E-16 同型，E2E-F-107） | APISIX upstream 节点是 seed 时解析的**静态列表**；BFF 的 gRPC 连接在 Nacos 上取一次就缓存 | `docker compose ... run --rm --no-deps emotion-echo-apisix-seed && docker compose ... restart emotion-echo-web-bff` |
| **MinIO `localhost:9000` 在 web 容器里不通**（E2E-F-113） | audioUrl 用绝对路径 `http://localhost:9000/...`，宿主的 9000 转发**仅对宿主浏览器生效**；web 容器内 `localhost` 是容器自己（连端口都没有）⇒ `<audio>` 拉不到 metadata | audioUrl 改成 BFF 反代 `/api/v1/voice/audio/:filename`；或 audioUrl 用容器内部 `emotion-echo-minio:9000`（仅 web 容器内生效） |

> **为何这些坑反复出现**：上表中三类坑都是"在 docker 跨视角（宿主 vs 容器 vs web 容器 vs 浏览器）下 host/网络语义不同"导致的——之前 plan §5 测试点只断言"URL 字符串相等"，**没断言"在发起者视角 host 可达"**（E2E-F-113 同型"断言全绿 ≠ 功能可用"）。**下一轮修 D-11 时必须把"发起者视角可达"作为强断言**（见 `stages/e2e-16-multimodal/plan.md` §5 测试点 2/3/4a 的加重条款）。

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

### 4.1 证据有效性（2026-09-18 新增，因 [anti-patterns.md](anti-patterns.md) AP-01/02 真实发生）

**`PASS` 的判据是"行为被验证"，不是"产出物存在"。** 以下写法**一律不算证据**：

| ❌ 无效证据 | 为什么 |
|-----------|--------|
| "脚本**已创建**" / "spec **已完成**" / "配置**已新增**" / "功能**已实现**" | 只证明文件存在，未证明行为正确 |
| "E2E-XX 阶段**已落地**"（跨报告引用） | 上游报告本身可能就是错的；EC-04 测试点 #4 即因此与代码事实相反 |
| 被注释掉的断言（soft-assert，永不失败的 spec） | 永远不可能红，等于没测 |
| `go build ./...` 通过 | **不编译测试文件**；必须用 `go vet ./...`（会编译 `_test.go`）或 `go test -run XXX` |

**有效证据的最低要求**：
- `[A]` 项：可复现的**命令 + 其实际输出片段**（或退出码），且**当场回读被引用的文件**确认（给 `文件:行号`）
- `[V]` 项：截图文件路径（`screenshots/<编号>-<简述>.png`），且**截图必须被查看**
- 涉及"已包含/已接入/已存在"的断言，**必须**给出被引用文件的具体行号

### 4.2 四值边界判据（防止 AP-03）

| 场景 | 正确取值 | 常被误标为 |
|------|---------|-----------|
| 该做的事**没做**（但环境允许） | `FAIL`（或按 §8 升级） | `BLOCKED` / `N/A` |
| 需求被**主动删除/放弃** | `BLOCKED` + 升级给用户批准 + 账本记 `🟡 降级并记录` | `N/A` / 账本标"已解决" |
| 同轮里**别的阶段能做的环境**，本阶段声明不可做 | 不允许——必须做 | `BLOCKED` |
| 当前配置下**语义上不可能验证**（如 dev 验 prod 语义） | `N/A` + 写明理由 | — |

> `BLOCKED` 必须写明**"为什么不可做"**，而不是"我没做"。

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
| **9** | **账本对账**（2026-09-18 新增） | `discovered-unresolved.md` ↔ `roadmap.md` | **属本阶段的每条 `E2E-F-xx` 要么已翻状态，要么在 report 写明"为何仍挂未解决"。存在未解决条目 ⇒ 阶段只能标 `partial`** |
| **10** | **第二方核对**（2026-09-18 新增） | 按 §13.3 清单 | **执行者不得自行宣布 `done`**；过渡期内由非执行者按 §13.3 逐条核对并附结果 |
| **11** | **复读关键断言**（2026-09-18 新增） | report 全文 | 报告中每句"已包含/已存在/已接入"**必须当场回读被引用文件**并在证据列给 `文件:行号`（防 AP-02） |

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

---

## 13. 收口审计（机器校验）—— 2026-09-18 新增

### 13.1 为什么需要这一节

2026-09-18 独立审查发现：**五个已标 `done` 的阶段无一完整满足 §7 收口契约**，错误模式高度重复（见 [anti-patterns.md](anti-patterns.md) 14 类）。

根因是**规范只在"应当"层**：§1-§12 都是文本要求 + 人工闸门，**没有任何机械校验**。执行者可以自己宣布 `done`、自己填 `PASS`、自己打勾自检——**没有任何东西会说"不，你没做到"**。

**因此：约束力必须来自机器，不能来自文本。** 本节定义"什么必须可机械校验"，作为 [remediation.md](remediation.md) R-03 的实现需求输入。

### 13.2 当前状态（2026-09-18）

`scripts/e2e_stage_audit.py` 的 **MVA（最小可用版）已落地**，覆盖 5 条断言（见下表 A1~A5）。剩余 12 条归 R-03。

```bash
# 审计单个阶段 / 全部阶段
python scripts/e2e_stage_audit.py --stage e2e-04
python scripts/e2e_stage_audit.py --all
python scripts/e2e_stage_audit.py --all --json     # 机器可读，供 CI 消费

# 自校验：用本次审查实测的已知缺口做回归样本
# 跑不出 ExpectedLow 说明审计器无效 —— 不得进入下一步
python scripts/e2e_stage_audit.py --selftest
```

**MVA 已实现的 5 条断言**：

| 编号 | 断言 | 反例 |
|------|------|------|
| A1 | `report.md` 存在且含必填章节（缺建议章节为 WARN） | AP-12 |
| A2 | 汇总行非占位符，且 `PASS+FAIL+BLOCKED+N/A` == 测试点表行数 | AP-12 |
| A3 | `plan.md` 的测试点编号集合 ⊆ `report.md` 的编号集合 | AP-07 |
| A4 | 证据列不含存在性措辞；证据**只有文件路径而无执行信号**同样不算 | AP-01/02 |
| A5 | 属本阶段且未了结的 `E2E-F-xx` 存在时，阶段状态不得为 `done`/`partial` | AP-04 |

**设计约束（已实测遵守）**：
- **必须能复现已知缺口**：`--selftest` 对 E2E-03/04/05 检出审查实测的缺陷（未检出即判定审计器无效）
- **不得误报**：对最接近合规的 E2E-02 不产生 FAIL；对 19 个未开工阶段（无详档，符合 just-in-time 约定）不产生 FAIL
- **状态判定不得靠全文关键词**：状态单元格描述瑕疵时会**提到** `BLOCKED` 等词，故按优先级 + 限定措辞判定

**覆盖现状（2026-09-18 R-03 收尾后）**：17 条里 **13 条已机械化**——①~⑦、⑨~⑫ 由 `scripts/e2e_stage_audit.py`（A1~A11）覆盖；⑧ 由 `check_soft_asserts.sh`；⑬ 由 `check_tdd_gate.sh`；⑭ 由 `check_orphan_outputs.sh`；⑮ 由 `check_adr_gate.sh`；⑰ 由 `check_residual.sh`。⑯ 已实现（`scripts/check_required_checks.py`）但**无法在 CI 内执行**——GitHub 未提供"读分支保护"的可授予权限给 `GITHUB_TOKEN`（实测 403），故刻意不放进 CI（只会失败或永远 skip 的 job 是噪声，会训练人忽略 CI）；改为**收口时的人工命令**：`GH_TOKEN=<admin PAT> python scripts/check_required_checks.py`。

**在自动化完全接管前**：人工核对仍按 §13.3 清单执行；**执行者不得自行宣布 `done`**。

### 13.3 必须机械校验的断言清单（17 条；✅ = MVA 已覆盖）

| # | 断言 | 对应反例 |
|---|------|---------|
| 1 | `stages/<阶段>/report.md` 存在，且含 §10 模板的全部必填章节 | AP-12 |
| 2 | report 汇总行**非占位符**（不含 `x`/`?`/`TBD`），且 `PASS+FAIL+BLOCKED+N/A` **等于**表格数据行数 | AP-12 |
| 3 | report 自检项中**不存在 `[x]` + "待…"** 的组合 | AP-12 |
| 4 | report 的"判定"列只含 `[A]`/`[V]`/`[M]`；"结果"列只含四值（可含修饰但基值必须合法） | §4 |
| 5 | plan 里出现的**全部编号项**（测试点 #N、缺陷 AN/BN/CN/DN）在 report 里**一一有结论**（集合包含关系） | AP-07 |
| 6 | 证据列**不含**"已创建/已新增/已配置/已实现/已落地"等存在性措辞 | AP-01 |
| 7 | 全部 `plan` 测试点中判定含 `[V]` 的，`screenshots/` 里**有对应截图**且文件非空 | AP-01 |
| 8 | 不存在被注释掉的断言文件被当作 `[A]` 证据（soft-assert 检测） | AP-01 | ✅ **已机械化**：`scripts/check_soft_asserts.sh`（含负向测试 `test_check_soft_asserts.sh`，5/5 用例；已接入 CI）。已知 1 处登记在 `scripts/soft_assert_allowlist.txt`（E2E-F-51） |
| 9 | 阶段相关每条 `E2E-F-xx` 的状态与 roadmap 的 done 状态**无冲突**（未解决条目所属阶段不得为 done） | AP-04 |
| 10 | `plan.md` / `roadmap.md` / `report.md` 三处的 `status` **一致** | AP-14 |
| 11 | 账本编号**连续无跳号无重复** | §5 |
| 12 | 阶段内所有相对链接**可达**（无坏链） | AP-14 |
| 13 | 改动含生产代码时，同批 commit 含 `_test.go` / `*.spec.ts` 变更 | AP-09 |
| 14 | 新增 `scripts/*` 与 `.github/workflows/*` **被引用**；新增 helper **有调用方** | AP-10 |
| 15 | 命中架构关键词（渲染模式/框架/存储/协议/认证）的改动，commit 含 **ADR 文件 + `decisions.md` 变更** | AP-08 |
| 16 | 报告中引用"CI 会拦/不可 merge"时，`required_status_checks` **非空**（API 可查） | AP-11 | ✅ **已实现**：`scripts/check_required_checks.py`（断言非空 + **不得**把带 `paths` 过滤的 job 列为 required，否则锁死）。**但不在 CI 内自动执行**：读分支保护需管理员权限，默认 `GITHUB_TOKEN` 无权（403）⇒ 列为**收口时第二方核对的必跑命令**（人工理由已明示） |
| 17 | 残留扫描：`*;D` 类空目录、无末尾换行文件、`git status` 之外的未跟踪残留 | AP-14 |

### 13.4 审计器的设计约束

- **必须对历史阶段可重放**：对 E2E-03/04/05 运行时应能**复现本次审查的结论**（作为回归样本）；跑不出已知缺口 = 审计器无效
- **接入 CI 且真能拦**：成为 `required_status_checks`（注意 §9 的 `enforce_admins` 锁死风险）
- **不做"零人工"承诺**：`[M]` 类判定、产品决策、根因判断仍需人
- **审计器自身的失效模式**也要防：`check_docker_digests.sh` 曾因只校验格式而给出**假绿**（AP-11 变体）——审计项必须校验**实质**而非形式

### 13.5 与 §7 收口契约的关系

§7 是**要求**（该做什么），§13 是**验证**（怎么证明做了）。两者必须配套：只加要求不加验证，就是本次审查暴露的问题。

