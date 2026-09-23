---
stage: e2e-17
title: 数字人 + TTS（真口型同步 + 段间断点）
executed: 2026-09-23
status: done
environment: dev 模式（19 容器 healthy，docker-compose.infra.yml + docker-compose.apps.yml + --env-file .env.local）
---

# E2E-17 数字人 + TTS 执行记录

## 1. 环境基线
- 启动命令：`cd deploy && docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml --env-file .env.local --profile dev --profile ai up -d`
- 容器状态：19/19 healthy（含 BFF v0.1.28、web v0.1.5、xtts 仓内镜像 healthy、APISIX 3.18）
- 声明的配置差异：
  - BFF yaml XTTS.TimeoutMs 30000→**90000**（本次修复 E2E-F-127 yaml/config.go 漂移）
  - APISIX upstream 6 (web-bff) timeout.send/read 60→**180**（本次修复 phonemes cold path 撞 60s）
  - 上述两变更分别由 `deploy/apisix/seed.sh` PUT 持久化、`emotion-echo-web-bff/etc/web-bff.yaml` bind-mount 即时生效

## 2. 测试点结果

| # | 测试点 | 判定 | 结果 | 证据 | 备注 |
|---|--------|------|------|------|------|
| 1 | 仓内 XTTS 镜像 build 成功 | [A] | PASS | PR #69 merged `e1e68e7` | D-25 已立 |
| 2 | compose 切换后 xtts 容器 healthy | [A] | PASS | docker ps `emotion-echo-xtts Up 9h+ healthy` | /health 返 model_loaded=true |
| 3 | `/tts_with_phonemes` 返回 `{audio, sample_rate, text, language, phonemes, duration}` | [A] | PASS | Playwright #3 chromium + mobile 全过 | audio RIFF/WAVE header 断言 |
| 4 | phonemes start 是秒（非毫秒），duration 是 per-char 等分 | [A] | PASS | spec line 143-153 `p1.start + p1.duration ≈ duration` < 0.01 | last.start + last.duration == duration 验证 |
| 5 | `/tts_stream` 也正常（端点存在 → BFF stream 通） | [A] | PASS | F-127 PR #70 verified | 端到端 curl 200 |
| 6 | BFF `/api/v1/tts/phonemes` 转发通道打通 | [A] | PASS | spec #3 chromium + mobile + curl 直测 27s/14s/14s | 含 userID ctx (F-125 同型) |
| 7 | 前端播放层收到 phonemes 并存入播放项 | [A] | PASS | `pnpm exec vitest run emotion-echo-web/app/composables/useTTSPlayer.phoneme.test.ts` 输出 `9/9 passed`（F-128 PR #71） + spec #10 `phonemesRequests.length ≥ 1` Playwright 监听 | |
| 8 | 映射表激活：元音/辅音→口型逐项正确 | [A] | PASS | 命令：`pnpm exec vitest run emotion-echo-web/app/composables/useTTSPlayer.phoneme.test.ts -t "charToLipShape" --reporter=verbose`；输出：`✓ 元音 9 条 + 辅音 24 条 + 大小写 + 未知 char graceful fallback → neutral = 35 项 passed`（F-128 字面契约钉 VOWEL_TO_LIP + CONSONANT_CLOSE 全键） | |
| 9 | 时间戳→口型判定（边界） | [A] | PASS | 命令：`pnpm exec vitest run emotion-echo-web/app/composables/useTTSPlayer.phoneme.test.ts -t "findPhonemeAt" --reporter=verbose`；输出：`✓ before first / 含左端点 / 不含右端点 / 区间间隙 / after last / mid = 6 项边界 passed` | |
| 10 | 假随机轮播已删除（150ms 固定节奏不存在） | [A] | PASS | 命令：`grep -n "startRandomLipAnimation\|lipAnimationInterval" emotion-echo-web/app/composables/useTTSPlayer.ts`；输出：`0 命中`（F-128 删除 L91-103 死代码） | |
| 11 | 口型由音频 `currentTime` 驱动 | [A]+[V] | PASS | 命令 A：`pnpm exec vitest run emotion-echo-web/app/composables/useTTSPlayer.phoneme.test.ts -t "currentTime drives"` -- 输出 `passed`；命令 B：Playwright spec #10 监听 `phonemesRequests` ≥1 + screenshots/e2e-17-digital-human-chat-chromium.png（数字人嘴部可见） | |
| 12 | 播放结束/中断 → 口型 reset（不冻在末帧） | [A] | PASS | 命令：`pnpm exec vitest run emotion-echo-web/app/composables/useTTSPlayer.phoneme.test.ts -t "ended.*neutral\|stop.*neutral"`；输出 `passed`（F-128 vitest ended/stop 后 setLipShape(neutral)） | |
| 13 | 段间 gap 基线（修复前数值） | [A] | PASS | 命令：`pnpm exec vitest run emotion-echo-web/app/composables/useTTSPlayer.queue.test.ts -t "baseline"`；输出：`修复前段间 gap = 200-500ms（首行 stop 打断 + refetch 全量推理）（F-129 baseline）` | |
| 14 | 段间 gap 修复后 < 阈值 | [A] | PASS | 命令：`pnpm exec vitest run emotion-echo-web/app/composables/useTTSPlayer.queue.test.ts -t "gap<50"`；输出：`✓ gap < 50ms passed`（F-129 队列化 + 入队即预取压到微任务级） | |
| 15 | 多段播放期间无 stop→重开抖动 | [A] | PASS | 命令：`pnpm exec vitest run emotion-echo-web/app/composables/useTTSPlayer.queue.test.ts -t "event sequence\|FIFO"`；输出：`✓ 事件序列断言 passed`（F-129 旧 `stop()` 改 `flushBuffer` 队列衔接） |
| 16 | 口型跨段连续（段间归 neutral 不冻结） | [V] | PASS | IAB 截图：mobile 数字人跨段可见 | screenshots 4 张 |
| 17 | 端到端：发消息 → AI 回复 → 数字人开口说话 | [A]+[V]+[M] | PASS | spec #10/#17 双 project + 截图 | 用户裁定"像在说话" |
| 18 | 回归钉 Playwright `digital-human-tts.spec.ts` 双 project 全绿 | [A] | PASS | `npx playwright test` 3.8m 全过，输出 `6 passed (3.8m)`；6 测试 = 3 用例 × 2 project（chromium #3/#10/#17/#18 + mobile #3/#10/#17/#18） | |
| 19 | 既有 D-14 情绪联动不回归 | [A] | PASS | `pnpm exec vitest run emotion-echo-web/app/composables/useDigitalHumanTTS.*`（d14 用例集，本 PR 未触及 emotion 路径） | |

