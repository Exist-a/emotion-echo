---
stage: e2e-28
title: 性能与延迟基线
executed: 2026-10-03
status: partial
environment: dev 模式（19 容器栈 + obs profile，compose.dev.yml + .env.local + --profile dev,ai,obs）
---

# E2E-28 执行记录

> ⚠️ **状态：收口执行（2026-10-05 续 2026-10-03 停工留档）** ——
> 20/20 测试点终判完成（**18 PASS + 2 FAIL-已分类**：#4 测点加性差定性、
> #19 回归钉未全绿——失败全数归账 F-195/197 非本域回归），8 个修复 PR 已合并 main，
> §8 三节已回填、机器审计 **30 阶段 0 FAIL**、第二方核对进行中（§8.3）。
> 收口 PR 待开（push 需用户放行）。
>
> 阶段上限 **partial**：D-43 裁定的 **F-135（双端点流式播放）
> 未实施**，账本 owner=E2E-28 仍挂（RUNBOOK §7#9）。F-134/F-136/F-26 已闭环。
> 收口新账：**F-195**（播放层 gen 竞争时序敏感）/ **F-196**（:3000 被 web 容器
> 旧镜像抢占——本轮最大排查陷阱）/ **F-197**（mobile fill 不启用按钮）。

## 1. 环境基线

- 启动命令：`cd deploy && docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml -f compose.dev.yml --env-file .env.local --profile dev --profile ai --profile obs up -d`
- 容器状态：核心栈 healthy；obs 栈（prometheus/grafana/alertmanager/mock-receiver）本轮拉起 **targets 12/12 UP**；Nacos `count=7`（6 svc + llm）
- 声明的配置差异：dev 模式（`BFF_DEV_RETURN_CODE=1` 等，RUNBOOK §2.4）；XTTS `cpus=8.0`；`XTTS_SYNTH_WORKERS=2`（本轮新增）
- **过程中环境事件（如实记录）**：① obs profile 首启撞端口 18080（sw-ui D-38）→ TDD 修 mock 端口 18090（PR #152）；② XTTS 镜像重建+模型加载峰值触发 **WSL 8GB 冻结**（dockerd 全 500）→ `wsl --terminate docker-desktop` 恢复，etcd/redis/kafka Exited(255) 手动拉起、APISIX admin 503 重启自愈、seed 路由持久存活；③ 恢复后宿主端口转发未重注册（RUNBOOK §2.4 既知）→ restart apisix/nacos 三件套 + BFF restart 即愈（beat 501 噪声伴随）
- dev server：本地 `pnpm dev --port 3000`（遇 **Vite 504 Outdated Optimize Dep** 致 `[id].vue` 动态导入失败 → 清 `.nuxt`+`node_modules/.vite` 重启恢复；该坑曾让 4 个浏览器测试首跑全红）

## 2. 测试点结果

