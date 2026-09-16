---
status: planned
priority: critical
owner: TBD
created: 2026-09-16
last-refresh: 2026-09-16
type: bug-fix
depends-on:
  - stage-108-sender-architecture-debt-fix-2026-09-16.md §四 (A7 详细登记)
  - stage-107-chat-new-conversation-fix-2026-09-16.md (端到端绿时的 APISIX 配置基线)
related-stages:
  - stage-94-code-review-2026-09-14-p0-closure.md (P0-R2-1 JWT cookie + APISIX jwt-auth 落地)
  - stage-97-round2-p0-closure.md (Round 2 P0 收口)
related-issues:
  - docs/plans/test-coverage-tracker-2026-09-16.md §一 E2E-2 (🔴)
  - docs/plans/test-coverage-tracker-2026-09-16.md §四 A7 (🔴)
related-architecture-debts:
  - X-4 鉴权 (横切链路): APISIX jwt-auth consumer + X-User-Id 注入
blocking:
  - sprint-109b-end-to-end-chat-2026-09-16.md (端到端验证前提)
  - sprint-109c-data-contract-smoke-2026-09-16.md (数据契约 smoke 前提)
  - 阶段 2 tests/e2e/ 全部 Playwright spec
  - test-coverage-tracker §二 X-1 outbox / X-2 sw8 / X-4 鉴权 端到端验证
---

# Sprint 109a — APISIX jwt-auth 401 诊断 + 修复

