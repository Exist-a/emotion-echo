---
status: active
priority: high
owner: TBD
created: 2026-10-13
type: phase-roadmap
supersedes: docs/plans/closure-phase-roadmap.md（单文件版 → 升入本目录）
related:
  - README.md / RUNBOOK.md / decisions.md / discovered-unresolved.md
  - ../plans/README.md（待排期清单库存）
---

# 「闭环」阶段（Phase Closure · CL）· 路线图

> **命名**：正式名 **「闭环」阶段**（英文 **Phase Closure**，编号 **CL-xx**）。
> **主线**：**用户实测找问题 + 逐条链路闭环**。
> **一句话目标**：让每条功能都能讲成"用户做了什么 → 系统怎么处理 → 结果怎么回来 → 我怎么验证它对"。

---

## §1 排序（2026-10-13 用户拍板）

用户定的优先级：**端侧再次延期先不做；可观测 / gRPC / Kafka 先做（"基础，不可避免会用到"）；然后 Prompt 组装、消息链路；其余按下排。**

| 档 | 内容 | 定位 |
|---|---|---|
| **T0 工程效率**（先做） | CI 瘦身（CL-00） | 先把 CI 变轻，后续并行才跑得快 |
| **T1 基础设施闭环**（先做） | 可观测 · gRPC 契约 · Kafka/可靠性 | 跨切面地基 → 先固化平台 |
| **T2 对话地基** | Prompt 组装层（**含同会话历史·滑动窗口**）· 消息链路 | 对话类功能公共底座 |
| **T3 对话价值闭环** | 自定义 Agent · 情绪回显+融合 | 用户最可见（面试/演示卖点） |
| **最后** | **记忆系统（CL-08a~e）**：用户事实 → 情绪模式 → 情节+RAG → 管理页 → 风险信号 | 用户裁定"放最后"（会话摘要并入 CL-04） |
| **常驻** | backlog 清销 | 插空核销 |
| **deferred** | 端侧（Lane O） | 再次延期，不进执行路径 |

**补充理由（本文件）**：可观测排 T1 之首——它是本阶段"实测找问题"方法的**眼睛**。**T1 内部微序**：① 埋点修复 → ② gRPC 契约 ∥ Kafka 可靠性 → ③ 面板/告警收尾（别先画空面板）。

**会话记忆决策（CL-D-01）**：拆两部分——① 同会话历史（滑动窗口，近 N 轮）落在 **CL-04**；② 跨会话记忆 = RAG，单列 **CL-08** 并**放最后**。

---

## §2 阶段总表（按执行序）

> 状态取值 `pending / in-progress / blocked / done / deferred`（唯一权威=本表）。
> **并行语义（2026-10-14 起，C 轨补记）**："一次只允许一个主线 `in-progress`"适用于**同一文件面**；
> 当多轨主文件面隔离（见 [parallel-contract.md](parallel-contract.md) §一）时，**可各自 `in-progress`**——
> 当前三轨并行：**A=CL-04 / B=CL-01 / C=CL-02**，唯一串行资源是 dev 栈（单锁排队）与 `proto/**`（C 轨预占）。
> ⚠️ A/B 两行状态由 **C 轨按事实回填**（其登记原写在各自分支上，main 曾滞后为 `pending`）；合并时以 `parallel-contract.md §三` 为准。

