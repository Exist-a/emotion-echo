# Lane O（端侧化 stage1）STATUS — T0+T1+T2#1+T2#3 收口（2026-09-24）

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

## 二、环境基线（协议 §五 要求记录）

- `main` = `a6c12fa`（PR #86 squash 后），与 origin/main 同步
- **T0 + T1 + T2#1 全程零 dev mode**：`.devmode-session` 锁未创建/未占用
- 测试环境：vitest（emotion-echo-web，pnpm）+ pytest（宿主 Python）
  - vitest `app/utils/offline/` + `app/pages/demo/` → 42 passed
  - pytest `scripts/on-device-perf/` → 24 passed · `scripts/on-device-golden/` → 13 passed
- worktree：`D:/源码/Emotion-Echo-lane-o-t2`（T2#1 临时）—— 本轮收口删除
- 分支：`test/on-device-webllm-demo-skeleton` 已 squash 合并 + 远端删除（AGENTS §2.5）

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
4. **T1/T2**：云端基线跑分（golden set 注入真实 model_fn —— `emotion-llm-service` 是 gRPC-only port 50051，HTTP 8000 仅有 /analyze；可用 `iter_chat_chunks` + mock fallback（`LLM_API_KEY` 空时）或走 DeepSeek）
5. **T2**：编译链路 + CDN **实测拉流**（5 候选 CDN 实测可达性 + CORS 6 项检查清单 —— 见 `on-device-compile-cdn-2026-09-24.md` §三/§五）
6. **T2**：性能基线**真机测量**（TTFT / tokens/sec / vram / model_load_ms 注入 PerfMeasurement —— 需 WebGPU + WebLLM 引擎真机，IAB 或 Playwright 实测）
7. ~~**T2**：WebLLM 最小 Demo 契约测试骨架~~ ✅ T2#1 完成（PR #86 `f2083fb` + `582269f`；Demo 占位页 + 4 接口 + 42/42 vitest PASS）
8. **T2**：WebLLM Demo **真引擎接入**（dynamic import `@mlc-ai/web-llm` —— 需 §六握手 + `package.json` optionalDependencies；`production bundle 不打包`契约由架构测试保证）
9. **T3**：Demo IAB 验证（唯一借 dev mode 窗口）+ `docs/plans/on-device-decision-pack.md` 决策材料包（**§十二 5 项只有用户拍板**）
10. **D-26 转 accepted**：条件 = §十二 5 项拍板 + 分项 D-26.1~5 补立（ADR §一自载）
11. **OND-F-01**（已登记）：golden set pytest 未接 CI
12. **OND-F-02**（已登记）：perf baseline 24 用例**同样未接 CI**（同一根因：`llm-test.yml` paths 不含 `scripts/`）
13. **OND-F-03**（账本回收）：WebLLM Demo vitest **已实证接 CI**（`web-test.yml` paths=`emotion-echo-web/**`，新文件 `app/utils/offline/**` 自动覆盖；PR #86 CI 27/27 绿即证据）。无需修

## 五、给下次会话的开场动作

1. 读 AGENTS §八 + `parallel-tracks.md` §五 → 开工三查（fetch/status、对方 STATUS 尾 3 行、`.devmode-session` 锁）
2. 读本文件 §四，**从第 4 项云端基线跑分 / 第 8 项 WebLLM Demo 真引擎接入**任选一项继续（T1 三任务 + T2#1 已收口）
3. **用户决议 §十二 决策 2**（MindChat vs Qwen3）—— 不在本轨决议权
4. 独占列红线与编号口径（两套号都查）见协议 §二/§三.资源3
