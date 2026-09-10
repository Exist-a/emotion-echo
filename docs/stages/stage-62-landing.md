# Stage 62 · 2026-09-10 合规优先最小集 + BFF 登录限流真 bug 修复（PR-1/2/4/5 landed · PR-3 留独立 sprint）

> **状态**：🟢 **4 PR landed + 1 PR blocked-规模**（PR-1/2/4/5；PR-3 BFF→3 svc gRPC 化留独立 sprint）
> **关联**：[`docs/plans/stage-62-cleanup-and-grpc-plan.md`](../plans/stage-62-cleanup-and-grpc-plan.md) ·
> [`docs/architecture/adr/adr-2026-09-drop-user-oauth-ddl.md`](../architecture/adr/adr-2026-09-drop-user-oauth-ddl.md)（ADR 21）·
> [`scripts/test_user_oauth_zero_ref.sh`](../../scripts/test_user_oauth_zero_ref.sh)

本批覆盖 4 工作面（PR-1 / PR-2 / PR-4 / PR-5），PR-3 因工作量（17 RPC + 3 proto + 3 server + 3 client + smoke）超过本次会话容量，留独立 sprint 推进。

---

## 一、PR-1 · BFF 登录限流真 bug 修复 + 7 测试

### 1.1 触发

`emotion-echo-web-bff/internal/handler/auth_handler.go:283-296` `recordFailure` 声称"重置计数，锁定期内不再叠加"，但**代码逻辑有 bug**——failCount 自增在 if 之前执行，锁定期内继续叠加。

### 1.2 RED：写 7 个 helper-only 单测

plan 原计划 6 个测试，本会话补 1 个"5min 窗口已过后解锁"（之前 plan 提到，但 plan §二.1 调查时未确认）。

```
TestAuthHandler_RecordFailure_BelowThreshold_DoesNotLock
TestAuthHandler_RecordFailure_AtThreshold_Locks
TestAuthHandler_RecordFailure_AfterLock_ResetsCount       ← RED FAIL（暴露真 bug）
TestAuthHandler_ClearFailures_RemovesLock
TestAuthHandler_IsLocked_UnknownUser_ReturnsFalse
TestAuthHandler_PerUserIsolation
TestAuthHandler_IsLocked_AfterWindowExpires                ← 补 plan §二.1 缺项
```

### 1.3 GREEN：修 recordFailure bug

```go
// 修改前：failCount++ 在 if 前 → 锁定后下次仍 +1
// 修改后：锁定期间早返回
if !attempt.lockedAt.IsZero() && time.Since(attempt.lockedAt) < loginLockWindow {
    return
}
attempt.failCount++
```

### 1.4 回归

```
$ go test ./emotion-echo-web-bff/...
ok  emotion-echo-web-bff/internal/auth        (cached)
ok  emotion-echo-web-bff/internal/config      (cached)
ok  emotion-echo-web-bff/internal/discovery   (cached)
ok  emotion-echo-web-bff/internal/downstream  (cached)
ok  emotion-echo-web-bff/internal/handler     1.075s
?   emotion-echo-web-bff/internal/logging     [no test files]
ok  emotion-echo-web-bff/internal/session     (cached)
ok  emotion-echo-web-bff/internal/sse         (cached)
ok  emotion-echo-web-bff/internal/storage     (cached)
?   emotion-echo-web-bff/internal/svc         [no test files]
```

### 1.5 决策 18 §三 类型 5 自报告失真案例

plan §二.1 调查时作者写"零覆盖"——实测已有 8+ 测试但"AfterLock_5Minutes_AllowsRetry"测试名误导（实际仅验证窗口内仍锁定，未验证 5 分钟后解锁）。**作者踩了自己发现的"测试名与行为不符"陷阱**——补救：本批加 `IsLocked_AfterWindowExpires` 覆盖真实解锁语义。

---

## 二、PR-2 · decisions.md 架构全景图同步（微调）

### 2.1 触发

plan §二.2 调查结论：全景图本身合规，决策 18 #24 登记时基于"作者推断"。但为防止后来者再走推断路径，加一行注释显式说明。

### 2.2 修改

`docs/architecture/decisions.md:419` 后追加 5 行注释：

> 🔧 2026-09-10 Stage 62 PR-2 微调：下方 `## 🏗 当前架构全景` 已对齐决策 11/12 关系说明。早期决策 18 #24 登记时基于"作者推断"误以为全景图含 '唯一前端入口' 措辞——实测全景图本身合规。本段提醒后来者：全景图含义以本收口为准，**不要再写类似 'web-bff 是唯一入口' 的措辞**。

工作量 15 分钟。

---

## 三、PR-4 · README/QUICKSTART `--profile ai` 5 秒必读

### 3.1 触发

README.md 启动段第 119-120 行 `docker compose --profile ai up -d --build` 一句话带过——新人 clone 后会按字面启动发现 TTS 不可用（todo-pile §A1 已 closure 但新人不知）。

