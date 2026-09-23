# ADR-2026-09 E2E-17 F-127 BFF `/api/v1/tts/phonemes` 端点落地记录

> **状态**：✅ Accepted（2026-09-23，E2E-17 plan §6 step 2 实施记录）
> **决策 D-25** 已决议 XTTS 镜像源切换；本文档只记录**端点层面的设计落地**
> **关联**：ADR-2026-09 XTTS v3（仓镜像）+ ADR-2026-09 XTTS v2（vendor 路线退役）+ [plan §2.A](../e2e-roadmap/stages/e2e-17-digital-human-tts/plan.md) + 账本 E2E-F-127

---

## 一、范围

本 ADR 不引入新架构决策（端点级落地，已在 plan §2.A 选 B 锁定），**只固化**:

1. **端点形态**：`POST /api/v1/tts/phonemes`（BFF 新增，与既有 `/tts/stream` 并存）
2. **下游通道**：`downstream/xtts.go` 新增 `XTTSClient.Phonemes(ctx, req) (*XTTSPhonemesResp, error)`，镜像 Stream 的 nil-safe + 错误透传
3. **配置变更**：`config.go` XTTS `TimeoutMs` 默认 30000→**90000**（E2E-F-115 同型但 XTTS 路径，CPU 推理实测需更长）
4. **回归钉强制同步**：新增 `XTTSClient.Phonemes` 接口方法后，`multimodal_tts_upload_handler_test.go` 旧 fakeXTTSForTTS 必须补同名方法（接口扩展强制旧 fake 同步，避免编译破坏）

---

## 二、为什么不复用 `/tts/stream`（避免内容混淆）

`/tts/stream` 返裸 WAV 流（音频播放用），`/tts/phonemes` 返 JSON（含 base64 音频 + phonemes 数组）—— **格式 + 用途都不同，混用会破坏既有消费者**。新端点最小化改动：
- 路由独立
- handler 独立（共用同一 TTSHandler 注入点 + NewXTTSClient）
- audio 内容是同一 WAV bytes（base64 编码而非流式）—— 前端拿到再 decode

---

## 三、phoneme 字段口径（**必须在 report.md 明确**）

仓 `server.py:319-326` 的 phoneme 是 **per-char 等分近似**：

```python
phonemes = [{
    "char": c,
    "start": round(i * char_duration, 3),         # 秒
    "duration": round(char_duration, 3),          # 秒 = total_duration / len(chars)
} for i, c in enumerate(chars)]
```

⚠️ **不是真 XTTS 字符级时间戳推理**（per-char 等分 = 平均时长），仅满足 D-03 "对齐"但不等于"真字符读音时机"。

---

## 四、调用链

```
前端 useTTSPlayer
  → POST /api/v1/tts/phonemes {text,language,speed}
  → BFF handler.phonemes (tts_handler.go)
    → upstream/xtts.go Phonemes
      → POST http://emotion-echo-xtts:8003/tts_with_phonemes
        → 仓 emotion-echo/xtts:v2.0.0 (server.py:283-341)
  ← JSON {audio base64, sample_rate, text, language, phonemes[], duration}
```

---

## 五、验证

- **单测**：10 项 RED→GREEN（5 downstream + 5 handler，含 nil-safe + 4xx 透传）
- **端到端**：curl → BFF → XTTS 仓镜像，`text="你好世界"` 返 `200 {code:0, data:{audio:base64, phonemes:[4 entries 你/好/世/界], duration:1.205}}` + WAV header `RIFF...WAVE` ✓ + `last.start+last.duration==duration` ✓
- **回归钉**：F-127 账本条目（10 项） + wantRoutes 同步
- **审计**：`go test ./...` + `go vet` + `go build` 全绿

---

## 六、不变更项

- D-25（仓镜像）：不变（本 ADR 不动镜像）
- D-03（真口型同步）：不变（端点只服务数据，口型驱动在前端 useTTSPlayer 改造时落地 —— plan §6 step 3）
- `/tts/stream` 端点：不变（继续裸 WAV 流式，phonemes 端点并存不取代）