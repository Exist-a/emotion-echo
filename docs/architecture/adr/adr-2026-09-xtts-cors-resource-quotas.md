# ADR-2026-09: XTTS 容器 CPU 限额 + APISIX CORS Origin 持久化

## Status

✅ **Accepted**（2026-09-24，E2E-17 收口实证落地）

## Context

E2E-17 阶段用户实测「嘴动没声音」+ 全站 `/api/v1/*` 503 共触发三个互相耦合的根因，全部与基础设施配置有关：

1. **XTTS 容器 CPU 限额 2 核 vs torch 8 线程超订 4 倍**（F-132）：
   `deploy/docker-compose.apps.yml` XTTS 段 `cpus: "2.0"`，容器内 `torch.get_num_threads()=8` ⇒
   40 字 /tts_with_phonemes 实测 **188s**（超 BFF/APISIX 180s 上限 → 504/502 → 前端无音频）。
   流式 10 字首字节 21.2s。
2. **XTTS `TimeoutMs: 30000` 与 config.go SetDefaults 90000 → 180000 漂移**（F-138）：
   yaml 不为 0 时 SetDefaults 不覆盖 → 实际生效 30s → BFF→XTTS client.Timeout 30s 撞底 → 502。
3. **`CORS_ALLOW_ORIGINS` 默认值漏 `http://127.0.0.1:3000`**（F-139）：
   用 `127.0.0.1:3000` 打开的前端对 APISIX 所有 `/api/v1/*` 的 preflight **返回 200 但零 CORS 响应头** ⇒
   浏览器 `Failed to fetch`（整页 API 不可用）。且 **`apisix-seed` 每次重跑会按该 env 覆盖 APISIX admin
   手工改动**（本轮实证：手工 PUT 加 `:3001` 后 `compose up` 触发 seed，白名单退回单条）。

## Decision

### 1. XTTS 容器 CPU 限额 → 8 核（F-132 修复）

**修法**：compose XTTS 段 `cpus: "2.0"` → `"8.0"`，memory 保留 `6144M`。

**钉守卫**：`scripts/check_xtts_cpu_limit.sh`（RED 2.0→FAIL / GREEN 8.0→PASS）。

### 2. XTTS TimeoutMs 校齐 + 测试钉守卫（F-138 修复）

**修法**：
- `emotion-echo-web-bff/etc/web-bff.yaml` `XTTS.TimeoutMs: 90000` → `180000`
- `emotion-echo-web-bff/internal/config/config.go` SetDefaults 同步 90000 → **180000**
  （必须同时改；yaml 非 0 时 SetDefaults 不覆盖）
- `emotion-echo-web-bff/internal/config/config_test.go` 新增
  `TestConfig_XTTSDefaultTimeoutIs180s` 字面量断言

### 3. APISIX CORS origin 白名单 → 四 origin（F-139 修复）

**修法**：compose apisix-seed env
`CORS_ALLOW_ORIGINS: "${CORS_ALLOW_ORIGINS:-http://localhost:3000,http://127.0.0.1:3000,http://localhost:3001,http://127.0.0.1:3001}"`
（`:3001` 供宿主机 `pnpm dev --port 3001` 本地源码调试）。

**持久化保证**：seed.sh 是 APISIX 配置的真源；任何手工 admin 改动必须在下次
`docker compose up` 后**仍能保留**——目前靠 compose env 注入实现，**手工 admin PUT
与 seed 的覆盖关系治理属 E2E-25 APISIX 范畴**（参见账本 F-139 / F-137 迁出声明）。

## Consequences

**正面**：
- IAB 实测全链路绿：8 核下 40 字 19.9s（**9.5x** 提升），端到端 200/27.6s/RIFF WAV 323116B
- APISIX preflight 六头齐全（`Access-Control-Allow-Origin / Allow-Methods / Allow-Headers
  / Allow-Credentials / Max-Age / Expose-Headers`）
- CORS origin 涵盖宿主机所有访问方式（`localhost` / `127.0.0.1` × `:3000` / `:3001`）

**负面 / 留账**：
- 仍需 E2E-25 治本：APISIX upstream healthcheck（自动剔除不健康节点）+ seed ↔ admin
  diff/锁机制（防止手工 admin 被下次 seed 覆盖）+ BFF 注册 Nacos 失败 fail-fast / 重试治本
  （账本 F-137 / F-139 已迁出 E2E-17，归属 E2E-25）
- XTTS 仍为单 worker 串行（账本 F-136 留账 E2E-18+，dev 缓解靠 F-132 已把单请求降到 19.9s）

## 实证

| 项 | 修前 | 修后 | 倍 |
|---|---|---|---|
| 40 字同步合成 | 188s（撞 180s 上限 504）| 19.9s | **9.5x** |
| 10 字流式首字节 | 21.2s | 4.0s | 5.3x |
| 端到端 40 字 | 504 / 无音频 | 200 / 27.6s / RIFF WAV 323116B | ✓ |
| APISIX 100 preflight CORS 头 | 0 | 6 | ✓ |
| 钉守卫 scripts/check_xtts_cpu_limit.sh | 不存在 | PASS（2.0→FAIL / 8.0→PASS）| ✓ |

## 调研依据

- [x] emotion-echo-models/XTTS/server.py uvicorn 单 worker（F-136 留账）
- [x] emotion-echo-web-bff/internal/config/config_test.go 已有
      TestConfig_XTTSDefaultTimeoutIs180s 钉 yaml 默认值
- [x] deploy/apisix/seed.sh:99-104（Stage 105 注释）+ services.env.example:30
- [x] deploy/apisix/seed.sh:387/457/559（CORS_ALLOW_ORIGINS 注入点）
- [x] 实测 preflight 响应头（修前零 CORS 头 / 修后六头）
- [x] APISIX admin routes/100 实际 allow_origins 值（修前单条/修后四条）
- [x] IAB fetch 探针 6 条请求链路（含 tts/phonemes 200 27.6s）
- [x] 容器镜像版本对照（web:v0.1.7 / web-bff:v0.1.30 已部署）

## 关联

- E2E-F-132 / E2E-F-138 / E2E-F-139 / E2E-F-137（已迁出 E2E-17 → 落账 E2E-25）
- ADR D-32（APISIX upstream 6 timeout 180s，已立，2026-09-23）
- PR #77（E2E-17 收口主 PR，6 commits：e425474 / 096d50f / f612ec5 / ff9c22c / a2bc4f0 / cb41246）