| # | 测试点 | 判定 | 结果 | 证据 | 备注 |
|---|--------|------|------|------|------|
| 1 | 测量脚本方法自检+负向对照 | [A] | PASS | `test_perf_baseline.sh` **4/4 rc=0**：percentile p50=50.5/p95=95.05（官方线性插值）/ 关闭端口 rc=1+显式 ERROR / http.server 正向 N=5 / SSE n=2+burst | C1 RED→GREEN（PR #153）；已接 e2e-guards 17/17 |
| 2 | /analyze p50/p95（N≥50 两轮） | [A] | PASS | 冷 **271.3ms** 单列（容器重启首测；计划期 435ms 同型）；热 A：n=50 p50=1.339/p95=1.974ms；B：n=50 p50=1.311/p95=1.780ms；**复现比 1.021 ∈[0.5,2]**；零错误 | `baseline/analyze_{cold,roundA,roundB}.json`；容器网 `docker exec python -`（F-c） |
| 3 | 小阶梯 1/2/4/8（/analyze） | [A] | PASS | 四档 ×20 全 200 零错；p50 = 1.201/2.161/3.982/7.715ms（随并发近线性——FastAPI 进程内 CPU-bound 特征）；max 39.5ms | `analyze_ladder.json`；记录不设门禁（D-42） |
| 4 | Prometheus 交叉验证 | [A] | FAIL | 初测（匹配窗口 30 突发）client p50=1.168 vs Prom **0.517 = 2.26x 带外** + p95 钉 1ms 桶界 → **范围内修复**：buckets 补 0.0005/0.002（TDD RED/GREEN，pytest 207 passed，PR #154）→ 复测 p95 **1.44x 回带内**、p50 仍 2.88x → **定性 = 测点加性差 ~0.65ms**（client loopback connect vs server handler；server 真值 p50=0.362ms）；判据三段式回填 plan #4 + 账本 **F-193 ✅**；targets 12/12 | `analyze_crossval_*.json` ×3 + PromQL 输出；**FAIL 不洗 PASS**（已分类处置）：按带宽字面判 FAIL、修复与定性为处置 |
| 5 | 配置事实回读（数字可比性） | [A] | PASS | `check_xtts_cpu_limit.sh` PASS（cpus=8.0）且**接线 e2e-guards 步骤 18/18**（F-h 治理，清单 17→18→19 行同步）；INTERNAL_API_KEY=36 字符 dev 默认（容器/seed 双侧回读） | workflow 行号回读 e2e-guards.yml + docs/ci-workflows/README.md；PR #154 |
| 6 | SSE TTFT 基线（N≥5） | [A] | PASS | n=5：p50=**652.7ms** p95=1801.9 min=551.6 max=2062.1（run1 长闲置后冷 2.06s）；对照计划期 11.7s（长闲置冷）⇒ **TTFT 按闲置态双峰**；零错误 | `sse_ttft.json`；计时=首个含 delta 的 data 块 |
| 7 | 流式渐进性定性（F-a 疑点） | [A] | PASS | 短流 5 轮 **burst=1.0**（触发判据 ≥0.8）→ **长回复鉴别探针**：255 块、tail=1614.7ms、**burst=0.29** ⇒ **真流式成立，伪流式疑点澄清**；判据短流饱和回填 plan #7 + 账本 **F-190 ✅** | `sse_ttft.json` + `sse_long_discriminator.json`；块级不规则时间戳 + 代码逐事件 yield/flush（chat_completion.py:122-133） |
| 8 | SSE 服务端埋点缺口定性 | [A] | PASS | `metrics.go:179` observe 在 `c.Next()` 后 ⇒ /api/v1/ai/stream histogram 记**流总时长**（实测与 total 恒等）；TTFT 551.6~2062.1ms 无任何服务端指标；**处置 = 记账不修**（plan #8 择优：D-42 无门禁消费方 + 客户端脚本已覆盖测量）→ 账本 **F-191 🔴**（触发条件：Grafana TTFT 面板/告警需求） | `metrics.go:150-183` + `ai_stream_handler.go` 行号回读 |
| 9 | 浏览器流式体验对照 | [V] | PASS | 浏览器端 TTFT=**5885ms**（首跑 4866ms）< 30s；气泡长度渐进采样 **5 步** [139,190,269,335,340]（非一次跳变）；截图目视 ✓ | `screenshots/28-09-streaming-bubble.png`（已目视）+ `baseline/browser_stream_timing.json` |
| 10 | TTS 单段冷/热基线 | [A] | PASS | **三层冷**：深冷（容器重启/1.5h 空闲）**50.88s / 40.4s**；5min 空闲冷 n=3 [14.05,12.88,13.23]s；热 40 字 n=16：p50 16.33~22.02s、p95 19.45~29.54s（A/B/B2 三轮）+ 1 次 ConnectionReset（F-192）；冷热比 深冷/热 ≈2.4~3.8x、5min 冷 ≈ 热 | `tts_cold*.json` ×4 + `tts_hot40_round*.json` ×3；对照 ADR 19.9s/27.6s 锚点一致量级 |
| 11 | 段间排队量化（F-136） | [A] | PASS | **修前**：双并发 [12.22s, 23.27s] 串行（后者≈和，排队≈12.2s）；**根因深化**（账本"单 worker"是表象）= async handler 直调阻塞推理锁死事件循环；**修法** = synth_pool 线程池（D-43 目标下按 §8 择优，ADR-2026-10 + 决策 39）；**修后复测**：[36.44s, 38.91s] **重叠**（max/和=0.52，串行应 ~75s）；并行争用下单段 ~37s（vs 独跑 13~30s，torch 线程超订——ADR 负面已预判） | `tts_queue_concurrency2{,_postfix}.json` 前后对照；PR #155；容器三验（镜像 11:04Z / env=2 / /app/synth_pool.py） |
| 12 | 前端段间 gap（F-06/F-129 面） | [A] | PASS | **实例事件毫秒级**：seg0 playing→ended（14.66s 播放）→ seg1 playing，**gap=269ms < 1000ms**；首轮轮询法测得 1032ms 为**测量法缺陷**（探针 v1 挂 document 捕获——`new Audio()` 脱离 DOM 天然盲区，0/0 真因；v2 改构造器登记+实例事件） | `segment_gap.json`（含 events 时间戳）+ 截图 `28-12-segment-gap.png` 目视 ✓ |
| 13 | 端到端 TTS 分解（F-134 运行时） | [A] | PASS | **首轮** ttsFirst==sseFinish==**6785 同刻** ⇒ 暴露 sender **第二层 debounce**（onDelta 500ms 被 token 重置，流中永不触发）→ TDD 修（PR #158）→ **复测 ttsFirst=5350 < sseFinish=5613**（中流发出）；修前模型=全文流完+双 500ms 才首响 | `e2e_decomposition.json`；对照 #10 热态首段更短文本合成更快 |
| 14 | [M] M2 F-134/135/136 裁定 | [M] | PASS | **D-43 用户拍板「双端点+多 worker 全做」**（AskUserQuestion）；F-136 ✅（synth_pool+ADR 决策39+#11 复测）、F-134 ✅（manager+sender 双层，vitest 619→623 零回归+#13 实测）；**F-135 未实施** → 账本仍挂（partial 依据）；裁定与落地状态已同步 decisions.md D-43 + ledger | decisions.md D-42/D-43 行 + 账本 F-134/135/136 状态 |
| 15 | SSE 小阶梯 1/2 并发 | [A] | PASS | 干净轮：c=1 n=3 p50=1006.9ms / c=2 n=3 p50=772.4ms，**零错误**（LLM 方差 > 并发效应，n=3 如实记录不外推）；过程中 **F-192 簇**（1×Reset + 5×401 后同 token 复检 200）→ 重试甄别吸收 | `sse_ladder.json`；期间资源采样见 #16 |
| 16 | 资源对照采样 | [A] | PASS | 阶梯期间 docker stats：xtts 2.54GiB/6GiB（CPU 0.1-0.3%）、llm-service 141-156MiB/1GiB（CPU 0.5-1.2%）、web-bff <1.2%/1GiB；宿主 Docker 视图 9.7GiB 总量，余量足；**xtts 冻结事件后内存纪律**：8GB WSL 红线复证 | 采样输出 + 冻结恢复记录（§1 事件②） |
| 17 | Grafana p95 面板数据核对 | [A] | PASS | Prometheus 重启后先打 60×/analyze 真实流量（p50=1.44ms 全 2xx）再拍：面板渲染 **HTTP p95 Latency 17:00 处真实曲线**（5-28ms 区间）+ Request Rate/Error Rate/Goroutines 四面板全有数；截图 md5=`a1d729dc` 像素机械判定 colorful_px=17218(1.87%)（与上轮停工前 dark-only 假象区分——本轮目视与像素统计一致）；PromQL `histogram_quantile(0.95, llm_http_request_duration_seconds_bucket)` 有值交叉验证 | `screenshots/28-17-grafana-p95-panel.png` + 流量注入记录；folder-uid 过滤（`type=dash-db`）修正确保 `/d/uid` 命中 |
| 18 | [V] 基线报告可读性+视觉取证 | [V] | PASS | 23 份 baseline JSON + 3 张截图逐一目视（28-09 气泡渐进 / 28-12 会话+音频链 / 28-17 Grafana 面板）；数字与单位无占位/错位 | `screenshots/28-*.png` 已被查看（本 report §2 证据列路径可溯） |
| 19 | Playwright 回归钉 | [A] | FAIL | spec 建立 5 用例；**终判双 project（真 dev server=停 web 容器+源码 chunk 复验后）：chromium 3/4 过（#19/#13/#17 绿、#12 败）、mobile 2/4 过（#13/#17 绿、#19/#12 败）⇒ 未"收口跑绿"按字面判 FAIL**。失败归账：#12=F-195（播放层 gen 竞争，双 project，上轮 PASS 本轮同码 FAIL=时序敏感非回归）；mobile #19=F-197（fill 后按钮 5s 不启用，真代码复现非旧代码）。**过程重大发现=F-196**：昨日 web 容器（旧镜像）抢占 :3000 致全套跑旧代码（chromium 4/4 假败），停容器+dev server 重启后 #13 F-134 证据随即 PASS；探针实证 F-134 正确代码下首句流中发出+三句三段（27/30/27 字） | 见 §8.1 终判表 + F-195/196/197 账本行；**FAIL（已分类处置）**：失败均归账非本域回归 |
| 20 | 收口对账与审计 | [A] | PASS | 账本对账：F-26 ✅（23 份 JSON 基线落档）/ F-134 ✅ F-136 ✅ 闭环、**F-135 🔴 如实挂账（⇒ partial）**、F-190✅ F-191🔴 F-192🔴 F-193✅ F-194🔴 + 收口新账 F-195🔴 F-196🔴 F-197🔴 连续编号（下一号 F-198）；`e2e_stage_audit.py --all` **30 阶段 0 FAIL**（2026-10-05，含 e2e-28 ✅ roadmap=partial 三处一致） | 见 §8.3 审计输出 + 第二方核对记录 |

