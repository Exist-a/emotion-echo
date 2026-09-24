---
stage: e2e-17
title: 数字人 + TTS（真口型同步 + 段间断点）
type: transformation
status: done
created: 2026-09-23
revised: 2026-09-24 (用户实测**确认听到声音**「行，听到了」 → 收口 → partial→done。最终根因 F-140 = DOM 音量未 clamp 致 play() 永不执行，已修+web:v0.1.7 部署+IAB 探针 play()→PLAYING 实证。配套：F-132 XTTS 核数 / F-133 stopTTS / F-131 强断言 / F-138 超时漂移。**APISIX 相关真 bug 全部迁出本阶段**：F-137 BFF-Nacos 启动竞态（实际归 [[ap-proto]) 转移 E2E-25 APISIX（已在 PR #77 修：dev 默认 backoff retry / prod fail-fast / dev-up.sh Nacos 注册校验 / /health 加 version）；F-139 CORS origin 缺失 → 落账 E2E-25）
depends-on: [e2e-16]
blocks: [e2e-28]
gate: []
related-findings: [E2E-F-05, E2E-F-06, E2E-F-126, E2E-F-127, E2E-F-128, E2E-F-129, E2E-F-130, E2E-F-131, E2E-F-132, E2E-F-133, E2E-F-138, E2E-F-140]
---

# E2E-17 数字人 + TTS — 详档（计划期校准版）

# E2E-17 数字人 + TTS — 详档（计划期校准版）

> **类型**：transformation —— 口型是随机轮播假动画、TTS 段间必然断点，两条链路"能出声、但体验是假的/断的"，必须先实现 D-03 再可验证。
> **依据**：D-03 决议（[decisions.md](../../decisions.md)，用户 2026-09-17 确认"做真口型同步 + 排查断点"）+ 账本 E2E-F-05/F-06（预探查 2026-09-17）+ **本轮计划期调研校准（2026-09-23）**。
> **方法论**：[RUNBOOK.md](../RUNBOOK.md)（状态机 / §4.1 证据有效性 / §7 收口契约 11 项 / §13 审计）+ [anti-patterns.md](../anti-patterns.md)（14 类反例）。

---

## 1. 阶段目标

让数字人说话从"**随机嘴巴动 + 段与段之间卡顿**"变成"**口型按 phoneme 时间戳对齐音频 + 多段连续播放无明显断点**"，并让这两件事可机械验证（时间戳驱动断言 + gap 时长测量）。

---

## 2. 范围与边界

### 做

**A0. XTTS 镜像基础设施（计划期发现，本阶段前置硬依赖 — 见 §7 调研依据）**

0. **build 仓内 XTTS 镜像**：`docker build -f emotion-echo-models/XTTS/Dockerfile -t emotion-echo/xtts:v2.0.0 emotion-echo-models/`（Stage 36 build 成功过，Dockerfile 包含 `server.py` / `TTS/` / `pcm_chunk_shape.py` / `torchaudio_shim.py` 等所有依赖；预计 140 秒构建 + 11.3 GB 镜像）
1. **切换 compose 镜像源**：`deploy/docker-compose.apps.yml:537` `image: ai4all/coqui:latest` → `image: emotion-echo/xtts:v2.0.0`（必须改，否则 /tts_stream 与 /tts_with_phonemes 都不通——见 §7）
2. **重启 xtts 容器 + 健康检查**：`docker compose --profile ai up -d emotion-echo-xtts` + `/health` 返 `{"status":"ok","model_loaded":true}` 才视为就绪（实测发现当前 Exited(137)，先诊断 OOM/重启失败原因）

**A. 真口型同步（E2E-F-05 / D-03 第 1 条）**

3. **XTTS 端核实**（先测后写）：curl `http://localhost:8003/tts_with_phonemes` 拿真实样例钉契约 — **真实字段**（按 `emotion-echo-models/XTTS/server.py:319-326` 读代码 + curl 实测双确认）：
   ```
   phonemes: [{char, start (秒), duration (秒)}]  // **注意：start 是秒不是毫秒**，duration 是 per-char 等分（不是真 XTTS 字符级时间戳推理，是"总时长 ÷ 字符数"的近似）
   duration: <total_duration>  // 总时长（秒）
   audio: <base64 WAV>, text, language, sample_rate
   ```
   ⚠️ 语义限制：per-char 等分意味着句子越长每字符占用越长，**这与真 phoneme alignment 有差距**——只是"近似口型时机"而非"真字符读音时机"。D-03 第 1 条目标是"对齐"，该近似能满足"对齐"但非"真字符读音"，**口径必须在 report.md 明确标注**。
4. **BFF 转发通道**：现 `tts_handler.go` 只有 `/api/v1/tts/synthesize`（ai-svc gRPC）+ `/api/v1/tts/stream`（XTTS 直连，**当前链路已坏：vendor 镜像无 /tts_stream**）→ 加 `/api/v1/tts/phonemes`（XTTS 直连到 `tts_with_phonemes`，镜像 /audio / sample_rate / phonemes / duration / text / language）。proto `SynthesizeSpeechResponse` 无 phonemes 字段 → **选 B 方案**（BFF 新 HTTP 端点直调 XTTS；proto 改动面是更大的、牵涉 ai-svc 重生成）。**改动最小 + 不破坏既有消费者**
5. **前端播放层改造**：`useTTSPlayer.ts`
   - 激活死代码映射表（L38-75 `VOWEL_TO_LIP` / `CONSONANT_CLOSE`，当前全文件无引用）
   - 删 `startRandomLipAnimation`（L91-103 每 150ms 轮播 5 口型的假动画）
   - **口型驱动改为按 phoneme 时间戳**：先 fetch `/api/v1/tts/phonemes` 拿 phoneme 数组 + audio base64；解码 WAV + 计算每个 char 在 audio buffer 中的时间（`char.start × 1000ms` 转 ms）；播放时 `audio.currentTime`（单位 s）命中 `char.start` 区间 → 查 `VOWEL_TO_LIP[char]` / `CONSONANT_CLOSE[char]` 拿口型 → `lipSyncCallback` → `DigitalHuman.client.vue` `setLipShape`（**注：现有 PCM 播放器无 currentTime 事件订阅，需查是否够用，不够则改 Web Audio API 路径**）
   - 播放结束/中断 → `resetLipShape`（闭嘴，防口型冻在末帧）
6. **数字人侧复用**：`DigitalHuman.client.vue:311 setLipShape` 与 BlendShape 过渡（L166 `lipShapeTransitionTime`）已就绪 —— 只验证驱动信号真实性，不动渲染层（除非时间戳驱动暴露渲染 bug → 记账）

**B. 段间断点排查与修复（E2E-F-06 / D-03 第 2 条）**

7. **量化现状**（先测后修，禁止没数据就改）：构造多段回复（AI 回复 > 500ms 生成时长），测量段间 gap：`audio.ended` → 下一段 `play()` 的时长；记录修复前基线
8. **修法方向**（按 D-03 影响面，实测后选）：
   - `useTTSManager.ts:42-52` 的 500ms debounce 分段策略重审（聚合窗口 vs 首包延迟权衡）
   - **段间不 `stop()` 重连**（`useTTSPlayer.ts:186` 的 `stop()` 是断点主因之一）：上一段未播完的 buffer 与下一段衔接（`useTTSPlayer` 的 `flushBuffer` / chunk 队列复用）
   - XTTS 每段冷启动（`tts_stream` per-segment）—— 若 BFF/xtts 侧无法预热，至少消除前端侧 stop→重开的 gap
   - **验收口径**：段间 gap 中位数 < 200ms（阈值开工时结合基线确认，写入 report；若客观不可达，记录实测值 + 原因 + 降级裁定，禁止拍脑袋宣布达标）
9. **口型跨段连续**：段间 gap 期间口型归 neutral/闭嘴而非冻结

**C. 测试基建**

10. **前端单测**（Vitest）：映射表激活契约（元音→口型逐项）、时间戳→口型判定（边界：start 命中/未命中/区间外/end）、假随机动画已删、播放结束 reset
11. **Playwright 回归钉**：`e2e/digital-human-tts.spec.ts` —— 发消息 → 数字人区域可见 + （fake media/audio stub 下）`setLipShape` 被按序调用（而非 150ms 固定节奏）+ 多段播放无 `stop` 抖动（可通过 spy 播放事件序列断言）
12. **gap 测量脚本**：`scripts/` 或 spec 内测段间 gap，输出数值作证据（[A]）

**D. 决策与账本**

13. **新决策 D-25**：XTTS 镜像 = 仓内 `emotion-echo/xtts:v2.0.0`（build 自 `emotion-echo-models/XTTS/Dockerfile`），替代当前 compose 中的 vendor `ai4all/coqui:latest` —— **根因：vendor 镜像 app.py 缺 /tts_stream 与 /tts_with_phonemes 端点**（`grep -n "tts" /tmp/coqui-app.py` 零命中），无法服务前端 `/api/v1/tts/stream` 与本阶段新增 `/api/v1/tts/phonemes`。ADR 收录在 `docs/architecture/adr/adr-2026-09-xtts-v3-repo-image-switch.md`，登记到 `decisions.md`
14. **新账本 F-126**：当前 `/api/v1/tts/stream` 实际链路已坏（无论 vendor 在不在 — vendor 无端点 → 404；vendor 不在 → 502）— 历史 dev mode TTS "working" 应为误会，**新决策后修通即本次修复**
15. F-05 / F-06 状态更新（实现落地后回填证据）；新发现按序登记 F-12x，**先登记再修**

### 不做（边界）

| 不做项 | 理由 |
|--------|------|
| E2E-28 性能基线（TTS 单段/段间 gap 的 p50/p95 阶梯压测） | D-03 只做"断点修复 + 单场景测量"；系统性基线归 E2E-28（本阶段的 gap 测量方法可复用过去） |
| 真 XTTS 字符级 phoneme alignment 推理改造 | 仓 `server.py` 是 per-char 等分近似，本阶段接受；真字符级推理是 XTTS 模型层工作，超出本阶段 |
| XTTS 模型/音色/质量调优 | 归模型侧，非前端播放链路 |
| 数字人外观/动作/表情系统（非口型部分） | `setEmotion` 已在 D-14 验证；本阶段只管口型信号 |
| TTS 双缓冲/预取等流式架构大改 | 若实测证明必须（gap 根因在架构），记账升级，不在本阶段顺手重构 |
| 真人主观听感/口型"像不像"评审 | [M] 类需用户裁定的点单列，不冒充机械验收 |
| i18n | D-04 候选未决 |
| **F-134 短句切段 + 流式 TTS（E2E-F-134，2026-09-23 IAB 用户产品决策，对标豆包）** | 改 `useTTSManager` 累积到标点（。/!/？/，）切段 + 调 XTls `/tts_stream` 流式 WAV；用户发消息 **5-15s 内听第一句**。**优先级降为体验优化**：F-132 已把 49 字 188s→19.9s（9.5x），单请求 27.6s 已可听；切段可进一步压到首句 4s。**代价**：`/tts_stream` 无 phoneme 数组 → VRM 嘴型不再按音素动（VRM 仍驱动但口型随机/默认）。**产品决策**：要"快"还是"真口型同步"？用户 2026-09-23 明确倾向"快"（对标豆包） |
| **F-135 折中：每段双端点（流式 + phoneme），保留真口型（2026-09-23 复盘候选）** | 每段**同时**调 `/tts_stream`（拿流式 WAV 字节给播放器，5-15s 出声）+ `/tts_with_phonemes`（拿 phoneme 数组给 VRM 嘴型同步）。**两全其美**但 XTls 单 worker 压力 ×2 → 必须先 F-136 多 worker 落地才扛得住；本轮不修 |
| **F-137 BFF 与 Nacos 启动竞态（2026-09-24 本 session 实测）** | BFF 启动时 Nacos 未就绪 ⇒ 静默 continuing 不注册自己 ⇒ APISIX upstream 6 解析 `nodes:{}` ⇒ 全站 `/api/v1/*` 503。BFF /health 200 + 网关 503 可同时成立。本 session 已临时 `docker restart emotion-echo-web-bff` 即愈；治本：① BFF 注册失败 fail-fast 或带重试；② `dev-up.sh` 起 BFF 前确认 Nacos healthy + 等自身注册成功；③ 增 `/health` 版本/git_sha 端点；④ `depends_on` BFF 用 nacos service_healthy |

---

## 3. 前置条件（**本轮校准已变更**）

| 条件 | 状态 |
|------|------|
| E2E-16 done（多模态，TTS/数字入口在聊天页） | ✅ 2026-09-23 收口（F-119 用户侧留账不阻塞） |
| D-03 已决议 | ✅ 用户 2026-09-17 确认（decisions.md） |
| **D-25 已决议**（XTTS 镜像切换） | 🟡 **本轮新决策（待 plan 校准落地后登记）** |
| `e2e_stage_audit.py --all` 0 FAIL | ✅ 2026-09-23 实测（30 阶段 0 FAIL） |
| `deploy/.env.local` 存在 | ✅（**严禁删除/覆盖**，AGENTS.md §四红线） |
| **仓内 XTTS 镜像已构建** `emotion-echo/xtts:v2.0.0` | ❌ **本阶段开工第 0 步**（Dockerfile 已就绪但从未构建/部署） |
| **compose 已切换** 到仓内 XTTS 镜像 | ❌ **本阶段开工第 1 步**（当前 `image: ai4all/coqui:latest` 缺端点） |
| **xtts 容器健康** + `/health` 返 ok | ❌ **本轮发现**：当前 `Exited(137)`（SIGKILL，疑似 OOM 或被冻结事件链 kill） |
| `/tts_with_phonemes` curl 拿真实样例 | ❌ **本阶段开工第 2 步**（先于计划中写条件） |
| 数字人 VRM 模型可加载（`/3d-models/digital-human.vrm`） | ⚠️ 开工时 IAB 实测 |
| dev 栈启动 | **必须带 `--env-file .env.local` 与 `--profile dev`**（见 RUNBOOK §2.1；缺 profile → 6 服务注册失败，E2E-F-108） |

```bash
cd deploy && docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml \
  --env-file .env.local --profile dev --profile ai up -d
# A0.2 切换镜像后必跑：
bash apisix/seed.sh && docker compose -f docker-compose.apps.yml --env-file .env.local up -d emotion-echo-web-bff
```

---

## 4. 测试点清单（**本轮校准已更新**）

判定标记：`[A]` 自动 · `[V]` 视觉 · `[M]` 需裁定。详见 [RUNBOOK.md](../RUNBOOK.md) §4。

| # | 测试点 | 判定 | 验证方式 | 证据 | 结果 |
|---|--------|------|---------|------|------|
| 1 | 仓内 XTTS 镜像 build 成功 | [A] | `docker build` 退出码 0 + 镜像存在 | build log | ⬜ |
| 2 | compose 切换后 xtts 容器 healthy | [A] | `docker ps --filter name=emotion-echo-xtts` healthy | docker ps | ⬜ |
| 3 | `/tts_with_phonemes` 返回 `{audio, sample_rate, text, language, phonemes:[{char,start,duration}], duration}` | [A] | curl + JSON schema 断言 | curl 输出 | ⬜ |
| 4 | phonemes start 是秒（非毫秒），duration 是 per-char 等分 | [A] | curl 样例 + 计算断言：`last.start + last.duration ≈ total.duration` | curl + python | ⬜ |
| 5 | `/tts_stream` 也正常（端点存在 → 现状 BFF 调它能流式吐 WAV） | [A] | curl 流式响应 | curl | ⬜ |
| 6 | BFF `/api/v1/tts/phonemes` 转发通道打通（含鉴权，与 /tts/stream 同源） | [A] | curl BFF + 断言字段透传 + 含 userID ctx | curl | ⬜ |
| 7 | 前端播放层收到 phonemes 并存入播放项 | [A] | Vitest：TTSResponse.phonemes → 播放队列 | 单测输出 | ⬜ |
| 8 | 映射表激活：元音/辅音→口型逐项正确 | [A] | Vitest 表驱动（VOWEL_TO_LIP + CONSONANT_CLOSE 全键） | 单测输出 | ⬜ |
| 9 | 时间戳→口型判定（边界：命中/未命中/区间外/结束归 neutral） | [A] | Vitest 边界表驱动（注意 start 单位是秒） | 单测输出 | ⬜ |
| 10 | 假随机轮播已删除（150ms 固定节奏不存在） | [A] | 字面量契约：`startRandomLipAnimation` 不再被调 / 代码不存在 | 静态契约输出 | ⬜ |
| 11 | 口型由音频 `currentTime` 驱动（非定时器） | [A]+[V] | 单测（timeupdate 区间断言）+ IAB 观察数字人嘴部 | 单测 + 截图 | ⬜ |
| 12 | 播放结束/中断 → 口型 reset（不冻在末帧） | [A] | Vitest：ended/stop 后 setLipShape(neutral) | 单测输出 | ⬜ |
| 13 | 段间 gap 基线（修复前数值） | [A] | 测量脚本输出 gap 中位数（注意：本轮测的是新镜像 build 出来的真实 gap） | 数值记录 | ⬜ |
| 14 | 段间 gap 修复后 < 阈值（阈值开工定） | [A] | 同 #13 复测 | 数值对照 | ⬜ |
| 15 | 多段播放期间无 stop→重开抖动（事件序列断言） | [A] | Playwright spy 播放事件 | 事件序列 | ⬜ |
| 16 | 口型跨段连续（段间归 neutral 不冻结） | [V] | IAB 截图/录屏段间帧 | 截图 | ⬜ |
| 17 | 端到端：发消息 → AI 回复 → 数字人开口说话 | [A]+[V]+[M] | Playwright 全链路 + IAB 截图 + 用户裁定"像在说话" | spec + 截图 | ⬜ |
| 18 | 回归钉：Playwright `digital-human-tts.spec.ts` 双 project 全绿 | [A] | `npx playwright test` 输出 | 24/? PASS | ⬜ |
| 19 | 既有 D-14 情绪联动不回归（onEmotionChange→setEmotion） | [A] | 既有 d14 测试仍绿 | 测试输出 | ⬜ |

汇总行（收口时填）：`PASS x / FAIL x / BLOCKED x / N/A x`

---

## 5. 风险与已知阻塞（**本轮校准已更新**）

| 风险 | 应对 |
|------|------|
| **vendor `ai4all/coqui` 镜像 app.py 缺 `/tts_stream` 与 `/tts_with_phonemes`**（计划期实测） | A0 切换仓内镜像为唯一方案；commit D-25 决策 + ADR |
| 仓内镜像 build 耗时（Stage 58 ~140 秒）+ 11.3 GB 磁盘 | 已备镜像构建缓存；首次 build 期间其他工作并行 |
| **xtts 容器当前 Exited(137)**（SIGKILL） | 诊断 OOM（compose limit 3072M）或冻结事件链；A0 启动时核对，必要时 `.wslconfig` 复查 |
| phoneme 是 per-char 等分而非真 XTTS 字符级时间戳 | report.md 明确口径；如需真 alignment 留作未来（XTTS 模型层） |
| Web Audio API vs 当前 PCM 播放器 currentTime 事件 | 单测阶段验证可读性；不够则切 Web Audio |
| 真人主观"像不像"无法机械判定 | #17 的 [M] 部分升级用户裁定 |
| 切换镜像后 `/api/v1/tts/stream` 链路过历史 fake 测试可能回归 | Playwright mobile-project 双 project 跑旧 tts 流式测试 |
| 仓内 XTTS 镜像与 vendor 音色差异 | 可接受（v2 ADR 已 retire 云 API，本地即定） |

---

## 6. 开工顺序（六步循环内 — **本轮校准已更新**）

0. **A0.1 build 仓内 XTTS 镜像** + A0.2 **切换 compose 镜像源 + 重启 xtts + /health 验证**
1. **前置实测**：curl `/tts_with_phonemes` 拿完整 JSON（钉 #3/#4） + curl `/tts_stream` 流式验证（#5） + gap 基线（#13）
2. **RED→GREEN BFF** `/api/v1/tts/phonemes` 端点（XTTS 直连 + 鉴权同 /tts/stream）
3. **RED→GREEN 前端播放层**：phomes 数组 → 播放队列 → currentTime 区间驱动 → 映射表激活 → 删 startRandomLipAnimation
4. **段间断点修复**：先测后修（按 D-03 影响面）
5. **Playwright 回归钉 + IAB 视觉证据**
6. **账本回填**（F-126 / F-05 / F-06 / 新发现） + report.md 按 §10 模板收口 + D-25 ADR 登记

---

## 7. 调研依据（AGENTS.md §〇.6 — 本轮校准前后对比）

### 计划期校准前（just-in-time 撰写阶段）

- 调研文件 4 个：useTTSPlayer.ts（关键片段）/ XTTS/server.py（grep）/ tts_handler.go（grep）/ DigitalHuman.client.vue（grep）
- ADR/决策：D-03 + decisions.md 决议表
- 默认假设：compose 跑 vendor 镜像但 `server.py` 的 `/tts_with_phonemes` 已返回时间戳（**未实测**）

### 计划期校准后（本轮 — 见 [plan 校准 commit](https://github.com/Exist-a/emotion-echo/pull/67/files)）

| 文件 | 关键发现 |
|------|---------|
| **已读代码（5 个关键 + 2 个测试相关）** | |
| `emotion-echo-models/XTTS/Dockerfile`（全） | `python:3.10-slim` builder + 复制 `XTTS/server.py` + 模型，**Stage 36 已 build 成功过**（stage-58 之前的 build 阻塞已解），从未被 compose 引用 |
| `emotion-echo-models/XTTS/server.py:283-341` | `/tts_with_phonemes` 真实字段：`phonemes: [{char, start (秒), duration (秒)}]` per-char 等分；duration = `len(audio)/SAMPLE_RATE / len(chars)` 近似；非流式 |
| `emotion-echo-models/XTTS/server.py:265-281` | `/tts_stream` 真实端点（`StreamingResponse` 流式 WAV）—— **vendor 镜像没有这两个端点中的任何一个** |
| **`/tmp/coqui-app.py`**（`docker cp` 抽出 vendor 镜像 `ai4all/coqui:latest` 的 app.py） | **确认**：`grep -n "tts" coqui-app.py` **零命中**。vendor 端点仅 `/voice/upload`、`/voice/generate`、`/voice/result`（multipart 上传 + 单次生成 + GET 取文件，**根本不是 streaming TTS**） |
| `emotion-echo-web-bff/internal/handler/tts_handler.go`（全） | `stream` handler 调 `h.xtts.Stream(...)` → `xtts.go` `POST /tts_stream` → vendor 必 404 |
| `emotion-echo-web-bff/internal/downstream/xtts.go:75` | `http.NewRequestWithContext(ctx, POST, c.baseURL+"/tts_stream", ...)` — 直连 vendor |
| `emotion-echo-web/app/composables/useTTSPlayer.ts:174-260` | `playStream` 流程：先 `stop()` → fetch `/api/v1/tts/stream` → PcmPlayer 边读边播；无 phoneme 驱动逻辑；`startRandomLipAnimation` 死代码活跃 |
| `emotion-echo-web/app/composables/useTTSManager.ts:42-52` | `playText` 500ms debounce + `originalPlayText(accumulatedText)` — 段间断点主因之一 |
| `emotion-echo-web/app/components/digital-human/DigitalHuman.client.vue:311-315` | `setLipShape(shape)` 只赋 `currentLipShape`，渲染驱动点在 render 循环 |
| `emotion-echo-web/app/composables/useDigitalHumanTTS.ts` | 包 `useTTSPlayer.playStream`，透传 lipSyncCallback |
| **测试基建现状** | |
| `emotion-echo-web/app/composables/*.test.ts` 列表 | **零** useTTSPlayer / useTTSManager 单测（仅有 architecture 静态契约测试） |
| `emotion-echo-web/e2e/*.spec.ts` | `chat-core.spec.ts` 涉及 ai/stream 但**未断言 TTS/数字人** |
| **ADR / 决策** | |
| `docs/architecture/adr/adr-2026-09-xtts-v2-decision.md` | vendor Coqui 为唯一 TTS 方案 — **v2 决策成立但缺乏"仓内 server.py 必须部署"约束** |
| `docs/architecture/decisions.md` D-03 | 真口型同步 + 排查断点（已落实） |
| **环境实测** | |
| `docker ps` xtts | **Exited(137)**（19 小时前 SIGKILL）— 容器不在跑 |
| `docker inspect ... State.Error` | 空（OOMKilled=false） — 非 OOM，可能是被冻结事件链 kill 或主动 stop |
| `deploy/docker-compose.apps.yml:536-575` | `image: ai4all/coqui:latest`、`profiles: ["ai"]`、`memory: 3072M` |
| **关键决策** | **新决策 D-25**（XTTS 镜像 = 仓内 v2.0.0 build，替代 vendor） — 开工第 0 步 |

### 计划期校准前 → 后的关键修正

| 项 | 修正前 | 修正后 |
|----|--------|--------|
| XTTS 端核实 #1 | "server.py:283 已返回时间戳" | **实测发现端点不存在于运行容器**——vendor 镜像 app.py 缺此端点；必须 build 仓内镜像 + 切换 compose |
| phoneme 字段 | "start_ms / end_ms" | **`start` 是秒（不是毫秒），`duration` 是 per-char 等分**（不是真字符级） |
| §4 #2 | "xtts healthy" | **Exited(137)，先诊断重启** |
| §3 前置 | 仅"XTTS 容器可用" | + **build 仓内镜像 + 切换 compose**（硬依赖） |
| §2 范围 | 13 条 | 15 条（新增 A0.0/0.1/0.2 镜像基础设施 + 决策 D-25 + 账本 F-126） |
| §5 风险 | 4 项 | 8 项（vendor 镜像缺端点 / 容器 SIGKILL / Web Audio API 兼容性 / 音色差异等） |