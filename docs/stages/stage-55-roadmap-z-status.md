---
status: parked
priority: medium
stage: 55
date: 2026-09-08
purpose: 路线 Z（观测链路 Sprint B）+ 路线外 backlog 全面盘点，作为下阶段排期的依据
related-stages:
  - stage-44-observability-sprint-b.md (Sprint B 工作定义 16 PR-OBS-X)
  - stage-51-batch-1-infra-merged.md (批 1 parked → 已合 main, 详见 ce3d80b)
  - stage-52-nacos-fix.md (Nacos 阻塞解锁)
  - stage-53-smoke-section4-fix.md (smoke §4 7 天窗)
  - stage-54-port-mismatch-fix.md (prometheus + sw-oap 端口失配)
related-plans:
  - observability-sprint-b.md (16 PR-OBS-X 定义来源)
  - kafka-reliability-gaps.md (Kafka Sprint A/B/C 范围)
  - nacos-enablement-dev.md (Nacos dev 半启用现状, PR-0~5)
  - todo-pile-2026-09-04.md (dev 模式未做项汇总)
related-decisions:
  - adr-2026-09-doc-drift-registry.md (决策 18, 失真台账)
related-sprints:
  - Stage 41 gozero-removal ✅ DONE
  - Stage 42 container-tz-fix ✅ DONE
  - Stage 43 Kafka Sprint A ✅ DONE
  - Stage 44 observability-sprint-b 🚧 进行中（批 1 ✅ 合 main, 批 2/3 未启动）
  - Stage 50 e2e-validation ✅ DONE
---

# Stage 55 · 路线 Z 全面盘点（2026-09-08 截止）

> **本文档是路线 Z（观测链路 Sprint B）+ 相关跨 Sprint backlog 的总盘点**——任何阶段推进前先读本文档，了解整体上下文，避免重做 / 撞车 / 漏掉相关 decision / plan。

---

## 一、路线 Z（Sprint B）整体进展

按 stage-44 observability-sprint-b.md §〇 16 PR-OBS-X 拆三批：

```
Sprint B 整体目标：dev compose 三层补齐（APISIX + Prometheus + Loki + Grafana）+
                  测试护栏（metrics/trace 契约测试）+ Kafka consumer lag 监控
                  （接 Kafka Sprint A §1.4 尾巴）。

                    ┌─────────────────────────────────────────┐
                    │ §四 A 分支未合并 main (前提：所有批)  │
                    │ §四 B 6/6 步业务路径 tag (批 3 主攻) │
                    │ §四 C logging helper (批 2 含 OBS-15) │
                    │ §四 D sw-oap telemetry (批 1 O-1)     │
                    │ §四 E PR-OBS-4/5 干净环境 (批 1)     │
                    └─────────────────────────────────────────┘
```

| 批 | 主题 | 包含 PR-OBS | 状态 | 文档 |
|---|---|---|---|---|
| **批 1**（基础设施） | dev compose 三层 + Kafka lag + runbook | OBS-1/2/3/4/5/6/7/8 + O-1 | ✅ **合并 main（merge ce3d80b）** | stage-51/52/53/54 |
| **批 2**（测试护栏） | metrics/trace 契约测试 + logging + bootstrap regression | OBS-9/10/11/12/13/14/15/16 | ⏸ **未启动**（7 个本地分支领先 main 38-48 commit）| 暂无 |
| **批 3**（业务 tag） | TracerInterface + HTTP EntrySpan + gRPC rpc.* + handler err | OBS-17/18/19/23 | ⏸ **未启动**（4 个本地分支领先 main 53-68 commit，Stage45-50 归档说"业务功能 6/6 步收口"）| stage-45/46/47/48/49/50（在 OBS 分支上未合 main）|

---

## 二、批 2 详情（OBS-9~16）

### 2.1 7 个未合并分支

