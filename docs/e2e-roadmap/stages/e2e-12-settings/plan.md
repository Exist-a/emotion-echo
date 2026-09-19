---
stage: e2e-12
title: 设置页
type: transformation
status: pending
created: 2026-09-19
depends-on: [e2e-01, e2e-11]
blocks: []
gate: []            # 见 §6「待确认决策 D-09」——本计划以推荐方案 A 为默认路径，故不设阻塞门
related-findings: [E2E-F-82, E2E-F-36]
---

# E2E-12 设置页

> 详档。执行协议见 [RUNBOOK.md](../../RUNBOOK.md)——执行前必读，本文件只描述"这个阶段测什么"。
> 执行完成后，在同一目录按 [_REPORT_TEMPLATE.md](../_REPORT_TEMPLATE.md) 写 `report.md`。
>
> **建档功课（2026-09-19，按 AGENTS.md「文档撰写前必须做的功课」）**：
> 已读实现：`emotion-echo-web/app/pages/chat/setting.vue`、`app/stores/user.ts`、`app/types/userConfig/userConfigType.ts`、`app/types/api.ts`、`app/plugins/init.ts`、`app/configs/userConfig/userConfig.ts`、`app/pages/chat/conversation/[id].vue`、`app/components/charts/BaseChart.vue`、`app/assets/scss/global.scss`、`emotion-echo-web-bff/internal/{downstream/user.go,handler/viewmodel.go,handler/user_handler.go}`、`emotion-echo-user-svc/internal/{model/user.go,types}`、`proto/user.proto`、`deploy/db/02-create-tables-in-schemas.sql`。
> 已查：`discovered-unresolved.md`（E2E-F-82 归属本阶段）、`RUNBOOK.md` §4/§7/§13、`decisions.md`（D-01~D-08 无涉及本阶段的门）、`scripts/check_adr_gate.sh`（架构关键词含 storage/proto 相关，改契约需 ADR）。
> 已实测确认的现状见 §1；**未做任何推测性结论**。

## 1. 阶段目标

让"设置页（`/chat/setting`）的字号与主题切换"从**操作看似成功、刷新即失效**变成**端到端已验证且真正持久化**。

**关键前提（建档实测确认，非推测）：持久化在当前架构上不可能实现。** 四层契约各缺一环，任一缺失都会让 `PATCH /api/v1/users/me {"config":{...}}` 被静默丢弃：

| 层 | 现状 | 证据（文件:行号） |
|----|------|------------------|
| 数据库 | `emotion_echo_user.users` **无 config 列** | `deploy/db/02-create-tables-in-schemas.sql:7-18`（列清单止于 `birthday`）；`grep -rn "config" deploy/db/*.sql` 零命中 |
| proto | `UpdateProfileRequest` 只有 nickname / avatar_url / gender | `proto/user.proto:112-118` |
| BFF 请求体 | `UpdateProfileReq` 只有 Nickname / Gender / Birthday / AvatarURL | `emotion-echo-web-bff/internal/downstream/user.go:36-41` |
| BFF 响应 | `toProfileVM` 硬编码 `Config: map[string]any{}` ⇒ 读回来永远是空对象 | `emotion-echo-web-bff/internal/handler/viewmodel.go:106` |

Go `json.Unmarshal` 忽略未知字段 ⇒ 前端 `PATCH` 拿到 200、`result.isOk` 为真、提示"成功"，而服务端什么都没存。这正是账本 [E2E-F-82](../../discovered-unresolved.md) 记录的死法，与 E2E-F-80（age 同样被丢弃）同源。

**因此本阶段定位为 `transformation` 而非纯 `verification`**：必须先扩契约（DB 列 → proto → BFF 透传 → VM 映射），"持久化"才具备可被验证的对象。扩契约的具体载体见 §6 待确认决策。

## 2. 范围与边界

### 做

