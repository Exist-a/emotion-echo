---
status: 后端切片 done（9/9 PASS + 第二方核对 PASS）；CL-07 整体仍 in-progress（前端半待 D 释放 [id].vue）
stage: CL-07
slug: cl-07-fusion-consumption
scope: 后端切片（fusion 消费）
owner: Lane-CL-G（接手原 C 会话位）
created: 2026-10-16
updated: 2026-10-16（补 T-9 端到端实拍）
---

# CL-07（后端切片）· 执行记录（report）

> **结论：9/9 测试点 PASS（含 T-9 端到端实拍）**；按 RUNBOOK §6.7，阶段 final `done` 须经**第二方核对**，**执行者不得自行宣布**。

## 起点

- **背景**：`fused_emotions`（三模态收敛后的综合情绪）**只写不读** —— 唯一消费点是按消息查的端点（前端零调用），按模态视图主动排除它。
- **触发**：用户指示**接手原 C 会话位**（CL-02 已 done 并撤会话）继续推进；本轨（G）选取 **CL-07 后端切片**（`fusion-result-consumption` Phase 1~3），因整阶段会与在飞 D 轨（CL-05）争 `pages/chat/conversation/[id].vue` + `stores/message.ts`。
- **开工前置**：契约 §五 五步 + 登记 PR [#232](https://github.com/Exist-a/emotion-echo/pull/232)（§三 占位 + §四 握手三行）；worktree `../Emotion-Echo-cl07`（off `origin/main`）。

## 实施（TDD RED→GREEN，3 阶段 + 端到端实拍）

| Phase | 改动 | RED → GREEN |
|---|---|---|
| 1 | `ai-svc/migrations/i010_create_daily_emotion_fused_v.sql`（视图）+ `analytics-svc/migrations/a012_grant_daily_emotion_fused_v.sql`（授权 analytics_reader）+ 2 个契约钉 | RED: 文件缺失 → GREEN |
| 2 | `analytics-svc/internal/repository/fused_report_repository.go`（接口+InMemory+Postgres）+ `svc/servicecontext.go` 加字段 + `logic/reports_daily_logic.go` 装配 + `main.go` 生产接线 + 测试 | RED: `undefined: FusedReportRepo` → GREEN |
| 3 | `proto/metric.proto`（`ReportsDailyResponse` 加 4 字段 + `FusionMethodBreakdown`）→ `bash proto/gen.sh metric.proto` 重新生成 → `grpcserver/metric_server.go` 映射 → BFF `downstream/analytics{,_grpc}.go` + `handler/analytics_view.go` + 测试 | RED: 编译失败/断言字段缺失 → GREEN |
| **T-9** | **起 dev 栈（`:cl07` 镜像）+ 网关端到端实拍** | 见下 §证据 |

**TDD 实际抓到的真缺陷（本轨就地修，不入账）**：
`InMemoryFusedReportRepo` 用 `time.Time` 作 map key ⇒ **含时区**：logic 层用 `time.Local` 解析日期、测试用 `time.UTC` 存 ⇒ 日历日相同但 key 不等 ⇒ **查不到数据**（gRPC 测试实测 `FusedFaceRatio` 期望 0.5 得 0）。
修复：归一到 UTC 零点（`fusedDayKey`）。**这是"契约测试抓 bug"的又一实例**（假绿陷阱：早前一次 `grep` 把 FAIL 吞了，险些误判全绿）。

## 起栈口径（本轨）

- 取锁：`deploy/.devmode-session`（owner `lane-cl-g`）；队列 §3.1 第 6 位（D/F 均已释放）。
- **worktree 构建 + worktree 起栈**：`docker compose -f deploy/docker-compose.infra.yml -f deploy/docker-compose.apps.yml -f deploy/compose.dev.yml -f deploy/docker-compose.cl07-verify.yml --env-file .env.local --profile dev up -d`
  （从 worktree 起 ⇒ `db-migrate` 挂到**本 worktree 的 migrations**，我新增的 i010/a012 才会被应用）
- 镜像：`emotion-echo/analytics-svc:cl07`、`emotion-echo/web-bff:cl07`（H5/条款 10 带轨后缀，不覆盖公共 tag）
- 前置：`deploy/tls/` 与 `.env.local` 从共享目录 **cp** 进 worktree（两者 gitignored；**未改动**共享 `.env.local`）

## 证据（命令 + 输出）

### 单元/契约级（无需栈）

| 项 | 命令 | 结果 |
|---|---|---|
| 视图契约钉 | `go test ./migrations/ -run TestI010 -count=1` | `ok`（PASS 1/1） |
| 授权契约钉 | `go test ./migrations/ -run TestA012 -count=1` | `ok`（PASS 1/1） |
| 仓储单测 | `go test ./internal/repository/ -run TestFusedReport -count=1` | `ok`（6/6） |
| logic 装配 | `go test ./internal/logic/ -count=1` | `ok` |
| gRPC 映射 | `go test ./internal/grpcserver/ -run 'Fused' -count=1` | `ok` |
| BFF 全量 | `cd emotion-echo-web-bff && go test ./... -count=1` | 全 `ok` |
| 构建/vet | 3 模块 `go build ./...` + `go vet` | 通过 |
| proto 生成一致性 | `bash proto/gen.sh metric.proto`（**未改 proto 时**） | `git status` **无 diff** ⇒ 工具链一致 |
| proto 布局守卫 | `python scripts/check_proto_layout.py` | ✅ 通过 |
| 其他本地守卫 | `check_view_consistency.py` / `check_tdd_gate.sh` / `check_residual.sh` / `check_orphan_outputs.sh` | 全 GREEN |

### T-9 端到端（dev 栈实拍，网关 `:19080` + Bearer）

| 步骤 | 命令/动作 | 输出（实测） |
|---|---|---|
| ① 迁移应用 | `docker logs emotion-echo-db-migrate` | `OK i010_create_daily_emotion_fused_v.sql`（0ms）· `OK a012_grant_daily_emotion_fused_v.sql`（0ms）· 退出码 0 |
| ② 视图存在 + 授权 | `psql -tAc "SELECT viewname … LIKE 'daily_emotion%'"` / `has_table_privilege('analytics_reader', …)` | `daily_emotion_fused_v` 在列；授权 = **`t`**（§契约 3 类 A4 缺陷已避） |
| ③ 服务注册 | Nacos `service/list` | `count:7`（6 应用 + llm-service） |
| ④ 基线 | `psql: COUNT(*) … user_id=1 AND day=2026-10-16` | 26 |
| ⑤ **实时触发** | 登录 `echo/echo123` → 建会话 705 → `POST /conversations/705/messages`（"我今天有点累，但也挺开心的"） | 落库 message id=**951** |
| ⑥ 融合链路 | 等 FusionWorker（5s tick）后查 `emotion_analysis` / `fused_emotions` | message 951 各 **1 行**；当日 fused **26 → 27** |
| ⑦ **报表实拍** | `GET /api/v1/reports/daily?user_id=1&date=2026-10-16` | HTTP 200，`fusedEmotion.total = 27`，`emotionDistribution=[neutral 25, anxious 1, happy 1]`，`methodBreakdown=[late_fusion_weighted 27]` |
| ⑧ **空态对照** | `GET …&date=2026-01-01`（无融合数据） | `fusedEmotion` **字段缺省**（omitempty 生效，不报错） |

证据文件：`evidence/t9-daily-report.json`、`evidence/t9-view-rows.txt`。

> **一致性**：报表 `fusedEmotion.total=27` 与视图 `SUM(cnt)=27`（25+1+1）**逐值相等** ⇒ 全链 `fused_emotions → 视图 → analytics-svc → gRPC → BFF → 前端契约` 在真实数据 + 实时触发下贯通。

## 第二方核对（RUNBOOK §6.7，2026-10-16）

**结论：PASS** —— 独立子代理逐条核对 **D1~D9 全过**，并做 **4 组突变实验**证明测试真能红
（`fusedDayKey` 改回恒等 / 掐断 gRPC 映射 / 去掉前端透传 / 删 `deleted_at` 过滤 ⇒ 对应测试均 FAIL，还原后工作区干净）。
它自建独立临时 worktree 取证，未触碰本分支、未提交/未 push。

关键实测补充（其自跑）：`'["text","face"]'::jsonb ? 'face'` → **t**（证明 i010 的 jsonb 数组判定语义正确）；
`SET ROLE analytics_reader; SELECT count(*) FROM …daily_emotion_fused_v` → **60**（授权真生效）；
报表 HTTP 重取与 `evidence/` **逐值相等**（total 27 / 25-1-1）。

发现 5 条（**均不改变 PASS**）：

| # | 严重度 | 现象 | 处置 |
|---|---|---|---|
| 1 | 中 | 核对开始时 cl07 worktree 停在 `docs/cl-07-lock-release`（非 feat 分支） | 流程性；已随并发推进回到 feat 分支，无需改代码 |
| 2 | 低 | `plan.md §A.4` 称"未起 dev 栈"与 `report.md` 的 T-9 实拍**自相矛盾** | ✅ **已修**（A.4 重写为"已起栈实拍"） |
| 3 | 低 | plan 计数/行号漂移（"6 测试函数 + 3 文件"、`main.go:196`） | ✅ **已修**（→ 7 个测试文件；`main.go:200`） |
| 4 | 低 | 真实数据 `available_modalities` 全为 `["text"]` ⇒ face/voice 占比未被真实数据触达 | ✅ **已在本报告记为覆盖缺口**（SQL 语义经其实测正确） |
| 5 | 低 | 迁移编号撞 CL-03 的 `a011_dlq_replay_runs.sql` | ✅ **已修**（本轨 `a011_*` → `a012_*`，含测试与文档引用；dev 库已重跑确认 a012 `OK`） |

## 结论

- **9/9 测试点 PASS**（T-1~T-9），含端到端实拍与空态对照。
- **交付物**：融合产物从 `fused_emotions` → `daily_emotion_fused_v` → analytics-svc 报表 → gRPC → BFF → 前端契约 **全链已通**（前端图表可接，属下一轮）。
- **已知覆盖缺口（第二方核对指出）**：真实数据里全部 fused 行的 `available_modalities` 均为 `["text"]` ⇒ `face_ratio`/`voice_ratio` 在**真实数据**下恒 0 —— "多模态占比"这一跳未被真实数据触达（`?` 的 SQL 语义已由第二方**实测**正确：`'["text","face"]'::jsonb ? 'face'` → `t`）。纯文本场景下属数据使然、非缺陷；待真实多模态融合行出现时自然覆盖。
- **新登账本**：`CL-F-26`（modality 维度生产未接线 + proto 无字段，同型"只算不消费"）。
- **未做（明确）**：前端半（`[id].vue` 顶部状态条 / 气泡融合标签 / 数字人情绪）—— 待 D 释放该页后另开。
- **第二方核对**：**PASS**（见上节）；后端切片可合并。**CL-07 整体**仍 `in-progress` —— 前端半（`[id].vue`）待 D 释放后另开。

## 经验

1. **"只算不消费"是本仓的反复模式**：fused（本次修）、modality（CL-F-26）均属同类。后端切片的价值不在"多写一个视图"，而在**把已算的产物送到用户能看见的地方**。
2. **改已应用迁移是红线**：`migrate.sh` 对 checksum 不一致直接 `die` ⇒ 新视图的授权必须走**新迁移**（a012），不能改 a004。
3. **proto 改动先验证生成可复现**：改前用 `gen.sh` 在未改 proto 时跑一遍，`git status` 无 diff 才算"工具链一致"。
4. **TDD 抓真 bug**：InMemory 时区键缺陷由 gRPC 层测试暴露（跨包调用路径才踩到）。
5. **假绿陷阱复现**：`go test … | grep -E '^(ok|FAIL|---)'` 会吞 FAIL 行 ⇒ **看原始输出**。
6. **起栈踩坑（本轨新增）**：`docker compose --env-file` 的**相对路径按 cwd 解析**（不是 compose 文件目录）⇒ 传 `../../…` 或绝对路径会 fail；正解 = 把 `.env.local` **cp 到 cwd**（worktree 根）再用 `--env-file .env.local`。另：从 worktree 起栈才能让 `db-migrate` 挂到 worktree 的 migrations（否则新迁移不生效）。
7. **实时触发是"最后一跳"的强证据**：历史数据已能验证消费链；但**发一条新消息 → 报表数字随之 +1** 才排除"读的是陈旧快照"的可能。
