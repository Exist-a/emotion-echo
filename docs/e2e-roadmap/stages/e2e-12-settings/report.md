---
stage: e2e-12
title: 设置页
executed: 2026-09-20
status: done
environment: dev 模式（17 容器 healthy，compose.apps.yml + .env.local）
---

# E2E-12 执行记录（report）

> **本轮性质：收尾轮 + 纠错轮。** 首轮（PR #28）已合并，但事后复核发现该轮报告**失真**：
> 把 4 个点标为 `BLOCKED`（其中 2 个实测本就通过）、把 plan 明确要求的 `matchMedia`
> 运行时跟随改写成"选项存在"并据此标 PASS。本轮把这两类问题一并纠正并实修，
> 详见 §3「首轮报告失真与纠正」。

## 1. 环境基线

- 启动命令：`cd deploy && docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml -f compose.dev.yml --env-file .env.local up -d`
- 容器状态：17/17 healthy
- 声明的配置差异：无（`NUXT_PUBLIC_DISABLE_AUTH=false` 正常模式）
- 前端镜像：`emotion-echo/web:v0.1.0`，**验收前已重建**（E2E-F-70）

## 2. 测试点结果

| # | 测试点 | 判定 | 结果 | 证据 |
|---|--------|------|------|------|
| 1 | 设置页可进入且当前值正确回填 | `[A]+[V]` | PASS | Playwright #1；`screenshots/04`（深色态回填"大/深色"） |
| 2 | 字号切换即时生效（消息气泡） | `[A]+[V]` | PASS | Playwright #2；`screenshots/03`（浅色态字号"中"） |
| 3 | 字号刷新后保持（服务端持久化） | `[A]` | PASS | Playwright #3；DB `SELECT config` 实测 |
| 4 | 字号跨会话保持（新 context 重登） | `[A]` | PASS | Playwright #4（全新 browser context，仅注入 token） |
| 5 | 主题切换即时生效 | `[A]+[V]` | PASS | Playwright #5；`screenshots/04`（`html class="dark"`） |
| 6 | 主题刷新后保持 | `[A]` | PASS | Playwright #6 |
| 7 | 主题跨会话保持 | `[A]` | PASS | Playwright #7（新 context 同时断言选中态 + `html.dark`） |
| 8 | 跟随系统：初始生效 + **运行时跟随** | `[A]` | PASS | Playwright #8（三段：初始/切浅/再切深）；vitest 4 条 |
| 9 | 暗色下图表跟随主题 | `[V]` | PASS | Playwright #9（深/浅计算样式对比）；`screenshots/05`（3 图跟随深色） |
| 10 | 冷启动无主题闪烁（FOUC） | `[A]+[V]` | PASS | Playwright #10（SSR HTML 含/不含 `class="dark"` 正反对照） |
| 11 | 小视口可用且无裁剪 | `[V]` | PASS | Playwright #11；`screenshots/06`（375×667，`scrollWidth==clientWidth==375`） |
| 12 | 值契约一致性 + 未知值不崩 | `[A]` | PASS | Playwright #12；vitest 3 条（px↔语义名唯一映射 + 未知值回退） |

汇总：**PASS 12 / FAIL 0 / BLOCKED 0 / N/A 0**

> `BLOCKED` = 0，未触发 RUNBOOK §4「BLOCKED > 1/3 不得判 done」。

### 截图清单（`screenshots/`，均标注视口）

| 文件 | 视口 | 覆盖 |
|------|------|------|
| `01-settings-page-dark-1280x720.png` | 1280×720 | #1（首轮） |
| `02-settings-page-dark-375x667.png` | 375×667 | #11（首轮） |
| `03-settings-page-light-1280x720.png` | 1280×720 | #2（浅色对照） |
| `04-settings-dark-applied-1280x720.png` | 1280×720 | #1/#5（深色生效，字号"大"选中） |
| `05-charts-dark-follow-1280x720.png` | 1280×720 | #9（3 个图表跟随深色） |
| `06-settings-dark-375x667.png` | 375×667 | #11（小视口无溢出） |

## 3. 发现与分类

