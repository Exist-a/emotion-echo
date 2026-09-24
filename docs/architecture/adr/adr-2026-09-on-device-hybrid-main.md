---
status: proposed
priority: high
created: 2026-09-24
related-plans:
  - ../../plans/on-device-hybrid-inference-2026-09-23.md（v0.2 可行性评估）
  - ../../plans/on-device-hybrid-inference-implementation-roadmap-2026-09-23.md（v0.3 实施路线图，§B.2 为本 ADR 立项模板）
---

# ADR-2026-09 端侧化混合推理主方案（D-26 · v0.3）

> **状态**：🟡 **proposed**（2026-09-24 立项，Lane O 阶段一开工触发）
> **转 accepted 条件**：v0.2 §十二 5 项决策全部由用户拍板 → 补立分项 ADR D-26.1~D-26.5 → 本 ADR 转 accepted（v0.3 §B.1 决议流程）。**5 项决策不在本 ADR 决议权**（AGENTS §八 规则 4）。
> **取代关系**：无（本项目首个端侧/离线决策，v0.3 §A.1 #1 核实）
> **关联**：D-25（XTTS v3，独立不冲突）；E2E-17 数字人/TTS（真口型同步与本方案"显性角标"独立，v0.3 §H.3）；并行隔离协议 [parallel-tracks.md](../../../_meta/parallel-tracks.md)

---

## 一、决策（v0.3 §B.2 模板）

按 [v0.2 §二技术选型](../../plans/on-device-hybrid-inference-2026-09-23.md)（WebLLM 主力 + MindChat 待验证双轨）/ [v0.3 §C阶段切分](../../plans/on-device-hybrid-inference-implementation-roadmap-2026-09-23.md)（阶段一 → 准备期 → 阶段二 → 阶段三 → 阶段四）实施**端侧优先 + 云端兜底**的混合推理架构；§十二 5 项决策门治理轮拍板后补 5 个分项 ADR（D-26.1 隐私定位 / D-26.2 模型选型 / D-26.3 来源告知 / D-26.4 离线范围 / D-26.5 摘要存储）。

**阶段一（本 ADR 立项时已开工）不依赖 §十二决策**（v0.3 §A.4），五任务：WebLLM 最小 Demo / MindChat 双轨验证 / 编译链路 + CDN / 性能基线 / golden set 骨架 + 云端基线。

## 二、影响面（v0.3 §B.2 模板）

- **web 单点插入口**：`useAIStreamHandler.sendAIStream` 替换为混合调度层 —— **阶段一禁触此文件**（并行协议 §二）；阶段二在四道门全开后实施
- **新增依赖**：`@mlc-ai/web-llm` + `opfs-polyfill` + WebGPU 类型 + 兼容性垫片（并行协议 §二 白名单，production bundle 不得打包）
- **离线层**：Dexie + outbox（已有基建，v0.2 §A.1 #3）+ JWT 离线宽限（新，依赖 E2E-29）+ AI 回复补传契约（依赖分项 D-26.1 选 (a)）
- **四道门**（v0.3 front-matter）：v1.0 封版 / E2E-17~30 收口 / §十二 5 项拍板 / 本 ADR 转 accepted —— 全开才进阶段二/三/四
- **后端模块**：阶段一零触碰（v0.3 §D.1 占用清单：BFF/chat-svc/user-svc/analytics-svc 均阶段二起）

## 三、调研依据（继承 v0.2 §A.2 + v0.3 §A.1）

- **v0.2 已读（7）**：`ai_stream_handler.go`、`personality_directive.go`、`downstream/llm.go`、`useAIStreamHandler.ts`、`useConversationSender.ts`、`chat_completion.py`、`personality_directive_test.go`
- **v0.3 已读（4）**：E2E roadmap 30 阶段、`adr-2026-09-xtts-v3-repo-image.md`（本 ADR 范式来源）、E2E-17 STATUS.md（教训关联）、`useAIStreamHandler.ts` 现状（240 行，Sprint 108 useState 化后仍未被端侧触碰）
- **外部核实**（v0.2 §A.2）：`mlc-ai/web-llm` `src/config.ts` 预置模型表 / caniuse WebGPU / ModelScope MindChat 存在性
- **本 ADR 立项时补充（2026-09-24）**：`docs/e2e-roadmap/decisions.md` 索引实测 D-25 为最后已用号（D-10~24 保留）→ **D-26 空闲**（E2E-17 收口时让位记录 + v0.3 全文一致）；`docs/architecture/decisions.md` 决策号已排至 32 → **本 ADR 登记为决策 33**（两套编号体系并行：D-NN 归 E2E 轨、决策 N 归 ADR 级，先例 D-09↔决策 26、D-14↔决策 31）

## 四、风险（继承 v0.2 §九 11 类 + v0.3 §F.2 6 类）

v0.2 §九 11 类（端侧质量不足 / 低端设备卡顿 / 首载等待 / WebGPU 兼容 / CDN 成本 / 推理崩溃 / MindChat 谱系不实 / 隐私定位未决 / JWT 过期 / 端云上下文不同步 / CDN 漏 wasm）+ v0.3 §F.2 6 类（dev 容器跑旧代码 / happy-dom 失真 / IAB cookie 局限 / BFF Nacos 同型 / lint 持续红同型 / ADR 门禁过宽同型）**全部继承**，不再复述。

## 五、验收契约指针

阶段验收按 v0.3 §E OC-01~OC-13 接口化契约 + §G 收口契约 4 张执行；阶段一收口 = 8 项 RUNBOOK 收口 + OC-11 golden set 端侧基线有分 + 模型选型材料齐（v0.3 §G.1）。

## 六、登记索引

- E2E 轨索引：[e2e-roadmap/decisions.md](../../e2e-roadmap/decisions.md) D-26 行
- ADR 级索引：[decisions.md](../decisions.md) 决策 33 行
