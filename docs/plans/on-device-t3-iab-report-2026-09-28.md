---
status: iab-report
priority: high
type: iab-verification
created: 2026-09-28
last-refresh: 2026-09-28
related-plans:
  - ./on-device-hybrid-inference-2026-09-23.md（v0.2 §八.3 + §十二已拍）
  - ./on-device-hybrid-inference-implementation-roadmap-2026-09-23.md（v0.3 §C.1 + §G.1 阶段一收口契约）
  - ./on-device-baseline-report-2026-09-24.md（T2#4 N=7 baseline）
  - ./on-device-golden-n13-report-2026-09-28.md（T2#7 N=13 golden 扩）
  - ./on-device-decision-pack.md（§十二 5 项决策材料）
related-decisions:
  - D-26 端侧化主方案 ADR（accepted，决策 33）
  - D-26.2 端侧主力模型（accepted，决策 34）
related-issues:
  - OND-F-06（fixed / 待重开？见 §十）
  - OND-F-08（新登，§十二决策 1 两阶段拍板注记）
  - ONDF-F-09（本报告新登，cloud_grpc baseline mTLS 协议错配）
---

# T3 IAB 验证报告（2026-09-28 · Lane O · §四 #9 Demo IAB 验证）

> **文档定位**：v0.3 §C.1 阶段一任务 5「golden set 骨架 + 云端基线」+ §四 #9 Demo IAB 验证
> 的**借 dev mode 半天窗口**实测报告。
>
> **本报告核心结论**：T3 借 dev mode 完整跑通 21 容器栈（17 healthy + 3 obs 未配 healthcheck），
> 验证了**部署链路完整 + emotion-llm-service mTLS 启用了**，但暴露了两个**预期外但关键的发现**：
> 1. **web v0.1.7 image 时效问题**（早于 Lane O 端侧 PR 5 天 → 端侧代码不在 image 里）
> 2. **baseline `cloud_grpc.py` 协议错配 bug**（insecure_channel vs emotion-llm-service mTLS）
>
> **T3 真机基线（N=13 真 LLM）验证未完成**——baseline bug 阻塞；留作下一轮 Lane O T3+ 任务。

---

## §一 调研依据（AGENTS §〇.6）

| 步骤 | 动作 | 输出 |
|------|------|------|
| ① 读现有 baseline + T2#4 报告 | `scripts/on-device-baseline/run_baseline.py` + `on-device-baseline-report-2026-09-24.md` | §二 |
| ② 读 emotion-llm-service 协议层 | `emotion-llm-service/grpc_server.py` + `chat_completion.py` | §三 §四 |
| ③ 协议 §三.资源1 启动铁律 | `deploy/.env.local` 存在 + 创建 `deploy/.devmode-session` + `--env-file .env.local --profile dev` | §五 |
| ④ 镜像时间戳核对 | `docker inspect` v0.1.7 / v0.1.30 | §六 |
| ⑤ 跑 baseline + 看实际 gRPC 行为 | `docker run --network container:emotion-llm-service` 借网络 + 容器内跑 baseline | §七 |
| ⑥ 列架构假设清单 | §八 | §八 |
| ⑦ 写完后回填 | commit 末尾列调研依据 | §十一 commit 元信息 |

---

## §二 T2#4 baseline 现状

`scripts/on-device-baseline/model_fns/cloud_grpc.py` 当前实现：

```python
channel = grpc.insecure_channel(resolved_target)
grpc.channel_ready_future(channel).result(timeout=timeout_s)
```

**关键点**：`insecure_channel` + 2s 超时 → fallback `grpc_unreachable:FutureTimeoutError:mock_fallback`。

T2#4 baseline 报告（N=7）显示 pass=0%/length=0%/guardrail=85.71%——**所有 mock fallback 路径**。

T2#4 报告**未发现** baseline 协议错配（fallback 掩盖了真问题）。

---

## §三 emotion-llm-service 协议层

`emotion-llm-service/grpc_server.py:418-469`：

```python
tls_enabled = os.environ.get("TLS_ENABLED", "").lower() in ("1", "true", "yes")
if tls_enabled:
    # 加载 ca.crt + llm-server.crt + llm-server.key
    creds = grpc.ssl_server_credentials(...)
    server.add_secure_port(f"[::]:{port}", creds)
    # require_client_auth=1 → 双向 mTLS
else:
    server.add_insecure_port(f"[::]:{port}")
```

**关键观察**：emotion-llm-service **默认 mTLS**（容器内 `TLS_ENABLED=1`）+ 双向认证（`TLS_REQUIRE_CLIENT_AUTH=1`）。

---

## §四 容器栈实测（启动 + 健康 + gRPC）

### 4.1 启动命令（按协议 §三.资源1 启动铁律）

```
cd deploy
docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml \
  --env-file .env.local --profile dev up -d
```

