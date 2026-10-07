---
status: landed
priority: high
created: 2026-10-06
owner: E2E-F-198
type: feature-plan
adr: docs/architecture/adr/adr-2026-10-tts-api-cosyvoice2.md
decision: D-44（e2e-roadmap）/ 决策 40（architecture）
branch: feat/f198-tts-cloud-provider
---

# F-198 TTS API 专项：在线 TTS 主链路改接 CosyVoice2（XTTS 降离线回退）

> 本文是 **E2E-F-198** 的实施计划（ADR-2026-10 已 accepted，选型已实测定稿，**仅剩端到端实施**）。
> 执行纪律继承 [AGENTS.md](../../AGENTS.md)（ALL CODE IS TDD）与
> [RUNBOOK.md](../e2e-roadmap/RUNBOOK.md)（证据有效性 / 范围外只记账 / 回归钉）。

---

## §A 上下文

### A.1 文档功课留痕（AGENTS.md §〇 硬规则）

**① 已读代码（10 个文件）**

| 文件 | 读出的关键事实 |
|------|--------------|
| `emotion-echo-web-bff/internal/handler/tts_handler.go` | 3 端点：`/api/v1/tts/synthesize`（→ai-svc）、`/tts/stream`（→XTTS 直连裸 WAV）、`/tts/phonemes`（→XTTS 直连 JSON）。**前端数字人走的是 `/tts/phonemes`** |
| `emotion-echo-web-bff/internal/downstream/xtts.go` | `XTTSClient` 接口 = `Stream` / `Health` / `Phonemes`；`XTTSPhonemesResp{Audio(base64 WAV), SampleRate, Text, Language, Phonemes[], Duration}`；nil-safe 纪律（未配置返 "xtts client not configured"） |
| `emotion-echo-web-bff/internal/config/config.go` | `XTTS HTTPService`（BaseURL/TimeoutMs，默认 180000ms）；**`LLM` 段是 secret 注入范式**：yaml 不写 APIKey（dev 留空→降级 mock），env `BFF_LLM_API_KEY` 覆盖，注释明写"切勿 commit 真实 key" |
| `emotion-echo-models/XTTS/server.py` | `SAMPLE_RATE = 24000`（:93）；`TTSRequest{text, language="zh-cn", speed=0.75, volume=2.0}`（:96-100）；**phonemes 是纯算术**（:330-332）`chars=list(text)` → `total_duration=len(audio)/SAMPLE_RATE` → `char_duration=total/len(chars)` per-char 等分，**不涉及任何模型推理** |
| `emotion-echo-web/app/composables/useTTSPlayer.ts` | `:215` **硬校验** `if (!audioB64 \|\| !phonemes \|\| phonemes.length===0 \|\| !duration) throw`；`:146` `base64ToWavBlob` 只做 `atob`+`Blob(type:'audio/wav')`，**不构造 WAV 头**；`:250` F-140 volume clamp 到 [0,1]；请求体透传原始 `volume`（服务端语义） |
| `deploy/docker-compose.apps.yml` | `:665` `BFF_LLM_API_KEY: ${BFF_LLM_API_KEY:-}`（**空默认 = 缺 key 自动降级**范式）；`:647` `XTTS_BASE_URL` |
| `scripts/lint_env_vars.sh` | **门禁**：apps.yml 里每个 `${VAR:-}` 必须在两份 `.env.local.example` 或 `BUILTINS` 白名单中，否则 FAIL ⇒ 新增 `TTS_API_KEY` 必须同步登记 |
| `docs/architecture/adr/adr-2026-10-tts-api-cosyvoice2.md` | 决策全文 + 实测数据 + 实施注意（中文 payload 禁 shell 内联） |
| `docs/e2e-roadmap/discovered-unresolved.md` | F-198（本专项）/ F-135（否决销账）/ F-195（段队列时序缺陷，未解）/ F-196（:3000 抢占）/ F-197（mobile fill 后按钮不启用） |
| 分支 `feat/e2e-28-f135-dual-endpoint-tts` | `emotion-echo-web/app/lib/pcmStreamPlayer.ts`（205 行，WebAudio int16 PCM 流式播放器）**未合并**，本专项 tier-2 可复用 |

**② 已查 ADR / 决策**：`adr-2026-10-tts-api-cosyvoice2`（决策 40，accepted）、`adr-2026-10-xtts-synth-threadpool`（决策 39，回退路径并发基础）、`adr-2026-09-xtts-v2-decision`、`adr-2026-09-xtts-cpu-quotas`（F-132）、e2e-roadmap `D-43`（superseded）→ `D-44`（accepted）。

