# Lane O（端侧化 stage1）STATUS — T0+T1 收口（2026-09-24）

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

## 二、环境基线（协议 §五 要求记录）

- `main` = `5ceb452`（PR #84 squash 后），与 origin/main 同步
- **T0 + T1 全程零 dev mode**：`.devmode-session` 锁未创建/未占用（协议 §四 T1 设计如此）
- 测试环境：pytest（宿主 Python），`scripts/on-device-perf/` → 24 passed · `scripts/on-device-golden/` → 13 passed
- worktree：`D:/源码/Emotion-Echo-lane-o`（T1 临时）—— 本轮收口删除
- 分支：`docs/on-device-t1-survey` 已 squash 合并 + 远端删除（AGENTS §2.5）

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
4. **T1/T2**：云端基线跑分（golden set 注入真实 model_fn —— 若走 BFF/dev mode **须先在协议 §四资源日历预约 + 拿锁**；纯直连 llm-service API 则零 dev mode，开工时先核实链路再定）
5. **T2**：编译链路 + CDN **实测拉流**（5 候选 CDN 实测可达性 + CORS 6 项检查清单 —— 见 `on-device-compile-cdn-2026-09-24.md` §三/§五）
6. **T2**：性能基线**真机测量**（TTFT / tokens/sec / vram / model_load_ms 注入 PerfMeasurement —— 需 WebGPU + WebLLM 引擎，IAB 或 Playwright 实测）
7. **T2**：WebLLM 最小 Demo（`/demo/local-llm` 独立路由 + 字面量契约测试 + 生产 build 排除）
8. **T3**：Demo IAB 验证（唯一借 dev mode 窗口）+ `docs/plans/on-device-decision-pack.md` 决策材料包（**§十二 5 项只有用户拍板**）
9. **D-26 转 accepted**：条件 = §十二 5 项拍板 + 分项 D-26.1~5 补立（ADR §一自载）
10. **OND-F-01**（已登记）：golden set pytest 未接 CI；**新增 OND-F-02**：perf baseline 24 用例**同样未接 CI**（同一根因：`llm-test.yml` paths 不含 `scripts/`）

## 五、给下次会话的开场动作

1. 读 AGENTS §八 + `parallel-tracks.md` §五 → 开工三查（fetch/status、对方 STATUS 尾 3 行、`.devmode-session` 锁）
2. 读本文件 §四，**从第 4 项云端基线跑分**继续（T1 三任务已收口）
3. **用户决议 §十二 决策 2**（MindChat vs Qwen3）—— 不在本轨决议权
4. 独占列红线与编号口径（两套号都查）见协议 §二/§三.资源3