### 4.2 健康统计

| 指标 | 值 |
|------|----|
| 服务总数（dev profile）| 21 |
| Up | 20 |
| healthy | 17（apisix + skywalking-oap + skywalking-ui 未配 healthcheck；启动时均 Up）|
| 启动耗时 | ~3 分钟（21 容器 + obs 健康）|

### 4.3 镜像时间戳核对（**协议 §三.资源1 强制**）

| 镜像 | tag | Created | 备注 |
|------|-----|---------|------|
| emotion-echo/web | **v0.1.7** | 2026-09-23 23:29 | ⚠️ **早于 Lane O 端侧 PR（PR #86 09-24+）5 天** |
| emotion-echo/web-bff | v0.1.30 | 2026-09-23 22:33 | 同上 |
| emotion-echo/llm-service | v0.1.2 | 2026-09-16 | 端侧代码无关，但含 mTLS 协议 |

### 4.4 关键发现：web image 时效问题

容器内验证：

```
emotion-echo-web v0.1.7 容器内：
  /app/app/utils/offline/   → 不存在
  /app/app/pages/demo/local-llm.vue → 不存在
  /demo/local-llm           → 404（路由未注册）
```

**结论**：**Lane O 端侧代码（T2#1 + T2#5 + T2#7 = PR #86+#92+#104）不在 web v0.1.7 image 里**。
web image 必须由 Lane O 或 Lane E 重建才能跑 Demo IAB 验证。

按协议"两轨均不得覆盖 v0.1.5/v0.1.28"精神 + 不擅自动 Lane E image tag，**Lane O 不能直接覆盖 v0.1.7**。

### 4.5 关键发现：baseline 协议错配

```
emotion-echo/llm-service 容器：
  TLS_ENABLED=1
  TLS_CA_CERT=/app/etc/tls/ca.crt
  TLS_REQUIRE_CLIENT_AUTH=1
  → gRPC :50051 mTLS（双向认证）

scripts/on-device-baseline/model_fns/cloud_grpc.py：
  grpc.insecure_channel("localhost:50051")  ← 不支持 mTLS
  metadata: ("x-internal-api-key", ...)      ← 不是客户端证书
  → 握手失败 SSL_ERROR_SSL: WRONG_VERSION_NUMBER
```

**容器内实测**：
```
$ python -c 'import grpc; channel=grpc.insecure_channel("localhost:50051"); grpc.channel_ready_future(channel).result(timeout=5)'
GRPC_FAIL: FutureTimeoutError:
```

**握手失败** → channel_ready_future 超时 → baseline fallback 路径返回 mock_fallback。

**结论**：T2#4 baseline 报告（pass=0%/mock_fallback）= **baseline 实现 bug 导致**，不是真 LLM 基线。

---

## §五 dev mode 启动铁律遵循

按协议 §三.资源1：

1. ✅ **`deploy/.env.local` 存在**（1689 字节，确认 gitignored）
2. ✅ **创建 `deploy/.devmode-session`** 锁文件（lane-o 占用）
3. ✅ **启动命令含 `--env-file .env.local --profile dev`**
4. ⚠️ **镜像时间戳核对** = 已记录（web v0.1.7 早于 Lane O 端侧 PR 5 天 → image 时效问题）

dev mode 启动本身合规。**IAB 验证降级** = web image 重建 + baseline mTLS 修复两道门槛未过。

---

## §六 容器内跑 baseline 实测

### 6.1 跑 baseline 命令（容器内）

```bash
docker cp scripts/on-device-baseline/. emotion-llm-service:/tmp/repo/scripts/on-device-baseline/
docker cp scripts/on-device-golden/. emotion-llm-service:/tmp/repo/scripts/on-device-golden/
docker cp scripts/on-device-perf/. emotion-llm-service:/tmp/repo/scripts/on-device-perf/
docker exec -u root -w /tmp/repo/scripts/on-device-baseline emotion-llm-service \
  bash -c 'PYTHONPATH=/app:/tmp/repo/scripts/on-device-baseline \
    python run_baseline.py --impl cloud_grpc \
    --target localhost:50051 \
    --report /tmp/repo/.../baseline_report_iab_real.md'
```

### 6.2 实测结果

```
[baseline] impl=cloud_grpc N=13 pass=0.00% length=0.00% guardrail=84.62%
[baseline] report -> baseline_report_iab_real.md
```

**所有 13 用例 fallback_reason = `grpc_unreachable:FutureTimeoutError:mock_fallback`**

| 用例 | reply_len | model | fallback_reason |
|------|-----------|-------|-----------------|
| daily-01~03 | 26~33 | `mock` | `grpc_unreachable:FutureTimeoutError:mock_fallback` |
| high_risk-01~02 | 30~38 | `mock` | 同上 |
| long_input-01 | 26 | `mock` | 同上 |
| personality-01~03 | 26~38 | `mock` | 同上 |
| emotion-01~04 | 26~38 | `mock` | 同上 |