**④ 外部依赖官方文档**：[SiliconFlow TTS 能力页](https://docs.siliconflow.cn/cn/userguide/capabilities/text-to-speech)（本轮 WebFetch 实读，契约见 §B.4）。

**③ smoke**：§2.4 六项数据契约与 TTS 链路无交集（TTS 不写 `user_behavior_events`）；本专项的"数据对不对"等价物 = §E 测试点 #12~#16 的**音频可听性 + 口型对齐 + 首声延迟**实测，判据见下。

### A.2 本文假设清单（与现状逐条对比）

| # | 假设 | 现状核对 | 结论 |
|---|------|---------|------|
| H1 | 前端只经 BFF 3 个 `/api/v1/tts/*` 端点取音频，不直连 XTTS | `useTTSPlayer.ts:195` 用 `API_ROUTES.ttsPhonemes`；`apiRoutes.ts:74,78` 只有 `ttsStream`/`ttsPhonemes` | ✅ 成立 |
| H2 | XTTS 的 phonemes 是等分近似而非真对齐，故 cloud provider 可自行算术复刻而**不损失**现有口型质量 | `server.py:330-332` 纯算术；`xtts.go` 注释明写"**不是**真 XTTS 字符级时间戳推理"、E2E-17 D-03 口径即"等分近似" | ✅ 成立（关键：本专项**不降低**口型保真度） |
| H3 | 前端能直接消费 cloud 返回的音频，无需改播放层 | `:215` 硬要 `phonemes`+`duration`；`:146` 要**完整 WAV 容器**（不建头） | ⚠️ **有条件成立**：cloud provider 必须①返回 WAV 容器②自算 phonemes/duration。否则前端整段 throw ⇒ **完全没声音**（比现状更糟） |
| H4 | 采样率两端一致 | XTTS `SAMPLE_RATE=24000`；SiliconFlow `wav/pcm` 默认 **44100**，支持 24000 | ⚠️ **必须显式 pin `sample_rate=24000`**，否则 duration 算术与音调全错 |
| H5 | `volume` 语义可平移 | XTTS `volume=2.0` 是**线性 PCM 增益**；SiliconFlow `gain` 是 **dB ∈ [-10,10]**，默认 0 | ❌ **不成立** ⇒ 决策点 **M1** |
| H6 | 缺 key 时能静默降级到 XTTS，dev 体验不劣化 | `BFF_LLM_API_KEY: ${...:-}` 空默认 + ai_stream mock 降级是既有范式 | ✅ 成立（照搬） |
| H7 | 本专项不触碰 ai-svc | `/tts/synthesize` 走 ai-svc，但数字人链路走 `/tts/phonemes`（XTTS 直连） | ✅ 成立 ⇒ provider 抽象落在 **BFF downstream 层**，ai-svc 零改动（决策点 **M3** 已按此收敛，见 §F） |

### A.3 范围

**做**：BFF TTS provider 接口隔离（cloud/local）+ CosyVoice2 provider + 回退链 + phonemes/duration 服务端复刻 + env/config/compose/门禁登记 + 实测与回归钉。

**不做（边界）**：
- 声音样本上传复刻（`references` 零样本克隆）—— 用户隐私考量，D-44 明确排除
- XTTS 容器删除 —— 保留作离线回退（ADR 明写"不删"）
- ai-svc `/api/v1/tts/synthesize` 改造 —— 非数字人链路（H7）
- F-195（段队列时序缺陷）/ F-197（mobile fill）根修 —— 属 E2E-30/专项，本专项**只记账不修**（RUNBOOK §5）
- 渗透测试 / 全站压测

---

## §B 关键设计事实（读码与官方文档得出，禁臆断）

### B.1 前端契约是不可绕过的硬约束

`useTTSPlayer.ts:215` 对 `/tts/phonemes` 响应做**三项联合硬校验**（audio / phonemes 非空 / duration），任一缺失即 `throw`，段被丢弃 ⇒ 用户侧"嘴不动也没声音"。因此 cloud provider **必须**在 BFF 侧补齐 `phonemes` 与 `duration`，不能只回传裸音频。

### B.2 phonemes 复刻 = 1:1 照搬 XTTS 算术

```
chars        = []rune(text)            # 注意：Go 侧按 rune 切，等价 Python list(str) 的码点语义
total        = audioSeconds            # 由 WAV data chunk 字节数 / (sampleRate*2) 得出（int16 单声道）
charDuration = total / len(chars)      # len(chars)==0 → 0
phonemes[i]  = {char: chars[i], start: round(i*charDuration,3), duration: round(charDuration,3)}
```

保真度与现状**完全等价**（H2）；`round(...,3)` 保持与 `server.py:334-341` 同精度，避免前端 `findPhonemeAt` 边界抖动。

### B.3 音频容器必须是 WAV

`base64ToWavBlob`（`:146`）不构造头 ⇒ 必须请求 `response_format=wav`。若走 `pcm` 则 BFF 需自行封 WAV 头（RIFF/fmt/data，int16LE 单声道 24000Hz）——**tier-2 流式才需要**（`pcmStreamPlayer` 吃裸 PCM）。本轮 tier-1 用 `wav`，零封装风险。

### B.4 SiliconFlow `/v1/audio/speech` 契约（官方文档实读）

| 参数 | 取值 | 本专项用法 |
|------|------|-----------|
| `model` | `FunAudioLLM/CosyVoice2-0.5B` | 配置注入，**禁硬编码**（ADR 硬规则） |
| `input` | 待合成文本（官方注："输入内容不要加空格"） | 透传段文本 |
| `voice` | 预置音色需**带模型名前缀** | 配置 `TTS_VOICE`，默认取 ADR 实测通过的音色 |
| `speed` | float，默认 1.0，**范围 [0.25, 4.0]** | XTTS 默认 0.75 ⇒ 需 clamp 到范围内 |
| `gain` | float dB，默认 0.0，**范围 [-10, 10]** | 见 M1 |
| `response_format` | `mp3` / `opus` / `wav` / `pcm` | **`wav`**（B.3） |
| `sample_rate` | wav/pcm 支持 8000/16000/**24000**/32000/44100，**默认 44100** | **显式 24000**（H4） |
| `stream` | bool | tier-1 = false；tier-2 = true |
| 鉴权 | `Authorization: Bearer <TTS_API_KEY>` | env 注入 |
| 计费 | 按 input 的 **UTF-8 字节数** | 中文 3 字节/字 ⇒ 成本按字节估 |

**实测基线（2026-10-06，ADR §实证）**：TTFB 0.31~0.35s；33 字完整返回 1.38~1.55s（≈2x 实时，4/4 无截断）；ASR 回读逐字一致。对照 XTTS 现状 15~36s ⇒ **首声 ~20 倍改善**。

### B.5 回退链语义（ADR §Decision.4）

```
provider=auto（默认）：
  cloud 可用（key 非空）→ 调 CosyVoice2
     ├─ 成功 → 返回
     └─ 失败（key 缺失 / 网络 / 4xx-5xx / 超时 / 空音频）→ 降级 local(XTTS) + **打降级日志（可观测）**
  cloud 不可用（key 空）→ 直接 local(XTTS)，不打错误日志（dev 常态，非异常）
provider=cloud → 只用 cloud，失败即失败（不静默降级；便于实测甄别）
provider=local → 只用 XTTS（回滚开关 / 离线环境）
```

**降级必须可观测**：`slog.Warn` 带 `reason` + 计数指标（沿用 BFF 既有 metrics 注册范式），否则"云端一直挂但没人知道"= E2E-22 抓过的静默失效同型。

### B.6 门禁连带（易漏，已实测确认）

`scripts/lint_env_vars.sh` 会因 apps.yml 新增 `${TTS_API_KEY:-}` 而 **FAIL**，除非同步：① `deploy/env/.env.local.example` 加 `TTS_API_KEY=`（空值，**不写真 key**）② `docs/env-templates/.env.local.example` 同步 ③ 或加进 `BUILTINS` 白名单。选 ①②（与 `BFF_LLM_API_KEY` 同待遇，语义更准确：它是用户 secret 而非 compose 内建）。

---

## §C 架构：provider 接口隔离

```
handler/tts_handler.go
   └─ TTSHandler{ ai, tts downstream.TTSProvider }      ← 由 xtts 字段改为 provider
         ↓
downstream/tts_provider.go        （新增）
   type TTSProvider interface {
       Synthesize(ctx, TTSPhonemesReq) (*XTTSPhonemesResp, error)   // 复用既有响应类型，前端零改
       Name() string                                                // "cloud" | "local"，供日志/指标
   }
   ├─ localProvider   → 包装既有 XTTSClient.Phonemes（零行为变更）
   ├─ cloudProvider   → SiliconFlow /v1/audio/speech（wav, 24000）+ §B.2 phonemes 复刻
   └─ fallbackProvider→ cloud → local（§B.5），降级日志 + 指标
```

**复用 `XTTSPhonemesResp` 作为统一返回类型**是刻意选择：前端契约（B.1）与 handler 封装（`OK(c, resp)`）零改动 ⇒ 改动面收敛在 downstream 层，回归风险最小。类型名后续可重命名为 `TTSSynthResp`（REFACTOR 阶段，不改 JSON 字段）。

config 新增（镜像 `LLM` 段范式，yaml **不写 APIKey**）：

```yaml
TTS:
  Provider: auto                                    # auto | cloud | local
  BaseURL: https://api.siliconflow.cn/v1
  Model: FunAudioLLM/CosyVoice2-0.5B
  Voice: <ADR 实测音色>
  SampleRate: 24000
  ResponseFormat: wav
  Timeout: 30                                       # 秒；cloud 实测 1.5s，30s 极宽裕
```

env 覆盖：`TTS_PROVIDER` / `TTS_API_KEY` / `TTS_API_BASE_URL` / `TTS_MODEL` / `TTS_VOICE`。
compose（web-bff 段，紧邻 `BFF_LLM_API_KEY`）：`TTS_API_KEY: ${TTS_API_KEY:-}`（**空默认 ⇒ 缺 key 自动降级 XTTS**，H6）。

🔴 **key 红线**（AGENTS.md §四 + memory `deploy-env-local-key-protection`）：真 key 只存在于 gitignored `deploy/.env.local`；本文、yaml、compose、example、测试 fixture **一律不得出现真 key**；启动容器必须带 `--env-file .env.local`。

---

## §D TDD 循环（严格 RED → GREEN → REFACTOR）

| # | 循环 | 🔴 失败测试（先写） | 🟢 最小实现 |
|---|------|------------------|-----------|
| T1 | `TTSProvider` 接口 + localProvider 包装 | `TestLocalProvider_Synthesize_DelegatesToXTTS`（httptest 假 XTTS，断言路径 `/tts_with_phonemes` + 响应透传）；`TestLocalProvider_Name` | 接口定义 + localProvider 转调 |
| T2 | cloudProvider 请求构造 | `TestCloudProvider_PostsCorrectContract`：断言 method/path `/v1/audio/speech`、`Authorization: Bearer`、body `model`/`input`/`voice`/`response_format=wav`/`sample_rate=24000`/`speed` clamp 到 [0.25,4]；**body 必须 UTF-8 中文原文**（用 `httptest` 读原始字节断言，钉住 B.4 与 ADR 实施注意） | cloudProvider HTTP 调用 |
| T3 | phonemes/duration 复刻 | `TestCloudProvider_ComputesPhonemesFromWav`：喂**构造的已知 WAV**（固定字节数 ⇒ 已知 duration），断言 `duration` 与 per-char `start/duration`（round 3 位）与 §B.2 公式一致；边界：单字符 / 空文本 / 含 emoji（rune 计数）/ 中英混排 | WAV data chunk 解析 + 等分算术 |
| T4 | 错误与降级 | `TestFallbackProvider_CloudOK_NoFallback`；`_Cloud5xx_FallsBackToLocal`；`_CloudTimeout_FallsBack`；`_EmptyAudio_FallsBack`；`_NoKey_UsesLocalDirectly`（且**不**打 Warn）；`provider=cloud` 时失败**不**降级 | fallbackProvider + 降级判定 |
| T5 | 降级可观测 | `TestFallbackProvider_LogsDegradation`（捕获 slog 输出断言含 `reason`）+ 指标计数递增 | 日志 + metrics |
| T6 | config/env 装配 | `TestConfig_TTSDefaults`（Provider=auto / SampleRate=24000 / Format=wav / Timeout=30）；`TestApplyEnvOverrides_TTSAPIKey`（`t.Setenv`）；**`TestConfig_TTSAPIKeyNotInYAML`**（钉住 secret 不进 yaml，同 `BFF_JWT_SECRET` 纪律） | config 结构 + SetDefaults + ApplyEnvOverrides |
| T7 | handler 接线 | `TestTTSHandler_Phonemes_UsesProvider`（注入 fake provider，断言 200 + 信封 `code=0` + `data.phonemes` 非空）；`_ProviderError_MapsStatus`（沿用 `statusFor`） | handler 字段替换 + 构造注入 |
| T8 | 门禁/契约守卫 | `bash scripts/lint_env_vars.sh` 必须 GREEN（先让它 RED：只改 compose 不改 example）；`grep -rn "siliconflow" --include=*.go` 断言**业务代码零硬编码域名**（只允许 config 默认值与测试） | compose + example + 白名单登记 |

**每个循环独立 commit**（`test:` 红 → `feat:` 绿），符合 AGENTS.md §2.1 与"单 PR = 一个 TDD 循环或一组相关循环"。

---

## §E 测试点清单（20 点）

> 编号用「测试点清单」标题格式，确保 `e2e_stage_audit.py` A3 能解析（F-180 教训：子表编号会被跳过）。
> 判定分级：`[A]` 自动可判 / `[V]` 需视觉或听觉证据 / `[M]` 需用户裁定。

| # | 测试点 | 判据（可机械核对） | 级 |
|---|-------|-----------------|----|
| 1 | provider 接口隔离落地 | `TTSProvider` 接口存在；handler 不再直接持有 `XTTSClient`；`go vet` + `go test ./...` 绿 | [A] |
| 2 | 业务代码零硬编码供应商 | `grep -rn "siliconflow\|CosyVoice" --include=*.go emotion-echo-web-bff/internal/{handler,downstream}` 只命中 config 默认值/注释，**无字面域名于请求构造处** | [A] |
| 3 | cloud 请求契约正确 | T2 测试绿 + 抓包/httptest 断言 `sample_rate=24000`、`response_format=wav`、`speed` 已 clamp | [A] |
| 4 | phonemes 复刻与 XTTS 等价 | T3 绿；同一文本经 cloud 与 local 两 provider，`phonemes` 结构同形（字段名/精度 3 位/per-char 等分） | [A] |
| 5 | duration 与真实音频时长一致 | 解析返回 WAV 的 data chunk 字节数 ÷ (24000×2) 与 `duration` 字段差 < 0.01s | [A] |
| 6 | 前端零改动即可播放 | `useTTSPlayer.ts` 无 diff；`:215` 三项硬校验全过（BFF 响应含 audio+phonemes+duration） | [A] |
| 7 | 缺 key 自动降级 XTTS | compose `TTS_API_KEY` 空 → `/tts/phonemes` 仍 200 且有音频；BFF 日志**无** Warn（dev 常态） | [A] |
| 8 | cloud 故障降级 + 可观测 | 注入不可达 BaseURL → 请求仍 200（走 XTTS）+ 日志含降级 `reason` + 指标 +1 | [A] |
| 9 | `provider=cloud` 不静默降级 | 同故障下返 5xx（非 200），证明回滚/甄别开关有效 | [A] |
| 10 | `provider=local` 回滚开关 | 有 key 也走 XTTS（响应头/日志/耗时特征可辨） | [A] |
| 11 | 中文 payload 无编码损坏 | 请求体含中文，**用 Go/node 构造 UTF-8**（禁 Git Bash 内联）；ASR 回读或人工试听确认念的是原文而非 mojibake | [A]+[V] |
| 12 | 首声延迟实测达标 | 33~50 字段落：BFF `/tts/phonemes` 端到端 p50 ≤ 3s（对照 XTTS 基线 15~36s，baseline JSON 留档） | [A] |
| 13 | 数字人端到端可听 + 口型动 | dev mode IAB/Playwright：发消息 → 听到语音 → 口型 BlendShape 随 `currentTime` 变化（非静止/非末帧卡死） | [V] |
| 14 | 多段连续播放无断点 | 长回复切 ≥3 段，段间 gap 实测（对照 baseline `segment_gap.json` 269ms）；**若复现 F-195 时序缺陷则判 FAIL-记账不修** | [V] |
| 15 | 音量语义不回归 | cloud 路径下前端 `audio.volume` clamp 仍生效（F-140 不复发）；主观响度与 XTTS 路径可比（M1 决议后） | [V] |
| 16 | 音色为官方预置（非克隆） | 试听确认使用配置的预置音色；`references` 参数**未**出现在请求体 | [V] |
| 17 | 计费口径可估 | 按 UTF-8 字节数估算单次成本并记入 report（中文 3 字节/字） | [A] |
| 18 | secret 红线零违反 | `git log -p` 全 diff 无 `sk-` 真 key；`.env.local` 未被修改/提交；example 只有空值 | [A] |
| 19 | env 变量门禁绿 | `scripts/lint_env_vars.sh` GREEN；`e2e_stage_audit.py --all` 0 FAIL | [A] |
| 20 | 回归钉 + 单测全绿 | `go test ./... -race` 绿；新增 Playwright/vitest 回归钉绿；**先验 :3000 服务身份**（F-196 铁律：chunk URL 形态甄别，否则前端证据无效） | [A] |

**收口判据**：`BLOCKED` 不得超过 1/3；#13/#14 必须有真实证据（截图/录音/时间戳 JSON），"已实现/已接入"**不算证据**（RUNBOOK §4.1）。

---

## §F 决策点（[M]，需用户裁定后才动手）

| # | 决策 | 选项 | 建议 |
|---|------|------|------|
| **M1** | `volume`（XTTS 线性增益 2.0）→ `gain`（dB [-10,10]）如何映射 | A. 忽略 volume，`gain=0`（云端原始响度）<br>B. 线性→dB 换算：`gain = 20*log10(volume)` ⇒ 2.0 → **+6.02 dB**（响度与 XTTS 路径对齐）<br>C. 配置固定 `TTS_GAIN`，不透传前端 volume | **B**：保持与现有 XTTS 路径主观响度一致，避免"换云端后声音变小"被当成新缺陷；clamp 到 [-10,10] |
| **M2** | 本轮是否做 tier-2 流式（`stream=true` + pcm + `pcmStreamPlayer`） | A. 只做 tier-1（非流式完整 WAV）<br>B. tier-1 + tier-2 一起做 | **A**：tier-1 已把首声 20~40s → ~1.5s（B.4 实测），收益的 95% 已到手；tier-2 需改前端播放路径 + 处理 phonemes 与流式的时序矛盾，且 F-195（段队列时序缺陷）未修、`pcmStreamPlayer` 未合并 ⇒ 风险叠加。tier-2 单独立项 |
| **M3** | provider 抽象落在 BFF 还是 ai-svc | A. BFF downstream 层<br>B. ai-svc（Python） | **A**：数字人链路 `/tts/phonemes` 本就是 BFF→XTTS 直连（H7），落 BFF 改动面最小、ai-svc 零改；且 BFF 已有 secret 注入范式（`BFF_LLM_API_KEY`） |

> M1/M2/M3 未裁定前**不写实现代码**（RUNBOOK 决策门纪律）。M3 已按 H7 收敛为 A，如无异议即按 A 执行。

---

## §G 执行与收口

**开工前置**
1. 用户裁定 M1 / M2（M3 默认 A）
2. `deploy/.env.local` 已含 `TTS_API_KEY`（ADR 记载 2026-10-06 用户提供并实测通过）——**开工时只验证"变量存在且非空"，不打印值**
3. dev mode 归属锁：启动 19 容器栈前登记 `deploy/.devmode-session`（AGENTS.md §八.2），收工删除

**执行顺序**：T1→T8 逐个 TDD 循环（每循环独立 commit）→ 门禁自检 → dev mode 实测 #11~#16 → 回归钉 → 账本闭环 → PR。

**实测铁律（本项目已踩过的坑，逐条防）**
- 🔴 **中文 HTTP payload 禁 Git Bash 内联构造**（编码损坏 ⇒ API 收到 mojibake ⇒ 合成"流利但胡说"的语音 + 假截断，曾误导出"服务端不稳"的错误结论）。测试与 curl 一律 node/Go 构造 UTF-8 + `--data-binary @file`
- 🔴 **先验 :3000 服务身份**（F-196）：dev mode 起容器栈后 `emotion-echo-web` 容器会以旧镜像抢占宿主 3000，`pnpm dev` 静默失效 ⇒ 一切前端实测证据无效。用 chunk URL 形态甄别（hash chunk = 容器旧镜像；源码路径 = 真 dev server）
- 🔴 **文本长度盲区**（F-138 教训）：curl 用"你好"2 字恒绿 ≠ 49 字真实回复可用。实测必须用**生产同形长度**文本
- 🔴 改 Go 后必须**重建镜像**再验收；改 compose 后 web-bff 必须重启
- 判 FAIL 前必须重试（F-192 宿主 ConnectionReset 簇 ~1/8 间歇假失败）

**收口契约**（RUNBOOK §13，逐条）：测试点 20/20 有结论 + 证据留档 → `§8` 回填 → `e2e_stage_audit.py --all` 0 FAIL → 第二方核对（子代理，任务书须明写"自证不可信 + 独立跑"）→ 账本 F-198 翻 ✅ + 新发现按 E2E-F-续号登记（**下一号 = F-199**）→ docs 级联（roadmap 指针 / decisions / ADR 状态）→ 收口 PR（squash + 立即删源分支，§2.5）→ 收工三查。

---

## §H 调研依据（commit message 末尾用）

调研依据：`tts_handler.go`、`downstream/xtts.go`、`config/config.go`、`XTTS/server.py:93,96-100,330-341`、`useTTSPlayer.ts:146,195,215,250`、`deploy/docker-compose.apps.yml:647,665`、`scripts/lint_env_vars.sh`、ADR `adr-2026-10-tts-api-cosyvoice2`（决策 40 / D-44）、SiliconFlow TTS 官方文档（WebFetch 实读）、账本 F-198/F-135/F-195/F-196/F-197/F-138。

---

## §I 执行记录（2026-10-07，PR #162 建档 + #163 实施，main=f7c51c4）

**T1~T8 全部落地**（test+impl 成对 commit；CI Linux 全绿含 `-race`）。

**决策点处置**：M1=B（`gain=20·log10(volume)` clamp [-10,10]，volume≤0 不发字段）/ M2=A（仅 tier-1）/ M3=A（BFF downstream 层）。

**20 测试点结论速览**：

| 组 | 结论 |
|----|------|
| #1~#6 provider/契约/复刻/前端零改动 | ✅ 单测全绿（web-bff 12 包 + shared）+ CI |
| #7 缺 key 自动降级无 Warn | ✅ 运行时实证（重建 BFF 后真链 200，BFF 日志 0 条 degraded） |
| #8 cloud 故障降级 + reason | ✅ 运行时实证（真 key + 上游哨兵缺陷期间：200 走 XTTS + `Warn(reason)` 留档）+ 单测 |
| #9 mode=cloud fail-loud / #10 mode=local | ⚠️ 单测覆盖；运行时探针因 shell env 未达 compose 插值未完成（非阻塞，模式开关本身有单测钉死） |
| #11 中文 payload 无编码损坏 | ✅ node 构造 UTF-8 `--data-binary`（红线执行），22 字原文到达上游并合成 |
| #12 首声延迟 | ✅ **p50=1.34s（4/4，网关端到端）** vs XTTS 同文本 18.8s ≈14x；证据 `tts_evidence.json`/bench 运行日志 |
| #13/#14 数字人可听+口型/多段 gap | 🔴 未做（浏览器 `[V]`）→ **转账本 F-199** |
| #15 响度 / #16 音色 | 🔴 未做（可听性主观项）→ 转 F-199 |
| #17 计费口径 | ✅ 按 UTF-8 字节计（官方文档）；22 字 120B/请求量级，~几元/月 |
| #18 secret 红线 | ✅ 全 diff 无 key；yaml 无 Key 字段（测试钉死）；`.env.local` 未动（key 由用户 2026-10-07 自行加入） |
| #19 env 门禁 | ✅ `lint_env_vars.sh` GREEN + 守卫 19/19 接 CI |
| #20 回归钉 | ⚠️ 后端契约由 T1~T8 单测钉死；浏览器回归钉随 F-199 一并落 |

**运行时抓到并 TDD 修掉的 2 个真缺陷**（本轮最大价值）：
1. **SiliconFlow WAV `data` 块流式哨兵 0xFFFFFF00 误判截断**——真 key 首测 cloud 返回 307KB 完整音频却被 `parseWavMeta` 拒收 → 按设计降级 XTTS → 专项目标整体落空。修法：data 声明尺寸不可信（哨兵/截断不可区分）一律以实际到港字节算 duration；fmt 块仍须完整。
2. **integration 测试构造签名未同步**——`test_integration_tag_compiles.sh`（守卫 8/8）在 CI 抓到，正是 E2E-23"build tag 代码无门禁编译"教训的活例证。

**方法论留痕**：`| tail` 吃掉管道退出码造成"构建成功"假象（本 pipefail 陷阱的新形态：无 pipefail 时 tail 恒 0 掩盖 compose 失败）；`--env-file` 相对路径随 cwd 解析（构建必须在 deploy/ 下跑）；worktree 缺 gitignored 证书/`.env.local` 时 Docker 把挂载点建成目录 → 服务 crash-loop（`deploy/tls/*` 需从主工作区复制）。

---

## §J F-199 浏览器端到端机械轮执行记录（2026-10-07 晚，IAB 实测）

**环境**：main=3bded37 / web-bff v0.1.32（15:40:50 构建，含 T1~T8 + T3 哨兵修复）/ 本地 `pnpm dev` :3000（chunk URL 源码路径甄别通过，F-196 铁律）/ Nacos count:7 / cloud provider + 真 key。会话 #550、演示账号 echo、5 条消息 14 段。证据包：[f198-browser-evidence/](f198-browser-evidence/f199-tts-browser-evidence.json)（JSON + 4 截图，截图已人工目视）；11 段 WAV 本地留存 `f199-audio/`（gitignored，不入库）。

| F-199 子项 | 结论 |
|----|------|
| ① 可听 | ✅ 14/14 段真实播放（HTMLMediaElement.play 只读探针），currentTime 逐段推进，零 console/page error |
| ① 口型 | 🔴 **FAIL 实锤**——BFF phonemes char=汉字（T3 复刻 XTTS per-char，两 provider 同病）vs 前端 `charToLipShape` 拉丁映射表 → 89/89 次回调 neutral，口型从未激活。**长期潜伏缺陷，非云端迁移引入**。修复三选项属 [M] 用户拍板（a BFF 出拼音韵母 / b 前端 pinyin-pro / c 伪口型不推荐）；回归钉随修复 TDD |
| ② 多段 gap | ✅ 5-41ms（基线 269ms）；F-195 未复现（21 fetch/14 play 差值 7 = 测试者自跑探针，已逐一对账） |
| ③ 响度 | 客观 ✅（peak -4.6~-10.2 / RMS -23.5~-26.3 dBFS，散差 2.8dB）；主观待用户 |
| ④ 音色 | 待用户听 `f199-audio/seg-01~11.wav` |

**新观察**：① `speed` 默认 0.75（useTTSPlayer.ts:429）→ 播放墙钟 ≈ 音频时长 ×1.33，设计行为非缺陷；② duration/bytes/phonemesTotal 3/3 自洽（24kHz mono 16bit），一次 '你好' 短文本上游不一致（duration=2.32s vs bytes=0.64s）未复现，记观察；③ SiliconFlow WAV data 块 size=0xFFFFFF00 哨兵在生产路径被 T3 修复正确钳制（python `wave` 模块等"信任头字段"的读法会得天文数字时长，已知坑）。

**F-198 专项整体**：API 链路 ✅（§I）；浏览器机械项如上；**完全闭环剩 = 口型 [M] 决策 + 修复 + 用户主观判定（③④）**。

### §J.1 口型修复 + 用户判定闭环（同日晚，D-45）

用户 AskUserQuestion 三项裁定：**① 口型修复 = 前端 pinyin-pro**（备选 BFF 拼音韵母/伪口型被否）；**③ 响度合格，保持 M1=B**；**④ 音色接受，保持 anna**。

**修复落地**（TDD RED→GREEN，commit 内测试+实现成对）：
- `useTTSPlayer.ts`：`charToLipShape` 拉丁表 miss 后走 `hanziToLipShape`——pinyin-pro 取默认读音 → 韵腹优先级 **a>o>e>i>u>ü 全词扫描**（ian 韵腹是 a、yue 是 ü）→ 复用同一张 `VOWEL_TO_LIP`；特判 **jqxy 后的 u 实为 ü**（鱼/去/需/举）；`Map` 缓存 4096 条（ontimeupdate ~4/s）；标点/无韵腹字符仍 neutral。
- 契约测试：韵腹 14 例 + 标点 3 例 + 多音字稳定性 + 缓存一致性（`useTTSPlayer.phoneme.test.ts`）；**RED 3 例确认红 → GREEN 18/18**；全仓 vitest **628/628**；lint 8 errors 为 pre-existing（旧测试块 require() 风格，范围外记账不修）。
- 依赖：`pinyin-pro`（parallel-tracks §六 已登握手行；Lane O 无需跟进）。

**修复后 IAB 复测**（会话 #550，HMR 载新码，第 6 条消息）：3/3 段播放，口型回调 **76 次中 60 次非 neutral（78.9%；ih16/ee11/oh13/aa18/ou2）**，修复前 89/89 全 neutral；零 console/page error；4 帧面部特写可见嘴部开合（证据截图 `f198-browser-evidence/screenshots/`）。

**F-198 专项自此完整闭环**（API 链路 §I + 浏览器机械轮 §J + 口型修复与主观判定 §J.1）。下一阶段 = E2E-29（横切：异常与安全，待建档）。