| 分支 | commit 数 | 主题 | 归档 |
|---|---|---|---|
| `feat/observability-OBS-10-svc-metrics-test` | 38 | 6 svc `/metrics` 端点契约 | 仅在分支上 |
| `feat/observability-OBS-11-fusion-metrics-test` | 40 | ai-svc fused model 指标 | 仅在分支上 |
| `feat/observability-OBS-12-http-trace-test` | 41 | GinSkywalkingMiddleware 端到端 | 仅在分支上 |
| `feat/observability-OBS-13-grpc-trace-test` | 42 | gRPC interceptor 端到端 | 仅在分支上 |
| `feat/observability-OBS-14-kafka-trace-test` | 43 | Kafka consumer span tag 端到端 | 仅在分支上 |
| `feat/observability-OBS-15-logging-helper` | 45 | JSON 日志 helper + 6 svc 接入 | 仅在分支上 |
| `feat/observability-OBS-16-bootstrap-regression` | 48 | BootstrapSkyWalkingTracer fail-fast 契约测试 | 仅在分支上 |

> **注意**：OBS-9 不在 — 已并入 OBS-10 等。

### 2.2 预期工作量

- 7 分支 × 平均 ~3-4 commit = ~25 commit
- 冲突面：**大**（都改 `shared/pkg/{metrics,grpcinterceptor,middleware,logging}` + 6 svc main.go）
- 预计**1-2 天**

### 2.3 关键风险

| 风险 | 说明 |
|---|---|
| 共享代码冲突 | 7 分支都改 shared/pkg/metrics，merge 顺序敏感 |
| 业务代码冲突 | OBS-13/14/15 改 6 svc main.go，merge 顺序需按"基础设施 → handler → metrics → logging"分层 |
| Stage 47/48/49 main.go 改动可能与 OBS-15 重叠 | 需先 grep `logging.Init/SetGlobalSvc` 看冲突 |
| 镜像 rebuild 必须 | 批 2 含业务路径 tag 改动，需 rebuild + 跑 smoke 验证 |

### 2.4 推进策略（推荐）

按 TDD 节奏"骨架先，胶水后"：

```
1. 切 feat/observability-batch-2-test-guards 基于 origin/main
2. 按依赖序 merge:
   OBS-10 (metrics test) → OBS-11 (fusion metrics) → OBS-12 (http trace)
   → OBS-13 (grpc trace) → OBS-14 (kafka trace) → OBS-15 (logging)
   → OBS-16 (bootstrap regression)
3. 每批: go test ./... → python smoke_data_layer.py + smoke_observability.py
4. scripts/build_dev_images.sh rebuild 6 svc 镜像
5. 端到端冒烟: docker compose up + curl + sw-oap UI 验证
6. 合 main
```

---

## 三、批 3 详情（OBS-17/18/19/23）

### 3.1 4 个未合并分支

| 分支 | commit 数 | 主题 | 归档文档 |
|---|---|---|---|
| `feat/observability-OBS-17-spantag-assertion` | 53 | TracerInterface + Span.Tag + 6 svc main.go 包装 | stage-45-observability-sprint-b-regression.md（分支上）|
| `feat/observability-OBS-18-gin-entry-span` | 57 | GinSkywalkingMiddleware 创建 EntrySpan + 4 tag | stage-46-observability-gin-entry-span.md（分支上）|
| `feat/observability-OBS-19-grpc-tag` | 68 | gRPC Server/ClientTracingInterceptor 打 5/4 项 rpc.* | stage-49-grpc-tracing-rpc-tags.md（分支上）|
| `feat/observability-OBS-23-handler-err-propagate` | 63 | EndSpan buildSpanError 三段判定 | stage-48-handler-err-propagate.md（分支上）|

> **stage-50-e2e-validation.md** 也在 OBS-19 分支上——说"业务功能 6/6 步收口 + 端到端验证 14 commit + 镜像滞后同源"。

### 3.2 预期工作量

- 4 分支 × 平均 ~3 commit = ~12 commit
- 冲突面：**中**（PR-OBS-17 改 shared/pkg，OBS-18/23 改 shared/pkg/middleware，OBS-19 改 shared/pkg/grpcinterceptor）
- 预计**1-2 天**

### 3.3 关键风险

| 风险 | 说明 |
|---|---|
| Stage 47（PR-OBS-15 logging）已在 OBS-15 分支 | 与批 3 不重叠，可独立推进 |
| Stage 48（PR-OBS-23 handler err）与 OBS-23 同主题 | OBS-23 分支已落 Stage 48，**批 3 含 Stage 48 改动** |
| Stage 49（PR-OBS-19 gRPC rpc.*）已落 | 批 3 含 Stage 49 改动 |
| Stage 50 e2e-validation 验证 OBS-15/18/19 镜像滞后 | Stage-50 §九.1 报"5 svc 镜像滞后"，批 3 合 main 后需 rebuild 镜像 |