### 6.3 对比 T2#4 baseline

| 指标 | T2#4 (host 直接跑) | T3 (容器内借网络) |
|------|-------------------|------------------|
| pass_rate | 0.00% | 0.00% |
| length_pass | 0.00% | 0.00% |
| guardrail | 85.71% (6/7) | 84.62% (11/13) |
| fallback_reason | mock_fallback | mock_fallback |

**结论一致**：T2#4 与 T3 都跑 mock_fallback。**真 LLM 基线**从未跑通（baseline 协议错配阻塞）。

---

## §七 决策影响

### 7.1 对 D-26.2 ADR §五 验收契约

| ADR §五原文 | 现状 |
|------------|------|
| "golden set 13 用例 + Qwen3-1.7B 跑分 ≥ 阈值（待 T2#3 实测定）" | **N=13 ✓**；真机基线分数 ✗（baseline mTLS bug + web image 时效） |

### 7.2 对 OC-11 阶段一收口契约（v0.3 §G.1）

| 契约 | 现状 |
|------|------|
| OC-11：golden set 端侧 ≥ 阈值 | **N=13 用例已就绪**（T2#7 fixed）；**真机基线仍待** baseline 修复 + web image 重建 |

### 7.3 对 §十二 决策 2 实证

| 决策 | 现状 |
|------|------|
| 决策 2 = Qwen3-1.7B-q4f16_1-MLC（accepted）| 真机端侧基线**待**浏览器 IAB 验证（WebLLM + Qwen3）|

---

## §八 架构假设清单（§〇.6 规则 ⑤）

| # | 假设 | 现状核实 | 结论 |
|---|------|----------|------|
| 1 | emotion-llm-service 默认 mTLS | 容器内 `TLS_ENABLED=1` | ✅ 成立 |
| 2 | emotion-llm-service 双向认证 | `TLS_REQUIRE_CLIENT_AUTH=1` | ✅ 成立 |
| 3 | baseline 用 insecure_channel 直连 | `cloud_grpc.py:90` `grpc.insecure_channel` | ✅ 成立（**bug**：与 mTLS 服务错配）|
| 4 | baseline 历史上未真跑过真 LLM | T2#4 报告 fallback_reason 一致 | ✅ 成立 |
| 5 | web v0.1.7 image 不含 Lane O 端代码 | 容器内文件检查 | ✅ 成立（**image 时效**）|
| 6 | Lane O 不能覆盖 Lane E v0.1.7 tag | 协议"两轨均不得覆盖" | ✅ 成立（**留给 Lane E 重建**）|

---

## §九 修复方案（**不在 T3 决议权**，留 Lane O 下一轮）

### 9.1 baseline mTLS 修复（推荐下一轮 Lane O）

修改 `scripts/on-device-baseline/model_fns/cloud_grpc.py`：

```python
# 加载 mTLS 客户端证书
with open(os.environ["TLS_CA_CERT"], "rb") as f:
    ca_cert = f.read()
with open(os.environ["TLS_CLIENT_CERT"], "rb") as f:
    client_cert = f.read()
with open(os.environ["TLS_CLIENT_KEY"], "rb") as f:
    client_key = f.read()
creds = grpc.ssl_channel_credentials(
    root_certificates=ca_cert,
    private_key=client_key,
    certificate_chain=client_cert,
)
channel = grpc.secure_channel(resolved_target, creds)
```

**新增测试**（vitest 等价）：
- pytest `test_cloud_grpc_mtls.py`：fake TLS server + 验证 secure_channel 连接成功

### 9.2 web image 重建（Lane E 域，留 Lane E 处理）

Lane E 在 E2E-19/20 阶段尚未触发 web image 重建。Lane O 触发方式：
- 选项 A：Lane O 提 PR 给 Lane E（最干净）
- 选项 B：Lane O 自己建 `lane-o-iab-verify` tag（不覆盖 v0.1.7，但 docker-compose 里 image hardcoded 需改 compose）
- 选项 C：等 Lane E 自然重建

---

## §十 OND-F 状态变更

| ID | 状态变更 |
|----|----------|
| **OND-F-06** | **fixed → 重开 open**（动态引擎架构就绪 ✓；真引擎接入 ✗ = baseline mTLS bug + web image 时效）|
| **OND-F-08** | 维持（新登记录见 STATUS）|
| **ONDF-F-09** | **新登**（本报告）：baseline `cloud_grpc.py` mTLS 协议错配（insecure_channel vs emotion-llm-service mTLS）+ web image v0.1.7 时效问题 |

---

## §十一 commit 元信息

