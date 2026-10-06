# ADR-2026-10: 在线 TTS 主链路改接 SiliconFlow CosyVoice2（XTTS 降级离线回退）

## Status

✅ **Accepted**（2026-10-06 用户拍板 D-44；决策 40 同日登记；实施 = E2E-F-198 专项）

## Context

**现状**：语音回复首声 20~40s——XTTS 在 CPU 上推理 49 字实测 15~36s（`emotion-echo-models/XTTS/`，容器 `cpus=8.0`，D-32/F-132 调优后仍受 CPU 算力硬顶）。

**D-43/F-135 双端点方案实测否决**（2026-10-06，详见 E2E 轨决策 D-44 证据链）：

1. XTTS 模型不并发安全：`gpt.py:570 store_prefix_emb` 每次推理覆盖写共享态 `gpt_inference.cached_prefix_emb` ⇒ 跨端点并发 = 数据竞争（已修 `INFERENCE_LOCK` 模型级互斥，XTTS v2.0.1）；
2. 互斥后 stream/phonemes 串行 ⇒ 双端点"早出声"收益归零（首声 22~34s ≈ 现状 20~30s）；
3. `inference_stream` 按文本片段 yield，切段后单句 = 单片段，无渐进流式（首块 36.8s = 完成时刻）；
4. 双端点净成本 = ×2 推理压力 + BFF 90s 整请求超时掐死队列尾。

**结论**：首声加速无法在 XTTS CPU 推理内达成 ⇒ 换外部流式 TTS API（D-44 用户拍板，代价知情：官方音色 + 联网依赖）。

**选型实测（2026-10-06，key 实测通过后）**：SiliconFlow 平台仅 2 个 TTS 模型，实测对照：

| 项 | CosyVoice2-0.5B ✅ | MOSS-TTSD-v0.5 ✗ |
|---|---|---|
| 流式首字节（TTFB） | **~0.33s**（4/4 稳定） | ~0.31s |
| 33 字完整合成返回 | **1.4~1.6s（≈2x 实时）**，4/4 无一截断 | 3~6.4s，但时长差 1.6~3x |
| 音色 | 8 个预置音色（anna/alex/…） | 无预置，必须 references 零样本克隆 |
| 内容忠实度 | ASR（SenseVoice/Qwen3-ASR）逐字回读原文 ✅ | 复读/续写参考音频内容，33 字念 23s ✗ |
| 输出格式 | pcm = 裸 int16 LE 单声道 24kHz | pcm 同格式但仅 44.1k 默认 |

## Decision

**在线 TTS 主链路 = SiliconFlow `FunAudioLLM/CosyVoice2-0.5B` 流式 API；XTTS（v2.0.1）降级为离线回退（key 缺失 / 断网 / API 异常时自动降级）。**

1. **Provider 接口隔离**：TTS 上游抽象为 provider 接口（`cloud` = CosyVoice2 API / `local` = XTTS），实施时挂在现链路（web → BFF `emotion-echo-web-bff/internal/downstream/ai.go` `/api/v1/tts/synthesize` → ai-svc → XTTS :8003）相应层，切换逻辑对前端透明；**禁止在业务代码里直写 SiliconFlow 域名/模型名**（经配置注入，便于换供应商）。
2. **配置（已就位 `deploy/.env.local`，红线同 LLM key：只进 gitignored `.env.local`，严禁提交）**：
   - `TTS_API_KEY`（`sk-` 前缀，2026-10-06 用户提供并实测通过）
   - `TTS_API_BASE_URL=https://api.siliconflow.cn/v1`
   - `TTS_MODEL=FunAudioLLM/CosyVoice2-0.5B`
3. **播放层复用**：F-135 前端实现（`pcmStreamPlayer.ts` WebAudio int16 PCM 流式播放器）保留在本地分支 `feat/e2e-28-f135-dual-endpoint-tts`（不合并），TTS API 落地时直接复用——CosyVoice2 pcm 输出与该播放器格式直配。
4. **回退链**：cloud provider 失败（key 缺失/网络/5xx）→ 自动降级 XTTS `local` provider（v2.0.1 含 `INFERENCE_LOCK`，回退路径并发安全）；降级事件打日志可观测。