| 范围 | 涉及文件 |
|------|---------|
| users 表新增 config 列（幂等迁移 + 权威 DDL 同步） | 新增 `emotion-echo-user-svc/migrations/u003_add_user_config.sql`；同步 `deploy/db/02-create-tables-in-schemas.sql:7-18` |
| user-svc 透传 config（model / types / logic / grpcserver） | `emotion-echo-user-svc/internal/{model/user.go,types,logic,grpcserver}` |
| proto 新增 config 字段（两端重新生成） | `proto/user.proto:112-118` |
| BFF 透传 config（请求体 + 响应 VM） | `emotion-echo-web-bff/internal/downstream/user.go:36-41`、`internal/handler/viewmodel.go:106`、`internal/handler/user_handler.go:69-82` |
| 前端设置页与 store 的读写闭环 | `emotion-echo-web/app/pages/chat/setting.vue`、`app/stores/user.ts:37-113`、`app/types/userConfig/userConfigType.ts`、`app/types/api.ts:88-91` |
| 跟随系统主题**运行时**变化（`matchMedia` 监听） | `emotion-echo-web/app/stores/user.ts:100-113`（`applyTheme` 现只在 init / setTheme 各调一次，无 `change` 监听） |
| 冷启动主题应用时机（消除首屏闪烁） | `emotion-echo-web/app/plugins/init.ts:14-16,24-28`（客户端插件，SSR 首屏无 dark class） |
| 契约漂移清理：① `fontSizeType` 定义为 px 值而实际流通语义名；② `configs/userConfig/userConfig.ts` 全仓零引用；③ `setting.vue` 的 `fontSizeLabel` 死代码 | `app/types/userConfig/userConfigType.ts:3`、`app/stores/user.ts:37-56`、`app/configs/userConfig/userConfig.ts:3-5`、`app/pages/chat/setting.vue:81-83` |
| 本阶段新增测试（设置页当前**零功能测试**，见下） | Playwright spec + 前端架构契约钉 + store 单测 + Go 侧契约测试 |

> **测试基线（建档实测）**：全仓对 `/chat/setting` 的引用只有 2 处 —— `e2e/a11y-baseline.spec.ts:17`（且该 spec 的断言被注释，属 E2E-F-51 soft-assert）与 `layouts/nav.vue:92`（导航项）。`grep -rln "setFontSize\|setTheme\|chat/setting" emotion-echo-web/{app,e2e} --include=*.test.ts --include=*.spec.ts` 除上述外零命中；`app/stores/` 下 414 行的 `user.ts` **无同名单测**（仅 `__tests__/message.test.ts`）。故本阶段的回归钉将是设置页的第一份有效覆盖。

### 不做（边界）

| 排除项 | 归属 |
|--------|------|
| 全站字号变量化（把字号铺到 `--ee-font-size` 等全局 token） | 非本阶段目标——页面文案明示"**消息文字的阅读尺寸**"，现作用域（消息气泡）与设计一致 |
| a11y 强制化（`a11y-baseline.spec.ts` 末段断言被注释） | E2E-04（已登记 E2E-F-51） |
| 未登录访问设置页的重定向 | E2E-01（`middleware/auth.global` 既有行为） |
| 资料其余字段落库（昵称/性别/生日/age） | E2E-F-80（E2E-11 留账） |
| 语言 / i18n 切换 | D-04 候选（E2E-F-29） |
| 响应式断点重构 | E2E-04（本阶段只验证 `setting.vue:244` 既有 600px 断点可用） |
| 暗色下第三方组件（Element Plus 等）适配 | 本阶段只覆盖 `--ee-*` token 与 ECharts |

## 3. 前置条件

| 条件 | 状态 |
|------|------|
| 依赖阶段完成 | ✅ E2E-01（登录会话，cookie 持久化）、E2E-11（我的空间；其收尾修复了 `.page-content` 滚动 = E2E-F-84） |
| dev 模式容器（infra + apps + dev overlay） | ⬜ 待启动 |
| `deploy/.env.local` 存在（LLM key 唯一存放点，AGENTS.md §四红线） | ⬜ 待核（`ls -l deploy/.env.local` 非空即可，不得读取内容到日志/commit） |
| 演示账号 `echo / echo123` | ⬜ 待核（E2E-08/09/10/11 复用） |
| DB 可访问（落库断言 #3 需要 psql） | ⬜ 待核 `docker exec emotion-echo-db psql -U ... -c '\dt emotion_echo_user.*'` |
| **D-09 存储载体已确认**（§6） | ⬜ 开工前确认一次；未确认则按推荐方案 A 执行 |
| 前端本地 dev server（改代码免 rebuild） | ⬜ `docker stop emotion-echo-web` 后 `cd emotion-echo-web && pnpm dev --port 3000`（RUNBOOK §2.1b） |
| 改 Go / proto 后**重建镜像再验收** | ⬜ 见风险 R1（E2E-F-70 教训） |