| CL | 阶段 | 档 | 含待排期计划 | depends-on | 状态 |
|---|---|---|---|---|---|
| **CL-00** | **CI 瘦身** | T0 | `.github/workflows/*` + `scripts/` 守卫 | — | ✅ **done**（PR #209） |
| **CL-01** | **可观测闭环** | T1 | `observability-instrumentation-and-dashboards` → `observability-business-metrics-wiring` → `grafana-dashboard-gaps`；`redis-two-layer-cache` | — | **in-progress**（B 轨 · PR #211 draft） |
| **CL-02** | gRPC 契约闭环 | T1 | `grpc-deadlink-actualization` | — | **in-progress**（C 轨 · PR #214 draft） |
| **CL-03** | Kafka/可靠性闭环（合并一次执行） | T1 | `outbox-trace-continuity`、`dlq-hardening-and-replay`、`kafka-pipeline-pending-decisions` | 与 CL-01/02 共享追踪面 | pending |
| **CL-04** | Prompt 组装层（含同会话历史·滑动窗口 + 会话摘要） | T2 | `prompt-pipeline-redesign` | — | **in-progress**（A 轨 · PR #213 draft） |
| **CL-05** | 消息链路闭环 | T2 | `frontend-message-cache-offline-resilience` | — | pending |
| **CL-06** | 用户自定义 Agent | T3 | `user-custom-agent-persona` | CL-04 | pending |
| **CL-07** | 情绪回显 + 融合消费 | T3 | `chat-emotion-realtime-display`、`fusion-result-consumption` | CL-04 | pending |
| **CL-08a** | 记忆 · 用户事实 | 最后 | `conversation-memory-pending-decision` | CL-04 | pending |
| **CL-08b** | 记忆 · 情绪模式 | 最后 | 同上 | CL-08a | pending |
| **CL-08c** | 记忆 · 情节 + RAG | 最后 | 同上 | CL-08a | pending |
| **CL-08d** | 记忆 · 管理页（查看/删除） | 最后 | 同上 | CL-08a~c | pending |
| **CL-08e** | 记忆 · 风险信号（安全） | 最后 | 同上 | — | pending |
| **CL-09** | backlog 清销（常驻） | 常驻 | `known-issues-backlog-2026-09-16`、`…runtime-bugs-2026-09-17`、`todo-pile-2026-09-04`、`three-vrm-usage-reference` | — | pending（插空） |
| **CL-10** | 端侧（Lane O） | deferred | `on-device-hybrid-inference`(+`…implementation-roadmap`) | 端侧决策门 | **deferred** |

---

## §3 排期路径

```
T0 工程效率（先做）
  CL-00 CI 瘦身（合并 job + 删冗余检查）
T1 基础设施
  CL-01 可观测（埋点修复 → 面板收尾）
  CL-02 gRPC 契约        ∥ 可并行
  CL-03 Kafka/可靠性      ∥ 可并行
T2 对话地基
  CL-04 Prompt 组装层（含同会话历史·滑动窗口）
  CL-05 消息链路           ∥ 可并行
T3 对话价值闭环（依赖 CL-04）
  CL-06 自定义 Agent
  CL-07 情绪回显 + 融合消费
最后
  CL-08a 记忆·用户事实 → CL-08b 记忆·情绪模式 → CL-08c 记忆·情节+RAG → CL-08d 记忆·管理页
  CL-08e 记忆·风险信号（安全，横切）
常驻   CL-09 backlog 清销（插空）
延期   CL-10 端侧（再次延期，先不做）
```

---

## §4 做到什么程度

> **口径（用户 2026-10-13）**：**只把"当前阶段"（CL-01）写到可执行的程度**；**未到的阶段不预设细节**——"到时候可能另一个样子"，只留目标一句，detail 进阶段时再定（防过度预设造成漂移）。

### CL-00 · CI 瘦身 —— 已定范围与验收标准

> **核心口径**：**降 job 数、不降覆盖**——砍"繁琐"，但不牺牲门禁效力。

**做什么**：① 把 `doc-drift-check.yml` 的 **24 个 job 合并成 1~2 个**（照 `e2e-guards.yml`：一个 job 跑多步）；② **删检查**：`e2e-stage-audit`（e2e 专属）、3 个 `helm-*-render`（k8s 死资产）、`docs-update`、`user-oauth-zero-ref`（一次性守卫）、e2e-guards 守卫 10/14/26（审计器自查）；③ 合并后删 `doc-drift-gate` + `doc-drift-needs-sync`（元守卫）；④ 分支保护改挂合并后的 job。
**保留**：`secrets-scan`/`tdd-gate`/`adr-gate`/`soft-assert-scan`/`residual-scan`/`orphan-outputs`/`ctx-logging-gate`/`env-vars-lint`/`yaml-lint`/`git-layout`/`docker-digests`/`routes-alignment`/`view-consistency`/`migrations-*`/`apisix-seed-structure`/`obs-monitoring-contract`/`bff-jwt-secret` + e2e-guards 守卫 1-8/11-13/15-25。

**验收标准**：A job 数显著下降（doc-drift-check 24→1~2）· B **覆盖不减**（保留的守卫仍真跑、仍能红）· C PR 绿 + 分支保护仍拦 · D CI 时长下降。

### CL-01 · 可观测闭环 —— 当前阶段（详）

> **本阶段核心验证（用户 2026-10-13）**：验证 **"那些指标/埋点是否都真的接入了面板并投入使用"**——端到端跑通「**埋点 → 暴露 → 采集 → 面板 → 被看/告警**」，**杜绝「埋了点没人看」**。本阶段别的不用多说，就盯这一条：**每个自定义指标都要在面板/告警上被真正消费到**（不全则不算闭环）。

