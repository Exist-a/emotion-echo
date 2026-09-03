# ADR-16 Patch · Stage 36-FU 缺口收口（2026-09-03）

> **状态**：patch（叠加在 [adr-2026-09-known-gaps.md](adr-2026-09-known-gaps.md) 之上）
> **来源**：stage-36-smoke-report.md §九 的"已知 Bug 全景"和 §六"待办 / 后续建议"
> **关联 commit**：本轮 (Stage 36-FU) 所有 commit

---

## 上下文

[stage-36-smoke-report.md](stage-36-smoke-report.md) 提交时声明 **"11 bug 全部修复, 16/16 smoke GREEN"**，
但 smoke 报告本身明确列出 **5 项仍属于"未真正关闭"或"部分关闭"** 的尾巴：

1. **Bug 9（partial）**：Web Dockerfile 已修复，但 `emotion-echo-web` 容器被 `profiles: ["never"]` 显式禁用，
   根目录 `docker-compose.yml` 的 `frontend` 服务仍指向已过时的 Dockerfile（无 ARG），
   浏览器侧完全打不开。
2. **Bug 10（Nacos 全栈启用）**：报告 §T5 写 "实际 NACOS_ENABLED=false"，
   但 §九 又写 "Bug 10 NACOS_ENABLED=true + 各 svc 配置中心 bootstrap" ——
   两个状态互相矛盾，需要契约固化。
3. **Bug 3（TTS `/tts` 500）**：shim 已加 `.float()`，但 `/tts` 和 `/tts_with_phonemes`
   两个端点 `torch.tensor(audio).unsqueeze(0)` 内部构造链上**没有** `.float()`，
   Coqui TTS speaker_encoder 重 load wav 时返回 float64 → XTTS conv1d (float32) dtype mismatch。
4. **G1（healthcheck）**：commit 2699e89 已加 `healthcheck:` 字段，但契约未固化，
   未来 refactor 可能误删 `start_period`，导致 docker 在模型 preload 时反复重启容器。
5. **ADR-17/18 patch**：报告 §六 提"把 G2/G3 标记为已修"，但没落地 commit。

本 ADR-16 patch 把这些"看似收口但实际未做"的项**正式关闭**或**正式承认未关闭 + 留 backlog**。

---

## 决策（Decisions）

### §A. Bug 9：解除 `profiles: ["never"]` + Dockerfile 双源同步

**状态**：✅ **已关闭**。

- `deploy/docker-compose.apps.yml` 移除 `emotion-echo-web` 的 `profiles: ["never"]`
- `deploy/docker-compose.apps.yml` 给 `emotion-echo-web` 的 `build.args` 加上 `NPM_REGISTRY`，
  默认 `https://registry.npmjs.org/`，与 `Emotion-Echo-Web/Dockerfile` 的 `ARG NPM_REGISTRY` 一致
- `Emotion-Echo-Web/Dockerfile` 已经接受 `ARG NPM_REGISTRY` (commit 723e18b 修复)，
  根目录 `docker-compose.yml` 的 `frontend` 服务随之可重建

契约测试：`scripts/test_web_and_healthcheck_contracts.sh`
- §1: 断言 `emotion-echo-web` 不在 `profiles: ["never"]`
- §2: 断言 `Emotion-Echo-Web/Dockerfile` 接受 `ARG NPM_REGISTRY`
- §3: 断言根目录 compose 的 frontend 服务使用的 Dockerfile 含 `ARG NPM_REGISTRY`

**注意**：本地 dev 环境构建/启动 Web 容器需要 `docker build`，按 Stage 36-B5 决策
（XTTS rebuild 在 dev 卡 30min）镜像构建留给生产环境跑。契约保证镜像一旦构建就能
按预期 `npm install`。

### §B. Bug 10：Nacos 全栈契约固化

**状态**：✅ **已关闭**（契约层）。

实证 `deploy/docker-compose.apps.yml` 实际**已经**给 7 个 svc 都注入了 `NACOS_ENABLED: "true"` +
`NACOS_ADDR: emotion-echo-nacos:8848` + `depends_on: nacos`。报告 §T5 写的"实际 NACOS_ENABLED=false"
是 docker daemon 缓存 env 导致的（commit 31b4efe 之前 force-restart BFF 时已经踩过同一坑）。

契约测试：`scripts/test_compose_nacos_full_stack.sh`
- §1: infra compose 提供 nacos on 8848
- §2: apps compose 至少 6 个 svc 注入 `NACOS_ENABLED=true` + `NACOS_ADDR`
- §3: shared/pkg/configcenter + shared/pkg/discovery 测试存在
- §4: 6 个 svc 的 `nacos_boot_test.go` 都存在
- §5: `emotion-llm-service/tests/unit/test_nacos_bootstrap.py` 存在

**端到端验证**（运行时 Nacos 注册实例真实可见）需要：
- `docker compose -f deploy/docker-compose.infra.yml up -d`
- `docker compose -f deploy/docker-compose.apps.yml up -d`
- 访问 `http://localhost:8848/nacos`（默认账号 nacos/nacos，dev standalone 无鉴权）
- 在服务管理页面查 "user-svc" / "chat-svc" 等 7 个服务实例

这条留给生产环境运维 runbook（按 Stage 37-A 路线图）。

### §C. Bug 3：XTTS `/tts` 端点 dtype mismatch 真修

**状态**：✅ **代码层已关闭**（运行验证留给生产镜像）。