环境启动命令（**必须带 `--env-file .env.local`**，见 AGENTS.md §四）：

```bash
cd deploy && docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml \
  -f compose.dev.yml --env-file .env.local up -d
```

## 4. 测试点清单

判定标记：`[A]` 自动可判 · `[V]` 视觉判定（需截图并被查看）· `[M]` 需人工/设计裁定。详见 [RUNBOOK.md](../../RUNBOOK.md) §4。

| # | 测试点 | 判定 | 验证方式 | 证据 | 结果 |
|---|--------|------|---------|------|------|
| 1 | 设置页可进入且当前值正确回填 | `[A]`+`[V]` | 点侧边栏"设置"→ URL=`/chat/setting`；断言选中态与 `GET /api/v1/users/me` 的 `config` 一致（`.font-size-btn.active` 文案 ↔ config.fontSize；`input[name=theme]:checked` ↔ config.theme），**不是**恒为"中/浅色" | 截图 + 只读 DOM/网络断言 | ⬜ |
| 2 | 字号切换即时生效（消息气泡） | `[A]`+`[V]` | 选"大"→ 进入会话页 → `page.evaluate` 读 `.bubble` 的 `getComputedStyle(...).fontSize` == `18px`；与切换前截图对比 | 计算样式输出 + 前后截图 | ⬜ |
| 3 | 字号刷新后保持（服务端持久化） | `[A]` | 选"大"→ `page.reload()` → 选中态仍为"大"；`psql -c "SELECT config FROM emotion_echo_user.users WHERE username='echo'"` 含 fontSize；**断言读回值 == 写入值**（防 px/语义名漂移，见 #12） | psql 输出 + 断言 | ⬜ |
| 4 | 字号跨会话保持（新 context 重登） | `[A]` | 新建 browser context（清 cookie 与本地存储）→ 重新登录 → 进入设置页 → 仍为"大"。**此点区分"服务端持久化"与"仅本地存储"** | 断言输出 | ⬜ |
| 5 | 主题切换即时生效 | `[A]`+`[V]` | 选"深色"→ 断言 `document.documentElement.classList.contains('dark')` 为真，且 `getComputedStyle(document.body).backgroundColor` == `rgb(23, 28, 26)`（`global.scss:30-42` 的 `--ee-bg: #171c1a`）；截图 | 计算样式输出 + 截图 | ⬜ |
| 6 | 主题刷新后保持 | `[A]` | `page.reload()` → html 仍含 `dark`，选中态仍为"深色" | 断言输出 | ⬜ |
| 7 | 主题跨会话保持 | `[A]` | 同 #4 流程，新 context 重登后仍为深色 | 断言输出 | ⬜ |
| 8 | "跟随系统"：初始生效 + **运行时跟随** | `[A]` | ① `emulateMedia({colorScheme:'dark'})` → 选"跟随系统" → html.dark 生效；② **不刷新**，`emulateMedia({colorScheme:'light'})` → html.dark 被移除。**② 当前实现必然 FAIL**：`applyTheme`（`stores/user.ts:100-113`）只在 init 与 setTheme 时执行，无 `matchMedia.change` 监听 | 断言输出 | ⬜ |
| 9 | 暗色下图表跟随主题 | `[V]` | 切深色后进 `/chat/user`，3 个行为图表的背景/坐标轴/文字可读（`BaseChart.vue:53-68` 的 MutationObserver 已实现跟随）；与浅色截图对比 | 深/浅两张截图 | ⬜ |
| 10 | 冷启动无主题闪烁（FOUC） | `[A]`+`[V]` | ① `[A]` 已登录（主题=深色）时请求 `/chat/setting` 的 **SSR HTML**，断言首屏即含 `class="dark"`（当前 `plugins/init.ts:14-16` 是客户端插件 ⇒ 预期 FAIL）；② `[V]` 冷启动首帧截图非浅色底 | HTML 片段 + 首帧截图 | ⬜ |
| 11 | 小视口（375×667）可用且无裁剪 | `[V]` | 用 Playwright 既有 `mobile` project（`playwright.config.ts:38-41`，Pixel 5）截图；断言两个控件均在视口内可点、`document.documentElement.scrollWidth <= 375`、且无元素被静默裁剪。**必须用小视口截图**——E2E-F-36 教训：大视口会躲过裁剪缺陷 | 小视口截图 + 宽度断言 | ⬜ |
| 12 | 值契约一致性 + 未知值不崩 | `[A]` | ① 写入表示唯一：现 `stores/user.ts:62-75` 送 px 值（`fontSizeToPx`）却把本地 `config.fontSize` 存成语义名（`small/medium/large`）⇒ 断言"选哪档、读回就是哪档"，不出现"选了中、读回 small"；② 直接向 DB 写入非法值（如 `{"theme":"neon"}`）→ 刷新页面不崩、回落到确定档位（`getUserConfig` 的 `|| 'light'` 路径） | 断言输出 + psql | ⬜ |