| 发现 | 分类 | 处理 |
|------|------|------|
| E2E-F-82：config 不落库（四层契约缺字段） | 范围内 | 修复（PR #28）；账本 `E2E-F-82` 已翻 ✅ |
| `setting.vue` 用 `ref` 快照而非 `computed`，回填不响应 | 范围内 | 修复（PR #28） |
| **#8 无 `matchMedia` 运行时监听**（plan 明确要求） | 范围内 | 本轮实修（§3.3） |
| **#10 SSR 首屏无主题 ⇒ FOUC**（plan 明确要求） | 范围内 | 本轮实修（§3.3） |
| **`fetchUserInfo` 拿到 config 后不应用主题**（首轮未发现，弱断言放过） | 范围内 | 本轮实修（§3.3） |
| **契约漂移 ①/②/③**（px↔语义名、死文件、死代码） | 范围内 | 本轮清理（§5） |
| **首轮报告失真：4 个假 BLOCKED + #8 挪球门** | 范围内（报告质量） | 本轮纠正（§3.1/§3.2） |
| 第一版 SSR 插件无条件声明 `htmlAttrs.class` 导致主题失效 | 范围内（自身引入的回归） | 本轮修复（§3.4） |
| `check_image_freshness.sh` 未被引用 / 缺末尾换行 | 范围外（门禁基建） | PR #28 修（`scripts/README.md` + 换行） |
| dev 环境浏览器直连 BFF 无 `X-User-Id` | 范围外 | 已澄清为**伪阻塞**——实测跨会话用例本就通过（§3.1） |

### 3.1 假 BLOCKED（4 个里有 2 个实测本就通过）

首轮把 #4/#7/#9/#10 标为 `BLOCKED`，理由写作"dev 环境浏览器直连 BFF 无 X-User-Id"。
**该理由是推断，当时并未实测。** 本轮实测：

- **#4 / #7 本就通过**（全新 context 只注入 token → 设置页正确读回 `大` / `深色`）。
- **#10 根本不是"阻塞"**，而是**功能未实现**（SSR 首屏 HTML 实为 `<html>`，无 class）。
- **#9 未核实**，本轮实测通过。

即：`BLOCKED` 被用来掩盖"没做完 / 没测过"，与 [anti-patterns.md](../../anti-patterns.md) 中
「用 N/A/BLOCKED 掩盖未做的工作」同型。

### 3.2 挪球门（#8 测试点语义被改写）

plan §2「做」表明确列出：**「跟随系统主题运行时变化（`matchMedia` 监听）」**。

首轮把 #8 的判定从 `[A]` 改为 `[M]`、文案从"初始生效 + 运行时跟随"改写为
"设置页加载跟随系统主题选项"，据此标 PASS。实测当时：

```
app/stores/user.ts:105  window.matchMedia(...).matches   ← 一次性读取
addEventListener / 'change' 出现次数：0                    ← 无运行时监听
```

**该功能当时完全不存在**，标 PASS 属挪球门（比不修更糟：报告读起来像已完成）。

### 3.3 本轮实修的真实缺陷（3 项，均走 TDD）

| 缺陷 | 根因（实测） | 修复 |
|------|-------------|------|
| **#8 无运行时跟随** | `applyTheme` 只在 init/setTheme 各执行一次，无 `matchMedia change` 监听 | 注册监听 + 切离 auto 时解绑（防重复注册/残留跟随） |
| **#10 首屏闪烁** | 主题只在客户端插件应用；SSR 无主题信息 ⇒ 先渲染浅色再补深色 | `applyTheme` 写 `ee_theme` 镜像 cookie（非 HttpOnly）→ 服务端插件读它并渲染 `htmlAttrs.class` |
| **#5/#9 主题不生效**（首轮未发现） | `plugins/init.ts` 的「fetchUserInfo 后重应用主题」**整块被 gated 在 `isAuthenticated`** 上；冷启动 `userInfo` 来自 localStorage、为空 ⇒ 整块跳过 ⇒ 服务端 config 取到了但主题从不应用 | 把"拿到 config 即应用主题"下沉到 `store.fetchUserInfo` —— 唯一权威点，任何调用方都自动获得正确主题 |

> 第 3 项是本轮最有价值的发现：它同时解释了首轮 #5 "为什么只查 radio 就通过了"
> —— radio 来自 `userInfo.config`（`setting.vue` 自己 fetch），而 `html.dark` 从未被应用。
> **弱断言（只查选中态、不查实际生效）放过了真 bug**，与 E2E-11 的 E2E-F-86 同型。

### 3.4 实现过程中自身引入的回归（已修）

第一版 SSR 插件**无条件**声明 `htmlAttrs.class`（浅色时 `class: ''`）。该值会进入 SSR
payload 并在客户端 hydration 时回写，**覆盖掉 `applyTheme` 加上的 `dark`** ⇒ 主题切换整体失效
（实测 #8/#9 双双变红）。改为**仅深色时才声明**，消除冲突。

## 4. 修复清单（TDD 记录）

| commit | 内容 | 先行的失败测试 |
|--------|------|---------------|
| `c72456a` | 四层 config 契约扩展（DB→model→types→repo→logic→gRPC→proto→BFF→前端） | Go 8 条 + vitest 6 条 RED |
| `851dfeb` | Playwright 浏览器验收 + Dockerfile 修复 | Playwright 先红后绿 |
| （本轮） | #8 matchMedia 运行时监听 | vitest 4 条 RED（`#8 跟随系统主题运行时监听`） |
| （本轮） | #10 SSR 首屏主题（cookie 镜像 + 条件 htmlAttrs） | vitest 2 条 RED（`#10 首屏主题镜像 cookie`） |
| （本轮） | fetchUserInfo 后应用主题（#5/#9 真根因） | vitest 2 条 RED（`#5/#9 拿到服务端 config 后必须应用主题`） |
| （本轮） | 契约漂移清理 ①（px ↔ 语义名 唯一映射） | vitest 1 条 RED（`#12 值契约一致性`） |

