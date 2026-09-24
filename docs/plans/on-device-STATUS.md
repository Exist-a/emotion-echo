# Lane O（端侧化 stage1）STATUS — T0 收口（2026-09-24）

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

## 二、环境基线（协议 §五 要求记录）

- `main` = `5e27523`，与 origin/main 同步，本轮收口时 working tree 干净（除他人未跟踪文件，未碰）
- **全程零 dev mode**：`.devmode-session` 锁未创建/未占用（T0 按协议 §四设计即为零运行时）
- 测试环境：pytest 9.1.1（宿主 Python），`python -m pytest scripts/on-device-golden/` → 13 passed
- worktree / 分支：全部清理（§2.5）

## 三、CI 覆盖现状（2026-09-24 实测 4 workflow）

| workflow | 对 Lane O PR 的行为 |
|----------|---------------------|
| `go-test.yml` | **每次 PR 必跑**（无 paths 过滤）——#78~#80 均绿（合并成功即 required checks 放行的实证；#79 首次合并曾被 `test (emotion-echo-ai-svc)` in-progress 实拦一次） |
| `doc-drift-check.yml` | 每次 PR 必跑 —— 本地同脚本 14/0，PR 内绿 |
| `web-test.yml` | paths=`emotion-echo-web/**` —— Lane O 文档/脚本 PR **不触发** |
| `llm-test.yml` | paths=llm-service + **只跑 `tests/unit/`** —— `scripts/on-device-golden/` **不在 CI 覆盖内**（见 OND-F-01） |

## 四、未做 ❌（按协议 §四 时间线归属）

1. **T1**：MindChat 双轨验证（存在性 + license + 与预置 Qwen3 同题 A/B → 支撑 D-26.2 决策材料）
2. **T1**：编译链路 + CDN 清单（**wasm 国内可达**实测口径，v0.2 §4.1）
3. **T1**：性能基线测量脚本（设备加载/生成速度/显存 —— 脚本骨架 TDD；真机测量与 WebLLM Demo 同排）
4. **T1/T2**：云端基线跑分（golden set 注入真实 model_fn —— 若走 BFF/dev mode **须先在协议 §四资源日历预约 + 拿锁**；纯直连 llm-service API 则零 dev mode，开工时先核实链路再定）
5. **T2**：WebLLM 最小 Demo（`/demo/local-llm` 独立路由 + 字面量契约测试 + 生产 build 排除）
6. **T3**：Demo IAB 验证（唯一借 dev mode 窗口）+ `docs/plans/on-device-decision-pack.md` 决策材料包（**§十二 5 项只有用户拍板**）
7. **D-26 转 accepted**：条件 = §十二 5 项拍板 + 分项 D-26.1~5 补立（ADR §一自载）
8. **OND-F-01**（已登记）：golden set pytest 未接 CI

## 五、给下次会话的开场动作

1. 读 AGENTS §八 + `parallel-tracks.md` §五 → 开工三查（fetch/status、对方 STATUS 尾 3 行、`.devmode-session` 锁）
2. 读本文件 §四，从 T1 第 1 项继续
3. 独占列红线与编号口径（两套号都查）见协议 §二/§三.资源3