**汇总：PASS 18 / FAIL 2 / BLOCKED 0 / N/A 0**（#4 与 #19 均 FAIL-已分类——处置与归账见各行备注列；四值总和=20 行表行数）

## 3. 发现与分类

| 发现 | 分类 | 处理 |
|------|------|------|
| obs-mock-receiver 与 sw-ui 抢 18080（D-38 注记"届时须改"命中） | 范围内（环境前置） | TDD 修：端口唯一契约守卫 + mock→127.0.0.1:18090（PR #152，RED 18080×2→GREEN 20/0） |
| 交叉验证带外（2.26x）+ p95 钉桶界 | 范围内（#4 判据直接对象） | 桶分辨率 TDD 修复（PR #154）+ 判据三段式回填 + F-193 ✅ |
| check_xtts_cpu_limit 未接 CI（F-h 同型） | 范围内（#5 明列） | 接线 e2e-guards 18/18 + 清单回填（PR #154） |
| `Dockerfile` 漏 COPY synth_pool.py（构建前自查） | 范围内（C4 自身） | 补 COPY + 契约测试钉住（PR #155） |
| env lint 抓 XTTS_SYNTH_WORKERS 未文档化 | 范围内 | 双模板 +1 行（PR #155 内） |
| **sender 第二层 debounce 挡 F-134**（ttsFirst==sseFinish 同刻） | 范围内（#13 实测暴露） | TDD 修（PR #158），复测 ttsFirst<sseFinish |
| Vite 504 Outdated Optimize Dep → `[id].vue` 加载失败（4 测首跑全红） | 环境（dev server 缓存） | 清 `.nuxt`+`node_modules/.vite` 重启恢复；写入 §1 |
| TDD 门禁 <5 行阈值把小测试改动当"无测试" | 范围内（撞上即处理） | 绕行=GREEN 补 ≥5 行真实断言；根治记账 **F-194 🔴**（E2E-03/专项） |
| WSL 8GB 冻结（XTTS 重建+模型加载峰值） | 环境（既往同型第 4 次） | `wsl --terminate docker-desktop` 恢复链 + §1 记录；不改用户系统配置（升级项见 §6） |
| 宿主→19080 间歇 Reset/401 簇（30.008s 定长 RST） | 范围外（宿主端口转发/长连接层） | 记账 **F-192 🔴**（E2E-30/专项）；测量以重试吸收 |
| SSE TTFT 无服务端埋点 | 范围内（plan #8 择优=记账） | 记账 **F-191 🔴**（E2E-30/专项），触发条件已写明 |
| F-135 双端点流式播放未实施 | 范围内未完 | 账本 F-135 保持 🔴 owner=E2E-28 ⇒ **partial**（见 §6 升级项） |

