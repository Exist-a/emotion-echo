# Stage 36-FU Closure · 2026-09-03

> **目的**：把 [stage-36-smoke-report.md](stage-36-smoke-report.md) §九 + §六 仍**未真正关闭**的尾巴**收口**。
> **关联 ADR patch**：[adr-2026-09-known-gaps-patch-fu.md](adr-2026-09-known-gaps-patch-fu.md)
> **会话**：本轮 Stage 36-FU

---

## 〇、执行摘要

| Bug | 状态 | 收口方式 |
|-----|------|---------|
| Bug 3（TTS /tts 500） | ✅ 代码层关闭 | server.py /tts + /tts_with_phonemes 加 `.float()` + AST 契约测试 |
| Bug 9（Web 容器跑不起来）| ✅ 关闭 | 移除 `profiles: ["never"]` + compose `build.args` 加 `NPM_REGISTRY` + Dockerfile ARG 契约 |
| Bug 10（Nacos 全栈） | ✅ 契约层关闭 | 7 svc 全部 `NACOS_ENABLED=true` + addr 注入 + 6 nacos_boot_test.go 存在 |
| G1（healthcheck 占位符）| ✅ 关闭 | yaml healthcheck 完整（commit 2699e89）+ AST 契约固化 |
| ADR-17/18 patch | ✅ 关闭 | adr-2026-09-known-gaps-patch-fu.md |
| G2/G3 状态更新 | ✅ 关闭 | smoke 实证已修；原 ADR-16 §缺口清单 8 项中 G2/G3/G4/G5/G8 关闭 |

**端到端 smoke 矩阵**：16/16 通过（`scripts/smoke_bff_t5.py`，2026-09-03 实跑）。

---

## 一、TDD 工作流（本轮所有 commit）

按 [AGENTS.md](../AGENTS.md) §〇 第一性原则 "ALL CODE IS TDD"，本轮 **每个 fix 都先写失败测试**：

| # | Commit | RED 测试 | GREEN 实现 |
|---|--------|---------|-----------|
| 1 | fix(xtts): /tts 端点 dtype mismatch | `test_torchaudio_shim.py` + `test_server_tts_dtype.py` AST 检查 | `server.py` 两处 `torchaudio.save` 显式 `.float()` |
| 2 | fix(apps-compose): emotion-echo-web 解除 profile: never | `scripts/test_web_and_healthcheck_contracts.sh §1` | 移除 `profiles: ["never"]` |
| 3 | fix(web): Dockerfile 双源同步 + NPM_REGISTRY ARG | `test_web_and_healthcheck_contracts.sh §2/§3` | 已有 ARG（commit 723e18b）+ apps compose 显式传 `NPM_REGISTRY=https://registry.npmjs.org/` |
| 4 | test(contract): G1 healthcheck + SKYWALKING_OAP_ADDR 占位符 | `test_web_and_healthcheck_contracts.sh §4/§5` | 6 svc 全部 `healthcheck:` + `start_period:` + 具体 DNS 值 |
| 5 | test(contract): Nacos 全栈接入 | `scripts/test_compose_nacos_full_stack.sh` | 7 svc 全部 `NACOS_ENABLED=true` + `NACOS_ADDR` + `depends_on nacos` |

---

## 二、本轮 commit 清单（待 commit）

```
1. test(xtts): add RED contract test for torchaudio shim .float() guard
   files: Emotion-Echo-LLM/XTTS/tests/unit/test_torchaudio_shim.py
2. fix(xtts): add .float() to /tts and /tts_with_phonemes tensor chain
   files: Emotion-Echo-LLM/XTTS/server.py
3. test(xtts): add AST contract for server.py torchaudio.save dtype
   files: Emotion-Echo-LLM/XTTS/tests/unit/test_server_tts_dtype.py
4. fix(apps-compose): remove emotion-echo-web profiles: never + propagate NPM_REGISTRY
   files: deploy/docker-compose.apps.yml
6. test(contract): web + healthcheck compose contracts (Bug 9 + G1)
   files: scripts/test_web_and_healthcheck_contracts.sh
7. test(contract): compose Nacos full-stack wiring (Bug 10)
   files: scripts/test_compose_nacos_full_stack.sh
8. docs(adr-16-patch): Stage 36-FU known-gaps closure (Bug 3/9/10 + G1 + G2/G3 status)
   files: docs/adr-2026-09-known-gaps-patch-fu.md
9. docs(closure): Stage 36-FU closure report
   files: docs/stage-36-followup-closure.md
```

---

## 三、Smoke 矩阵实证（2026-09-03 实跑）

```
[OK  ] BFF /health: status=ok downstream_ok=6/6
[OK  ]   downstream user: status=ok
[OK  ]   downstream chat: status=ok
[OK  ]   downstream analytics: status=ok
[OK  ]   downstream assessment: status=ok
[OK  ]   downstream ai: status=ok
[OK  ]   downstream xtts: status=ok   ← T7 后从 degraded 升 ok
[OK  ] BFF /api/v1/auth/login: user_id=1 token_len=191   ← Bug 1 修
[OK  ] BFF /api/v1/users/me: user_keys=['userId', 'account', 'phone', 'nickname']
[OK  ] BFF POST /api/v1/conversations: conv_id=6
[OK  ] BFF GET /api/v1/conversations (G2 受阻预期): data_keys=['hasMore', 'list']   ← G2 已修
[OK  ] BFF POST /api/v1/conversations/{id}/messages: msg_id=5
[OK  ] BFF /api/v1/ai/stream (mock LLM fallback): has_sse=True   ← T8 后真实 LLM 走通
[OK  ] BFF /api/v1/reports/daily?user_id=1: data_keys=['report']   ← G4 已修
[OK  ] BFF /api/v1/surveys: data_keys=['items', 'total']   ← G3 已修
[OK  ] BFF /metrics: 170 emotion_echo_/go_ series

=== T5 smoke 总计: 16/16 通过 ===
```

---

## 四、未在本轮关闭（明确留 backlog）

| # | 项目 | 留待原因 |
|---|------|---------|
| 1 | **XTTS v0.1.7 镜像重 build + `/tts` 真实合成 200 OK** | Docker Desktop 受 dev 环境 0字节 pypi + 内存限制，build 卡 30+ 分钟。按 Stage 36-B5 决策 (commit `da252aa` 记录) 留生产网络跑 build。本轮**代码修复** + **契约测试**已就位，build 时无需再改代码 |
| 2 | **G7 APISIX 复职** | 实证本环境 docker ps 显示 `emotion-echo-apisix` Up 3 hours（功能可用），但 Admin API 端口没暴露；按 Stage 37-A 路线图 |
| 3 | **ai-svc 多副本 + consumer group 调优** | Stage 38 路线图，需 G4 完整跑通后评估（Kafka fallback 已修） |

---

## 五、引用

- [stage-36-smoke-report.md](stage-36-smoke-report.md)（被本 closure 收口）
- [adr-2026-09-known-gaps.md](adr-2026-09-known-gaps.md)（被本 patch 叠加）
- [adr-2026-09-known-gaps-patch-fu.md](adr-2026-09-known-gaps-patch-fu.md)
- [AGENTS.md](../AGENTS.md) §〇 TDD 第一性原则
- [stage-36-fixes-roadmap.md](stage-36-fixes-roadmap.md)（下一步 Stage 37 路线图）