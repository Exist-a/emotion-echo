---
stage: e2e-17
title: 数字人 + TTS（真口型同步 + 段间断点）
type: transformation
status: pending
created: 2026-09-23
depends-on: [e2e-16]
blocks: [e2e-28]
gate: []
related-findings: [E2E-F-05, E2E-F-06]
---

# E2E-17 数字人 + TTS — 详档

> **类型**：transformation —— 口型是随机轮播假动画、TTS 段间必然断点，两条链路"能出声、但体验是假的/断的"，必须先实现 D-03 再可验证。
> **依据**：D-03 决议（[decisions.md](../../decisions.md)，用户 2026-09-17 确认"做真口型同步 + 排查断点"）+ 账本 E2E-F-05/F-06（预探查 2026-09-17）。
> **方法论**：[RUNBOOK.md](../RUNBOOK.md)（状态机 / §4.1 证据有效性 / §7 收口契约 11 项 / §13 审计）+ [anti-patterns.md](../anti-patterns.md)（14 类反例）。

---

## 1. 阶段目标

让数字人说话从"**随机嘴巴动 + 段与段之间卡顿**"变成"**口型按 phoneme 时间戳对齐音频 + 多段连续播放无明显断点**"，并让这两件事可机械验证（时间戳驱动断言 + gap 时长测量）。

## 2. 范围与边界

### 做

**A. 真口型同步（E2E-F-05 / D-03 第 1 条）**

1. **XTTS 端核实**：`emotion-echo-models/XTTS/server.py:283-341` 的 `/tts_with_phonemes` 已返回字符级时间戳（`phonemes: [{char, start_ms, end_ms}?]` 以实际响应为准，开工第一步先 curl 拿真实样例钉契约）——核实是否非流式、时间戳粒度、与 `/tts_stream` 的关系
2. **BFF 转发通道**：现 `/api/v1/tts/stream`（`tts_handler.go` → `SynthesizeSpeech`）返回裸 WAV 无时间戳通道；proto `SynthesizeSpeechResponse` 也无 phonemes 字段 ⇒ 需要一个带时间戳的返回通道（选项：① 前端直调 BFF 新端点 `/api/v1/tts/phonemes` 转发 XTTS；② proto 加字段走 gRPC。**开工时按"改动最小 + 不破坏既有 /tts/stream 消费者"裁定，裁定记入 report**）
3. **前端播放层改造**：`useTTSPlayer.ts`
   - 激活死代码映射表（L38-75 `VOWEL_TO_LIP` / `CONSONANT_CLOSE`，当前全文件无引用）
   - 删 `startRandomLipAnimation`（L91-103 每 150ms 轮播 5 口型的假动画）
   - 口型驱动改为按 phoneme 时间戳：音频 `currentTime` 命中 `[start_ms, end_ms)` 区间 → 映射口型 → `lipSyncCallback` → `DigitalHuman.client.vue` `setLipShape`
   - 播放结束/中断 → `resetLipShape`（闭嘴，防口型冻在末帧）
4. **数字人侧复用**：`DigitalHuman.client.vue:311 setLipShape` 与 BlendShape 过渡（L166 `lipShapeTransitionTime`）已就绪 —— 只验证驱动信号真实性，不动渲染层（除非时间戳驱动暴露渲染 bug → 记账）

**B. 段间断点排查与修复（E2E-F-06 / D-03 第 2 条）**

5. **量化现状**（先测后修，禁止没数据就改）：构造多段回复（AI 回复 > 500ms 生成时长），测量段间 gap：`audio.ended` → 下一段 `play()` 的时长；记录修复前基线
6. **修法方向**（按 D-03 影响面，实测后选）：
   - `useTTSManager.ts:46-52` 的 500ms debounce 分段策略重审（聚合窗口 vs 首包延迟权衡）
   - 段间不 `stop()` 重连：上一段未播完的 buffer 与下一段衔接（`useTTSPlayer` 的 `flushBuffer` / chunk 队列）
   - XTTS 每段冷启动（`inference_stream` per-segment）—— 若 BFF/xtts 侧无法预热，至少消除前端侧 stop→重开的 gap
   - **验收口径**：段间 gap 中位数 < 200ms（阈值开工时结合基线确认，写入 report；若客观不可达，记录实测值 + 原因 + 降级裁定，禁止拍脑袋宣布达标）