**做什么（范围 In）**：
1. **修 GORM 埋点**（错挂 `Raw()` processor ⇒ ORM 查询零 span；`shared/pkg/skywalking/gorm_tracing.go:27-32`）+ 回归钉；
2. **Redis 埋点接线**（`InitRedis` 零生产调用，接 web-bff redis client）；
3. **HTTP 出站埋点**（统一 `RoundTripper`：exit span + sw8 注入；ai-svc aiclient + BFF downstream）；
4. **Kafka DLQ 生产端 span**；
5. 两个"只定义未埋点"的 fusion 指标（`emotion_fusion_calls_total` / `_duration_seconds`）**接线或删**；
6. **web-bff `SetTracer` 对齐**（或文档记录原因）；
7. **面板/告警**（承接 `observability-business-metrics-wiring`，**不重复造**）；
8. **采集增量**（`/health` 探测；exporter / recording rules **按内存评估后定**）。

**范围 Out（不做）**：不改 19 个已定义指标的语义 / 不动 `charts/**`（k8s 冻结）/ **默认不新增容器**（内存红线）/ 异步 trace 连续性归 **CL-03**（本阶段只做"埋点本身真生效"，含 DLQ span）。

**约束（写死）**：① 不改指标语义；② 内存红线；③ **span 只在有 `ctx` 时建，不硬造根 span**；④ 面板以既有计划为准；⑤ **诚实性——"埋点接了但没验证"不算完成，必须见 span 真的出现在 SkyWalking UI**。

**做到什么程度（DoD）**：
- 功能：ORM 查询在 SkyWalking 出 span（修前无/修后有）；HTTP 出站出 exit span；面板能答"**是否在降级 / 是否在跟 mock 对话**"；**一条告警真触发并送达**。
- 机器：GORM processor 回归钉 + ORM 出 span；RoundTripper 单测；DLQ span 单测；`SetTracer` 装配断言；看板每个 expr 返回非空 series（`smoke_observability.py`）；**负向对照**（故意触发一次，观测 firing→送达）。

### CL-02 · gRPC 契约闭环 —— 已细化验收标准

> **验收标准（用户 2026-10-13）**：后端**各服务间调用严格走 gRPC**；**HTTP 仅作显式降级**（`*_TRANSPORT=http` 回滚开关）——降级**可接受**，不算违规。（gRPC 搞了很久，本阶段要把它**坐实**。）

**配套清理（坐实 gRPC 面）**：gRPC 面上 7 条死链（`StreamMessages` / `Logout` / `VerifySecurityAnswer` / `AnalyzeBatch` + 3 条 mental-health）逐条**接真 / 归档 / 清除**——确保面上**没有"构建了没人用"的 RPC**；守卫从"数标记条数"升级为"**每条 RPC 有明确状态 + `live` 必有调用方**"。

**待确认边界**：ai-svc → FER/SenseVoice/XTTS 是**模型服务**（本身 HTTP/FastAPI），是否算本阶段的"服务间调用"？（倾向：不算，模型服务走 HTTP 是设计。）

### CL-03 · Kafka/可靠性闭环 —— 已定验收标准

> **验收标准（用户 2026-10-13）**：**修完之后链路贯通**——一条消息的 trace 从入站 → outbox → relay → Kafka → 消费端**连成一条**，且**能结合可视化链路（SkyWalking UI）查看**。本阶段就盯这一条，别的从简。

**做什么**（合并三份计划，主体是前两份）：
- ① **outbox → Kafka 的 trace 断点修复**（outbox 加 `trace_sw8` 列 + relay 恢复父 ctx）；
- ② **DLQ 硬化四问题**（双失败黑洞 / Noop 静默 / 只进不出 / 堆积不告警）；
- ③ **Kafka 待决策收尾**（D1 长期项 / D6 / D7 / D8-2；D3/D5 待多副本触发，本阶段只登记）。

### CL-04 · Prompt 组装层 —— 已定验收标准

> **核心口径**：**"AI 记得你上一句说了什么"**——从无记忆单轮 → 有上下文的连续对话。

**做什么**：① 抽出统一组装层（零回归）；② 注入同会话历史（滑动窗口）；③ 预算/参数（`MaxTokens`/`Temperature` + 截断）；④ 配置化；⑤ 清死路径 + 修 `file_context.py` bug。

**验收标准**：A 零回归（无画像/无历史时输出**逐字等于旧输出**）· B 记得上文（多轮体现）· C 可控（设了 `MaxTokens`/`Temperature` + 超长截断）· D 配置化（system 文案集中一处）· E 死路径清零 + `file_context` 多附件全输出。

