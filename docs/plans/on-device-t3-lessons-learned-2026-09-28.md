---
status: lessons-learned
priority: high
type: engineering-retrospective
created: 2026-09-28
last-refresh: 2026-09-28（Lane O T3 借 dev mode 实测后集中文档化）
related-plans:
  - ./on-device-t3-iab-report-2026-09-28.md（T3 完整报告 · 369 行）
  - ./on-device-STATUS.md §一.13（T3 收口段）
  - ./on-device-findings.md OND-F-09（本教训的核心账本条目）
  - ./on-device-baseline-report-2026-09-24.md（T2#4 baseline 报告 · 已确认有同样 bug 但未发现）
related-decisions:
  - D-26 端侧化主方案 ADR（accepted，决策 33）
  - D-26.2 端侧主力模型 ADR（accepted，决策 34）—— **本拍板对真机基线未生效**
related-issues:
  - **OND-F-09**（核心账本）：baseline `cloud_grpc.py` mTLS 协议错配 + web v0.1.7 image 时效
---

# T3 IAB 验证工程教训集中文档（2026-09-28 · Lane O）

> **文档定位**：T3 借 dev mode 半天窗口实测后，**两个核心发现**的工程教训集中文档化。
>
> **目的**：
> 1. **明确 §十二决策 2（D-26.2 = Qwen3-1.7B 真机基线）未生效**——baseline 从未真跑通过真 LLM
> 2. **明确 §十二决策 1-5 全部拍板生效前提**——baseline mTLS 修复 + web image 重建两道门槛
> 3. **把发现从"ONDF-LMF/OPS 调试痕迹"升级为"工程教训"**——避免后续 Lane O / Lane E 重复踩坑
>
> **详细报告**：[`on-device-t3-iab-report-2026-09-28.md`](./on-device-t3-iab-report-2026-09-28.md)（369 行；本文件是浓缩版）

---

## §〇 TL;DR — 一句话总结

**T2#4 baseline 报告（PR #91 `aeb3f24`）的 0% pass 是 mock fallback 路径的结果，不是真 LLM 评估**。T3 容器内实测发现 baseline `cloud_grpc.py` 用 `grpc.insecure_channel` vs emotion-llm-service v0.1.2 mTLS 协议错配 → 13/13 用例 fallback `grpc_unreachable:FutureTimeoutError:mock_fallback`。

---

## §一 发现 1：baseline `cloud_grpc.py` mTLS 协议错配

### 1.1 现象

T2#4 baseline 报告（`on-device-baseline-report-2026-09-24.md`）：N=7 / pass=0% / length=0% / guardrail=85.71%
T3 容器内重跑（`on-device-t3-iab-report-2026-09-28.md` §七）：N=13 / pass=0% / length=0% / guardrail=84.62%
**两次 fallback_reason 一致**：`grpc_unreachable:FutureTimeoutError:mock_fallback`

### 1.2 根因对比

| 组件 | 期望 | 实际 |
|------|------|------|
| emotion-llm-service gRPC | `add_secure_port` (mTLS) | `TLS_ENABLED=1` 默认开启（`grpc_server.py:418-469`）|
| `TLS_REQUIRE_CLIENT_AUTH` | 双向认证 | `1`（默认）|
| baseline `cloud_grpc.py:90` | `grpc.secure_channel` + 客户端证书 | **`grpc.insecure_channel`**（明文）+ `x-internal-api-key` metadata |

### 1.3 触发链

```
baseline → grpc.insecure_channel("localhost:50051")
  → emotion-llm-service 启 mTLS 期待 TLS 握手
  → 客户端发明文 ClientHello
  → 服务端 SSL_ERROR_SSL: WRONG_VERSION_NUMBER
  → channel_ready_future 超时（2s）
  → cloud_grpc.py:99 返回 _mock_fallback_reply()
  → ReplyResult(text=mock 文案, fallback_reason="grpc_unreachable:FutureTimeoutError:mock_fallback")
  → evaluate_case(text=mock 文案) → length 挂 / 长度合规率 0% / guardrail 84.62%（mock 文案不含诊断词/贴标签词）
```

### 1.4 为什么 T2#4 没发现

