# Lane O（端侧化 stage1）STATUS — T0+T1+T2#1+T2#3+T2#4+T2#5+T2#6+T2#7+T2#8 收口（2026-09-28）

> **本轨进度事实源**（[parallel-tracks.md](../_meta/parallel-tracks.md) §五 指定路径）。
> 格式：已做 ✅ / 未做 ❌ 分列，**禁止美化**（照 E2E-17 STATUS.md 范式）。
> 下次 Lane O 会话开工前必读本文件尾部 + §四待办。

## 一、T0 已完成（3 PR 全 squash 合入 main）

| 项 | PR | 验证 |
|----|----|------|
| 双轨并行协议 + AGENTS §八挂钩 + devmode 锁 gitignore + 本账本 | #78 `356c753` | `e2e_stage_audit.py --all` 30 阶段 0 FAIL |
| **D-26 主方案 ADR**（proposed，v0.3 §B.2 模板）+ 双 decisions 索引（D-26 行 + 决策 33 行）+ 协议编号口径勘误 | #79 `f7441d3` | ADR gate GREEN · doc-drift 14/0 |
| **golden set 评分骨架**（`scripts/on-device-golden/`：7 用例 × 5 分层 + 护栏/长度/特征三率纯函数 + runner model_fn 注入） | #80 `5e27523` | 严格 RED→GREEN 两段 commit，pytest **13/13 PASS**（本地复跑 main 亦过） |

**过程纠偏**（已固化进协议 §三.资源3）：初判"D-26 被占用、应改 D-33/E2E 从 D-34 起"为**误判**——项目两套并行编号（`e2e-roadmap/decisions.md` D-NN 系列 vs `architecture/decisions.md` 决策 N 系列）被我混淆。三方互证后确认：**D-26 归端侧 / Lane E 从 D-27 起（D-NN）、决策 33 归端侧 / Lane E 从 34 起（决策 N）**。

## 一.5、T1 已完成（2026-09-24 续接，PR #84 squash 合入 main=5ceb452）

| 项 | commit | 验证 |
|----|--------|------|
| **MindChat 双轨验证调研材料**（存在性 + License=GPL-3.0 强传染 + 无 WebLLM 编译产物 + 与 WebLLM Qwen3 对比方法骨架 + 20 组题 T2 实跑设计） | `a651ecf` | ModelScope API `GET /api/v1/models/X-D-Lab/MindChat-Qwen2-0_5B` 实测响应 + WebLLM `src/config.ts` raw v0_2_84/base |
| **编译链路 + CDN 清单**（v0.2 §4.1 4 件套产物清单 + 5 候选 CDN：阿里云 OSS 推荐 + wasm 必须自部署 + CORS 契约 6 项检查 + 自编译命令骨架 + sha256 校验） | `ac269f5` | 国内镜像社区资料 + 阿里云 OSS CORS 规则模板 + MLC-LLM 编译参数 |
| **性能基线测量脚本骨架 TDD**（`scripts/on-device-perf/`：PerfMeasurement schema + aggregate + thresholds + compare_local_vs_cloud + render_report） | `7c42411` | **pytest 24/24 PASS**（先 RED 后 GREEN，本地复跑亦过） |

**关键事实新发现**（送 §十二 决策 2 模型选型）：
1. MindChat License = **GPL-3.0**（强传染 + 商用需邮件授权，README 明文）—— v0.2 §2.2 评估表漏此约束
2. MindChat ModelInfos 仅 1 个 1.24GB BF16 safetensor，**无 WebLLM 编译产物** —— 选 MindChat 必须自编译
3. WebLLM Qwen3-1.7B-q4f16_1 vram = **2037MB**（实测 config.ts，与 v0.2 §2.2 估计 2.0-2.2GB 一致）
4. **wasm 必须自部署**（`raw.githubusercontent.com` 国内不通，无现成国内镜像）
5. MindChat 上次更新 = 2024-02-06（**19 个月未更新**），与 v0.2 §二"项目 2024 年后未见活跃"一致

