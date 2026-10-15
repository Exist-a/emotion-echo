---
status: partial
stage: CL-07
slug: cl-07-fusion-consumption
scope: 后端切片（fusion 消费）
owner: Lane-CL-G（接手原 C 会话位）
created: 2026-10-16
---

# CL-07（后端切片）· 执行记录（report）

> **结论：partial** —— 代码三阶段全绿，但**端到端数据契约实拍（T-9）未做**（dev 栈锁被 D 轨持有，本轨排第 6 位）。
> 按 RUNBOOK §6.4「属本阶段未解决的项存在 ⇒ 只能 partial」，**不得判 done**。

## 起点

- **背景**：`fused_emotions`（三模态收敛后的综合情绪）**只写不读** —— 唯一消费点是按消息查的端点（前端零调用），按模态视图主动排除它。
- **触发**：用户指示**接手原 C 会话位**（CL-02 已 done 并撤会话）继续推进；本轨（G）选取 **CL-07 后端切片**（`fusion-result-consumption` Phase 1~3），因整阶段会与在飞 D 轨（CL-05）争 `pages/chat/conversation/[id].vue` + `stores/message.ts`。
- **开工前置**：契约 §五 五步 + 登记 PR [#232](https://github.com/Exist-a/emotion-echo/pull/232)（§三 占位 + §四 握手三行）；worktree `../Emotion-Echo-cl07`（off `origin/main`）；`.devmode-session` 由 D 轨持有 ⇒ **不起栈**（本切片全 🟢）。

## 实施（TDD RED→GREEN，3 阶段）

| Phase | 改动 | RED → GREEN |
|---|---|---|
| 1 | `ai-svc/migrations/i010_create_daily_emotion_fused_v.sql`（视图）+ `analytics-svc/migrations/a011_grant_daily_emotion_fused_v.sql`（授权 analytics_reader）+ 2 个契约钉 | RED: 文件缺失 → GREEN |
| 2 | `analytics-svc/internal/repository/fused_report_repository.go`（接口+InMemory+Postgres）+ `svc/servicecontext.go` 加字段 + `logic/reports_daily_logic.go` 装配 + `main.go` 生产接线 + 测试 | RED: `undefined: FusedReportRepo` → GREEN |
| 3 | `proto/metric.proto`（`ReportsDailyResponse` 加 4 字段 + `FusionMethodBreakdown`）→ `bash proto/gen.sh metric.proto` 重新生成 → `grpcserver/metric_server.go` 映射 → BFF `downstream/analytics{,_grpc}.go` + `handler/analytics_view.go` + 测试 | RED: 编译失败/断言字段缺失 → GREEN |

**TDD 实际抓到的真缺陷（本轨就地修，不入账）**：
`InMemoryFusedReportRepo` 用 `time.Time` 作 map key ⇒ **含时区**：logic 层用 `time.Local` 解析日期、测试用 `time.UTC` 存 ⇒ 日历日相同但 key 不等 ⇒ **查不到数据**（gRPC 测试实测 `FusedFaceRatio` 期望 0.5 得 0）。
修复：归一到 UTC 零点（`fusedDayKey`）。**这是"契约测试抓 bug"的又一实例**（假绿陷阱：早前一次 `grep` 把 FAIL 吞了，险些误判全绿）。

## 证据（命令 + 输出）

| 项 | 命令 | 结果 |
|---|---|---|
| 视图契约钉 | `cd emotion-echo-ai-svc && go test ./migrations/ -run TestI010 -count=1` | `ok`（PASS 1/1） |
| 授权契约钉 | `cd emotion-echo-analytics-svc && go test ./migrations/ -run TestA011 -count=1` | `ok`（PASS 1/1） |
| 仓储单测 | `go test ./internal/repository/ -run TestFusedReport -count=1` | `ok`（6/6） |
| logic 装配 | `go test ./internal/logic/ -count=1` | `ok` |
| gRPC 映射 | `go test ./internal/grpcserver/ -run 'Fused' -count=1` | `ok` |
| analytics-svc 全量 | `go test ./... -count=1` | 全 `ok`（无 FAIL） |
| BFF 全量 | `cd emotion-echo-web-bff && go test ./... -count=1` | 全 `ok`（无 FAIL） |
| 构建 | 3 模块 `go build ./...` + `go vet` | 通过 |
| proto 生成一致性 | `bash proto/gen.sh metric.proto`（**未改 proto 时**） | `git status` **无 diff** ⇒ 工具链与仓库一致（protoc 32.1 / go 1.36.11 / grpc 1.6.2） |
| proto 布局守卫 | `python scripts/check_proto_layout.py` | ✅ 所有检查通过 |

**未做（BLOCKED，非"未做"）**：T-9 端到端数据契约实拍 —— 需 dev 栈，锁在 D 轨（`deploy/.devmode-session` owner=`lane-cl-d`），本轨排第 6 位。

## 结论

- **partial**：Phase 1~3 代码全绿、8/9 测试点 PASS、1/9 BLOCKED（需栈）。
- **交付物**：融合产物从 `fused_emotions` → `daily_emotion_fused_v` → analytics-svc 报表 → gRPC → BFF → 前端契约 **全链已通**（前端图表可接，属下一轮）。
- **新登账本**：`CL-F-24`（modality 维度生产未接线 + proto 无字段，同型"只算不消费"）。
- **未做（明确）**：前端半（`[id].vue` 顶部状态条 / 气泡融合标签 / 数字人情绪）—— 待 D 释放该页后另开。

## 经验

1. **"只算不消费"是本仓的反复模式**：fused（本次修）、modality（CL-F-24）、此前的 `EmotionDistributionByModality` 均属同类。后端切片的价值不在"多写一个视图"，而在**把已算的产物送到用户能看见的地方**。
2. **改已应用迁移是红线**：`migrate.sh` 对 checksum 不一致直接 `die` ⇒ 新视图的授权必须走**新迁移**（a011），不能改 a004。
3. **proto 改动先验证生成可复现**：改前用 `gen.sh` 在未改 proto 时跑一遍，`git status` 无 diff 才算"工具链一致"，否则要全量 diff 审查。
4. **TDD 抓真 bug**：InMemory 时区键缺陷由 gRPC 层测试暴露（跨包调用路径才踩到）；若只做仓储单测（同包、同一时间对象）会漏。
5. **假绿陷阱复现**：一次 `go test ... | grep -E '^(ok|FAIL|---)'` 吞掉了 FAIL 行，险些误判全绿 ⇒ **看原始输出**。
