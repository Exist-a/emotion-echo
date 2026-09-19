---
stage: e2e-11
title: 我的空间
type: verification
status: partial
created: 2026-09-19
depends-on: [e2e-01, e2e-08, e2e-10]
blocks: [e2e-12, e2e-14, e2e-15]
gate: []
related-findings: [E2E-F-11, E2E-F-13]
---

# E2E-11 我的空间

> 详档。执行协议见 [RUNBOOK.md](../../RUNBOOK.md)——执行前必读，本文件只描述"这个阶段测什么"。
> 执行完成后，在同一目录按 [_REPORT_TEMPLATE.md](../_REPORT_TEMPLATE.md) 写 `report.md`。

## 1. 阶段目标

让"我的空间"页（`/chat/user`）的四条链路从"**代码存在但实测不可用**"变成"**端到端已验证**"：资料读取、资料修改、头像上传（MinIO）、3 个对话行为图表。

## 2. 范围与边界

### 做

| 范围 | 涉及文件 |
|------|---------|
| BFF `GET /api/v1/user/profile` 透传真实用户数据 | `emotion-echo-web-bff/internal/handler/user_handler.go` |
| 头像字段贯穿 4 层链路（user-svc types → proto → BFF downstream → ProfileVM） | `emotion-echo-user-svc/internal/{types,logic,grpcserver}`、`emotion-echo-web-bff/internal/{downstream,handler}` |
| analytics-svc 3 个 UserBehavior gRPC stub 对接 logic 层 | `emotion-echo-analytics-svc/internal/grpcserver/metric_server.go` |
| BFF user-behavior 响应 → 前端契约变换 | `emotion-echo-web-bff/internal/handler/{analytics_handler.go,analytics_view.go}` |
| 我的空间页前端缺陷（notify 引用 / 图标 / 空态 / 上传 / 表单回填） | `emotion-echo-web/app/pages/chat/user/index.vue` |
| 资料修改（昵称校验 + 保存） | 同上 |
| 头像上传链路（前端 → BFF multipart → MinIO → user-svc 落库） | 同上 + `avatar_handler.go` |
| 3 个行为图表渲染 + 空态可读 | 同上 |
| 退出登录弹框 → 跳转 | 同上 |

### 不做（边界）

| 排除项 | 归属 |
|--------|------|
| 测评图表（心理健康评估雷达图） | E2E-14 |
| 设置页（字体 / 主题切换与持久化） | E2E-12 |
| MinIO 匿名读权限与 buckets 策略调优 | E2E-27 |
| 心理测验列表/答题/结果链路 | E2E-13 |
| **年龄落库**（见 §6 风险 R3，需 proto 变更，记账） | 新开 E2E-F 条目 |
| schema 变更 | — |

## 3. 前置条件

| 条件 | 状态 |
|------|------|
| 依赖阶段完成 | ✅ E2E-01（登录会话）、E2E-08（会话管理）、E2E-10（聊天核心） |
| dev 模式容器（infra + apps + dev overlay） | ⬜ 待启动 |
| `deploy/.env.local` 存在（LLM key 唯一存放点，AGENTS.md §四红线） | ✅ 1689 bytes |
| 演示账号 `echo / echo123` | ✅ 已存在（E2E-08/09/10 复用） |
| MinIO 容器健康（头像上传依赖） | ⬜ 待验证 |
| `emotion_echo_analytics.user_behavior_events` 有事件数据 | ⬜ 待通过聊天链路产生 |
| user-svc / analytics-svc / BFF 镜像含本轮修复 | ⬜ **改 Go 后必须 rebuild 镜像再验收**（见风险 R1） |

环境启动命令（**必须带 `--env-file .env.local`**，见 AGENTS.md §四）：

```bash
cd deploy && docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml \
  -f compose.dev.yml --env-file .env.local up -d
```

## 4. 测试点清单

判定标记：`[A]` 自动可判 · `[V]` 视觉判定（需截图并被查看）· `[M]` 需人工/设计裁定。