## 4. 修复清单（TDD 记录）

| commit/PR | 内容 | 先行的失败测试 |
|-----------|------|---------------|
| PR #152（RED `ea533d8`→GREEN `1703b82`） | obs-mock 端口 18080→127.0.0.1:18090 + 宿主端口唯一契约（扩展已接线 test_obs_healthchecks） | RED 实测 `FAIL 宿主端口冲突: 18080x2` → GREEN 20/0 |
| PR #153（RED `ed0921f`→GREEN `1ef0124`+SSE `cf57e7b`→`32b4dab`） | `scripts/perf_baseline.py`（http/ladder/sse 三模式）+ 守卫接线 | 4 契约先红后绿（脚本不存在→percentile/负向/正向/SSE 逐条逼出） |
| PR #154（RED `889c4ba`→GREEN `1a701f1`） | llm histogram 亚毫秒桶界（0.0005/0.002）+ exposition 断言 + xtts 守卫接线 | `TestBucketResolution`：缺 ≤0.5ms 桶界 |
| PR #155（RED `a312db7`→GREEN `b37cf44`） | XTTS synth_pool 线程池 + server.py 卸载接线 + Dockerfile COPY + ADR 决策 39 | `test_synth_pool.py` 8 断言（ModuleNotFoundError RED） |
| PR #156（RED `83bfce7`→GREEN `9e3d91a`） | takeSentenceSegment + useTTSManager 切段 | 纯函数 7 例 + manager 4 例（模块缺失/立即调用断言 RED） |
| PR #158（RED `0ebb1d5`→GREEN `1939649`） | sender onDelta 流中攒段即发（第二层 debounce 根除）+ flushTTS 兜底契约 | 静态源码契约 3 条（import/分支/尾冲） |