7. **口型跨段连续**：段间 gap 期间口型归 neutral/闭嘴而非冻结

**C. 测试基建**

8. **前端单测**（Vitest）：映射表激活契约（元音→口型逐项）、时间戳→口型区间判定（边界：start_ms 命中/未命中/区间外）、假随机动画已删、播放结束 reset
9. **Playwright 回归钉**：`e2e/digital-human-tts.spec.ts` —— 发消息 → 数字人区域可见 + （fake media/audio stub 下）`setLipShape` 被按序调用（而非 150ms 固定节奏）+ 多段播放无 `stop` 抖动（可通过 spy 播放事件序列断言）
10. **gap 测量脚本**：`scripts/` 或 spec 内测段间 gap，输出数值作证据（[A]）

**D. 账本**

11. F-05 / F-06 状态更新（实现落地后回填证据）；新发现按序登记 F-126 起，**先登记再修**

### 不做（边界）

| 不做项 | 理由 |
|--------|------|
| E2E-28 性能基线（TTS 单段/段间 gap 的 p50/p95 阶梯压测） | D-03 只做"断点修复 + 单场景测量"；系统性基线归 E2E-28（本阶段的 gap 测量方法可复用过去） |
| XTTS 模型/音色/质量调优 | 归模型侧，非前端播放链路 |
| 数字人外观/动作/表情系统（非口型部分） | `setEmotion` 已在 D-14 验证；本阶段只管口型信号 |
| TTS 双缓冲/预取等流式架构大改 | 若实测证明必须（gap 根因在架构），记账升级，不在本阶段顺手重构 |
| 真人主观听感/口型"像不像"评审 | [M] 类需用户裁定的点单列，不冒充机械验收 |
| i18n | D-04 候选未决 |

## 3. 前置条件

| 条件 | 状态 |
|------|------|
| E2E-16 done（多模态，TTS/数字入口在聊天页） | ✅ 2026-09-23 收口（F-119 用户侧留账不阻塞） |
| D-03 已决议 | ✅ 用户 2026-09-17 确认（decisions.md） |
| `e2e_stage_audit.py --all` 0 FAIL | ✅ 2026-09-23 实测（30 阶段 0 FAIL） |
| `deploy/.env.local` 存在 | ✅（**严禁删除/覆盖**，AGENTS.md §四红线） |
| XTTS 容器可用 + `/tts_with_phonemes` 可达 | ⚠️ 开工第一步 curl 实测（**E2E-16 曾观察到 xtts 停止导致 TTS 502**，E2E-16 无害但本阶段是硬前置——不通则 BLOCKED 并记账） |
| 数字人 VRM 模型可加载（`/3d-models/digital-human.vrm`） | ⚠️ 开工时 IAB 实测 |
| dev 栈启动 | **必须带 `--env-file .env.local` 与 `--profile dev`**（见 RUNBOOK §2.1；缺 profile → 6 服务注册失败，E2E-F-108） |

```bash
cd deploy && docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml \
  --env-file .env.local --profile dev up -d
# 重建 BFF/web 后必跑（运维铁律）：
bash apisix/seed.sh && docker compose -f docker-compose.apps.yml --env-file .env.local up -d emotion-echo-web-bff
```

## 4. 测试点清单（初稿，开工首日按实测校准）

判定标记：`[A]` 自动 · `[V]` 视觉 · `[M]` 需裁定。详见 [RUNBOOK.md](../RUNBOOK.md) §4。