> **目的**：修 A7 (APISIX jwt-auth 401) — 所有 /api/v1/* 返 401，但 token 本地验算 OK、consumer 配置 OK、route 100 配置 OK。
>
> **为什么是 critical**：阻塞 E2E-2 chat / X-1 outbox / X-2 sw8 / X-4 鉴权 全部端到端验证 + 阶段 2 全部 Playwright。
>
> **现状**（Sprint 108 浏览器实测）：
> - Stage 107 端到端绿时（同 session 早期）所有 API 200 OK
> - Sprint 108 重启 web 容器后所有 /api/v1/* 突然 401
> - token payload `{key:user, sub:1, exp:1789639352}` 本地 HS256 验算 PASS（secret=dev-bff-secret）
> - APISIX consumer 配置 `{key:user, secret:dev-bff-secret, HS256}` PASS
> - APISIX route 100 jwt-auth 配置 `{store_in_ctx:true, cookie:access_token}` 看起来正常

---

## 一、调研依据（AGENTS.md §〇 硬规则）

| 文件 / 现象 | 用途 |
|---|---|
| `deploy/apisix/seed.sh` | consumer + route 注册脚本 |
| `deploy/apisix/seed_test.js` (40 用例) | seed.sh 静态结构断言（不含运行时 jwt 验签）|
| `emotion-echo-web-bff/internal/handler/auth_handler.go` | BFF 签发 JWT（确认 secret=dev-bff-secret）|
| `emotion-echo-web-bff/main.go:412` | BFF `/api/v1/ai/stream` 注册 |
| `deploy/docker-compose.apps.yml` | apisix-seed + emotion-echo-apisix + emotion-echo-web-bff 依赖图 |
| `docs/stages/stage-108-sender-architecture-debt-fix-2026-09-16.md §四` | A7 完整现象 + 4 个假设根因 |
| `docs/stages/stage-107-chat-new-conversation-fix-2026-09-16.md §六` | Stage 107 修后实测 POST /conversations 返 200（基线）|
| `docs/plans/test-coverage-tracker-2026-09-16.md §一 E2E-2 + §四 A7` | 当前状态追踪 |

### 架构假设清单（写前对齐）

| 假设 | 验证 |
|---|---|
| A7 根因不在 sender（已 Sprint 108 useState 化完成）| ✅ sender 是前端 composable，与 APISIX 网关无关 |
| A7 根因不在 BFF（BFF 日志无任何请求到达记录）| ✅ BFF 日志完全空，请求被 APISIX 拦截在网关层 |
| A7 根因可能在 (a) APISIX cache (b) jwt-auth plugin 状态 (c) seed 重跑覆盖 (d) skywalking-logger/file-logger 副作用 (e) consumer secret mismatch (f) etcd 数据被覆盖 | 🟡 4 个假设根因都在 stage-108 §四 列出，需逐一排查 |

---

## 二、假设根因（按优先级排查）

### H1：APISIX jwt-auth consumer 配置在某个时间点被改

**测试方法**：
```bash
# 直读 admin API consumer 完整配置
curl -sS http://localhost:9180/apisix/admin/consumers/emotion_echo_bff \
  -H "X-API-KEY: WhZEPlrGviCSXlKFfALZlQWinluoGAbj" | python -m json.tool
```

**期望**：plugin.jwt-auth = `{key:user, secret:dev-bff-secret, algorithm:HS256}`（Stage 102/105 落地值）

**实测**：已读，OK ✅（line 12 显示 `{key:user, secret:dev-bff-secret, algorithm:HS256}`）

### H2：APISIX route 100 jwt-auth plugin 配置缺失 header 解析

**现象**：jwt-auth plugin 默认查 Authorization header，但 route 100 配置 `{store_in_ctx:true, cookie:access_token}` 只显式配 cookie——**header 解析可能未启用**

**测试方法**：
```bash
curl -sS http://localhost:9180/apisix/admin/routes/100 \
  -H "X-API-KEY: WhZEPlrGviCSXlKFfALZlQWinluoGAbj" | python -c "
import sys,json
d=json.load(sys.stdin)
print(json.dumps(d['value']['plugins'].get('jwt-auth',{}),indent=2))
"
```

**期望**：jwt-auth 配置应包含 `header:Authorization`（或保持默认但文档说默认 header=authorization）

**APISIX jwt-auth 默认行为**：默认 `header=authorization`（小写），但可能 Stage 102/105 改动后变成了只读 cookie。

### H3：apisix-seed 重跑导致 consumer 配置被覆盖但 etcd 缓存未清

**触发场景**：
- Stage 103/107 多次重启 web 容器
- web depends_on apisix-seed: service_started
- 容器重启可能触发 seed 重跑（depends_on logic）
- 但 seed.sh 是 idempotent 的（PUT = 幂等覆盖）

**测试方法**：
```bash
# 看 etcd 里 consumer 的 update_time vs create_time
docker exec emotion-echo-etcd etcdctl get /apisix/consumers --prefix -w json | python -c "
import sys,json
d=json.load(sys.stdin)
for kv in d.get('kvs',[]):
    print(kv.get('key','').decode(), 'mod_revision=', kv.get('mod_revision'), 'version=', kv.get('version'))
"
```

### H4：APISIX skywalking-logger / file-logger 在某个请求路径上抛错

**测试方法**：
```bash
# 关掉 file-logger 看是否还 401
curl -X PATCH http://localhost:9180/apisix/admin/routes/100 \
  -H "X-API-KEY: WhZEPlrGviCSXlKFfALZlQWinluoGAbj" \
  -d '{"plugins":{"jwt-auth":{...}}}'  # 暂时删 skywalking-logger / file-logger
```

### H5：APISIX 内部 etcd 数据被损坏

**测试方法**：
```bash
# 重启 apisix 容器清内存缓存
docker restart emotion-echo-apisix
sleep 5
curl http://localhost:19080/api/v1/auth/login ...  # 重测
```

### H6：APISIX 版本升级导致 jwt-auth plugin 行为变更

**测试方法**：查 APISIX 版本日志。

---

## 三、TDD 实施步骤（待执行）

### Step 1 · RED：写"APISIX jwt-auth 拒绝合法 token 应该返 200"契约测试

新建 `deploy/apisix/test_jwt_auth_runtime.sh` 或 `deploy/apisix/test_jwt_auth_e2e_test.py`：

```bash
# 伪代码示意
TOKEN=$(curl -sS http://localhost:19080/api/v1/auth/login -X POST \
  -H "Content-Type: application/json" \
  -d '{"username":"echo","password":"echo123","rememberMe":true}' \
  | python -c "import sys,json;print(json.load(sys.stdin)['data']['accessToken'])")

# 直 curl 必须 200
HTTP=$(curl -sS -o /dev/null -w "%{http_code}" \
  http://localhost:19080/api/v1/user/profile \
  -H "Authorization: Bearer $TOKEN")

# 断言
[ "$HTTP" = "200" ] || die "APISIX jwt-auth rejected valid token: HTTP=$HTTP"
```

**预期**：当前 FAIL（HTTP=401），修复后 PASS。

### Step 2 · 诊断（按 H1-H6 顺序排查）

每个假设执行对应测试，记录哪个假设成立。

### Step 3 · GREEN：根据诊断结果修复

可能修法：
- H2 成立：route 100 jwt-auth plugin 加 `header:Authorization` 显式配置
- H3 成立：consumer secret 不匹配 → 重置 consumer + 重启 APISIX
- H4 成立：删 file-logger / skywalking-logger 验证
- H5 成立：清 etcd + 重跑 apisix-seed
- H6 成立：回滚 APISIX 版本

### Step 4 · 回归：跑 §2.4 数据契约 smoke（sprint-109c）

---

## 四、DoD（Definition of Done）

- [ ] `test_jwt_auth_runtime.sh` 测试从 FAIL → PASS
- [ ] 浏览器实测：POST /conversations + POST /messages + POST /ai/stream 全部 200
- [ ] 浏览器实测：SSE /ai/stream 返回 data: [DONE]
- [ ] BFF 日志可见 ChatCompletion 调用记录
- [ ] chat-svc 日志可见 message 创建记录
- [ ] ai-svc 日志可见 fusion tick 处理 msgID
- [ ] docs/stages/stage-109a-apisix-jwt-401-fix-2026-09-16.md 落地
- [ ] test-coverage-tracker §一 E2E-2 + §四 A7 状态更新
- [ ] §2.5 收口 + push

---

## 五、工作量估计

- Step 1 RED: 30 min
- Step 2 诊断: 1-2 hour（6 个假设逐一排查，可能 1-2 个就定位）
- Step 3 GREEN: 30 min - 1 hour（取决于根因）
- Step 4 回归 + 文档: 30 min

**总计**: 2.5-4 hour = **半个工作日到 1 个工作日**

---

## 六、风险与回退

**风险**：
- 改 APISIX plugin 配置可能影响其他路由（X-1 outbox / X-2 sw8 / X-4 鉴权 都用 route 100）
- 重启 APISIX 容器会清空 JWT 验签 cache，导致所有客户端 401 几秒

**回退方案**：
- git revert 全部 Sprint 109a commits
- APISIX admin API PATCH 恢复原 plugin 配置
- 如果 etcd 损坏 → 从 etcd backup 恢复（如果有）

---

## 七、调研依据未做完（写前必补）

- [ ] 读 APISIX 启动日志（`docker logs emotion-echo-apisix` 全部）
- [ ] 读 APISIX 当前 plugin config 版本（与 Stage 102/105 收口时对比）
- [ ] 查 skywalking-logger / file-logger 在 APISIX 3.x 是否有 known issue
- [ ] 查 APISIX jwt-auth 3.x 文档（默认是否查 header）
- [ ] 读 emotion-echo-apisix Dockerfile 看 APISIX 镜像版本

---

## 八、commit 计划

| # | Commit | 文件 |
|---|---|---|
| 1 | `test(apixix): RED 钉住合法 JWT 必须通过 jwt-auth 验证` | `deploy/apisix/test_jwt_auth_runtime.sh` (新建) |
| 2 | `fix(apixix): 根据 H1-H6 诊断结果修复 (具体内容待 Step 2-3 后定)` | `deploy/apisix/seed.sh` 或 admin API PATCH |
| 3 | `docs(stage-109a): APISIX jwt-auth 401 诊断 + 修复 + 根因 + 回归验证` | `docs/stages/stage-109a-apisix-jwt-401-fix-2026-09-16.md` |