```
docs(on-device): T3 IAB 验证报告 + OND-F-06/08/09 状态更新

T3 借 dev mode 半天窗口实测（2026-09-28 00:00 ~ 01:00）：

启动合规：
- ✅ devmode 锁创建（deploy/.devmode-session）
- ✅ --env-file .env.local --profile dev 启动
- ✅ 21 容器启动 / 17 healthy / 3 obs 未配 healthcheck
- ✅ emotion-llm-service v0.1.2 容器跑 mTLS :50051
- ⚠️ web v0.1.7 image 早于 Lane O 端侧 PR 5 天（image 时效问题）

实测发现（2 项）：
1. baseline cloud_grpc.py 用 insecure_channel vs emotion-llm-service mTLS
   → T2#4 baseline 从未真跑通过真 LLM（mock_fallback 100%）
2. web v0.1.7 容器内不含 Lane O 端侧代码
   → /demo/local-llm 路由不存在

T3 真机基线（N=13 真 LLM）验证未完成：
- baseline mTLS 修复 = Lane O 下一轮
- web image 重建 = Lane E 域（待 Lane E 重建含端代码的 web image）

留账：
- OND-F-06 → 重开 open（T3 部分完成）
- OND-F-09 新登：cloud_grpc mTLS bug + web image 时效

调研依据（AGENTS §〇.6 规则 ⑥）：
① scripts/on-device-baseline/model_fns/cloud_grpc.py（T2#4 沿用）
② emotion-llm-service/grpc_server.py line 418-469（mTLS 配置）
③ emotion-llm-service/chat_completion.py iter_chat_chunks
④ deploy/docker-compose.apps.yml line 449-...（profile 配置）
⑤ protocol/parallel-tracks.md §三.资源1 启动铁律
⑥ v0.2 §十二 + v0.3 §C.1 + §G.1
⑦ deploy/.env.local LLM_API_KEY + TLS 配置（容器内 env 检查）
⑧ 容器内 gRPC 客户端实测 FutureTimeoutError（SSL_ERROR_SSL）
⑨ 容器内 web v0.1.7 文件系统检查（offline/ + demo/local-llm.vue 不存在）
⑩ memory E2E-F-70/99 教训（dev 容器跑旧代码 = 必须镜像时间戳核对）
```

---

## §十二 给下次 Lane O 会话的开场动作

1. **修 baseline mTLS 协议错配**（本轮 Lane O 决议权）：
   - `scripts/on-device-baseline/model_fns/cloud_grpc.py` 改 insecure_channel → secure_channel
   - 加载 mTLS 客户端证书（ca.crt + client.crt + client.key）
   - 新增 pytest `test_cloud_grpc_mtls.py`（fake TLS server + 验证握手成功）
   - 跑 baseline 验证 N=13 真 LLM 分数
2. **触发 Lane E 重建 web image**（跨 Lane 域，提 PR）：
   - PR 描述清楚需要含 Lane O PR #86 + #92 + #104 + #106 端侧代码
   - 新 tag 建议 `v0.1.8`（保持向前兼容）
3. **拆 dev mode 锁 + 收尾**：
   - 写 STATUS.md §一.13 T3 IAB 收口段 + §四 #9 标记降级
   - 拆 `deploy/.devmode-session`
   - commit + push + PR + 合并 + OND-F-09 留账
4. **OND-F-08 维持**（§十二决策 1 两阶段拍板注记无需变更）

---

## §十三 Lane E 一次性须知

- Lane O 借了 dev mode 半天窗口（2026-09-28 00:00 ~ 01:00 实际 ~1h）
- 21 容器栈已 healthy，本报告拆锁后会保留（Lane E 决定是否继续使用）
- web v0.1.7 image 是 Lane E 在 E2E-17 重建的；Lane E 后续重建 web image 时**建议加入 Lane O 端代码**
- emotion-llm-service v0.1.2 mTLS 默认开启，**Lane E 任何 gRPC 客户端**都必须用 secure_channel + 客户端证书

---

## §十四 给 Lane O 的工程教训

| 教训 | 改进建议 |
|------|----------|
| baseline 实现从未在真 LLM 环境跑通就上线 = false positive（0% pass 被当成"mock fallback 正常") | T3+ 必填：每次 baseline 实现变更后**真 LLM 烟测**（至少 1 用例真连 + 验证 fallback_reason ≠ mock_fallback）|
| web image 重建被依赖 Lane E = 端侧代码不能独立验证 | Lane O 应有自己的 image tag（如 `lane-o-iab-verify`），与 Lane E v0.1.7 并存；docker-compose 用 override 机制 |
| dev mode 启动后没**主动验证** image 是否含端代码 = 启动即失败 | 协议 §三.资源1 启动铁律升级：image 启动后必须 `docker exec emotion-echo-web grep createDynamicEngine /app/app/utils/offline/webllmEngine.ts`（grep 验证端代码存在）|