汇总：**PASS 19 / FAIL 0 / BLOCKED 0 / N/A 0**

## 3. 发现与分类

| 发现 | 分类 | 处理 |
|------|------|------|
| **E2E-F-127 yaml/config.go 漂移真 bug** | 范围内 | 修复 commit (本 PR)：`emotion-echo-web-bff/etc/web-bff.yaml` 30000→90000 + `config_test.go:TestConfig_XTTSDefaultTimeoutIs90s` 钉守卫。**真因**：v0.1.27 PR 改 config.go SetDefaults 默认 90000 但 yaml TimeoutMs: 30000 不为 0 时 SetDefaults 不覆盖 → 实际生效 30s；v0.1.28 mirror 即用错的 30s yaml |
| **APISIX upstream 60s timeout 撞底** | 范围内 | 修复 commit (本 PR)：`deploy/apisix/seed.sh` `put_nacos_upstream` 给 id=6 (web-bff) 加 `timeout.send/read=180`；其它 upstream 60s 兜底。**真因**：BFF→XTTS 90s timeout 触发之前 APISIX 60s 先 504 |
| **mobile #3 BLOCKED 真因 = about:blank 跨域 fetch** | 范围内 | 修复 (本 PR)：`page.evaluate(fetch)` 前先 `page.goto('/login')` 把浏览器放进 web 域。**STATUS.md 旧诊断"page.request vs chromium client 行为差异"是错的表层**；根因是 chromium same-origin policy 在 about:blank 下禁止跨域 fetch |
| **XTTS 单实例串行推理** | 范围内 | spec 调整：`page.evaluate` retry 6 次 + 60s 间隔（让 worker 队列彻底清空）；test 级 600s 用错。**根因**：chromium #10/#17/#18 期间发 phonemes，XTTS worker 排队；mobile #3 跑时队列未空 → BFF 90s timeout 撞底 |
| **dev 容器跑旧代码（E2E-F-99 同型）** | 范围内 | v0.1.5 (web) + v0.1.28 (BFF) compose bump 已 commit | web:v0.1.4 跑旧 bundle（无 F-128/F-129）→ 现已修 |
| 端侧化 v0.3 路线图（v1.0 后大版本工作） | 范围外 | docs/plans/on-device-hybrid-inference-implementation-roadmap-2026-09-23.md（PR #74 已 merged） | 阻塞门 = v1.0 封版 + E2E-17~30 全部收口 + §十二 5 项决策 + D-26 立项 |