**已定参数（2026-10-13 用户，按推荐）**：
- **滑动窗口**：默认**最近 10 条消息（≈5 轮）**，上限 **20 条（≈10 轮）**，可配；**以 token 预算为硬顶**（条数只是粗筛）。
- **死路径**：`with_intent` 意图注入（`llm-service/intent.py:113 inject_style`）**接回**（BFF 下发 `with_intent=true`）；`useAIStream` / `buildRCTPrompt` **删除**。
- **配置载体**：**Go 常量集中**（不引外置模板；将来需热更再局部替换）。

> ⚠️ **会话记忆另有专项**（用户 2026-10-13：陪伴类需重点设计，非"滑动窗口 + RAG"这么简单）——已展开为 **CL-08a~e**（见 §2 / §4）；详设文档待补。

### CL-05 · 消息链路闭环 —— 已定验收标准

> **核心口径**：**"消息链路闭环"**——分页契约对齐 + 缓存接线 + 断网可补发，不再是半成品。

**做什么**：① 消息**游标分页**端到端（proto→chat-svc→BFF→前端）；② 修"取最早 N 条"（改 `ORDER BY id DESC`，最新在前）；③ 会话列表分页两端对齐；④ `messageCache` 接线（读加速 + 写穿 + **Dexie 索引修复**）；⑤ 状态机规范 + 乐观插入；⑥ `retryCount` + 退避 + 上限；⑦ **断网重连重扫**（复用 `client_msg_id` 幂等）；⑧ 缓存治理；⑨ 补测试。

**验收标准**：A 翻页真生效（能翻第二页 + 默认先给最新）· B 刷新不丢（缓存接线）· C 断网能补发（联网自动重试、**幂等不重复**）· D 状态机 + 重试（`sending`/`sent`/`failed` 真赋值、`retryCount` 有上限）· E 回归钉（Playwright 翻页/刷新/断网 + 单测；**E2E-F-205/206 关闭**）。

**已定决策（2026-10-13 用户，按推荐）**：**D-B 会话列表 = B1 游标化**（与消息一致，改后端 proto+SQL）；**D-G 多标签页 = defer**（本期不做）。（计划内 [已定] 项：D-A 游标键=`id`/backward；D-C 缓存只做读加速+离线队列；D-D 状态机单一来源；D-E 重试 MAX=3/指数退避/复用 `clientMsgId`；D-F trim 治理。）

### CL-06 · 用户自定义 Agent（**多角色**）—— 已定范围与验收标准

> **核心**：用户自定义**多个** Agent（各有人设 / 音色），**在会话界面像切模型一样切换**；**切换只限本会话**、**切了就用新的**。默认「小E」（女生）。

**做什么**：
- ① **角色管理**（侧边栏新标签「**角色**」）：增 / 改 / 删；**每用户含默认最多 5 个**（默认「小E」**可删可改**）。
- ② **字段**：名字 / 性别 / 音色（8 选 1）/ 背景人设 / 语气风格 / 边界。
- ③ **会话级切换**：每个会话绑定一个"当前 Agent"，切换**只影响本会话**（别的会话可挂别的 Agent）。
- ④ **消息记 `agent_id`**：每条 AI 消息记录是**哪个 Agent** 回的（先看看效果）；切换前的历史**保留 + 可标注**（"这条是谁回的"）。
- ⑤ **注入**：Agent 人设注入 prompt（槽位，走 CL-04 组装层），与测评画像共存（**Agent 人设 > 测评**）。
- ⑥ **音色**：TTS 音色由 Agent 决定 —— `FunAudioLLM/CosyVoice2-0.5B:<alex|anna|bella|benjamin|charles|claire|david|diana>`（男声 alex/benjamin/charles/david；女声 anna/bella/claire/diana）；换 Agent → 换音色；**硬约束：`phonemes`/`duration` 仍算得出**（口型同步）。
- ⑦ **头像 = 待定**；**数字人形象 = 后续再说**（先不变）。

**验收标准**：A 能建/改/删多个角色（≤5，默认可删改）· B 会话界面可切换、**只影响本会话**、切了就用新的 · C 每条 AI 消息记 `agent_id`、历史可标注 · D 音色随角色变且口型仍同步 · E 安全全过（见下）。

