# CL-02 · Phase 1 扫描与守卫落地证据（🟢 不需 dev 栈）

> 采集时间：2026-10-14 · C 轨（Lane-CL-C）· worktree `../Emotion-Echo-cl02` · 基线 `main@22f8ee9`
> **未起 dev 栈**（A 轨持 `.devmode-session` 至 12:00）——本文件所有结论均为**静态证据**。

---

## 1. 全量清点：43 条 RPC（机械可复现）

```console
$ python - <<'PY'   # 逐 proto 解析 rpc 声明
... re.match(r'\s*rpc\s+([A-Za-z0-9_]+)\s*\(', line) ...
PY
total rpc: 43
```

| proto | rpc 数 |
|---|---|
| `proto/agent.proto` | 5 |
| `proto/chat.proto` | 8 |
| `proto/emotion_llm.proto` | 4 |
| `proto/emotion_query.proto` | 7 |
| `proto/metric.proto` | 9 |
| `proto/user.proto` | 10 |
| **Σ** | **43** |

## 2. 标记点与"未决"的对应（10 处 ↔ 7 条 RPC）

```console
$ grep -rn "E2E-31 已知未接线" --include='*.go' --include='*.py' . | wc -l
10
```

| RPC | 标记处 |
|---|---|
| `StreamMessages` | `chat_server.go`(1) + `chat_grpc.go`(1) = 2 |
| `Logout` | `user_server.go`(1) + `user_grpc.go`(1) = 2 |
| `VerifySecurityAnswer` | `user_server.go`(1) + `user_grpc.go`(1) = 2 |
| `AnalyzeBatch` | `grpc_server.py`(1) = 1 |
| `MentalHealthHistory` | `metric_server.go`(1) = 1 |
| `MentalHealthTrigger` | `metric_server.go`(1) = 1 |
| `MentalHealthTrend` | `metric_server.go`(1) = 1 |
| **Σ** | **10** |

⇒ 结论：**E2E-31 的"7 条死链"是完整的**（未发现第 8 条）。曾一度怀疑
`GetEmotionByMessage / GetEmotionByConversation / GetFusedEmotion` 也无调用方，**经查证否定**：

```console
$ grep -rnE "\.(ByMessage|ByConversation|ByFusedMessage)\(" --include='*.go' . | grep -v _test.go | grep -v downstream/emotion_query.go
emotion-echo-web-bff/internal/handler/emotion_query_handler.go:46:  e, err := h.query.ByMessage(...)
emotion-echo-web-bff/internal/handler/emotion_query_handler.go:66:  f, err := h.query.ByFusedMessage(...)
emotion-echo-web-bff/internal/handler/emotion_query_handler.go:102: items, total, err := h.query.ByConversation(...)
emotion-echo-web-bff/internal/history...  emotion_history.go:54: rows, _, err := s.query.ByConversation(...)
```

## 3. 🔴 关键发现：**RPC 名 ≠ 客户端方法名**（14 条），故证据不能靠名字 grep

机械用 RPC 名去 grep "谁在调它"会把**活链误判成死链**。实证（首次扫描得 6 条 metric RPC 的
调用方为空 → 实为方法名不同）：

| RPC | 客户端方法（真实调用点） |
|---|---|
| `ReportsDaily` | `DailyReport`（`analytics_handler.go:113`） |
| `ReportsTrend` | `TrendReport`（`:129`） |
| `UserBehaviorDayNight` | `DayNightPattern`（`:169`） |
| `UserBehaviorDepth` | `InteractionDepth`（`:186`） |
| `UserBehaviorFrequency` | `FrequencyTrend`（`:212`，另 `:194`） |
| `MentalHealthAssessment` | `MentalAssessment`（`:227`） |
| `ListMyResults` | `ListResults`（`survey_handler.go:109`） |
| `GetSurveyResult` | `GetResult`（`:123`） |
| `GetEmotionByMessage` | `ByMessage`（`emotion_query_handler.go:46`） |
| `GetEmotionByConversation` | `ByConversation`（`:102`） |
| `GetFusedEmotion` | `ByFusedMessage`（`:66`） |
| `UpdateProfile` | `UpdateMe`（`user_handler.go:78`） |
| `GetUserById` | `GetByID`（`user_handler.go:93`） |
| `ChatCompletion` | `ChatCompletion`（`downstream/llm_grpc.go:148`，业务入口 `ai_stream_handler.go:431` `h.llm.StreamChat`） |