## 5. §2 范围完成度（含首轮漏做项）

| plan §2「做」条目 | 状态 |
|------------------|------|
| users 表新增 config 列（迁移 + 权威 DDL 同步） | ✅ `u003_add_user_config.sql` |
| user-svc 透传 config（model/types/logic/grpcserver） | ✅ |
| proto 新增 config 字段（两端重新生成） | ✅ `bash proto/gen.sh user.proto` |
| BFF 透传 config（请求体 + 响应 VM） | ✅ |
| 前端设置页与 store 读写闭环 | ✅ |
| **跟随系统主题运行时变化（matchMedia 监听）** | ✅ **本轮实修**（首轮漏做且被改写语义） |
| **冷启动主题应用时机（消除首屏闪烁）** | ✅ **本轮实修**（首轮漏做） |
| **契约漂移清理 ① fontSizeType 语义名** | ✅ 本轮：`fontSizeType`=语义名、新增 `fontSizePxType`=wire；`api.ts` 不再用并集藏漂移；store 本地也存 px（去掉 `as any`） |
| **契约漂移清理 ② configs/userConfig/userConfig.ts 零引用** | ✅ 本轮删除（`git rm`；`app/configs/chartConfig/` 仍在用，保留） |
| **契约漂移清理 ③ setting.vue fontSizeLabel 死代码** | ✅ 本轮删除（连带其唯一消费者 `fontLabels`） |
| 本阶段新增测试 | ✅ 见 §6 |

## 6. 回归钉

- **Playwright**：`emotion-echo-web/e2e/settings.spec.ts` — **12 个用例，12/12 PASS**
- **vitest**：`app/stores/__tests__/user-config.test.ts` — **17 个用例全 PASS**
- **vitest 全量**：50 文件 / **427 测试 PASS**（首轮 416，本轮 +11）
- **typecheck**：`vue-tsc --noEmit` exit=0
- **ESLint**：`settings.spec.ts` 0 problems
- **Go**：user-svc + BFF 全绿（首轮 8 条新测试）

### 测试基建修复（本轮）

`import.meta.client` / `import.meta.server` 在 vitest 下恒为 `undefined`（Nuxt 编译期注入，
vitest 不经 Nuxt 编译）⇒ 带 `if (!import.meta.client) return` 守卫的客户端逻辑
（如 `applyTheme`）在测试里被**静默跳过、根本测不到**。
`vite` 的 `define` 对 `import.meta.*` 不生效（实测仍 undefined），故在
`vitest.config.ts` 加 transform 插件做等价替换（client=true / server=false）。
**该修复让全项目所有客户端守卫代码从此可测**，不只本阶段。

## 7. 待决策 / 升级项

无。

## 8. 收口自检

- [x] `git status` 干净（feature 分支工作区无未提交改动）
- [x] `git status -sb` 无本地未推送 commit
- [x] `git branch --merged main` 除 main 外为空（PR 合并后已删源分支）

## 9. §2.4 数据契约判定

| 契约 | 适用性 | 结论 |
|------|--------|------|
| §契约 5（schema 与写入端一致性） | **不适用** | 该契约的对象是「VARCHAR(32) NOT NULL **枚举列**」，要求断言写入值 ∈ enum 集合。本次新增的 `users.config` 是 **JSONB 开放容器**，其值域刻意不由服务端约束（用户可存任意个性化键）；值域收敛发生在前端 `getUserConfig` 的 `pxToFontSize`/`|| 'light'` 回退，已由测试点 #12 与 vitest「未知值回退」用例覆盖。**理由已写入 ADR「后果与约束」段**。 |
| §契约 1/2/3/4/6 | 不涉及 | 本阶段未改事件发布链 / analytics 写入 / BFF 报表端点 / dev 模式 KAFKA 路径 |

## 10. 账本与路线图

- **E2E-F-82**：🟡 留账 → ✅ 已解决（PR #28）
- **roadmap**：E2E-12 → ✅ done

## 11. CI 与门禁

- 首轮 PR #28 CI 20/23（3 FAIL：ADR 门禁 / 孤立产出物 / E2E 收口审计 A6），根因与修法见 PR #28 提交说明。
- 本轮 PR 前本地逐项复验：`check_adr_gate` / `check_tdd_gate` / `check_residual` / `check_orphan_outputs` / `e2e_stage_audit --all` 全 GREEN。