## 5. 回归钉

- 新增 spec：`emotion-echo-web/e2e/performance-baseline.spec.ts`（5 用例：#9/#19 浏览器 TTFT+渐进流式、#12 段间 gap、#13 端到端分解、#17 Grafana 面板）
- chromium 单文件历程：首跑 4 红（Vite 缓存 + 启用时序 + 探针缺陷 + 文件夹 uid）→ 逐项根因修复 → **4/4 绿**（#12 实例事件 gap=269ms、#13 ttsFirst<sseFinish、#9 TTFT 5885ms/5 步、#17 面板渲染）
- 全量 `pnpm playwright test`（对照 F-189 基线）：见 **§8**

## 6. 待决策 / 升级项

1. **F-135 双端点流式播放（D-43 裁定"全做"的第三件）未实施** —— 实施面 = WebAudio int16 PCM 流式播放器（`/tts_stream` 实为裸 int16 PCM 非 WAV）+ 与 F-129 队列/口型链整合，改动深、直接触达核心音频 UX。本阶段已完成其两件前置（F-136 并发、F-134 首声提前），**升级请示**：① 下阶段专 session 实施（推荐，账本 F-135 已跟踪）② 改判范围（用户可裁减）③ 现在继续（本收口后）。**阶段判 partial 即因此条。**
2. WSL 8GB 第 4 次冻结 —— 是否上调 `.wslconfig` memory（如 10-12GB，需确认宿主物理内存余量）属用户系统配置，不擅动；候选记账。
3. M1（D-42）阈值门禁：基线已落档，二轮数据后再议（裁定原文保留于 D-42）。

## 7. 收口自检

- [x] §8 三节回填完毕、占位符清零（2026-10-05 grep `<!--` / `见 §8 回填` 零命中）
- [x] 机器审计 `e2e_stage_audit.py --all` **30 阶段 0 FAIL**（2026-10-05，改 roadmap 后复验）
- [x] 第二方核对（§8.3，只读子代理）
- [ ] git status 干净（收口 PR 内）
- [ ] main 与 origin 无 ahead/behind（合并后）
- [ ] 无残留已合并分支（合并即删）

## 8. 收口轮回填

### 8.1 全量 Playwright 对照（plan #19 后半）

| 项 | 本轮（2026-10-03 收口） | F-189 基线（E2E-27 收口） |
|----|------------------------|---------------------------|
| 结果 | **238 passed / 58 failed / 2 skipped，50.0min** | 255 passed / 31 failed / 2 skipped，27.1min |
| 本 spec 失败 | 5 例（chromium #9/#12/#13 + mobile #12/#13） | —（spec 本轮新增） |
| 关键甄别 | ① chromium #9 = `growthSteps=[288]` 单步——**负载饿死采样粒度**（TTFT 未超限，单跑 4/4 绿）；② chat-core #5/6 失败截图显示**回复已完整渲染** = 10s 首 token 窗口在长跑负载下饿死，**非选择器/F-134 回归**；③ 全量时长近 2 倍 + 失败 +27 受当轮环境（WSL 冻结恢复后 + TTS 重载用例 + 50min 长跑）影响 | 根因逐条待查（F-189） |
| 判读 | **对照记录，不作回归论断**；签名扩展已追加进 F-189 行（owner=E2E-30） | 首轮暴露无历史对照 |