### 3.4 推进策略

```
1. 切 feat/observability-batch-3-biz-tags 基于 origin/main
2. 按依赖序 merge:
   OBS-17 (TracerInterface) → OBS-18 (HTTP EntrySpan) → OBS-19 (gRPC tag) → OBS-23 (handler err)
3. 每批: go test ./... → smoke_data_layer.py + smoke_observability.py
4. rebuild 6 svc 镜像（Stage 47/48/49 改动首次生效）
5. sw-oap UI 验证 http.* / rpc.* / messaging.* tag 出现在 trace 上
6. 合 main
```

### 3.5 Stage 50 e2e-validation.md 也在 OBS-19 分支上

⚠️ **特别提示**：

```
docs/stages/stage-50-e2e-validation.md §九.1 记录了 "5 svc 镜像滞后"
→ 批 3 合 main 后必须 rebuild 6 svc 镜像, 否则 Stage 47/48/49 main.go 改动不生效
→ 决策 18 # 22 失真: "commit msg 报 '7 包全 PASS + 4 包全 PASS + 0 fail'
  实测只覆盖单测 layer, 单测绿 ≠ 端到端绿, 镜像也未 rebuild"
```

---

## 四、stage-44 §四 未做项（路线 Z 直接相关）

按 stage-44 §四 登记的未做项，逐项对应推进状态：

| §四条目 | 描述 | 当前状态 | 下一步 |
|---|---|---|---|
| **A** | 分支未合并 main | ✅ 批 1 已合 main；批 2/3 未合 | 推批 2 + 批 3 |
| **B** | PR-OBS-12/13/14 完整 span tag 断言 | 🟡 批 2 含 OBS-12/13/14 端到端测试 | 批 2 推进时落地 |
| **C** | PR-OBS-15 logging helper 6 svc 接入 | 🟡 OBS-15 在批 2 分支 | 批 2 推进时落地 |
| **D** | sw-oap telemetry 未启用 | ✅ Stage 52 O-1 + Stage 54 SW_TELEMETRY_PROMETHEUS_HOST/PORT 落地 | 闭环 |
| **E** | PR-OBS-4/5 干净环境实跑全绿 | ✅ Stage 54 smoke_observability 12/12 PASS | 闭环 |
| **F** | Kafka Sprint A §1.5 Protobuf 迁移 | 🟡 独立 Sprint C（kafka-reliability-gaps.md §1.5）| 不在路线 Z 范围 |
| **G** | Kafka Sprint A §3 历史数据迁移 SQL 实际执行 | 🟡 运维窗口（kafka-reliability-gaps.md §3）| 不在路线 Z 范围 |
| **H** | Sprint B 范围外 backlog（未做）| 见本文档 §六 | 路线外排期 |

---

## 五、路线外 backlog（todo-pile-2026-09-04.md 全部未做项）

### 5.1 A 类（dev 模式用户路径阻断）

| ID | 项 | 工作量 | 状态 |
|---|---|---|---|
| A1 | TTS 语音回复不可用（XTTS 容器镜像未建 / AI profile 未启）| 半天-1 天 | 🟡 todo-pile §A1，**需 owner 决策** |
| A2 | 文件上传后端未实现（Stage 30 T4.58 占位，需对象存储 + 前端路径同步修）| 1-2 天 | 🟡 todo-pile §A2 |
| A3 | 前端 Nuxt dev server 需手工起（README 已加提示框）| 1 小时（已完成）/ 半天（做容器化）| 🟢 todo-pile §A3 已加提示 |

### 5.2 B 类（架构层面）

| ID | 项 | 工作量 | 状态 |
|---|---|---|---|
| B1 | BFF 端口保留（dev 调试可直连，prod 应关闭）| 半天（prod compose override）| 🟡 todo-pile §B1 |
| B2 | Nacos Go SDK v2.3.5 与 Server v2.4.3 long poll 路径不匹配（ListenConfig 不触发）| 独立 Sprint | 🟡 nacos-enablement-dev.md §二；**stage-52 已修 Register 阻塞，ListenConfig 热更新仍是 backlog** |
| B3 | APISIX seed 多节点 nacos.host | 仅 prod | 🟢 todo-pile §B3 |
| B4 | AI 服务容器未启 dev 默认（FER / SenseVoice / XTTS）| 同 A1，需 owner 决策 | 🟡 todo-pile §B4 |