T2#4 报告**接受**了 "0% pass" 作为预期结果（`on-device-baseline-report-2026-09-24.md §一` 写 "本 baseline 报告值仅作占位；真实 Qwen3-1.7B 真机基线须 T3 借 dev mode 窗口完成"）。

**盲点**：把"mock fallback 路径下 length_pass_rate 必挂"当预期（**对**），但**没追问为什么 fallback_reason 全是 mock_fallback**（应该警觉）：
- `LLM_API_KEY` 空 → mock fallback 触发（chat_completion.py:101）
- gRPC 连不上 → mock fallback 触发（cloud_grpc.py:99）
- 两个不同根因，**同一个 fallback_reason**——T2#4 没区分

### 1.5 影响

| 影响维度 | 详情 |
|----------|------|
| **D-26.2 真机基线未生效** | ADR §五 验收契约"golden set 13 用例 + Qwen3-1.7B 跑分 ≥ 阈值"——0% pass **不构成真机基线数据** |
| **OC-11 阶段一收口契约未满足** | v0.3 §G.1 要求"golden set 端侧 ≥ 阈值"——**真机基线数据不存在** |
| **§十二决策 2 实证基础不成立** | decision-pack.md §二.5.2 "真机基线分数填到 §二决策 2 拍板后 OC-11 阈值"——**真机基线分数 = 0**（mock 评估） |
| **§十二决策 1-5 全部拍板生效前提** | D-26.2 真机基线是 §十二决策所有联动契约的基础——**没有真基线 = §十二决策仍待 baseline 修复后验证** |

### 1.6 修复方案（Lane O 决议权，下一轮）

修改 `scripts/on-device-baseline/model_fns/cloud_grpc.py`：

```python
import os
# 加载 mTLS 客户端证书（容器内 /app/etc/tls/）
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

新增 pytest 烟测 `test_cloud_grpc_mtls.py`：fake TLS server + 验证握手成功。

修后跑 baseline N=13，**真 LLM 回复**才能作为 §十二决策实证依据。

---

## §二 发现 2：web v0.1.7 image 时效问题

### 2.1 现象

T3 启动容器栈（dev mode）后：
- `docker exec emotion-echo-web` 查 `/app/app/utils/offline/` → **不存在**
- `docker exec emotion-echo-web` 查 `/app/app/pages/demo/local-llm.vue` → **不存在**
- `curl http://localhost:3000/demo/local-llm` → **404**

### 2.2 根因对比

| 镜像 | Created | Lane O 端侧 PR |
|------|--------|---------------|
| `emotion-echo/web:v0.1.7` | **2026-09-23 23:29:52** | T2#1 PR #86 = 2026-09-24 03:11（5 天后）|
| `emotion-echo/web-bff:v0.1.30` | 2026-09-23 22:33 | 同上 |

**web image 在 Lane O 端侧代码合并前 5 天构建**，端代码不在 image 里。

### 2.3 协议 §三.资源1 启动铁律本应捕获

按 [parallel-tracks.md §三.资源1](docs/_meta/parallel-tracks.md)：
> 1. 启动 dev mode 前**必须**读该文件（devmode 锁）
> 2. 镜像时间戳晚于最新修复 commit
> 3. 写 STATUS 环境基线

**T3 启动了 dev mode + 核对了镜像时间戳**——但核对方式是"Created 早于 Lane O PR"（PASS 因为 protocol 要求"镜像晚于**修复** commit"——但 Lane O PR 不算 Lane E 的"修复 commit"）。

**铁律改进建议**：核对方式应升级为"`grep createDynamicEngine /app/app/utils/offline/webllmEngine.ts` 验证端代码存在"——不依赖 commit 时间戳。

### 2.4 影响

| 影响维度 | 详情 |
|----------|------|
| **T3 Demo 页 IAB 验证降级** | `/demo/local-llm` 路由不存在 → 无法浏览器 IAB 验证 WebLLM 真引擎接入 |
| **§十二决策 2 真机基线（WebLLM 端侧）未生效** | 不仅 baseline mTLS bug + web image 也缺端代码 |
| **Lane O 端代码独立性** | Lane O 不能独立验证端代码——依赖 Lane E 重建 web image |

### 2.5 修复方案（Lane E 域，Lane O 触发 PR）