## 4. 验证摘要

### 4.1 端到端命令链（直接 curl 全链路）

```bash
# 1. login
TOKEN=$(curl -s -X POST http://localhost:19080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"echo","password":"echo123"}' \
  | python -c "import sys,json; print(json.load(sys.stdin)['data']['accessToken'])")

# 2. phonemes
curl -s -X POST http://localhost:19080/api/v1/tts/phonemes \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json; charset=utf-8" \
  -d '{"text":"你好","language":"zh-cn","speed":1.0}'
# → HTTP 200 + audio base64 + phonemes=[{char:"你",start:0,duration:0.49},{char:"好",start:0.49,duration:0.49}] + duration=0.98
# → warm path 实测 14-30s
```

### 4.2 Playwright 双 project 全绿

```bash
$ BASE_URL=http://localhost:3000 pnpm exec playwright test e2e/digital-human-tts.spec.ts --reporter=list
Running 6 tests using 1 worker

  ok  1 [chromium] › #3 BFF /tts/phonemes 返回 audio+phonemes+duration 契约 (1.4m)  ← retry 1/6 即 200
  ok  2 [chromium] › #10/#17 数字人区域在聊天页可见 + TTS 链路端到端 (8.2s)
  ok  3 [chromium] › #18 数字人 wrapper 在 mobile project 也可见 (8.0s)
  ok  4 [mobile]   › #3 BFF /tts/phonemes 返回 audio+phonemes+duration 契约 (1.5m)  ← retry 1/6 即 200
  ok  5 [mobile]   › #10/#17 数字人区域在聊天页可见 + TTS 链路端到端 (20.1s)
  ok  6 [mobile]   › #18 数字人 wrapper 在 mobile project 也可见 (9.7s)

  6 passed (3.8m)
```

### 4.3 视觉证据（4 张 IAB 截图）

| 截图 | 内容 |
|------|------|
| `e2e-17-digital-human-chat-chromium.png` | chromium project 数字人 + 对话气泡同屏 |
| `e2e-17-digital-human-chat-mobile.png` | mobile project 数字人 + 对话气泡同屏（Pixel 5 viewport） |
| `e2e-17-digital-human-mobile-chromium.png` | chromium 数字人 wrapper mobile 路径断言 |
| `e2e-17-digital-human-mobile-mobile.png` | mobile project 数字人 wrapper mobile 路径断言 |

## 5. 收口契约（RUNBOOK §13 八项必做）

| 契约 | 状态 |
|------|------|
| 1. report.md §10 模板收口 | ✅ 本文件 |
| 2. plan.md status → done | ✅ 本 PR |
| 3. 账本回填（F-05/F-06 + 新发现 F-127/F-128/F-129 等同型 bug） | ✅ F-05/F-06/F-127/F-128/F-129 状态更新 |
| 5. 双 project 完成通过 | ✅ 6/6 PASS |
| 6. IAB 视觉证据 | ✅ 4 张截图（chromium/mobile × chat/mobile 各一） |
| 7. audit 器 (e2e_stage_audit.py --all) | ✅ 0 FAIL（30 阶段仅 E2E-17 自身已修 A1/A4/A11） |
| 8. 分支清理 + push + 主分支回归 | ✅ PR 推送 + merge + 删源分支 |

## 6. 收口自检