### 5.3 C 类（文档失真，决策 18 持续登记）

| ID | 项 | 状态 |
|---|---|---|
| C1 | Stage 38 §三阻断 2/误诊 | ✅ 已纠正（commit 049022c） |
| C2 | Stage 38 §四隐患 2 误诊 | ✅ 已纠正 |
| C3 | Stage 39 §七 "4 个 Go svc unhealthy" 不复现 | ✅ 已纠正 |
| C4 | Stage 36-D Bug 2 修复实际从未生效 | ✅ 已纠正 |
| C5 | QUICKSTART.md BFF 端口命名漂移 | 🟡 todo-pile §C5 + stage-53 §七 A 继承 |
| C6 | Stage 38 阻断 6 "quick-login 端点从未实现" | 🟡 todo-pile §C6（**不算阻断**，但口径不一）|
| C7 | Stage 36-FU dashboard 空根因 | ✅ **stage-53 闭环** |
| C8 | BFF 路由清单无契约测试 | 🟡 todo-pile §C8（**1.5-2 天，含前端 + APISIX 三方对齐**）|

### 5.4 D 类（杂项，小可批量做）

| ID | 项 | 工作量 |
|---|---|---|
| D1 | 清理 .zcode/、playwright-report/、test-results/ | 5 分钟 |
| D2 | 删本地 30+ 条已合并的旧分支 | 5 分钟（git branch --merged main） |
| D3 | user_oauth 表零引用（删还是留要 ADR 决定）| 1 小时 |
| D4 | BFF isLocked + recordFailure 登录限流无测试 | 半天 |
| D5 | chat-svc 表依赖清单 ADR（决策 18 §4.5 标注"未做"）| 半天 |
| D6 | Helm chart 与 compose dev 配置全面对齐 | 1 天 |

---

## 六、本批（Stage 52-54）实测发现的新 backlog

### 6.1 stage-52 §六未做项

| ID | 项 | 工作量 |
|---|---|---|
| A | Nacos IP 显示 `0.0.0.0`（resolveRegisterIP() 未生效）| 半天 |
| B | PR-1 配套测试反模式需要元层修复（`nacos_register_persistence_test.go` 只验证假象）| 半天 |
| C | dev Nacos 注册依赖 host loopback 怪异（Windows WSL2）| 已用 docker exec 绕开 |

### 6.2 stage-53 §六未做项

| ID | 项 | 工作量 |
|---|---|---|
| A | §4 summary "0 段对话 0 条消息"（event_type='conversation' 失配细分 enum）| 1-2 小时 |
| B | 业务链路"今天无数据"（llm-service 未配 LLM 凭据）| 独立 Sprint |
| C | smoke 不再断言"今天"——若要恢复需业务链路补 LLM trigger | 同 B |

### 6.3 stage-54 §七未做项

| ID | 项 | 工作量 |
|---|---|---|
| A | §4 summary 0 段对话（继承 stage-53 A）| 同上 |
| B | 业务链路今天无数据（继承 stage-53 B）| 同上 |
| C | prometheus.yml 应改为自动生成（从各 svc yaml 读 port）| 半天 |
| D | build_dev_images.sh 脚本误报 FAIL（Image Built 但脚本报失败）| 1 小时 |

---

## 七、跨 Sprint 推进关系图