- 选项 A：Lane O 提 PR 给 Lane E，请求重建 web image（建议 tag `v0.1.8`）含 Lane O PR #86+#92+#104+#106 端代码
- 选项 B：Lane O 自己 rebuild `lane-o-iab-verify` tag（不覆盖 v0.1.7），改 compose file image 引用——但改 compose = 共享列
- 选项 C：等 Lane E 自然重建（含 E2E-19/20 阶段后续）

**推荐 A**（最干净，跨 Lane 协作）。

### 2.6 Dockerfile 重建失败（顺带发现）

`emotion-echo-web/Dockerfile:18-29` 有注释行隔断 `&&` 链 bug（worktree rebuild 失败）。
Lane O 不擅自动 Dockerfile（Lane E 域），但记录在 IAB 报告 §四.4 + 本文件 §二.6。

---

## §三 §十二决策拍板生效前提矩阵

| 决策 | 拍板结果 | 生效前提 | 当前状态 |
|------|----------|----------|----------|
| D-26.1 隐私定位 = (a) | ✅ accepted | 代码改动 = 0（管道零改动）| ✅ 立即生效 |
| D-26.2 模型 = Qwen3-1.7B | ✅ accepted | ① baseline mTLS 修复（§一）+ ② web image 重建（§二）+ ③ 浏览器 IAB 真机验证 | ❌ 三道门槛未过 |
| D-26.3 来源 = 角标 | ✅ accepted | chat-svc 加 `reply_source` 字段 + 前端角标组件 | ⚠️ 拍板后代码待写（Lane O 决议权）|
| D-26.4 离线 = L0+L1 | ✅ accepted | OC-02~OC-05 实现 + E2E-29 收口 | ❌ 强依赖 E2E |
| D-26.5 摘要 = (a) 联动 | ✅ accepted | OC-08 端云共用组装层 + chat-svc 摘要端点 | ⚠️ 拍板后代码待写 + 依赖 E2E-23 |

**结论**：
- **D-26.1 立即生效**（无需代码改动）
- **D-26.3/4/5 部分生效**（决策材料 + ADR 已立；代码实现待启动）
- **D-26.2 未生效**（真机基线不存在，决策实证基础不成立）

---

## §四 工程教训（Lane O 全局）

### 4.1 baseline 实现"自我证明"机制缺失

**问题**：baseline 实现从未在真 LLM 环境跑通就上线 = false positive（0% pass 被当成"mock fallback 正常"）

**改进**：T3+ 必填——
- 每次 baseline 实现变更后**真 LLM 烟测**（至少 1 用例真连 + 验证 fallback_reason ≠ mock_fallback）
- pytest 新增 `test_cloud_grpc_smoke.py`：用 fake TLS server + 真 LLM key + 验证回复文本 ≠ mock 文案（长度 100~300 + 不含诊断词 + 含 hotline）

### 4.2 web image 重建依赖跨 Lane

**问题**：web image 重建被依赖 Lane E = 端侧代码不能独立验证

**改进**：Lane O 应有自己的 image tag（如 `lane-o-iab-verify`），与 Lane E v0.1.7 并存。
- docker-compose 用 override 机制（`--image` 覆盖 service-level image）
- compose file 不改，只在启动命令加 override flag

### 4.3 dev mode 启动后**主动验证** image 内容

**问题**：dev mode 启动后没**主动验证** image 是否含端代码 = 启动即失败

**改进**：协议 §三.资源1 启动铁律升级——
- 当前核对：`docker inspect <image> Created` 时间戳
- 升级后：`docker exec emotion-echo-web grep createDynamicEngine /app/app/utils/offline/webllmEngine.ts`（grep 验证端代码存在）
- 时间戳核对 = **必要非充分**；grep 验证 = **必要充分**

### 4.4 协议错配时 fallback 路径掩盖根因

**问题**：mock fallback 路径掩盖了协议错配根因——T2#4 报告只看到 fallback_reason，没追问 fallback_reason 一致 100% = 必有系统性根因

**改进**：pytest 烟测**断言 fallback_reason 分布**——
- 期望分布：N=13 时 fallback_reason 至少 2 种类型（mock_fallback / grpc_unreachable / upstream_error / etc）
- 不允许 100% fallback_reason 单值（除全 mock 场景显式标注）

### 4.5 status: proposed ADR 的"拍板后"状态转换遗漏

**问题**：ADR front-matter 有 `status: proposed` 字段，但没有"接受拍板后流转为 `accepted`"的明确机制