| # | 测试点 | 判定 | 验证方式 | 证据 | 结果 |
|---|--------|------|---------|------|------|
| 1 | `/user/profile` 返回真实用户数据（非硬编码"体验用户"） | `[A]` | 登录后 `GET /api/v1/user/profile`，断言 `nickname ≠ "体验用户"` 且 `avatar` 字段存在 | curl 输出 | ⬜ |
| 2 | 修改昵称成功 + 页面显示新昵称 | `[A]`+`[V]` | `PATCH /api/v1/users/me {nickname}` → 200；刷新页面昵称更新 | curl + 截图 | ⬜ |
| 3 | 昵称格式校验（2-12 字符，CJK+alnum+下划线） | `[A]`+`[V]` | 输入 1 字符 / 13 字符 / 特殊字符 → 出现提示且**不发请求** | 网络面板 + 截图 | ⬜ |
| 4 | 年龄校验（0-130 范围） | `[A]`+`[V]` | 输入 -1 / 131 → 出现提示且不发请求 | 网络面板 + 截图 | ⬜ |
| 5 | 头像上传成功 + MinIO 存储 + 页面头像更新 | `[A]`+`[V]` | 上传 ≤2MB 图片 → `POST /api/v1/user/avatar` 200 → MinIO 有对象 → 页面头像变化 | curl + MinIO list + 截图 | ⬜ |
| 6 | 头像 >2MB 被前端拦截 | `[A]`+`[V]` | 上传 >2MB 图片 → 不发请求 + 提示出现 | 网络面板 + 截图 | ⬜ |
| 7 | 昼夜使用模式饼图有数据可渲染 | `[V]` | 发 ≥3 条消息产生事件 → 等 30s → 刷新 → 饼图出现 | 截图 | ⬜ |
| 8 | 近 30 天对话频次折线图有数据可渲染 | `[V]` | 同上条件 → 折线图出现数据点 | 截图 | ⬜ |
| 9 | 互动深度指标柱状图有数据可渲染 | `[V]` | 同上条件 → 柱状图出现（5 个指标柱） | 截图 | ⬜ |
| 10 | 图表空态可读（无事件数据时显示"暂无数据"，且不与图表同时出现） | `[V]` | 无事件数据时刷新 → 仅显示空态占位 | 截图 | ⬜ |
| 11 | 退出登录弹框确认 → 跳转登录页 | `[A]`+`[V]` | 点"退出登录"→ 弹框 → 确认 → URL 变为 `/login` | 截图 + URL | ⬜ |
| 12 | 侧边栏"我的空间"导航可进入 | `[V]` | 点侧边栏 → URL = `/chat/user` → 内容渲染 | 截图 | ⬜ |

汇总：12 个测试点 —— `[A]` 6 个、`[V]` 9 个（其中 #2/#3/#4/#5/#6/#11 为 `[A]+[V]` 双证据）、`[M]` 0 个。

**测试点 #4 的范围说明**：只验证**前端校验**（非法值被拦截、不发请求）。年龄**落库**存在独立契约缺口（BFF/proto 无 age 字段），不在本测试点断言范围内，见 §6 风险 R3。

## 5. 验收标准（DoD）

- [ ] 全部 12 个测试点通过（或发现问题已分类：范围内修复 / 范围外记账本）
- [ ] 修复项走完 TDD（Red → Green → Refactor）——已完成的 4 组修复各自带 RED 证据（见 report §4）
- [ ] 已验证行为固化为 Playwright spec 回归钉
- [ ] roadmap 状态更新 + 账本更新（含 R3 新增条目）
- [ ] §2.5 收口自检三连通过
- [ ] §2.4 数据契约 smoke 适用项通过

## 6. 已知风险

| # | 风险 | 应对 |
|---|------|------|
| R1 | 改 Go 后未重建镜像 → 验收测的是旧代码（E2E-10 教训） | 验收前 `docker compose build` 三个受影响服务 + 确认镜像 digest 变化 |
| R2 | `user_behavior_events` 无数据 → 图表测试点（#7~#9）无法验证 | 先通过聊天链路（E2E-10 已验证可用）产生事件，等 outbox→Kafka→consumer 落库 30s |
| R3 | **年龄落库契约缺口**：BFF `UpdateProfileReq` 只有 nickname/gender/birthday/avatarUrl，proto `UpdateProfileRequest` 无 age/birthday 字段 ⇒ 前端 `PATCH {age}` 被静默丢弃 | 本阶段**不修**（需 proto 变更 + 两端重新生成，属独立工作项）→ 记入账本；测试点 #4 只断言前端校验 |
| R4 | MinIO 未随 dev profile 启动 → 头像上传（#5）BLOCKED | 启动后先 `docker compose ps` 确认 MinIO healthy；未启动则降级为 BLOCKED 并记账 |
| R5 | gRPC proto 响应形状与 logic 返回类型不匹配（ChartDataPoint vs 具体结构） | 已在 metric_server.go 用 helper 做显式转换（参照 ReportsDaily 既有模式） |

## 7. 产出物

- Playwright spec：`emotion-echo-web/e2e/my-space.spec.ts`
- 前端静态契约钉：`emotion-echo-web/app/pages/chat/user/e2e-11-my-space-contract.architecture.test.ts`
- 执行记录：`docs/e2e-roadmap/stages/e2e-11-my-space/report.md`
- 截图：`docs/e2e-roadmap/stages/e2e-11-my-space/screenshots/`
