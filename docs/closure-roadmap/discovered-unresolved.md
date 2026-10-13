---
status: active
priority: high
created: 2026-10-13
type: phase-ledger
---

# 「闭环」阶段发现账本（discovered-unresolved · CL-F-xx）

> 记录本阶段**用户实测 / 建档预探查**中发现、但**不属于当前阶段范围**、未修复的问题。
> 只增不删：已解决项标 ✅ 保留记录。**归属**写未解决的主责阶段（CL-xx，编号见 `roadmap.md`）。

## 账本

| 编号 | 来源 | 现象 | 根因 | 归属 | 状态 |
|------|------|------|------|------|------|
| CL-F-01 | 建档（2026-10-13） | 前端 `messageCache.ts` 全仓零调用（IndexedDB 缓存模块没接线）；`init.ts:108` 离线重试是 TODO | 功能未接线 | CL-05 | 待解决 |
| CL-F-02 | 建档（2026-10-13） | 多模态融合产物 `fused_emotions` 前端零消费（BFF `/emotion/*` 端点齐备无人调） | 功能未接线 | CL-07 | 待解决 |
| CL-F-03 | 建档（2026-10-13） | 对话 prompt 只发 `[system, 最后一条 user]`（多轮＝无记忆单轮） | 组装层缺历史注入 | CL-04 | 待解决 |
| CL-F-04 | 建档（2026-10-13） | GORM 埋点错挂 `Raw()` processor（`shared/pkg/skywalking/gorm_tracing.go:28-31`）⇒ ORM 查询零 span | 注册到错误 processor | CL-01 | 待解决 |
| CL-F-05 | 建档（2026-10-13） | 7 条死 RPC（StreamMessages/Logout/VerifySecurityAnswer/AnalyzeBatch + 3 mental-health）无生产调用方 | proto 预留/未接出 | CL-02 | 待解决 |

> 上表为建档期预探查的**种子账**（对应各待排期计划的核心缺口）；用户实测中新增的发现续号追加。

## 使用约定

- 6 列硬契约：`编号 | 来源 | 现象 | 根因 | 归属 | 状态`（缺列会被审计漏读）。
- 编号续 **CL-F-xx**（**不占用** E2E 的 `E2E-F-xx`）。
- 确认是运行时 bug 且修复落地后 → 标 ✅ 并回填到对应 landed 计划或 `docs/plans/known-issues-*`。
- 归属方未确定时写"待定 + 触发条件"，**不挂已 done 阶段**（防归属误判）。