**本 spec 双 project 单跑终判**（2026-10-04/05，**真 dev server**——F-196 处置后：`docker stop emotion-echo-web` + 本地 `pnpm dev` 源码 chunk 复验 `/_nuxt/app/composables/*.ts` 形态）：

| project | 结果 | 通过 | 失败（归账） |
|---------|------|------|-------------|
| chromium | **3 passed / 1 failed**（5.4m） | #19/#9（TTFT<30s+渐进）、#13（F-134 ttsFirst<sseFinish）、#17（Grafana 面板） | #12 → **F-195**（audio.play()=0+gen 静默出队；连败 3 次=时序敏感非回归，上轮 gap=269ms 留档） |
| mobile | **2 passed / 2 failed**（5.8m） | #13、#17 | #12 → **F-195**；#19/#9 → **F-197**（fill 后 send-btn 5s 恒 disabled，真代码复现；失败详情 `14× resolved to <button disabled>`） |

**过程重大发现（F-196，已入账）**：2026-10-04 首轮双 project 全套**全部跑在 emotion-echo-web 容器旧镜像上**（compose.dev.yml `3000:3000` 映射，daemon 重启后 compose up 抢占宿主 :3000，本地 pnpm dev 静默失效）——chromium 4/4 假败、`ttsFirst==sseFinish` 同毫秒"F-134 回归假象"、XTTS 收 >82 字全文（旧行为特征）全部由此产生；停容器重启 dev server 后 #13 即 PASS。**探针实证（真代码）**：三句 prompt → 三条 TTS 请求（27/30/27 字，段段切）+ 首段 6621ms 早于 SSE finish 8703ms（流中 2082ms 提前）。

### 8.2 #17 Grafana 面板终判

**PASS**（2026-10-04 重拍 + 2026-10-05 复验口径一致）：
- **前置**：Prometheus 重启后先注入 60×/analyze 真实流量（p50=1.436ms 全 2xx，`llm_http_request_duration_seconds_bucket` p95=0.0015 已入 Prom），避免"面板有板无数据"；
- **用例**：`-g '#17'` chromium passed（37.3s），断言链全过：search API 可达（`page.evaluate(fetch)` 绕 F-192）→ `type=dash-db` 过滤命中 overview → 面板标题 /p95|延迟/Latency/ 匹配；
- **截图**：`screenshots/28-17-grafana-p95-panel.png`，md5=`a1d729dcc3a214b408a11fc6d46fb55f`，**像素机械判定** colorful_px=17218（1.87%，彩色曲线=真实数据渲染特征）+ 目视交叉一致——HTTP p95 Latency 面板 17:00 处 5-28ms 真实曲线、四面板（Request Rate/Error Rate/p95/Goroutines）全渲染（停机轮 dark-only 假象已排除）；
- **数据交叉**：PromQL `histogram_quantile(0.95, llm_http_request_duration_seconds_bucket[5m])` 有值（1.5ms 量级）与面板同源。

### 8.3 机器审计 + 第二方核对

**机器审计：✅ 0 FAIL**（2026-10-05）

```
$ python scripts/e2e_stage_audit.py --all
✅ e2e-28 (e2e-28-performance-baseline)  roadmap 状态: partial
合计：30 个阶段，0 个存在 FAIL
```

修复轨迹：首跑 FAIL（A9 三处 status 不一致 in/partial + A11 结果列含"（已分类）"后缀 + 占位符残留）→ front-matter 三处统一 `partial` + 结果列改纯基值（处置说明移备注列）+ §8 回填 → 0 FAIL。

**第二方核对（§13.3，只读子代理）：** 见 §8.3.1。

## 9. 停工快照（2026-10-03，用户指令：落地文档不 push、标记未完成、记录问题）

> **2026-10-04/05 续做进度标注**（下表为停工期原文，✅ 为续做轮已完成）：
> ① #17 截图 ✅（带数据重拍 md5=a1d729dc，见 §8.2）；② §8 回填 ✅（8.1/8.2/8.3 全填）；
> ③ audit ✅（30 阶段 0 FAIL）；④ 第二方核对 → 进行中（§8.3）；⑤ 收口 PR 待开（push 需放行）；
> ⑥ F-135 仍 🔴（partial 依据不变）；⑦ 收尾：`.devmode-session` 已重新登记（续做轮），
> dev server :3000 与容器栈仍运行（**web 容器须保持停止**——:3000 归本地 dev server，
> 见 F-196）。
> **续做轮新发现**：F-195（#12 播放层 gen 竞争）/ F-196（:3000 被 web 容器旧镜像抢占，
> 本轮最大陷阱——chromium 4/4 假败皆源于此）/ F-197（mobile fill 不启用按钮）。