**协议合规**：
- 全程零 dev mode（`.devmode-session` 锁未创建）—— 协议 §四 T1 设计如此
- 未触碰 useAIStreamHandler.ts / docs/e2e-roadmap/** / deploy/ / .github/workflows
- §十二 5 项决策权属用户，**本会话未擅自决议**
- 未登 D-NN / 决策 N 新号

**门禁**：
- `e2e_stage_audit.py --all` → 30 阶段 0 FAIL（合并前后一致）
- pytest `scripts/on-device-perf/` → 24/24 PASS
- pytest `scripts/on-device-golden/` → 13/13 PASS（无回归）

## 一.6、T2 #1 已完成（2026-09-24 续接，PR #86 squash 合入 main=a6c12fa）

| 项 | commit | 验证 |
|----|--------|------|
| **WebLLM Demo 契约测试骨架 TDD**（`emotion-echo-web/app/utils/offline/`：deviceCapability + routeDecision + webllmEngine + Demo 占位页 `pages/demo/local-llm.vue`） | `f2083fb` + `582269f`（lockfile 同步） | **vitest 42/42 PASS**（4 文件）+ RED→GREEN 节奏 + 协议 §二 Lane O 独占列全守 |

**关键设计点**（v0.3 §C.1 阶段一任务 1 落地）：
- **`detectWebGPU` 异步化**（sync helper 不能 await `requestAdapter` Promise → `unsupported` 区分）
- **路由决策优先级** = 高危 > 设备 > 超长 > 离线（v0.2 §5.1 "高危×离线"精神推广 —— **安全护栏原则**：危机响应永远第一优先级，不能因端侧条件不满足被静默忽略）
- **`HIGH_RISK_HOTLINE_TEMPLATE` 常量** + `fallback_hotline` 路由（v0.2 §5.1 高危×离线兜底 + §6.1 热线必含）
- **WebLLMEngine interface + Stub**（4 方法 init/chat/abort/dispose；Stub 返回固定占位字符串，T3 才接 `@mlc-ai/web-llm` dynamic import）
- **Demo 占位页**（`<ClientOnly>` + 设备能力 + 路由决策展示 + 显式 WIP 角标，**不调真引擎**）

**协议合规（最严守的一轮）**：
- 全程零 dev mode
- **未触碰** `useAIStreamHandler.ts` / `package.json` / `nuxt.config.ts` / `app/middleware/auth.global.ts` / `docs/e2e-roadmap/**` / `deploy/` / `.github/workflows`
- 路由认证不引入 AuthMiddleware 共享列握手 —— Demo 设计为**无 auth**（不调 useApi/useUserStore）
- §十二 5 项决策权属用户，未决策加码
- 未登 D-NN / 决策 N 新号

**测试覆盖**：
- `deviceCapability.test.ts` — 5 用例（mock navigator 注入 → 3 态）
- `routeDecision.test.ts` — 16 用例（6 决策分支 + 优先级 + 边界）
- `webllmEngine.architecture.test.ts` — 9 用例（静态源扫描 interface + Stub 隔离）
- `local-llm.architecture.test.ts` — 12 用例（路由 12 项契约）
- **合计 42/42 PASS**

**门禁**：
- `e2e_stage_audit.py --all` → **30 阶段 0 FAIL**（合并前后一致）
- PR #86 CI：**27/27 check-runs 全绿**

## 一.7、T2#3 已完成（2026-09-24 续接，§十二 决策 2 落地 · main=待合并）

| 项 | 文件 | 验证 |
|----|------|------|
| **端侧主力模型选型决策材料**（10 维度决策矩阵 + License 实测 + 学术综述印证） | `docs/plans/on-device-model-selection-decision-material-2026-09-24.md` | ModelScope API + GitHub API 实测 MindChat GPL-3.0 + Qwen3 Apache 2.0 + Emo-gml 综述 |
| **D-26.2 端侧主力模型 ADR**（status: **proposed**，用户 2026-09-24 会话口头授权"选 B 吧"） | `docs/architecture/adr/adr-2026-09-on-device-model-selection-qwen3.md` | 10 维度决策矩阵 B 优 9/10；Apache 2.0 商用须知 5 项必做清单 |
| **两套决策索引同步** | `docs/e2e-roadmap/decisions.md` D-26.2 行 + `docs/architecture/decisions.md` 决策 34 行 | 与 D-26 主方案 + D-26.1/3/4/5 流程对齐 |

**选型结论（送 §十二 决策 2 用户拍板）**：**WebLLM 预置 Qwen3-1.7B-q4f16_1-MLC（Apache 2.0）**

**不选 MindChat 核心理由**（决策材料 §五）：
1. **GPL-3.0 copyleft** = 战略层面锁定商用（GPL-3.0 §7 不可撤销；项目若未来转商用须整个代码 GPL 化）
2. **MLC-LLM 自编译成本高**（编译环境 + 编译算力 + CDN 自部署）
3. **学术综述印证**：Emo-gml/Awesome-Mental-Health-LLMs TAFFC 2026 综述——心理垂直 LLM 质量 = base model × instruction tuning × 强 prompt × 护栏代码（通用基座 + §6.5 方法论可达可用线）
4. **跨项目 License 一致**：Qwen3 Apache 2.0 与项目 emotion-echo-web 默认 + 各 Go svc Apache-2.0 一致；GPL-3.0 冲突
5. **维护活跃度**：Qwen3-2507（2025-08 最新）vs MindChat 19 个月未更新

**分级加载方案**（v0.2 §二）：
- 桌面独显：Qwen3-4B-q4f16_1（vram 3432MB）
- 桌面集显/笔记本：Qwen3-1.7B-q4f16_1（默认，vram 2037MB）
- 移动端：Qwen3-0.6B-q4f16_1（vram 1403MB）

**协议合规**：
- 全程零 dev mode（纯调研 + 文档）
- 未触碰 useAIStreamHandler.ts / package.json / nuxt.config.ts / auth.global.ts / 共享文件
- §十二 5 项决策权属用户，**本会话口头授权 = 决策材料，非正式拍板**
- 登 D-26.2（D-NN 体系）+ 决策 34（决策 N 体系）双编号

**门禁**：
- `e2e_stage_audit.py --all` → 30 阶段 0 FAIL（待 PR 合并后验证）

## 一.8、T2#4 已完成（2026-09-27 续接，§四 #4 云端基线跑分 · main=待合并）

| 项 | 文件 | 验证 |
|----|------|------|
| **云端基线跑分 TDD + 实跑**（`scripts/on-device-baseline/`：model_fn 工厂 + 3 实现 + CLI + golden_bridge + 14 pytest + 实跑 N=7 baseline） | `scripts/on-device-baseline/{model_fn_factory.py,model_fns/,run_baseline.py,golden_bridge.py,test_baseline.py,baseline_report.{md,json}}` | **pytest 14/14 PASS**（≥10 任务要求达成）· cloud_grpc 实跑 N=7 → pass=0% / length=0% / guardrail=85.7%（mock fallback 路径，与 §十二决策无关） |
| **云端基线跑分决策材料**（D-26.2 转 accepted 的实证基础 + 架构假设清单 + T3+ 行动项） | `docs/plans/on-device-baseline-report-2026-09-24.md` | §〇.6 文档功课 6 步全做 + §六 不做清单 + §六 行动项 3/3 归属拆分 |
| **账本补登 OND-F-04 / OND-F-05**（T2#4 收口新增） | `docs/plans/on-device-findings.md` OND-F-04 + OND-F-05 行 | open + 待 T3 真机基线复核 |

**实跑结果诚实标注**（决策材料 §三 §四）：
- **本次测的是 emotion-llm-service 在 `LLM_API_KEY` 空时的 mock fallback**（`chat_completion.py:80` `make_mock_chunks` 4 变体 ~50 字固定文案）
- mock 输出 26~38 字 → 全部 length 越界 → pass_rate = 0%（**预期结果**，不是 bug）
- guardrail 85.7% = high_risk-01 唯一挂 hotline_missing（mock 4 变体里无热线模板，**预期**）
- 真实 Qwen3-1.7B 基线**必须** T3 借 dev mode 窗口跑（**不在** T2#4 决议权）

**关键设计点**（决策材料 §四 §六）：
- **ReplyResult 数据类**：model_fn 不只返回 str（golden runner 的 ModelFn 契约），还带 model / fallback_reason / latency_ms 进 baseline 报告（区分"真 LLM 命中 vs mock 兜底 vs 上游失败"）
- **golden_bridge 桥接**：on-device-golden/ 无 __init__.py（不是 Python package），baseline 用 importlib spec_from_file_location 跨目录加载；不改既有文件结构
- **grpc_unreachable 优雅降级**：cloud_grpc 连不上 localhost:50051 时**复用 chat_completion.make_mock_chunks**（**同一** mock 路径，等价于"emotion-llm-service 在 LLM_API_KEY 空时"），保证无容器也能跑 baseline
- **cloud_deepseek 不静默降级**：直连模式无 key 抛 ConfigurationError（让用户知道 key 状态，掩盖 key 过期是反模式）
- **record 录播**：inner_fn 落盘 + 回放（CI 加速 + 调试复盘 + 不依赖容器复跑）

**协议合规**：
- 全程零 dev mode（无 19 容器栈 + 无 `.devmode-session` 锁）
- **未触碰** useAIStreamHandler.ts / package.json / nuxt.config.ts / auth.global.ts / docs/e2e-roadmap/** / deploy/ / .github/workflows
- §十二 5 项决策权属用户，**未决策加码**；D-26.2 仍 proposed
- 未登 D-NN / 决策 N 新号（D-26 / 33 + D-26.2 / 34 仍为 Lane O 全部已占编号）

**测试覆盖**：
- `test_baseline.py` —— 14 用例（factory 3 + cloud_grpc 4 + cloud_deepseek 1 + record 3 + baseline_run 3）

**门禁**：
- `e2e_stage_audit.py --all` → 30 阶段 0 FAIL（待 PR 合并后验证；按协议 §三.资源2 合并后必跑）
- pytest `scripts/on-device-baseline/` → 14/14 PASS
- pytest `scripts/on-device-golden/` → 13/13 PASS（无回归）
- pytest `scripts/on-device-perf/` → 24/24 PASS（无回归）

## 一.9、T2#5 已完成（2026-09-27 续接，§四 #8 WebLLM Demo 真引擎接入架构就绪 · PR #92 已 squash 合并）

| 项 | 文件 | 验证 |
|----|------|------|
| **`createDynamicEngine()` 工厂**（dynamic import + state 暴露 + 不静默降级） | `emotion-echo-web/app/utils/offline/webllmEngine.ts` | `await import('@mlc-ai/web-llm')` 唯一引用方式；chat() 已 init 后抛清晰错误指引 T3 |
| **`DynamicEngineState` + `DynamicEnginePhase`** 类型导出 | `emotion-echo-web/app/utils/offline/webllmEngine.ts` | 4 态联合 idle / importing / loaded / unavailable |
| **架构契约测试**（17 用例 TDD） | `emotion-echo-web/app/utils/offline/__tests__/webllmEngine.dynamicImport.test.ts`（新文件） | 工厂契约 5 + 错误语义 2 + 静态源架构 6 + 类型契约 2 + 静态 import 检测 2 |
| **Demo 页加 Dynamic engine 面板**（按钮触发 + phase 显示 + 错误展示） | `emotion-echo-web/app/pages/demo/local-llm.vue` | UI 测试 ID：`dynamic-phase` / `dynamic-trigger` / `dynamic-error` |
| **`optionalDependencies` 加 `@mlc-ai/web-llm@^0.2.84`**（§六握手列） | `emotion-echo-web/package.json` + `pnpm-lock.yaml` | `parallel-tracks.md §六` 已登 2026-09-27 握手行 |
| **顺手修 T2#1 PR #86 遗留 lint 债** | `emotion-echo-web/app/utils/offline/__tests__/webllmEngine.architecture.test.ts:47` | `regexp/no-unused-capturing-group`（1 处正则 capturing group → 非捕获组） |

**关键设计点**：
- **`createDynamicEngine()` 工厂**：返回 `{ engine, state }` 二元组；首次 `init()` 触发 `await import('@mlc-ai/web-llm')`；**不**真创建 MLCEngine（避免 ~1GB 权重下载）；失败抛清晰错误（AGENTS §3.2 不静默降级）
- **`DynamicEnginePhase`** 4 态联合：UI 直接显示 phase + error
- **production bundle 隔离契约**（架构测试 17 用例强制）：
  - `@mlc-ai/web-llm` 仅以 `await import('@mlc-ai/web-llm')` 形式出现
  - 无任何 `import X from '@mlc-ai/web-llm'` 静态写法
  - 真实隔离策略（worker 入口 / vite external / CDN）留 T3 IAB 验证时决

**T2#5 边界**（避免范围漂移）：
- ❌ **不**真创建 MLCEngine（避免 ~1GB 下载，需 dev mode 窗口 + 真实 GPU）
- ❌ **不**接 chat() 到真引擎流式（依赖 §十二决策 1 拍板）
- ✅ architecture-ready + 链路验证 + 状态对外暴露 + 17 用例架构契约

**协议合规**：
- 全程零 dev mode（dev mode 锁在 lane-e E2E-19 占用，T2#5 不需要）
- ✅ §六握手：`package.json` 改动已在 `parallel-tracks.md §六` 登行（**一次性**注记）
- ✅ 未触碰 useAIStreamHandler.ts / nuxt.config.ts / auth.global.ts / docs/e2e-roadmap/** / deploy/ / .github/workflows
- §十二 5 项决策权属用户，**未决策加码**
- 未登 D-NN / 决策 N 新号（D-26 / 33 + D-26.2 / 34 仍为 Lane O 全部已占编号）

**测试覆盖**：
- `webllmEngine.dynamicImport.test.ts` — 17 用例（工厂 5 + 错误语义 2 + 静态源架构 6 + 类型 2 + 检测 2）
- `webllmEngine.architecture.test.ts` — 9 用例（既有 + 顺手修 1 处）
- `deviceCapability.test.ts` — 5 用例（既有，无回归）
- `routeDecision.test.ts` — 16 用例（既有，无回归）
- `local-llm.architecture.test.ts` — 12 用例（既有 + 加 dynamic 测试 ID）
- **合计 59/59 PASS**

**门禁**：
- vitest `app/utils/offline/__tests__/` → **47/47 PASS**（含新加 17 用例）
- vitest `app/pages/demo/` → **12/12 PASS**（既有 12 用例全过）
- npx eslint app/utils/offline/ app/pages/demo/ → **0 错 0 警告**（顺手清 1 债）
- e2e_stage_audit.py --all → **30 阶段 0 FAIL**（Lane E 审计未被打破）

## 一.10、T2#6 已完成（2026-09-27 续接，§四 #9 §十二决策材料包 · PR #95 已 squash 合并）

| 项 | 文件 | 验证 |
|----|------|------|
| **§十二 5 项决策一站式材料包**（选项 + 风险 + 推荐 + 派生契约表） | `docs/plans/on-device-decision-pack.md`（新建，301 行） | 决策 1 隐私定位 + 决策 2 模型（D-26.2 现状）+ 决策 3 角标 + 决策 4 L0+L1 + 决策 5 摘要 |
| **§六 拍板请求**（用户一次性回模板） | 同上 §六 | 5 项可逐项选；推荐组合与 v0.2 §十二 倾向一致 |
| **§七 给下次 Lane O 开场动作**（拍板后立即执行清单） | 同上 §七 | 4 个分项 ADR + decisions 双登记 + D-26 accepted 状态流转 |
| **§九 给 Lane E 一次性须知**（合并后审计） | 同上 §九 | 决策 1=(a)+决策 4=L0+L1+决策 5=(a) 与现有 E2E 数据契约兼容 |

**协议合规**：
- ✅ docs-only PR（仅写 Lane O 独占列 `docs/plans/on-device-decision-pack.md`）
- ✅ 未触碰 useAIStreamHandler.ts / package.json / nuxt.config.ts / docs/e2e-roadmap/** / deploy/
- ✅ §十二 5 项决策权属用户，**未决策加码**；本材料只推荐 + 拍板请求
- ✅ 未登 D-NN / 决策 N 新号（D-26.1/3/4/5 留待拍板后由 Lane O 立）

**门禁**：
- e2e_stage_audit.py --all → **30 阶段 0 FAIL**
- 不改代码 / 不改测试 —— 纯决策材料
- main squash merged PR #95（7aa8eed）+ 源分支已删 + worktree 已删

## 一.11、T2#7 已完成（2026-09-28 续接，OND-F-05 fixed：golden set 扩 N=13 · PR #104 已 squash 合并）

| 项 | 文件 | 验证 |
|----|------|------|
| **golden_set.jsonl 扩 N=7 → N=13**（daily +1 / high_risk +1 / personality +2 / emotion +2） | `scripts/on-device-golden/golden_set.jsonl` | pytest `scripts/on-device-golden/` → **13/13 PASS**（schema + 5 层覆盖） |
| **新增 6 用例覆盖典型心理疏导场景**（不绑定 §十二决策方向） | 同上 | daily-03 升职 vs 异地父母 / high_risk-02 自残已发生 + 主动求助 / personality-02 意外怀孕 / personality-03 考公 vs 一线 / emotion-03 朋友升职（喜忧参半）/ emotion-04 母亲去世（情绪压抑） |
| **baseline 重跑 N=13 → 84.62% guardrail** | `scripts/on-device-baseline/baseline_report.{md,json}`（重生成） | 与 T2#4 N=7 baseline 对比：pass 持平（mock fallback 预期），guardrail -1.09pp（新增 high_risk-02 mock 不含 hotline） |
| **golden-n13 报告**（D-26.2 ADR §五验收契约前置） | `docs/plans/on-device-golden-n13-report-2026-09-28.md` | §一 调研依据 + §三 设计原则 + §四 实测 + §五 决策影响 + §七 给下次开场动作 + §十 commit 元信息 |

**关键设计点**：
- **不绑定 §十二决策**：扩 6 用例仅覆盖"场景 × 层"，不引入"端云对照/离线/本地双轨"维度（用户拍 §十二任一方向后真机基线直接可用）
- **expect 字段沿用既有契约**：length [100,300] / must_contain: ["我理解"] / must_contain: ["400-161-9995"] / must_not_contain 触发护栏 / ends_with_question
- **用例为原创**：未引用真实患者案例（v0.2 §六.6 心理疏导典型场景，无版权风险）

**协议合规**：
- ✅ 仅触碰 Lane O 独占列（`scripts/on-device-golden/` + `docs/plans/on-device-*`）
- ✅ 未触碰 useAIStreamHandler.ts / package.json / nuxt.config.ts / docs/e2e-roadmap/** / deploy/
- ✅ §十二 5 项决策权属用户，**未决策加码**
- ✅ 未登 D-NN / 决策 N 新号

**门禁**：
- pytest `scripts/on-device-golden/` → **13/13 PASS**（含 schema + 5 层覆盖）
- pytest `scripts/on-device-baseline/` → **14/14 PASS**（无回归）
- pytest `scripts/on-device-perf/` → **24/24 PASS**（无回归）
- 合计 **51/51 PASS**
- e2e_stage_audit.py --all → **30 阶段 0 FAIL**

**T2#7 边界**（避免范围漂移）：
- ❌ **不**接 WebLLM 真引擎（T3 任务，需 dev mode + 真 GPU）
- ❌ **不**跑真实 Qwen3-1.7B 真机基线（T3 任务，需 §十二决策 2 拍板 + dev mode）
- ✅ N=13 用例就绪 + baseline 占位报告 + OND-F-05 fixed

## 一.12、T2#8 已完成（2026-09-28 续接，**§十二 5 项决策用户拍板** + D-26 + 5 分项 ADR 全部 accepted · main=待合并）

| 项 | 文件 | 拍板结果 |
|----|------|----------|
| **D-26.1 隐私定位** | `docs/architecture/adr/adr-2026-09-on-device-privacy-position.md` | 🟢 **(a) 推理本地 + 消息照常上传（两阶段拍板：前期 (a) + 未来 (b) 等排期）** |
| **D-26.2 端侧主力模型** | `docs/architecture/adr/adr-2026-09-on-device-model-selection-qwen3.md` | 🟢 **accepted = WebLLM 预置 Qwen3-1.7B-q4f16_1-MLC（Apache 2.0）** |
| **D-26.3 来源告知** | `docs/architecture/adr/adr-2026-09-on-device-source-disclosure.md` | 🟢 **(a) 「本地完成」角标** |
| **D-26.4 离线范围** | `docs/architecture/adr/adr-2026-09-on-device-offline-scope.md` | 🟢 **L0 + L1（L2 二期）** |
| **D-26.5 摘要存储** | `docs/architecture/adr/adr-2026-09-on-device-memory-storage.md` | 🟢 **(a) 随 D-26.1 联动（a）服务端生成下发** |
| **D-26 主方案 ADR** | `docs/architecture/adr/adr-2026-09-on-device-hybrid-main.md` | 🟢 **accepted**（v0.3 §B.1 决议流程）|
| **decisions.md 双索引登记** | `docs/e2e-roadmap/decisions.md` + `docs/architecture/decisions.md` | 6 行新增/更新（D-26 / 26.1 / 26.2 / 26.3 / 26.4 / 26.5 + 决策 33~38）|
| **v0.2 §十二 + v0.3 §B.1 同步** | `docs/plans/on-device-hybrid-inference-2026-09-23.md` + `on-device-hybrid-inference-implementation-roadmap-2026-09-23.md` | 倾向列 → 实际拍板列（5 项全 accepted）|
| **拍板机制** | AskUserQuestion 工具（user `decided-by: user` 字段记录） | 2026-09-28 Lane O 会话一次性拍完 5 项 |

**关键观察**：
- **决策 1 是两阶段拍板**：用户原文"前期先用 a，b 等排期"——前期 (a) 立即生效；未来切 (b) 时**再开新 ADR 替换**（不自动切换）
- **决策 5 强联动决策 1**：当前 D-26.5=(a) 服务端生成下发；D-26.1 未来切 (b) 时 D-26.5 同步切 (b) 本地
- **阶段一收口契约满足**（v0.3 §G.1）：D-26 + 5 分项全部 accepted → **Lane O 可进 v0.3 §C.3 阶段二（混合架构开发）**
- **强依赖确认**：E2E-21/23/29/30 仍强依赖（v0.3 §C.3 前置）；stage2 不是"现在做"，是"§十二拍板 + E2E 收口后可开工"

**协议合规**：
- ✅ docs-only + ADR-only 改动（4 新建 + 2 改 + decisions.md × 2 + v0.2/v0.3 × 2）
- ✅ 未触碰 useAIStreamHandler.ts / package.json / nuxt.config.ts / docs/e2e-roadmap/** / deploy/ / .github/workflows
- ✅ §十二 5 项决策权属用户，**已正式拍板**（用户 2026-09-28 通过 AskUserQuestion 工具）

**门禁**：
- e2e_stage_audit.py --all → **30 阶段 0 FAIL**
- pytest 51 passed（无回归）
- ADR 6 个全部 accepted 状态流转一致

## 二、环境基线（协议 §五 要求记录）

- `main` = `c289db5`（PR #105 squash 后，T2#7 STATUS 补账），与 origin/main 同步
- **T0 + T1 + T2#1 + T2#3 + T2#4 + T2#5 + T2#6 + T2#7 + T2#8 全程零 dev mode**：devmode 锁 lane-e E2E-19 补 IAB 占用（until 2026-09-28 23:00），Lane O 不抢
- 测试环境：vitest（emotion-echo-web，pnpm）+ pytest（宿主 Python）
  - vitest `app/utils/offline/` + `app/pages/demo/` → **59 passed**（T2#5 +17）
  - pytest `scripts/on-device-perf/` → 24 passed · **`scripts/on-device-golden/` → 13 passed（T2#7 扩 N=13）** · **`scripts/on-device-baseline/` → 14 passed（T2#4）**
- worktree：`D:/源码/Emotion-Echo-lane-o-adrs`（T2#8 临时，本轮收口待删）
- 分支：`docs/on-device-adr-proposed-1-3-4-5` 待 PR 合并后 + 远端删除（AGENTS §2.5）

## 三、CI 覆盖现状（2026-09-24 实测 4 workflow）

| workflow | 对 Lane O PR 的行为 |
|----------|---------------------|
| `go-test.yml` | **每次 PR 必跑**（无 paths 过滤）——#78~#80 均绿（合并成功即 required checks 放行的实证；#79 首次合并曾被 `test (emotion-echo-ai-svc)` in-progress 实拦一次） |
| `doc-drift-check.yml` | 每次 PR 必跑 —— 本地同脚本 14/0，PR 内绿 |
| `web-test.yml` | paths=`emotion-echo-web/**` —— Lane O 文档/脚本 PR **不触发** |
| `llm-test.yml` | paths=llm-service + **只跑 `tests/unit/`** —— `scripts/on-device-golden/` **不在 CI 覆盖内**（见 OND-F-01） |

## 四、未做 ❌（按协议 §四 时间线归属）

1. ~~**T1**：MindChat 双轨验证~~ ✅ T1 完成（PR #84 `a651ecf`）
2. ~~**T1**：编译链路 + CDN 清单~~ ✅ T1 完成（PR #84 `ac269f5`；**国内可达实测拉流**留 T2）
3. ~~**T1**：性能基线测量脚本骨架~~ ✅ T1 完成（PR #84 `7c42411`；**真机测量**留 T2）
4. ~~**T1/T2**：云端基线跑分（golden set 注入真实 model_fn —— `emotion-llm-service` 是 gRPC-only port 50051，HTTP 8000 仅有 /analyze；可用 `iter_chat_chunks` + mock fallback（`LLM_API_KEY` 空时）或走 DeepSeek）~~ ✅ T2#4 完成（PR 待合并；详见 §一.8 + `docs/plans/on-device-baseline-report-2026-09-24.md`）
5. **T2**：编译链路 + CDN **实测拉流**（5 候选 CDN 实测可达性 + CORS 6 项检查清单 —— 见 `on-device-compile-cdn-2026-09-24.md` §三/§五） |
| 5'. ~~**T2#7**：golden set 扩 N=13（解决 OND-F-05）~~ ✅ T2#7 完成（PR #104 `f09bc06`；golden_set.jsonl 7 → 13 用例 + baseline_report 重生成 + docs/plans/on-device-golden-n13-report-2026-09-28.md） |
6. **T2**：性能基线**真机测量**（TTFT / tokens/sec / vram / model_load_ms 注入 PerfMeasurement —— 需 WebGPU + WebLLM 引擎真机，IAB 或 Playwright 实测）
7. ~~**T2**：WebLLM 最小 Demo 契约测试骨架~~ ✅ T2#1 完成（PR #86 `f2083fb` + `582269f`；Demo 占位页 + 4 接口 + 42/42 vitest PASS）
8. ~~**T2**：WebLLM Demo **真引擎接入**（dynamic import `@mlc-ai/web-llm` —— 需 §六握手 + `package.json` optionalDependencies；`production bundle 不打包`契约由架构测试保证）~~ ✅ T2#5 完成（PR #92 `7aaac7d6`；架构就绪 + 17 用例契约测试 + package.json 握手；T3 IAB 才接真引擎流式）
9. ~~**T3**：Demo IAB 验证（唯一借 dev mode 窗口）+ `docs/plans/on-device-decision-pack.md` 决策材料包（**§十二 5 项只有用户拍板**）~~ ✅ T2#6 完成 decision-pack（PR #95 `7aa8eed`）；Demo IAB 验证仍待 dev mode 窗口（§四 #9 拆分为 T2#6 已完成 decision-pack + T3 待 IAB 验证）
10. ~~**D-26 转 accepted**：条件 = §十二 5 项拍板 + 分项 D-26.1~5 补立（ADR §一自载）~~ ✅ T2#8 完成（PR 待合并；§十二 5 项全部用户拍板 + D-26 + 5 分项 ADR 全部 accepted + decisions.md 双索引登记 + v0.2 §十二 + v0.3 §B.1 同步）
11. **OND-F-01**（已登记）：golden set pytest 未接 CI
12. **OND-F-02**（已登记）：perf baseline 24 用例**同样未接 CI**（同一根因：`llm-test.yml` paths 不含 `scripts/`）
13. **OND-F-03**（账本回收）：WebLLM Demo vitest **已实证接 CI**（`web-test.yml` paths=`emotion-echo-web/**`，新文件 `app/utils/offline/**` 自动覆盖；PR #86 CI 27/27 绿即证据）。无需修

## 五、给下次会话的开场动作

1. 读 AGENTS §八 + `parallel-tracks.md` §五 → 开工三查（fetch/status、对方 STATUS 尾 3 行、`.devmode-session` 锁）
2. 读本文件 §四，**从第 8 项 WebLLM Demo 真引擎接入**继续（T0 + T1 + T2#1 + T2#3 + T2#4 + T2#5 + T2#6 + T2#7 + T2#8 已收口；§十二拍板 + D-26 accepted；stage2 = 借 dev mode + E2E-21/23/29/30 收口后可开工）
3. **用户决议 §十二 决策 2**（MindChat vs Qwen3）—— 不在本轨决议权
4. 独占列红线与编号口径（两套号都查）见协议 §二/§三.资源3