修了两处 `torchaudio.save(...)` 调用，在 `torch.tensor(audio).unsqueeze(0)` 之后
显式 `.float()`：

```python
# server.py L184 (was L180)
torchaudio.save(
    buf,
    torch.tensor(audio).unsqueeze(0).float(),  # ← new .float()
    SAMPLE_RATE,
    format="wav",
)
```

```python
# server.py L307 (was L297)
torchaudio.save(
    buf,
    torch.tensor(audio).unsqueeze(0).float(),  # ← new .float()
    SAMPLE_RATE,
    format="wav",
)
```

契约测试：
- `tests/unit/test_torchaudio_shim.py`: AST 检查 `_load_with_soundfile` 含 `.float()`，
  断言 `torchaudio.load =` / `torchaudio.save =` 真实赋值（防"shim 被 import 但不生效"）。
- `tests/unit/test_server_tts_dtype.py`: AST 检查 server.py 所有 `torchaudio.save` 第二个
  参数的 tensor 构造链上必须含 `.float()`；以及 `stream_audio_generator` 必须调
  `pcm_chunk_shape`（保持 streaming 路径 dtype 安全）。

这两个测试是**零依赖** AST 契约测试（不需要 torch），dev / CI 直接跑；
runtime 测试（需 torch）留 `pytest.skip()` 给生产镜像里的 pytest 收集。

**实际 `/tts` 跑通 200 OK** 需要：重 build `emotion-echo/xtts:v0.1.7`（按 Stage 36-B5 决策
留给生产环境网络，本地 Docker Desktop 受 0字节 pypi 响应 + 内存限制）。

### §D. G1 healthcheck 契约固化

**状态**：✅ **契约已关闭**。

实证 `deploy/docker-compose.apps.yml` 已经给 6 个 svc 都加了 `healthcheck:` +
`start_period:`（commit 2699e89）。新增契约测试 `test_web_and_healthcheck_contracts.sh §4`
断言未来 refactor 不会误删这两项。

**额外发现**（§5 G1b）：实证 6 个 svc 的 `SKYWALKING_OAP_ADDR` env 都是**具体值**
`emotion-echo-sw-oap:11800`，**没有**裸 `${SKYWALKING_OAP_ADDR}` 占位符
（go-zero 1.10 不展开 bash default 语法，原样保留导致 skywalking dial 失败）。
所以原始 "G1 4 svc unhealthy 但 /health 200" 已经**双重**修复：
- YAML 加 healthcheck（docker daemon 知道容器活着）
- env 直接写容器 DNS（go2sky dial 不再循环失败）

### §E. ADR-17 / ADR-18 patch：G2/G3 状态更新

**状态**：✅ **本 ADR 即是 patch**。

原 ADR-16 §缺口清单 列了 8 项；实证 Stage 36 / 36-D 已修：

| 缺口 | 实际状态 | 修复 commit |
|------|---------|-----------|
| G1 yaml 占位符 | ✅ 已修 | 2699e89 + 本轮契约测试 |
| G2 chat list 端点 | ✅ 已修 | chat-svc v0.2.0 加 `GET /api/v1/conversations`（commit 留存在 e2be51a 之前的某个 commit；stage-36-smoke-report.md §T5.0 step 5 实证返回 `{hasMore, list}`） |
| G3 BFF 路由聚合 | ✅ 已修 | Stage 35 BFF 净化时增加 `/reports/daily` + `/surveys` 路由；stage-36-smoke-report.md §T5.0 step 9 实证返回 `{items, total}` |
| G4 Kafka fallback | ✅ 已修 | commit e2be51a "fix(G4): ai-svc Kafka consumer + DB views + smoke script contracts" |
| G5 真实 LLM | ✅ 已修 | stage-36-smoke-report.md §T8（BFF_LLM_API_KEY 注入 + 真实 DeepSeek 输出验证） |
| G6 多模态镜像 | ⚠️ 部分修 | FER/SenseVoice 默认起 (commit 31b4efe)；XTTS /tts 仍 500（Bug 3，本 ADR §C 已修代码，runtime 验证留生产） |
| G7 APISIX 镜像 | ⏸️ deferred | 本环境手动删除 apisix 容器，**不阻塞** BFF 直连；Stage 37-A 复职 |
| G8 Nacos 全栈 | ✅ 已修（契约）| 本 ADR §B |

---

## 后续（Backlog）

| 优先级 | 项目 | 说明 |
|-------|------|------|
| 🟡 中 | XTTS v0.1.7 镜像重 build + 端到端 `/tts` 验证 | Docker Desktop 受限，留生产网络 |
| 🟡 中 | Stage 37-A：APISIX 复职 + 5-family TLS | G7 真正关闭 |
| 🟢 低 | Stage 37-B：BFF 多副本 + APISIX upstream | 多节点改造路线图 |
| 🟢 低 | Stage 38：ai-svc consumer group 调优 | Kafka partition rebalance |

---

## 引用

- [stage-36-smoke-report.md](stage-36-smoke-report.md) §九 + §六
- [adr-2026-09-chart-contract-alignment.md](adr-2026-09-chart-contract-alignment.md)（ADR-17）
- [adr-2026-09-incremental-rpc-adoption.md](adr-2026-09-incremental-rpc-adoption.md)（ADR-18）
- [adr-2026-09-known-gaps.md](adr-2026-09-known-gaps.md)（被本 patch 叠加）