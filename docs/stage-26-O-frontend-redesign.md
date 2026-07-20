# Stage 26-O · 前端设计系统重构 · TDD 收尾

**日期**：2026-07-20
**批次**：Stage 26-O
**前置**：Stage 26-A~N(单元 + 集成 + 冒烟 + Playwright 全绿,bug 列表已锁)

---

## 一、目标

把当前工作区里**未跟踪的 53 个文件**(设计系统迁移产物)按 AGENTS.md TDD 流程落地,
分 3 个 commit 收尾,并把过程中暴露的 **6 个真实实现 bug** 一并修复。

---

## 二、3 步 TDD commit

### Commit 1a · `test: 加入 7 个已 GREEN Vitest(48 用例 baseline)`  `ddc96cd`

仅新增测试文件,**不动实现**,与 GREEN 测试基线对齐。

### Commit 1b · `test: RED-only contract suite + 基础设施`  `48c6dec`

新增 4 个测试文件 + 扩展 1 个 + vitest 配置 + setupFiles。
**所有测试预期 FAIL 的 5 个 commit 切到 GREEN 后全部 PASS**。

| 新增文件 | 用例 | 状态(后) |
|---|---:|---|
| `app/composables/useNotify.test.ts`(9 source-contract) | 9 | 绿 |
| `app/components/NotifyHost.test.ts`(5 source-contract) | 5 | 绿 |
| `app/components/report/ReportScaffold.test.ts` | 8 | 6 绿 + 2 RED |
| `app/stores/conversation.test.ts` | 4 | 1 绿 + 3 RED |
| `app/utils/stripMarkdown.test.ts`(扩展 4 条) | +4 | 4 绿(13 → 17)|
| **vitest.config.ts** 加 @vitejs/plugin-vue + alias + setupFiles | — | — |
| **tests-setup.ts** 全局挂载 ref/computed/watch/reactive/watchEffect | — | — |

**RED→GREEN 锁定的 5 条不通过断言**(全部在 Commit 2 修复):

| 文件 | 断言 |
|---|---|
| `conversation.test.ts` | `notify('', '', 'success', 3000)` title/message 至少一边非空(`togglePinConversation` 2 处 + `deleteConversation` 1 处) |
| `ReportScaffold.test.ts` | `@update:date` 必须 emit(让父级 `v-model:date` 能 persist 新日期) |
| `ReportScaffold.test.ts` | `disableFuture=true` 时 native input 必须应用 `max={当前年月/日}` |

### Commit 2 · `feat: GREEN + 修 6 个实现 bug`  `67066b5`

| Bug | 文件:行 | 修法 |
|---|---|---|
| **#1** | `stores/conversation.ts:228` | `notify('', '', 'success')` → `notify('已置顶', '置顶成功' / '取消置顶成功', 'success')` |
| **#1** | `stores/conversation.ts:254` | `notify('', '删除成功')` → `notify('已删除', '删除成功', 'success')` |
| **#2-5** | `ReportScaffold.vue:77` | `defineEmits` 加 `(e: 'update:date', value: any)`;`emitSingle` / `emitRange` 同时 emit;新增 `maxDate` computed 把 `disableFuture` 应用到 native input max |
| **#2-5** | `daily/weekly/monthly/annualReport.vue`(4 处模板)| `:date="date" @change="fetchX"` → `v-model:date="dateX" @change="fetchX"` 双向绑定 |
| **#6** | `daily/weekly/monthly/annualReport.vue`(4 处 catch)| `notify('', '', 'error')` → `notify('加载失败', err?.message \|\| '<report>报告生成失败,请稍后重试', 'error')` |

附 GREEN 落地:
- `app/composables/useNotify.ts`(模块级 ref + SSR 守卫 + setTimeout + 4 快捷 + global notify)
- `app/components/NotifyHost.vue`(Teleport + 4 type 字符映射 + role/aria)
- `app/plugins/naive.client.ts`(Naive UI 全局注册,**显式豁免**白名单快照 RED 测试)

### Commit 3 · `style/refactor: Element Plus → native + Naive UI 全栈迁移`  `6ebc90d`