汇总：12 个测试点 —— `[A]` 10 个、`[V]` 6 个（其中 #1/#2/#5/#10 为 `[A]+[V]` 双证据）、`[M]` 0 个。

**建档已预判为 FAIL 的点（3 个）**：#3/#6（契约缺字段，E2E-F-82）、#8（无 `matchMedia` 监听）、#10（首屏闪烁）。这三点是本阶段的实修目标；其余为"确认现状 + 防回归"。

**范围说明（同 E2E-11 #4 的写法）**：#2 只断言**消息气泡**字号生效（与页面文案一致）；#11 只断言**既有 600px 断点**可用，不做断点重构。

## 5. 验收标准（DoD）

- [ ] 全部 12 个测试点有结论（或发现问题已分类：范围内修复 / 范围外记账本）
- [ ] 修复项走完 TDD（Red → Green → Refactor），每项带**先行的失败测试**证据
- [ ] 已验证行为固化为 Playwright spec（`settings.spec.ts`）且**跑过至少一次且绿**
- [ ] 契约扩展附带 ADR（`docs/architecture/adr/adr-2026-09-user-config-persistence.md`）+ `decisions.md` 登记（D-09）——符合 RUNBOOK §13.3 #15 与 AP-08
- [ ] 迁移幂等性：`u003_add_user_config.sql` 可重复执行，且 `deploy/db/test_migrations_contract.sh` 通过（真相源：`02-*.sql` 与 `migrations/` 一致）
- [ ] §2.4 §契约 5 适用（本案触及 schema）：对新增 `config` 列有断言"写入值 ∈ 合法集合"的契约测试
- [ ] roadmap 状态更新 + 账本更新（**E2E-F-82 必须翻状态**，否则阶段只能标 `partial`；RUNBOOK §7 #9 账本对账）
- [ ] §2.5 收口自检三连通过
- [ ] §13.3 §13.3 中 #13（生产代码改动须同批含 `_test.go`/`*.spec.ts`）、#17（残留扫描）通过

## 6. 已知风险