**同型陷阱（会误导证据采集）**：

- `SendMessage` 名字撞 Kafka producer：`ai-svc/internal/consumer/dlq.go:169 p.producer.SendMessage(msg)`（与 chat RPC 无关）。
- `Register` 名字撞 Nacos：`ai-svc/nacos_boot.go:86 reg.Register(ctx, instance)` 与 BFF 路由注册 `main.go:501`（均非 user RPC）。
- `ChatCompletion` 唯一"同名命中"在测试脚本 `emotion-llm-service/tests/e2e/stage91_pdf_sentinel.py:116`。

⇒ 这两条直接决定守卫设计：**证据 = 清单登记的 (文件, 符号)，守卫校验该文件内出现该符号调用**，
而不是"用 RPC 名 grep 全仓"。

## 4. 守卫落地与负向对照（T-09~T-14）

产物：`scripts/rpc_status.tsv`（43 行，7 列）+ `scripts/check_rpc_status.sh`（A1~A5）。

```console
$ bash scripts/check_rpc_status.sh
== RPC 状态守卫（CL-02）==
PASS: A1 覆盖 43/43 条 RPC 均已在清单登记
PASS: A2 live 36 条 RPC 均有已登记且可校验的调用方证据
PASS: A3 标记数 10 与清单 marker_sites 之和一致
PASS: A4 终态规则（archived 带标注 / deleted 无残留）——当前 archived/deleted 行数：0
PASS: A5 HTTP 回滚开关豁免声明在位（*_TRANSPORT=http 是降级开关，不算死链）
-- undecided = 7（收口判据要求 0）--
== PASS ==
```

**负向对照（证明守卫真能红，非"永真断言"）**：

```console
# 对照 1：把 SendMessage 的证据符号改成不存在的名字
FAIL: A2 live 但调用方证据不足 —— SendMessage(调用点.../chat_handler.go无NoSuchMethod_XYZ( )
== FAIL ==

# 对照 2：给 live 行 Login 挂一个标记（标记数 10→11，且标记落在非 undecided 行）
FAIL: A3 标记数 10 ≠ 清单推导的 11
FAIL: A3 标记只能挂在 undecided 行 —— 违规：Login(live,标记1)
== FAIL ==
```

还原后复跑 → `== PASS ==`（exit 0）。

**与旧守卫共存（实测）**：新脚本**不含**标记字面量（标记串从 `check_dead_grpc_inventory.sh` 读取），
故旧守卫的标记计数不被污染 —— 两者可同时绿：

```console
$ bash scripts/check_dead_grpc_inventory.sh; echo rc=$?    # 旧：清单真实性（EXPECTED=10）
PASS: 未接线标记 10/10，清单与代码一致
rc=0
$ bash scripts/check_rpc_status.sh;     echo rc=$?          # 新：每条 RPC 有状态 + live 有调用方
== PASS ==
rc=0
```

> 踩坑记录：初版把标记字面量写进新脚本 ⇒ 旧守卫计数变 11≠10 **立刻红**（守卫互撞）。
> 修法 = 标记串**单一事实源**（从旧守卫读），而非各自硬编码。

> **A3 的设计要点**：标记预期数**由清单推导**（Σ`marker_sites`），不再硬编码 `EXPECTED=10` ——
> 消掉旧守卫"改一处忘改另一处"的同步负担（来源计划 §8 D4 建议"叠加"，本实现为**取代 + 保留旧守卫**）。

## 5. 未做 / 待办（诚实声明）

- **A4 目前是空断言**：`archived`/`deleted` 行数为 0（7 条 `undecided` 尚未裁决）。Phase 2/3 落地后 A4 才有实义。
- **未接入 CI**：按 plan §3，Phase 6（T-24）才接 `e2e-guards.yml`；接入后须复跑负向对照证明"接进 CI 仍能红"。
- **未起 dev 栈**：`*_TRANSPORT` 运行时环境值（T-15 的 env 部分）、清除后全栈可起（T-21）均**未取证**。
- **清单证据为"调用点存在性"证据**：它证明"有生产调用点"，**不证明该调用路径当前可用**（后者属 T-15/T-21 的运行时范畴）。