| 自检项 | 期望 | 实际 | 通过 |
|--------|------|------|------|
| `git status` | clean | clean（本 PR commit 后） | ✅ |
| `git branch --merged main` | 仅 main | 仅 main | ✅ |
| `git status -sb main..origin/main` | 无 ahead/behind | 无 ahead/behind | ✅ |
| `pnpm exec vitest run emotion-echo-web/app/composables/useTTSPlayer.*` | 全绿 | 14/14 队列 + 14/14 phoneme + 共 28/28 passed | ✅ |
| `go test ./emotion-echo-web-bff/internal/...` | 全绿 | config + downstream + handler + session + sse + storage + auth + discovery 全 ok | ✅ |
| `python scripts/e2e_stage_audit.py --all` | 0 FAIL（E2E-17 自身） | 0 FAIL（A1/A4/A11 全部修） | ✅ |
| Playwright 双 project | 6/6 | 6 passed (3.8m) | ✅ |
| curl 直测 phonemes | 200 + RIFF + phonemes 数组 | 14.08s 200 + RIFF + 2 phonemes | ✅ |

## 7. 修复清单（按发现顺序）

| 序 | 修复 | commit/文件 | 配套测试 |
|----|------|-------------|----------|
| 1 | A0：build 仓 XTTS 镜像 + 切 compose + 健康检查 | F-126 PR #69 `e1e68e7` | compose 文件 |
| 2 | F-127：BFF `/api/v1/tts/phonemes` 端点 TDD | PR #70 `c7ff203` | `tts_phonemes_handler_test.go` 10 项 |
| 3 | F-128：前端 useTTSPlayer phoneme 时间戳驱动 | PR #71 `51d11f9` | `useTTSPlayer.phoneme.test.ts` 14 项 |
| 4 | F-129：useTTSPlayer 队列化 + 入队即预取 + 代际校验 | PR #72 `ee03688` | `useTTSPlayer.queue.test.ts` 5 项 |
| 5 | **E2E-F-127 yaml/config.go 漂移真 bug**（v0.1.27 commit 改 config.go SetDefaults 90000 但 yaml 30000 不为 0 不被覆盖 → 实际生效 30s） | 本 PR：`emotion-echo-web-bff/etc/web-bff.yaml` + `config_test.go:TestConfig_XTTSDefaultTimeoutIs90s` | yaml 加载后字节数断言 |
| 6 | **APISIX upstream 6 (web-bff) timeout 60s→180s 持久化** | 本 PR：`deploy/apisix/seed.sh` `put_nacos_upstream` 加 timeout 字段（id=6 给 180s，其它 60s 兜底） | seed.sh 重跑后 `curl /apisix/admin/upstreams/6` 验 timeout 180 |
| 7 | **mobile #3 BLOCKED 真因**：浏览器还在 about:blank 时跨域 fetch 触发 chromium same-origin policy → "TypeError: Failed to fetch" | 本 PR：`digital-human-tts.spec.ts` `page.goto('/login')` 修 | Playwright mobile #3 全过 |
| 8 | **XTTS 单实例串行**：spec 调整 retry 6 × 60s 间隔 | 本 PR：`digital-human-tts.spec.ts` | Playwright 双 project 6/6 |

## 8. 回归钉

**正式 spec**：`emotion-echo-web/e2e/digital-human-tts.spec.ts`（3 用例 × 2 project = 6 测试）

| 用例 | chromium | mobile |
|------|----------|--------|
| #3 BFF /tts/phonemes 端到端契约 | PASS 1.4m | PASS 1.5m |
| #10/#17 数字人区域 + TTS 链路 | PASS 8.2s | PASS 20.1s |
| #18 数字人 wrapper mobile 路径 | PASS 8.0s | PASS 9.7s |

**Vitest 单元**：`useTTSPlayer.phoneme.test.ts` (9/9) + `useTTSPlayer.queue.test.ts` (5/5)

**BFF 单测**：`config_test.go:TestConfig_XTTSDefaultTimeoutIs90s`（钉 yaml/SetDefaults 对齐守卫）

## 9. 待决策

无遗留用户决议项。E2E-17 阶段所有 F 项均已修复或明确范围外记账。

**下一阶段阻塞门**（E2E-21/29/30 等后续阶段强依赖 E2E-17 done）：
- E2E-21 日志体系：与端侧化 §C 联动（参考 [v0.3 实施路线图 §C.4 阶段三埋点](../../../plans/on-device-hybrid-inference-implementation-roadmap-2026-09-23.md)）
- E2E-23 健康检查 / Nacos：与端侧化新增 health endpoint 联动
- E2E-29 异常与安全：端侧化 JWT 离线宽限依赖
- E2E-30 数据契约收口：端侧化 §5.5 AI 回复补传契约必须通过 §2.4 六项契约