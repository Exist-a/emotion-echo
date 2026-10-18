# T-18 音色对照样音（供人耳裁定）

同文本、同语速（1.0）、同响度（volume=2.0），只换 `voice`：

| 文件 | voice | 时长 |
|---|---|---|
| `phase8-tts-voice-anna.wav` | `FunAudioLLM/CosyVoice2-0.5B:anna`（女声，配置默认） | 4.04s |
| `phase8-tts-voice-alex.wav` | `FunAudioLLM/CosyVoice2-0.5B:alex`（男声） | 3.92s |

文本：`你好，我是你的陪伴者。今天过得怎么样？`

**机器侧已证**：两个请求体的 `voice` 不同 ⇒ 上游返回的音频 MD5 不同
（`anna=44d3bf0b…` / `alex=2359c1e0…`），且两次都返回 `phonemes`（19 条）+ `duration` > 0
⇒ 口型链路在换音色时未回归（T-13）。

**需要人裁的部分（[V]）**：换成不同音色后**听起来**确实是不同的人（T-18）。
机器能证明"参数生效且音频不同"，但"是否可听出差异/是否悦耳"只有耳朵能判。

⚠️ 证据副本的 WAV 头已修正（上游返回的 data chunk size 是流式哨兵 `0xFFFFFF00`，
见 memory F-198；前端 `base64ToWavBlob` 不建头所以能正常播，但通用播放器会读错总时长）。
