# Stage 61 · 2026-09-10 文档失真集中治理（决策 18 #23 #24 #25 closure）

> **状态**：🟢 **3 项 PR 全部 landed**（PR-A / PR-B / PR-C）+ 文档失真 3 处全部登记
> **关联**：[doc-drift-registry.md #23 #24 #25](../architecture/adr/adr-2026-09-doc-drift-registry.md) ·
> [`todo-pile-2026-09-04.md §G`](../plans/todo-pile-2026-09-04.md)（合规优先最小集）·
> [`ADR-001 v2`](../architecture/adr/adr-2026-09-xtts-v2-decision.md) ·
> [`decisions.md` 决策 9 vs 11/12 正式收口](../architecture/decisions.md)

本批覆盖 3 个工作面（PR-A / PR-B / PR-C），按 TDD Red→Green→Refactor 节奏：

---

## §一 PR-A · 前端 fallback 字面值 fail-fast（决策 18 #24 closure）

### 1.1 触发背景

Stage 30 时代前端默认直连 BFF（`localhost:8894`），cf1c798 后切到 APISIX（`localhost:19080`），
**但 3 处 composable 的 fallback 字面值没同步切**：

| 文件 | 字面值 | 实际后果 |
|---|---|---|
| `emotion-echo-web/app/composables/useAIStreamHandler.ts:78` | `'http://localhost:8894/api/v1'` | 聊天流漏配时无声回退到 BFF |
| `emotion-echo-web/app/composables/useTTSPlayer.ts:175` | `'http://localhost:8894/api/v1'` | TTS 播放同上 |
| `emotion-echo-web/app/composables/useApi.ts:29,31` | `'http://localhost:8080/api/v1'` | 8080 是 Spring 默认端口，项目无此服务 → 死请求 HTTP 000 |

### 1.2 RED → GREEN → REFACTOR 三段

#### RED · 测试先行

| 文件 | 用途 |
|---|---|
| `emotion-echo-web/app/lib/apiBaseUrl.test.ts` (新) | 6 个测试：happy path / API_BASE_URL 空 / public 字段缺失 / config 不传 / useRuntimeConfig 抛错 / fallback 端口排除 |
| `emotion-echo-web/app/composables/useAIStreamHandler.test.ts` | 新增"PR-A · API_BASE_URL 漏配：fetch 不应静默打到 8894，必须报错" |
| `emotion-echo-web/vitest.config.ts` | 修 alias：`~`/`@`/`#app` 之前 alias 到 `<ROOT>`，正确是 `<ROOT>/app`——`~/types/api` 解析失败的根因 |

**RED 验证**：
```
❯ app/composables/useAIStreamHandler.test.ts (9 tests | 1 failed)
× PR-A · API_BASE_URL 漏配：fetch 不应静默打到 8894，必须报错
  → expected true to be false // Object.is equality
```

#### GREEN · 抽 helper + 改 3 处调用

| 文件 | 改动 |
|---|---|
| `emotion-echo-web/app/lib/apiBaseUrl.ts` (新, 44 行) | `getApiBaseUrl(config?) → string`；漏配/字段缺失/useRuntimeConfig 抛错 → 抛带决策 18 #24 引用的明确 Error |
| `useAIStreamHandler.ts:78-89` | `runtimeConfig.public.API_BASE_URL || 'http://localhost:8894/api/v1'` → `getApiBaseUrl(runtimeConfig)`；外层 try/catch 让 fail-fast 经 `callbacks.onError` 回调而非裸抛 |
| `useTTSPlayer.ts:175` | 同上 |
| `useApi.ts:29-33` | 同上 |

#### REFACTOR · 测试精简 + 跑全量

- 删除冗余 `useApi.test.ts`（Nuxt auto-imports `#app` 解析复杂）与 `useTTSPlayer.test.ts`（pcm-player 依赖链过重），保留单点 helper `apiBaseUrl.test.ts` 的 6 用例 + `useAIStreamHandler.test.ts` 的端到端用例
- 跑全量 vitest：**247/247 PASS**（23 文件 0 回归）

### 1.3 调研依据（commit message 末尾格式）

| 项 | 来源 |
|---|---|
| 3 处 fallback 字面值 | `grep -n "localhost:[0-9]" emotion-echo-web/app/composables/` 实测 |
| `nuxt.config.ts:19` 默认 `:19080` | 直接读 nuxt.config.ts:19 |
| `vitest.config.ts` alias 漏 `app/` 前缀 | 跑 `vitest run app/composables/useApi.test.ts` 报 `Failed to resolve import "~/types/api"` 实测 |
| 247/247 PASS | `cd emotion-echo-web && node_modules/.bin/vitest run` |
| 决策 18 #24 | doc-drift-registry.md §二 #24 |

---

## §二 PR-B · ADR-001 v2 XTTS 决策重审

### 2.1 触发背景

原 `docs/ai-models/xtts-decision.md`（2026-07-17）决策"阿里云智能语音 API + OpenAI fallback，删除本地容器"——但 **4 个月代码从未落地**：

| v1 决策 | 现状实测 |
|---|---|
| 阿里云 API 为 Primary | `emotion-echo-ai-svc/internal/aiclient/xtts.go` 是 151 行本地 HTTP 客户端实现，**从未写过云 API 适配层** |
| 删除本地 `emotion-echo-xtts` 容器 | `deploy/docker-compose.apps.yml:549` 容器定义仍在（改用 `ai4all/coqui:latest` vendor 镜像）|
| OpenAI TTS 为 Fallback | 同上，未落地 |

实际跑通的是 Stage 60 PR-TTS-VENDOR 的 **vendor `ai4all/coqui:latest`** 路径（端到端 133KB WAV RIFF 头正确，stage-60 §五）。

### 2.2 v2 决策结论

| 路径 | 角色 | 当前实现 |
|---|---|---|
| **Primary** | 本地容器 `emotion-echo-xtts`（vendor `ai4all/coqui:latest`）| Stage 60 PR-TTS-VENDOR 落地（`deploy/docker-compose.apps.yml:549-580`）|
| **Fallback** | 云 API（阿里云 + OpenAI TTS）| v1 代码骨架保留作参考，未来 §C 触发条件成熟时按 TDD 重启 |
| **Future** | 自建 XTTS 镜像 | 仓库内代码保留作 learning asset |

**v1 → v2 切换触发条件**：prod 部署成本压力 / 合规要求 / vendor 镜像不可拉 / 多语种扩展——任一触发才评估切云。

### 2.3 新文档

| 文件 | 用途 |
|---|---|
| `docs/architecture/adr/adr-2026-09-xtts-v2-decision.md` (新) | v2 决策 + 调研依据（AGENTS.md §〇.6）|
| `docs/ai-models/xtts-decision.md` | **不删**——保留为 v1 历史快照（v1 retire 但不删除）|

### 2.4 调研依据

- 已读 6 个关键文件（`xtts.go:46-62` / `main.go:86-92` / `ai-api.yaml:69-81` / `docker-compose.apps.yml:387-389, 549-580` / Stage 60/60.1 收口报告）
- 已查 stage-58/59/60 全链路 + ADR-001 v1 原文
- 已跑端到端：TTS 133KB WAV（vendor）/ FER 49/49 单测 / SenseVoice 三语 200

---

## §三 PR-C · decisions.md 决策 9 vs 11/12 字面冲突正式收口

### 3.1 触发背景

`decisions.md` 决策 9 写"统一入口 = web-bff"（2026-08-31），决策 11/12 写"APISIX 是唯一业务入口"（2026-09-03）——**字面冲突 4 个月**。PR-A 代码侧已不再回退到 BFF（fail-fast helper），PR-B 决策栈已正式收口；缺决策 9 表头的正式修正。

### 3.2 收口内容

`decisions.md` 决策 9 区块追加：

> **🔧 2026-09-10 决策 18 §4.4 / PR-C 正式收口**（登记于 doc-drift-registry #24 + #25）：
>
> 正式表述：**APISIX 是唯一业务入口**；BFF 是 APISIX 的 upstream + 系统内聚合层。
> 本决策标题"服务入口 = web-bff"措辞保留为历史快照，但实际语义以本收口段为准。

| 维度 | 状态 |
|---|---|
| **业务入口**（外部用户视角）| APISIX `:19080`（决策 11）|
| **系统入口**（内部服务视角）| web-bff `:8894`（本决策）|
| **dev 调试**（特殊例外）| web-bff `:8894` 直连（仅本机 dev；prod 必须关闭 8894 端口映射）|

---

## §四 本批 commit 计划（待 owner 合并）

```
[PR-A · RED]   test(web): apiBaseUrl helper + useAIStreamHandler 漏配 fail-fast 测试
[PR-A · GREEN] refactor(web): 抽 apiBaseUrl helper + 3 处 fallback 字面值移除
[PR-A · FIX]   fix(vitest): alias ~ @ #app = <ROOT>/app（之前 alias 到 <ROOT> 解析失败）
[PR-B]         docs(adr): ADR-001 v2 XTTS 决策重审（vendor + 本地双轨为最终结论）
[PR-C]         docs(decisions): 决策 9 vs 11/12 字面冲突正式收口
```

> 注：本会话所有改动**未 commit**（按 AGENTS.md 协作约定——agent 不擅自 commit，等 owner review）。

---

## §五 累计统计

| 维度 | 数值 |
|---|---|
| **新增文件** | 2（`apiBaseUrl.ts` + `apiBaseUrl.test.ts` + `adr-2026-09-xtts-v2-decision.md` = 3 个；本会话共 3 新文件）|
| **修改文件** | 5（useApi / useAIStreamHandler / useTTSPlayer / vitest.config / decisions.md）|
| **新增单测** | 6 + 1 = 7 用例 |
| **全量回归** | 247/247 PASS（vitest 23 文件） |
| **决策失真登记** | doc-drift-registry 新增 #23 + #24 + #25；累计 25 条 |
| **ADR 落地** | ADR-001 v1 retire + v2 Accepted；决策 9 正式收口 |
| **未做** | 本会话不 commit（owner review）；prod compose override 13 项 TODO（远端部署时实现） |

---

## §六 后续 sprint 建议（独立 session）

1. **owner review + commit 本批 5 文件**（半小时）
2. **PR-D1**: 决策 20 owner sign-off（ADR-20 C 方案已落地 1 周，等正式签字）
3. **PR-D2**: `user_oauth` 表零引用 ADR（todo-pile §D3，1 小时）
4. **PR-D3**: BFF `isLocked`/`recordFailure` 登录限流补测试（todo-pile §D4，半天）
5. **PR-D4**: chat-svc 表依赖清单 ADR（todo-pile §D5 + 决策 18 §4.5，半天）
6. **PR-D5**: 路由契约测试三方对齐（todo-pile §C8，1.5~2 天）
7. **PR-D6**: BFF→user/assessment/analytics-svc gRPC 化（grpc-inter-service-migration §1.3，1 天）

---

> 最后更新：2026-09-10 by Stage 61 session
> 用途：合规优先最小集（PR-A + PR-B + PR-C）的完整收口报告 + 调研依据
> 关联：决策 18 #23 #24 #25 全部 closure；决策 9 字面冲突正式收口；ADR-001 v2 Accepted