**安全（本功能最危险处，分层防护，2026-10-13 用户认可）**：① **结构隔离 + 指令层级**（用户设定放明确分隔段 `<user_agent>…`，**不可覆盖系统安全规则**；安全规则**后置**声明不可覆盖）· ② **输入过滤**（越狱/注入句式：忽略以上指令 / 你现在是 / 输出你的系统提示）· ③ **人设审批**（保存时审：有害 / 冒充真人 / 宣称资质 → 拒；**v1 先规则式关键词黑名单，LLM-judge 作增强**）· ④ **输出护栏**（自伤/危机/违法）· ⑤ **危机处理独立于人设**（人设关不掉；**单列到 CL-08e**）· ⑥ 字段长度上限 + **受限槽位**（不让写裸 system prompt）· ⑦ 审计/限流/举报 · ⑧ 红队用例回归。
> **核心原则**：用户在「**安全信封**」内自定义人设，信封本身（安全规则 + 危机处理）**动不了、关不掉**。

### CL-07 · 情绪回显 + 融合消费 —— 已定范围与验收标准

> **核心口径**：**"融合算了就得用起来"**——回显给用户 + 进报表 + 影响回复。

**做什么**：① 对话页顶部「当前情绪」状态条；② 消息气泡融合标签；③ 数字人 `setEmotion` 拿到真值；④ 报表出现**融合分布**；⑤ 融合 → prompt 注入（CL-D-07：Redis 用户级最新，取不到降级）。

**验收标准**：A 顶部状态条（多模态消息 → 显示；纯文本**不**误标多模态）· B 消息融合标签 · C 报表含融合分布 · D 数字人真值 · E 融合进 prompt（取不到降级）· F 回归钉。

**已定（2026-10-13 用户）**：
- **状态条只写"用户是什么情绪状态 + 预设颜色"**，**不要花里胡哨一堆字**。
- **报表是给用户看的 ⇒ 不放"融合方法占比/置信度"这类内部技术项**（只放用户可懂的融合分布）。
- 数字人情绪打通 = **方案 A**（前端并发拉 `/fused`，最小改动）。
- 消息级融合标签 = **逐条拉 `/fused`、可见才拉**。

### CL-08a ~ CL-10 —— 未到阶段（只留目标，detail 进阶段时定）

| CL | 目标（一句） |
|---|---|
| CL-08a | 记忆·**用户事实**（抽取 → 存 → 常带注入） |
| CL-08b | 记忆·**情绪模式**（趋势 / 触发 / 缓解因素） |
| CL-08c | 记忆·**情节 + RAG**（事件抽取 + 向量召回） |
| CL-08d | 记忆·**管理页**（用户查看/删除） |
| CL-08e | 记忆·**风险信号**（安全，横切） |
| CL-09 | backlog 清销（常驻）：known-issues 残留 + todo-pile 核销 |
| CL-10 | 端侧（deferred，先不做） |

---

## §5 与既有轨的关系

| 轨 | 状态 | 与本阶段关系 |
|---|---|---|
| Lane E（`docs/e2e-roadmap/`） | 收尾中（28 done / 3 partial） | 本阶段接棒；纪律继续适用（本目录自带精简版） |
| Lane O（端侧化） | 独立推进 → **本阶段延期** | CL-10 标 deferred |
| `docs/plans/` | 库存源 | 每个 CL 落地后，对应计划迁 `legacy-plans/landed/` |

---

## §6 决策门

| 门 | 事项 | 关联 | 状态 |
|---|---|---|---|
| CL-D-01 | 会话记忆形态 | CL-04 / CL-08 | ✅ 已定：同会话=CL-04 滑动窗口；跨会话=RAG=CL-08 放最后 |
| CL-D-02 | 端侧决策 | CL-10 | ✅ 已定：继续延期，不定 |
| CL-D-03 | 阶段名 | 全阶段 | ✅ 已定：采用「闭环」 |
| CL-D-04 | CL-03 三份计划粒度 | CL-03 | ✅ 已定：合并一次"异步闭环"执行 |
| CL-D-05 | CL-07 是否提前 | CL-07 | ✅ 已定：保持 T3 |
| CL-D-06 | 排期顺序 | 全阶段 | ✅ 已定：见 §1 |

> **未到阶段的执行细节不作预设**；进入某阶段时（just-in-time）再逐条提出——见 `decisions.md` 使用约定。

---

## §7 调研依据（本次实读）

- 用户 2026-10-13 排序 + 决策（本文直接落地）
- `docs/plans/README.md`（20 项待排期清单，按真实代码核对后重建）
- `docs/plans/conversation-memory-pending-decision-2026-09-24.md`（D-a~D-c 选项）
- 各待排期计划 front-matter；`docs/e2e-roadmap/{README,RUNBOOK,anti-patterns}.md`
- 本会话前段对 5 个新计划的代码级核对（含 GORM 埋点逐行复核）

**未能确认**（需后续）：各阶段工作量人天、用户实测起点、各阶段内部待拍板项（进阶段时提）。
