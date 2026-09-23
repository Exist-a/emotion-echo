# ADR-2026-09 · APISIX upstream timeout = 180s（覆盖 web-bff phonemes cold path）

> **决策日期**：2026-09-23（E2E-17 step 5 收口）
> **决策者**：user（用户决议）+ ZCode Bot（E2E-F-127 yaml 漂移真因诊断）
> **状态**：✅ **Accepted**（实施中，commit 2c45265 待合 main）

## 一、上下文

E2E-17 step 5 收口期间双 project 跑 Playwright 时 mobile #3 持续 502：
- BFF 日志报 `xtts phonemes: ... context deadline exceeded (Client.Timeout exceeded while awaiting headers)`
- 上轮 v0.1.28 commit `c7ff203` 已修 config.go `SetDefaults` 默认 90000ms，但 yaml `TimeoutMs: 30000` 不为 0 时 SetDefaults 不覆盖 → **实际生效 30s**（真 bug E2E-F-127）
- 修 yaml 后实测：BFF→XTTS 等到 90s（90.011s 502）—— **APISIX upstream 默认 60s read/send timeout 先 504 触发**

## 二、决策

| 维度 | 选择 |
|------|------|
| APISIX upstream 6 (web-bff) timeout | **`send: 180, read: 180, connect: 10`**（覆盖 phonemes cold path 100s+ + 长字符 LLM stream） |
| 其它 nacos upstream (1~5) timeout | 默认 60s 兜底（grpc 默认 5-30s 远低于此） |
| 配置位置 | `deploy/apisix/seed.sh` `put_nacos_upstream` 函数（id=6 case 单独给 180） |

## 三、影响面

- **APISIX 配置**：动态 PUT upstream 6 立即生效（已实操）+ seed.sh 持久化（重启 APISIX 不丢）
- **BFF yaml**：30000→90000（`emotion-echo-web-bff/etc/web-bff.yaml`）+ config_test.go 钉守卫
- **docker compose**：BFF image tag v0.1.28→v0.1.29（防 v0.1.28 mirror 内置的旧 yaml 污染；实际 bind-mount 改 yaml 即时生效，**v0.1.29 mirror 重建从未成功（docker daemon RPC 错误），bind-mount + docker restart 是替代快路径**）

## 四、调研依据（继承 plan §7 + commit memory）

- `git log -p emotion-echo-web-bff/etc/web-bff.yaml` → `TimeoutMs: 30000` 历次改动从未同步
- BFF config.go:154-165 `SetDefaults` 仅在 `c.XTTS.TimeoutMs == 0` 时覆盖
- docker exec curl XTTS 直测 cold 29s warm 14s（仓内 11.3 GB 镜像 CPU 推理）
- APISIX admin /apisix/admin/upstreams/6 GET timeout=60 修前实测
- E2E-F-127 yaml/config.go 漂移 = 上轮 STATUS.md 完全漏诊断的真 bug

## 五、风险（继承 v0.2 §九 + 增量）

| # | 风险 | 应对 |
|---|------|------|
| 1 | APISIX upstream 180s timeout 让长 LLM chat 流式 / SSE 流式也宽限到 3 分钟 | 这是设计目的（chat 流式覆盖 F-115 同型），不是风险 |
| 3 | seed.sh PUT 失败时 upstream 6 维持旧 60s → 撞底 | 已加 PUT 失败回退脚本日志 + 启动时 health probe；D-09 同型教训 |
| 4 | docker daemon 资源紧张时 `docker compose build` 撞 RPC error（实测） | bind-mount 改 yaml + docker restart 是分钟级快路径；build 失败不再卡流程 |

## 六、测试

- `pnpm exec playwright test e2e/digital-human-tts.spec.ts` 双 project **6/6 PASS**（3.8m）
- `python scripts/e2e_stage_audit.py --stage e2e-17` 0 FAIL
- `bash scripts/check_adr_gate.sh` GREEN（本 ADR 注册后）
- `go test ./emotion-echo-web-bff/internal/...` 全绿（含 config_test.go 新增 `TestConfig_XTTSDefaultTimeoutIs90s` 钉 yaml/SetDefaults 对齐守卫）

## 七、决策登记

`docs/architecture/decisions.md` 决策 32（v0.3 实施路线图 §B.2 模板）。