| # | 风险 | 应对 |
|---|------|------|
| R1 | **改 Go / proto 后未重建镜像 → 验收测的是旧代码**（E2E-F-70 教训） | 验收前 `docker compose build user-svc web-bff`，并核对镜像 `Created` 时间晚于本次提交时间 |
| R2 | proto 生成方式不一致 → 生成物与既有风格/版本漂移（R 系列教训：改 proto 必须复现原生成方式） | 先查 `buf.gen.yaml` / `protoc` 既有调用方式，按原方式重新生成，不改生成器版本 |
| R3 | 新增列与权威 DDL 不一致 → 触发 migration 契约测试红（`deploy/db/test_migrations_contract.sh`） | 迁移与 `deploy/db/02-create-tables-in-schemas.sql` **同批**改；迁移用 `IF NOT EXISTS` 保幂等 |
| R4 | JSONB 读写：GORM 字段类型与 `NULL` vs `{}` 语义 | 未设置 = `NULL`（不用 `{}` 默认值，避免"看起来设置了"）；Go 侧用 `[]byte`/`datatypes.JSON` 并加读写往返测试 |
| R5 | 首屏无闪烁（#10）依赖 SSR 侧能读到 config，而 token 在 HttpOnly cookie（R-09 改动）| 若 SSR 拿不到用户 → 用本地镜像（cookie/localStorage）做首屏兜底，服务端仍是权威；此为**实现细节**，不改变本阶段方向 |
| R6 | 小视口验证被大视口截图蒙混（E2E-F-36 教训：截图用大视口躲过裁剪） | #11 强制用 `mobile` project；report 中截图必须标注视口尺寸 |
| R7 | 账本对账：E2E-F-82 归属本阶段 | 收口时翻状态；若因方案选择未修，须按 §4.2 标 `BLOCKED` + 升级批准，**不得**标 `N/A` 掩盖 |
| R8 | 测试点 #1 的"正确回填"在首轮必然是 FAIL（VM 恒空 config）| 属预期内 FAIL，进修复队列；`[A]` 断言须写到"与 GET config 一致"而非"存在某个选中态"，避免弱断言放过（E2E-11 的 E2E-F-86 教训） |

### 待确认决策 D-09：`config` 的存储载体

| 方案 | 做法 | 优点 | 缺点 | 是否改契约 |
|------|------|------|------|-----------|
| **A. 服务端持久化（推荐）** | `users` 加 `config JSONB` + proto 字段 + BFF 透传 | 跨会话/跨设备；与**页面既有设计意图一致**（`stores/user.ts:59-97` 早已在调 `updateProfile({config})` 写服务端）；直接关闭 E2E-F-82 | schema + proto 变更（4 层）、需迁移与 ADR | ✅ 是 |
| B. 仅本地存储 | localStorage/cookie 镜像，删掉前端那段服务端写入 | 零后端改动、最快 | 换浏览器/清缓存即丢；等于把 E2E-F-82「已实现却失效」改为「主动降级」，**须用户批准并在账本标 🟡 降级并记录**（防 AP-05） | ❌ 否 |

**推荐 A**，理由：前端写入路径已按服务端持久化写好（`setFontSize`/`setTheme` 都先调 API 成功再更新本地，`stores/user.ts:66-77,86-95`），A 是"补齐契约让已有实现真正生效"，而不是新增设计；B 则要反过来删除既有代码，且不满足 roadmap 中本阶段标题的「持久化」。

**方案 B 对本档的影响**（若用户选 B）：§2「做」表中的契约扩展 4 行全部删除、测试点 #4/#7 改为 `N/A`（须写明理由）、#3/#6 降级为"本地持久化"验证、R3/R4 风险消失。

> 本决策**未**列入 `gate`，故不阻塞开工：执行者按推荐方案 A 推进；用户若在开工前后选择 B，按上表调整本档即可（不涉及已决议方向变更，故无需按 §8 升级）。

## 7. 产出物

- Playwright spec：`emotion-echo-web/e2e/settings.spec.ts`（覆盖 12 个测试点，含 `mobile` project）
- 前端架构契约钉：`emotion-echo-web/app/pages/chat/e2e-12-settings-contract.architecture.test.ts`
- store 单测：`emotion-echo-web/app/stores/__tests__/user-config.test.ts`（`user.ts` 当前零单测）
- Go 侧契约测试：user-svc（config 读写往返）、BFF（`UpdateProfileReq.Config` 透传 + `toProfileVM` 映射）
- 迁移：`emotion-echo-user-svc/migrations/u003_add_user_config.sql` + `deploy/db/02-create-tables-in-schemas.sql` 同步
- ADR：`docs/architecture/adr/adr-2026-09-user-config-persistence.md` + `docs/e2e-roadmap/decisions.md` 新增 D-09
- 执行记录：`docs/e2e-roadmap/stages/e2e-12-settings/report.md`
- 截图：`docs/e2e-roadmap/stages/e2e-12-settings/screenshots/`（每个 `[V]` 点 ≥1 张，标注视口尺寸）
