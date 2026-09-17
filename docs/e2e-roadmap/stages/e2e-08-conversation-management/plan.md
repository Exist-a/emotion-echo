---
stage: e2e-08
title: 历史会话管理
type: verification
status: pending
created: 2026-09-17
depends-on: [e2e-01, e2e-07]
blocks: [e2e-10]
related-findings: []
---

# E2E-08 历史会话管理

## 1. 阶段目标

验证会话侧的完整 CRUD 与分组展示：列表加载、分组、进入、重命名、删除、置顶、空态。近期 Sprint 111 修过"sidebar 会话标题空白"（A11）、Sprint 112 修过"我的空间被踢回 login"，说明这条链路的**展示层**问题较多，需系统走查。

## 2. 范围与边界

### 做

| 功能 | 涉及 |
|------|------|
| 会话列表加载与排序 | `web/app/pages/chat/conversation/index.vue` |
| 时间分组（今天/昨天/更早） | `web/app/composables/useConversationGrouper.ts` |
| 点击进入会话 | `/chat/conversation/:id` |
| 新建会话 | `/chat/conversation/new`（`useConversationSender` createFlow） |
| 重命名 | BFF `chat_handler.go` |
| 删除 | `deleteconversationlogic.go`（**注意**：现为物理删，E2E-06 改软删后本阶段需复验） |
| 置顶 pin/unpin | 迁移 `c004`（pinned 列） |
| 空态 | 无会话时的展示 |
| 长标题/空标题兜底 | `a11-sidebar-fallback.architecture.test.ts`（21 item 空标题 fallback） |
| 状态同步 | `web/app/stores/conversation.ts` |

### 不做（边界）

- 消息内容本身（归 E2E-10 聊天核心）
- 消息搜索
- 会话导出/分享
- 移动端折叠行为（归 E2E-04 的 mobile project 抽查）

## 3. 前置条件

| 条件 | 状态 |
|------|------|
| E2E-01（会话状态稳定） | ⏳ |
| E2E-07（认证链路稳定，避免被登录问题干扰观测） | ⏳ |
| 测试账号下有 ≥5 个会话（覆盖各分组） | 需准备种子数据 |
| E2E-06 软删除改造已完成 → 删除行为需按新语义验证 | ⏳ |

## 4. 测试点清单

| # | 测试点 | 验证方式 | 证据 | 结果 |
|---|--------|---------|------|------|
| 1 | 列表加载且无空白标题 | 断言每个 item 标题非空（A11 回归钉） | 截图 + 断言 | ⬜ |
| 2 | 时间分组正确 | 构造跨天会话，断言分组归属正确（今天/昨天/更早） | 截图 | ⬜ |
| 3 | 点击进入对应会话 | 断言 URL 与内容匹配（**不是错位的另一个会话**） | 截图 + 断言 | ⬜ |
| 4 | 新建会话 | 断言创建成功并跳转新会话 | 截图 | ⬜ |
| 5 | 重命名会话 | 改名后断言列表与详情均更新；刷新后持久 | 截图 + 刷新验证 | ⬜ |
| 6 | 置顶 pin | pin 后断言排到顶部；刷新后保持；unpin 恢复 | 截图 | ⬜ |
| 7 | 删除会话 | 断言从列表消失 + 详情不可访问 | 截图 | ⬜ |
| 8 | 删除后的软删除语义（E2E-06 后） | 断言 DB 行仍在、`deleted_at` 有值；列表/统计不返回 | 断言 | ⬜ |
| 9 | 空态 | 新账号无会话时的展示（非空白页/非报错） | 截图 | ⬜ |
| 10 | 长标题截断 | 超长标题断言不破版 | 截图 | ⬜ |
| 11 | 删除/置顶的越权防护 | 用 A 用户 token 操作 B 用户会话 → 断言拒绝（IDOR，历史 Sprint 112 修过 `userIDQuery` 强依赖） | 断言 | ⬜ |
| 12 | 列表分页/大量数据 | 50+ 会话下断言加载正常无卡死 | 截图 | ⬜ |

## 5. 验收标准（DoD）

- [ ] 12 个测试点通过
- [ ] 范围内 bug 按 TDD 修复
- [ ] 回归钉写入 `emotion-echo-web/e2e/conversation-management.spec.ts`
- [ ] A11（标题空白）行为被 spec 钉死，防复发

## 6. 已知风险

| 风险 | 应对 |
|------|------|
| E2E-06 的软删除改造若未同步改列表查询，会出现"删了还在" | 测试点 8 专门覆盖；两阶段需衔接 |
| 侧栏在移动端折叠，断言选择器可能不稳 | 用桌面 viewport 为主做功能断言，移动端只做布局抽查 |
| 会话数据依赖聊天产生，种数据耗时 | 用 API 直接建会话作为种子（不做 UI 造数） |

## 7. 产出物

- Playwright spec：`emotion-echo-web/e2e/conversation-management.spec.ts`
- 执行记录：`stages/e2e-08-conversation-management/report.md`