**改进**：v0.3 §B.1 决议流程明确化——
- ADR 创建时 = `proposed` + `decided-by: null`
- 用户拍板后 = `accepted` + `decided-by: user` + `accepted: <date>`
- 拍板记录字段永久保留在 ADR front-matter（不可删除，便于审计）
- STATUS.md §四 同步划掉（与 v0.2 §十二拍板列表一致）

---

## §五 留账与下一轮 Lane O 工作

### 5.1 OND-F 状态更新

| ID | 状态 |
|----|------|
| **OND-F-09**（核心）| **open**（本文件正式落地）|
| OND-F-06 | 重开 open（dynamic engine 架构就绪 ✓；真引擎接入 ✗ = 待 baseline 修复 + web image 重建）|

### 5.2 下一轮 Lane O 必做（按优先级）

1. **修 baseline mTLS**（Lane O 决议权）—— 落实 §一.6
2. **触发 Lane E 重建 web image**（跨 Lane 域）—— 落实 §二.5
3. **重跑 T3 IAB 验证**（借 dev mode + 修后 baseline + 重建后 image）

### 5.3 §十二决策落地代码（不依赖 dev mode，可独立推进）

| 决策 | 代码落地 | Lane O 决议权 |
|------|----------|--------------|
| D-26.3 | chat-svc 加 `reply_source` 字段 + 前端角标组件 | ✅ 协议 §六 + §二 |
| D-26.5 | 摘要 L0/L1/L2/L3 token 硬预算实现 | ✅ |
| 端云共用组装层（OC-08 骨架） | 不依赖 E2E-23 health 接口，可先实现 interface + 端侧 / 云端两套 | ✅ |

---

## §六 给 Lane E 的教训同步

| 教训 | 改进建议 |
|------|----------|
| web image 重建不及时 = Lane O 端代码不能独立验证 | Lane E 重建 web image 时**主动**检查 Lane O 是否有最新端侧 PR（如 `gh pr list --label lane-o`）|
| emotion-llm-service mTLS 默认开启，但 baseline 实现未更新 | Lane E gRPC 客户端参考实现应该**包含 mTLS 客户端证书加载**（避免下游客户端踩坑）|
| Dockerfile 注释行隔断 `&&` 链 bug | 重建时顺手修（Lane E 域）|

---

## §七 文档引用链

| 引用 | 用途 |
|------|------|
| [`on-device-t3-iab-report-2026-09-28.md`](./on-device-t3-iab-report-2026-09-28.md) | T3 完整 IAB 报告（369 行；本文件是浓缩版）|
| [`on-device-STATUS.md` §一.13](docs/plans/on-device-STATUS.md) | T3 收口段 + §四 #9 划掉 |
| [`on-device-findings.md` OND-F-09](docs/plans/on-device-findings.md) | 核心账本条目 |
| [`on-device-decision-pack.md` §六.1](docs/plans/on-device-decision-pack.md) | §十二决策拍板后 Lane O 阻塞项（待同步 update） |
| [`parallel-tracks.md §三.资源1`](docs/_meta/parallel-tracks.md) | dev mode 启动铁律（本文件 §四.3 建议升级） |

---

## §八 commit 元信息

```
docs(on-device): T3 借 dev mode 半天窗口实测后集中文档化教训（T3·Lane O）

核心交付：
- 文档化 2 项 T3 关键发现：baseline mTLS bug + web v0.1.7 image 时效
- §十二决策拍板生效前提矩阵（D-26.1 立即生效；D-26.2 未生效 = 待真机基线）
- 4 项工程教训（baseline 烟测 / Lane O 独立 image tag / 启动后 grep 验证 /
  fallback_reason 分布断言 / ADR 状态流转明确化）
- 给 Lane E 的 3 项同步教训

调研依据：
- on-device-t3-iab-report-2026-09-28.md（完整报告 369 行）
- scripts/on-device-baseline/model_fns/cloud_grpc.py:90 insecure_channel
- emotion-llm-service/grpc_server.py:418-469 mTLS 配置
- 容器内 gRPC 客户端实测 + emotion-llm-service 容器日志
- memory E2E-F-70/99 教训（dev 容器跑旧代码）
- parallel-tracks.md §三.资源1（dev mode 启动铁律）
```