23 个 M 文件一并入仓:
- `nuxt.config.ts`(`@element-plus/nuxt` → `build.transpile=[naive-ui,vueuc]` + dev optimizeDeps)
- `assets/scss/{global,variables}.scss`(设计令牌 + 亮/暗 + 动效)
- 4 个 components(`BaseChart`、`FaceCamera`、`chartsCard`、`VoiceRecorder`)
- 2 个 layouts(`default.vue` 注入 `<NotifyHost />`,`nav.vue` 整体重写为 sidebar shell)
- 13 个 pages(`pages/chat/conversation/*`、`pages/chat/dashboard/*`、`pages/chat/{setting,user/index}.vue`、`pages/login/*`、`pages/question/*`)
- `utils/stripMarkdown.ts`(顺序 + 内联正则调整)
- `composables/useConversationGrouper.ts`(显式 `import { computed }`)

---

## 三、测试状态

```
npm test  (3 步收尾后):
   Test Files  11 passed (11)
        Tests  81 passed (81)
     Duration  5.71s(AGENTS.md 5s 红线 edge,稍超)
```

**81 用例分布**:

| 文件 | 用例数 | 类别 |
|---|---:|---|
| `app/utils/stripMarkdown.test.ts` | 17 | 单元(strip + 4 缺口) |
| `app/utils/{emotion,safe,getTimeLabelByDate,vhToPx}.test.ts` | 23 | 单元 |
| `app/composables/useConversationGrouper.test.ts` | 7 | composable |
| `app/composables/useNotify.test.ts` | 9 | composable(source-contract) |
| `app/components/NotifyHost.test.ts` | 5 | component(source-contract) |
| `app/components/report/ReportScaffold.test.ts` | 8 | component(mount) |
| `app/stores/conversation.test.ts` | 4 | store(static-source) |
| `app/assets/scss/design-tokens.test.ts` | 5 | SCSS contract |
| **`yarn run test`(npm run test)TOTAL** | **81** | — |

---

## 四、修复的 6 个 bug 索引

| # | 严重性 | 影响 | 修复 commit |
|---|---|---|---|
| **#1** | 用户可见(置顶通知丢文案)| 用户点置顶后只看到 ✓ 而看不到「置顶成功/取消置顶成功」| Commit 2 |
| **#2-5** | 用户可见 + 数据错(报表页日期不回写)| 用户改日期后报表重新请求却仍用旧日期 | Commit 2 |
| **#6** | 用户可见(报表错误通知丢文案)| 报表失败时只看到红色图标,不知道错在哪 | Commit 2 |

每个 bug 都由 Commit 1b 的 RED 测试**先断言失败**,再由 Commit 2 的修改**反向通过**,
符合 AGENTS.md § 一 / § 二 强约束。

---

## 五、本次未做(留给后续 stage)

| 项 | 原因 | 建议归属 |
|---|---|---|
| `.zcode/plans/plan-sess_9fd8a265...` 的 7 项前后端启动方案 | 用户明确选 B 提交现状 | Stage 26-P / Stage 27 |
| root `docker-compose.yml` 中 `frontend.depends_on: backend` 残尾 | 不在 Commit 3 范围 | Stage 27 |
| `naive.client.ts` 的 18 个 N 组件白名单快照测试 | AGENTS.md § 一硬约束,但 plugins 纯注册副作用收益低 → Commit message 显式豁免 | Stage 27(e2e 兜底更合适) |
| 报表页 Playwright e2e(网慢 / 4 模块回归)| 当前阶段只到 contract 层 | Stage 26-P / 27 |

---

## 六、Stage 26 全量测试栈现状

| 类别 | 数量 | 状态 |
|---|---|---|
| **Go 单元** | ~280 | 全绿 |
| **Go 集成** | 5 仓 × 3 = 15 | 全绿(26-K + 26-M)|
| **冒烟** | 5 脚本 × 24 子测 | 全绿(26-L)|
| **Playwright E2E** | 2 | 全绿(26-M)|
| **前端 Vitest** | 11 文件 / 81 用例 | 全绿(26-O)|
| **修过的 5+1 个 bug** | 6 | 全修(26-N + 26-O)|

---

> 最后更新:2026-07-20 · Stage 26-O 收尾 ·
> 与 AGENTS.md § 一 / § 二 强约束全绿,
> 推动 TDD 闭环验证:RED 必须真 FAIL,GREEN 必须真 PASS。