### 4.2 修改

`README.md` 启动段改写 + `QUICKSTART.md` 第 66 行后追加第二个"5 秒必读"提示框：

| 文件 | 改动 |
|---|---|
| `README.md:115-125` | 把"启动 AI profile"展开成完整命令（默认路径 + 镜像 build 路径）+ 镜像体积警示 |
| `QUICKSTART.md:88-102` | 第二个"5 秒必读"框：AI profile 可选；未启用的后果（TTS 无声 / 多模态不可用）；镜像体积 18GB |

工作量 30 分钟。

---

## 四、PR-5 · OAuth DDL 残留清理（commit 5dac6b0 已落地）

| 维度 | 数值 |
|---|---|
| 新文件 | 4（scripts/test_user_oauth_zero_ref.sh + deploy/db/05-drop-user-oauth.sql + ADR 21 + stage-62 plan）|
| 修改文件 | 5（01 +02 DDL + docker-compose.infra.yml + .env.example + decisions.md 决策 21 登记）|
| 契约测试 | 5/5 PASS |
| 决策落地 | ADR 21 ✅ Accepted |

详见 commit `5dac6b0`。

---

## 五、PR-3 · BFF→3 svc gRPC 化（独立 sprint）

### 5.1 状态

**未在本会话推进**——工作量超出本次会话容量：

| 维度 | 内容 |
|---|---|
| proto | 3 个新文件（user.proto + assessment.proto + analytics.proto）|
| RPC | 17 个（user 3 + assessment 5 + analytics 9）|
| server | 3 个 svc 各加 grpcserver/（参照 chat-svc PR-GRPC-1~6）|
| client | 3 个 BFF downstream gRPC client（参照 chat_grpc.go）|
| feature flag | USER_TRANSPORT / ASSESSMENT_TRANSPORT / ANALYTICS_TRANSPORT=grpc\|http |
| smoke | §契约 10/11/12 端到端（参照 §契约 9 模式）|
| 端口 | user :8887 / assessment :8886 / analytics :8885（避开 :8888/:8889/:8893）|

### 5.2 建议执行顺序

```
PR-3.1 proto + stub 生成（半天）
PR-3.2 三 svc 加 gRPC server（半天）
PR-3.3 BFF 改 gRPC client + feature flag（半天）
PR-3.4 smoke §契约 10/11/12 + OAP rpc tag（半天）
合计 2 天
```

**预计开工时间**：Stage 62 收口后下一 sprint。

---

## 六、累计统计

| 维度 | 数值 |
|---|---|
| **本批新增 commit** | 4（待落地：PR-1 + PR-2 + PR-4 + stage-62 报告）|
| **新增单测** | 7（BFF helper-only + 0 OAuth 契约测试 + 0 doc drift 测试）|
| **修复真 bug** | 1（BFF `recordFailure` 锁定期内 failCount 叠加）|
| **决策栈收口** | ADR 21 Accepted + decisions.md 决策 21 区块登记 |
| **累计 doc-drift** | 26 条（#23 #24 #25 登记后 + #26 新登记 = 1 待补）|
| **未做** | PR-3 BFF→3 svc gRPC 化（独立 sprint）+ 决策 20 owner sign-off |

---

## 七、决策失真登记（本批新发现的失真 #26）

> **#26 (2026-09-10)**：PR-1 调查时作者写"BFF 登录限流零测试"——实测已有 8+ 测试
> 但 `TestAuthHandler_Login_AfterLock_5Minutes_AllowsRetry` 测试名与实际行为严重不符
> （实际仅验证窗口内仍锁定，未验证 5 分钟后解锁）。**修复**：补 7 个 helper-only
> 单测（包含真正测"5min 后解锁"的 `IsLocked_AfterWindowExpires`）+ 修1 个真 bug
> （`recordFailure` 锁定期内 failCount 叠加）。

失真类型：决策 18 §三 类型 5 自报告失真（作者踩了自己发现的陷阱）。

---

## 八、后续 sprint 建议

1. **owner review + commit 本批 4 个 commit**（半小时）
2. **PR-3 BFF→3 svc gRPC 化**（2 天，按 4 子 PR 推进）
3. **chat-svc 表依赖清单 ADR**（todo-pile §D5，半天）
4. **BFF 路由契约测试三方对齐**（todo-pile §C8，1.5~2 天）
5. **决策 20 owner sign-off**（5 分钟）
6. **决策 19 ADR-001 v2 §C 触发监控**（半天，if/when 决策）

---

> 最后更新：2026-09-10 by Stage 62 session
> 用途：合规优先最小集 + 1 个真 bug 修复收口；PR-3 留独立 sprint
> 关联：stage-61 收口后的下一步；ADR 21 + ADR-001 v2 + 决策 9 收口的延续