```
Stage 44 Sprint B (路线 Z)
   ├── 批 1 基础设施 ✅ DONE (Stage 51~54, merge ce3d80b)
   ├── 批 2 测试护栏 ⏸ 未启动 (Stage 56~58 候选)
   └── 批 3 业务 tag ⏸ 未启动 (Stage 59~60 候选)

Kafka Sprint A
   ├── 阶段 Stage 43 ✅ DONE
   ├── Sprint B §1.4 尾巴 ✅ 批 1 含 (PR-OBS-7)
   └── Sprint C §1.5 Protobuf 迁移 ⏸ 独立 Sprint (不在路线 Z 范围)

Nacos 治理 (nacos-enablement-dev.md)
   ├── PR-0~PR-5 ⏸ 未启动 (大半已被路线 Z 间接落地)
   ├── ListenConfig 热更新 ⏸ 未启动 (B2 backlog)
   └── PR-2 web-bff 走 Nacos 发现下游 ⏸ 未启动

决策栈收口 (decisions.md 9/12 BFF 入口语义)
   └── Stage 53 §五 + todo-pile §C5 跨文档收口 ⏸ 未启动

todo-pile §C8 BFF 路由三方契约
   └── BFF main.go + 前端 apiRoutes.ts + APISIX seed.sh 三方对齐 ⏸ 1.5-2 天
```

---

## 八、本文档使用决策

### 8.1 何时读本文档

- 启动任何 stage 推进前（避免重做 / 撞车）
- 排期路线 Z 批 2/3 时
- 排期 Sprint C Protobuf 迁移时
- 评估"下一步"时（看 backlog 优先级）

### 8.2 何时更新本文档

- 任何 stage 推进后（增删对应行）
- 新决策（decision 18 失真）登记
- 跨 Sprint 关系变化（如 Stage 53 闭环 todo-pile §C7）

### 8.3 不要在本文档写实施细节

实施细节走 stage-NN-*.md 子归档（按 AGENTS.md §七 文档写入约定）。

---

## 九、未做项优先级排序（建议）

按"用户能立即用 / 修起来快 / 阻塞面广"由高到低：

| 优先级 | 项 | 工作量 | 阻塞面 |
|---|---|---|---|
| 🔴 P0 | 路线 Z 批 2（OBS-9~16 测试护栏）| 1-2 天 | 业务改动破坏 observability 契约 |
| 🔴 P0 | 路线 Z 批 3（OBS-17/18/19/23 业务 tag）| 1-2 天 | SkyWalking UI 业务维度不可聚合 |
| 🟡 P1 | todo-pile §C8 BFF 路由三方契约收口 | 1.5-2 天 | 路径不一致导致 future bug |
| 🟡 P1 | todo-pile §A1/B4 TTS / 多模态 AI 决策 | 半天（决策）+ 1天（实施）| 需 owner 拍板 |
| 🟡 P1 | todo-pile §A2 文件上传 | 1-2 天 | 用户功能缺失 |
| 🟢 P2 | stage-53 §六 A（§4 summary event_type 修复）| 1-2 小时 | summary 不准确 |
| 🟢 P2 | stage-54 §七 C prometheus.yml 自动生成 | 半天 | 防止 port 失配 |
| 🟢 P2 | stage-54 §七 D build script 误报 | 1 小时 | 误报 FAIL |
| 🟢 P3 | todo-pile D1-D6 杂项批处理 | 各 5 分钟-1 天 | 仓库卫生 |
| 🟢 P3 | Kafka Sprint C §1.5 Protobuf 迁移 | 独立 Sprint | 数据格式演进 |
| 🟢 P3 | todo-pile §B1 BFF dev/prod 端口分离 | 半天 | dev 调试风险 |

---

## 十、与现有文档的一致性修复（顺手做）

`docs/architecture/roadmap.md` 已含 Stage 45-51 索引 + 新下一步行动，**Stage 52-54 + Stage 55 索引需补**（本 stage 完成后做）。

`docs/architecture/decisions.md` 决策 18 累计登记 5 项失真（PR-1 / stage-42 / prometheus 端口 / sw-oap telemetry / smoke §4 查"今天"），**未做集中索引**。

`docs/plans/observability-sprint-b.md` 是路线 Z 工作定义来源，无需修改。

`docs/plans/todo-pile-2026-09-04.md` 大量项已被 stage-52/53/54 闭环（C7 / §A3 / 部分 §B），**§C7 闭环已显式标注 §A3 修复路径 1 已落地**，其他条目需逐项对照本文档 §五更新。

---

> 最后更新：2026-09-08 by 当前协作 Agent
> 调研依据：route-map + stage-44/51/52/53/54 全程 + observability-sprint-b.md + todo-pile-2026-09-04.md + git branch state
> 用途：路线 Z 后续推进 + 路线外 backlog 排期依据