| # | 测试点 | 判定 | 验证方式 | 证据 | 结果 |
|---|--------|------|---------|------|------|
| 1 | `/tts_with_phonemes` 返回字符级时间戳（真实样例钉契约） | [A] | curl 真实端点 + 断言字段结构 | 响应 JSON 样例 | ⬜ |
| 2 | BFF 时间戳转发通道打通（新端点或 proto 扩展，按开工裁定） | [A] | curl BFF + 断言 phonemes 数组非空 | curl 输出 | ⬜ |
| 3 | 前端播放层收到 phonemes 并存入播放项 | [A] | Vitest：TTSResponse.phonemes → 播放队列 | 单测输出 | ⬜ |
| 4 | 映射表激活：元音/辅音→口型逐项正确 | [A] | Vitest 表驱动（VOWEL_TO_LIP + CONSONANT_CLOSE 全键） | 单测输出 | ⬜ |
| 5 | 时间戳→口型区间判定（边界：命中/未命中/区间外/结束归 neutral） | [A] | Vitest 边界表驱动 | 单测输出 | ⬜ |
| 6 | 假随机轮播已删除（150ms 固定节奏不存在） | [A] | 字面量契约：`startRandomLipAnimation` 不再被调 / 代码不存在 | 静态契约输出 | ⬜ |
| 7 | 口型由音频 `currentTime` 驱动（非定时器） | [A]+[V] | 单测（timeupdate 区间断言）+ IAB 观察数字人嘴部 | 单测 + 截图 | ⬜ |
| 8 | 播放结束/中断 → 口型 reset（不冻在末帧） | [A] | Vitest：ended/stop 后 setLipShape(neutral) | 单测输出 | ⬜ |
| 9 | 段间 gap 基线（修复前数值） | [A] | 测量脚本输出 gap 中位数 | 数值记录 | ⬜ |
| 10 | 段间 gap 修复后 < 阈值（阈值开工定） | [A] | 同 #9 复测 | 数值对照 | ⬜ |
| 11 | 多段播放期间无 stop→重开抖动（事件序列断言） | [A] | Playwright spy 播放事件 | 事件序列 | ⬜ |
| 12 | 口型跨段连续（段间归 neutral 不冻结） | [V] | IAB 截图/录屏段间帧 | 截图 | ⬜ |
| 13 | 端到端：发消息 → AI 回复 → 数字人开口说话 | [A]+[V]+[M] | Playwright 全链路 + IAB 截图 + 用户裁定"像在说话" | spec + 截图 | ⬜ |
| 14 | 回归钉：Playwright `digital-human-tts.spec.ts` 双 project 全绿 | [A] | `npx playwright test` 输出 | 24/? PASS | ⬜ |
| 15 | 既有 D-14 情绪联动不回归（onEmotionChange→setEmotion） | [A] | 既有 d14 测试仍绿 | 测试输出 | ⬜ |

汇总行（收口时填）：`PASS x / FAIL x / BLOCKED x / N/A x`

## 5. 风险与已知阻塞

| 风险 | 应对 |
|------|------|
| xtts 容器停止 → TTS 502（E2E-16 观察） | 开工第一步 curl `/tts_with_phonemes`；不通先起容器 + 记账 |
| `/tts_with_phonemes` 非流式，与流式播放架构冲突 | 记录实测形态；若必须流式时间戳 → 记账升级（可能触发 D-03 补充裁定） |
| gap 根因在 XTTS 每段冷启动（服务端），前端无法消除 | 实测区分前端/服务端 gap 占比；服务端部分记账归 E2E-28/模型侧 |
| BlendShape 渲染不吃高频口型切换（性能/平滑） | `lipShapeTransitionTime` 已有过渡；实测卡顿则记账 |
| 真人主观"像不像"无法机械判定 | #13 的 [M] 部分升级用户裁定，不冒充 [A] |

## 6. 开工顺序（六步循环内）

1. 前置实测：xtts `/tts_with_phonemes` curl 样例 + VRM 加载 + gap 基线（#1/#9）—— **先测后写**
2. 按实测校准本 plan 测试点（时间戳字段名/通道方案/gap 阈值）
3. RED→GREEN A 链（BFF 通道 → 前端播放层 → 映射激活）
4. RED→GREEN B 链（gap 测量 → 修复 → 复测）
5. Playwright 回归钉 + IAB 视觉证据
6. 账本回填（F-05/F-06/F-12x）+ report 按 §10 模板收口