### 否决的备选

| 备选 | 否决理由 |
|---|---|
| 继续优化 XTTS 本地推理（多 worker/双端点） | D-44 实测否决：模型不并发安全 + 串行化后无收益 + 无渐进流式 |
| 本地 GPU（RTX 4050 级） | 首声可到 2~4s 且免费，但需改部署面 + 音色不变；留作后续可选项，本轮不采用 |
| 云 GPU 服务器 | 700~3000 元/月，成本与收益不成比 |
| MOSS-TTSD-v0.5 | 实测不适合短句朗读：无预置音色 + 复读参考内容（选型对照见上表） |

## Consequences

**正面**：

- 首声 20~40s → **亚秒级 TTFB + ~1.5s 完整首句**（≈20 倍），实测 4/4 稳定
- 零 GPU 运维，XTTS 容器保留作离线回退（不删）
- 播放层零改造（PCMStreamPlayer 直配）

**负面 / 留账**：

- 联网依赖 + 按量费用（SiliconFlow，量级 ~几元/月）；供应商锁定风险由 provider 接口隔离兜底
- 音色变为官方预置音色（不做声音样本上传复刻——用户隐私考量，知情拍板）
- **实施注意（本轮实测教训）**：含中文的 HTTP 请求体严禁 shell 内联构造（Git Bash 编码损坏 ⇒ API 收到 mojibake，合成"流利但胡说"的语音 + 假截断，曾误导出"服务端不稳"错误结论）；测试与客户端一律 UTF-8 构造 payload。Go `http.Client` 原生 UTF-8，生产链路无此风险
- `XTTS_BASE_URL`/180s 超时等既有配置不动（回退路径语义不变）

## 实证

| 项 | XTTS 现状 | CosyVoice2 API（2026-10-06 实测） |
|---|---|---|
| 流式首字节 | 无流式（首块=完成）36.8s | **0.31~0.35s**（4/4） |
| 33 字完整返回 | 15~36s（CPU 推理） | **1.38~1.55s**（4/4） |
| 内容忠实度 | 忠实 | ASR 回读逐字一致 |
| 并发安全 | 需 INFERENCE_LOCK（v2.0.1 已修） | 服务端托管 |

## 调研依据

- [x] 代码回读：`emotion-echo-models/XTTS/synth_pool.py`（INFERENCE_LOCK/locked_stream）、`server.py`、`gpt.py:570`（并发不安全根因）、`tests/unit/test_inference_lock.py`（8 测试）；`emotion-echo-web-bff/internal/downstream/ai.go`（TTS 转发链）、`internal/config/config.go`（XTTS 180s 超时）；`emotion-echo-web/app/composables/useTTSPlayer.ts`、`app/lib/pcmStreamPlayer.ts`（播放层，本地分支）
- [x] 选型实测数据（2026-10-06）：SiliconFlow `/v1/models` 全量列表（TTS 仅 2 个）、`/v1/audio/speech` 流式/非流式 × pcm/wav/mp3 × 音色/采样率矩阵、ASR 回读闭环
- [x] 官方文档：[SiliconFlow TTS 能力页](https://docs.siliconflow.cn/cn/userguide/capabilities/text-to-speech)（参数表/voice 格式/references 克隆）
- [x] E2E 轨决策 D-43 → D-44、账本 F-135（否决销账）/F-198（本 ADR 的实施专项）
- [x] 部署红线：`deploy/.env.local` key 管理（F-69 先例 + memory: deploy-env-local-key-protection）

## 关联

- E2E 轨决策：**D-44**（F-135 否决 + TTS API 路线拍板）
- 架构决策：**决策 40**（`docs/architecture/decisions.md`）
- 账本：**E2E-F-198**（实施专项，owner = 下一阶段）、E2E-F-135（否决销账）
- 前序 ADR：`adr-2026-10-xtts-synth-threadpool.md`（决策 39，XTTS 回退路径的并发基础）、`adr-2026-09-xtts-cpu-quotas.md`（F-132）