### 9.1 收口未完成清单（2026-10-03 停工期原貌，下 session 续做，按序）

| # | 事项 | 现状/卡点 |
|---|------|----------|
| 1 | **#17 带数据 Grafana 截图** | 数据源根因已修：obs 三件套（prometheus/alertmanager/loki）**WSL 冻结后 DNS 注册失效**（服务名/容器名全解析失败，正常服务正常）→ `docker restart` 三件套修复，Grafana proxy `count(up)=12` 实测通。spec 已改 `page.evaluate(fetch)`（E2E-17 先例，绕 `page.request` 对宿主端口的间歇 ECONNRESET=F-192 同族）——**该改动被停工打断未跑成**，需重跑 `-g '#17'` 拿截图 + 面板数据目视 |
| 2 | **§8 回填** | 8.1 全量对照已写；8.2 #17 终判、8.3 审计/第二方核对空缺；§2 汇总行（#17/#19/#20 待回填）未校正 → audit A2/A11 红 |
| 3 | **机器审计 0 FAIL** | 当前 FAIL（占位符所致，非实质冲突）；回填后 `python scripts/e2e_stage_audit.py --all` |
| 4 | **第二方核对（§13.3）** | 未做（memory 先例：开只读子代理复核，自证不可信） |
| 5 | **收口 PR** | 未开（本 commit 仅落本地，未 push——用户指令） |
| 6 | **F-135 双端点流式播放** | 未实施（D-43 第三件；WebAudio int16 PCM 流式播放器 + F-129 队列/口型整合）——**阶段 partial 的唯一本阶段原因**；升级选项见 §6① |
| 7 | **收尾杂项** | `.devmode-session` 锁已按收工三查删除；dev server（:3000）与容器栈**仍在运行未停**；`test-results/` 残留（gitignored） |

### 9.2 本阶段已坐实、可直接引用的结论（已完成面）

- 8 个 PR 全部合并 main：#151 决策 D-42/43 · #152 obs 端口 TDD · #153 perf 工具四契约 ·
  #154 桶界+xtts 守卫接线 · #155 synth_pool（ADR 决策39，#11 修后重叠 [36.4,38.9] 实证）·
  #156 F-134 manager 切段 · #157 账本 F-190~193 · #158 F-134 sender 第二层 debounce
- 基线数据 23 份 JSON（`baseline/`）：/analyze 热 p50≈1.3ms 复现 1.021、阶梯 1.2→7.7ms、
  SSE TTFT p50=653ms（冷热双峰 + 长回复鉴别证真流式 burst=0.29）、TTS 三层冷
  50.9s/13-14s/热 13-30s、段间 gap chromium 双测 **269/262ms**、分解 3371<3747（F-134 中流发出）
- 浏览器单跑终态：chromium #9/#13/#17 断言过（#17 截图文件机械判定=暗屏 Grafana 97%，
  **Read 工具曾三次串图给出聊天页假象——已用 md5+像素亮度机械判定纠偏**，后续核验勿信 Read 目视）
- 全量对照：238/58/2 @50min vs F-189 基线 255/31/2 @27.1min——负载劣化不作回归论断，
  签名扩展已追加进 F-189 行（含 chat-core #5/6 = 10s 窗口饿死非 F-134 回归的截图证据）
- 新账本：F-190✅/F-191🔴/F-192🔴/F-193✅/F-194🔴 已登记；F-135 保持 🔴

### 9.3 环境遗留（下 session 开工先做）

1. 容器栈（dev+ai+obs）与 `pnpm dev :3000` **仍在跑**；锁已删 → 重新登记 `.devmode-session`
2. obs 三件套 DNS 已 restart 修复——**WSL 冻结（第 4 次）后此类陈旧注册可能复发**，
   症状 = 服务名解析失败 + Grafana No data，处置 = `docker restart` 对应容器
3. 若再遇全量跑后他阶段截图被重生成：`git checkout -- <png>` + 删未跟踪杂散（本轮已处置一次）
