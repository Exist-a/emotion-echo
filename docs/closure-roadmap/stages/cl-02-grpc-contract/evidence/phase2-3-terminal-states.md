# CL-02 · Phase 2/3 终态落地证据（清除 3 条 + 归档 4 条）

> 采集时间：2026-10-14 · C 轨（Lane-CL-C）· worktree `../Emotion-Echo-cl02` · 基线 `main@adc08bb`
> **未起 dev 栈**（A/B 轨优先）——本文件全部为**静态 + 单测证据**；运行时证据（全栈冒烟）见 §5 待办。

---

## 1. 裁定 → 终态映射（§7 D1~D7）

| RPC | 终态 | 依据 |
|---|---|---|
| `StreamMessages` | **归档** | D2：ADR-2026-09-decision-4-closure §四 #3「故意不做」 |
| `MentalHealthHistory` / `Trigger` / `Trend` | **归档** | D1：ADR-2026-09-chart-contract-alignment §E「5 个无消费端点本轮不接（需 UX 设计）」 |
| `Logout` | **清除** | D3：ADR-2026-10-jwt-key-rotation-dual-key「不做 token 黑名单/主动吊销」+ D-46 备选③被否 |
| `VerifySecurityAnswer` | **清除** | D4：与 `…ByUsername` 族内冗余、零 handler 调用 |
| `AnalyzeBatch` | **清除** | D4：无批量业务（唯一引用是测试 fake） |

## 2. RED（先钉后改）

```console
$ bash scripts/check_rpc_status.sh        # 清单改成终态后、代码未改前
PASS: A1 覆盖 43/43 条 RPC 均已在清单登记
PASS: A2 live 36 条 RPC 均有已登记且可校验的调用方证据
FAIL: A3 标记数 10 ≠ 清单推导的 0
FAIL: A4 终态规则 —— StreamMessages(archived 但 proto/chat.proto:65 邻域无 deprecated/reserved)
      AnalyzeBatch(deleted 但仍在 proto) MentalHealthHistory(...) MentalHealthTrigger(...)
      MentalHealthTrend(...) Logout(deleted 但仍在 proto) VerifySecurityAnswer(deleted 但仍在 proto)
PASS: A5 HTTP 回滚开关豁免声明在位
== FAIL ==   (exit=1)
```

> **RED 运行暴露了守卫自身的两个缺陷（已修，否则是"永真断言"）**：
> ① `deleted` 断言原写 `rpc[[:space:]]+${rpc}\(`，而 proto 实际是 `rpc Logout (`（名字与括号间有空格）⇒ **永不命中**；
> ② `archived` 断言原只看**整文件**是否含 `reserved/deprecated` ⇒ 文件里任何一处出现即通过，不指向被归档的那条。
> 现分别改为 `${rpc}[[:space:]]*\(` 与「按清单 `proto:line` 取该声明邻域（+0..+4 行）」。

## 3. GREEN（改完代码）

### 3.1 两条守卫

```console
$ bash scripts/check_rpc_status.sh
PASS: A1 覆盖 proto 40 条全部已登记；清单 43 条 = proto 40 + 已清除 3
PASS: A2 live 36 条 RPC 均有已登记且可校验的调用方证据
PASS: A3 标记数 0 与清单 marker_sites 之和一致
PASS: A4 终态规则（archived 带标注 / deleted 无残留）——当前 archived/deleted 行数：7
PASS: A5 HTTP 回滚开关豁免声明在位
-- undecided = 0（收口判据要求 0）--
== PASS ==   (exit=0)

$ bash scripts/check_dead_grpc_inventory.sh      # 旧守卫（D6：期望值改为从清单推导）
== 未接线清单守卫（E2E-31 T-1 · CL-02 起期望值由 rpc_status.tsv 推导）==
PASS: 未接线标记 0/0，清单与代码一致
(exit=0)
```

### 3.2 负向对照（3 组，证明断言不是永真式）

```console
# ① 把一条 deleted 行改成 live（proto 里已无该 RPC）
FAIL: A1 覆盖不一致 —— proto 缺登记：[] · 清单多出非 deleted 项：[ Logout(status=live)]
# ② 改错某条 live 行的证据符号
FAIL: A2 live 但调用方证据不足 —— SendMessage(调用点.../chat_handler.go无NoSuchMethod_XYZ( )
# ③ 临时插一个未登记的 E2E-31 标记
FAIL: 标记数 1 ≠ 预期 0
```

还原后两条守卫均 `exit=0`。

### 3.3 编译与测试（全模块）

```console
$ for m in shared chat-svc user-svc analytics-svc ai-svc assessment-svc web-bff; do (cd $m && go build ./...); done
→ 7/7 build OK
$ go vet ./...  → 7/7 OK
$ go test -count=1 ./...  → 7/7 PASS
$ cd emotion-llm-service && python -m pytest tests/unit -q  → 208 passed
$ python -m compileall grpc_server.py emotion_llm_pb2.py emotion_llm_pb2_grpc.py  → OK
```

**工具链一致性（复现原生成方式）**：`protoc 32.1` + `protoc-gen-go v1.36.11` + `protoc-gen-go-grpc 1.6.2`
+ Python `grpc_tools`（grpc 1.82.1，与提交的桩头 `GRPC_GENERATED_VERSION = '1.82.1'` 一致）。

## 4. 改动面清单（11 个源文件 + 10 个生成物）

| 类别 | 文件 | 改动 |
|---|---|---|
| proto | `proto/{chat,metric,user,emotion_llm}.proto` | 归档 4 条加 `option deprecated = true` + 依据注释；清除 3 条删 RPC 与消息 + 留痕注释 |
| 生成物 | `emotion-echo-shared/pkg/emotion{chat,analytics,user,llm}/*.pb.go`（8）+ `emotion-llm-service/emotion_llm_pb2{,_grpc}.py`（2） | 按原工具链重生成（仅预期增删） |
| server | `chat-svc/.../chat_server.go`、`analytics-svc/.../metric_server.go`、`user-svc/.../user_server.go`、`user-svc/.../server.go`（拦截器跳过清单） | 归档注释 / 删实现 / 删跳过项 |
| BFF | `downstream/{chat_grpc,user_grpc,user}.go` | 删 3 个死方法（含 HTTP 实现）+ 接口项 + 头注释订正 |
| 测试 | `ai-svc/.../grpc_analyzer_test.go`、`shared/pkg/grpcinterceptor/stream_test.go`、`user-svc/.../user_server_{sprint_e,security_answer}_test.go`、`web-bff/.../chat_grpc_test.go` | 删被测已删的用例 / stream 拦截器用例改用 `ChatCompletion` 承载 |
| python | `emotion-llm-service/grpc_server.py` | 删 `AnalyzeBatch` 实现 + 留痕注释 |

**测试覆盖未净损失**（逐条交代）：
- `Logout` 用例删除：被测 RPC 已不存在（D3），登出语义由 BFF 清 cookie 承担。
- `VerifySecurityAnswer`（按 userID）匿名用例删除：其契约（"匿名调用不被 userid 拦截器拒绝"）**仍由同文件 3 个 `…ByUsername` 用例覆盖**。
- stream 拦截器 4 个用例：**不是删除而是换载体**——从 `AnalyzeBatch` 改为 `ChatCompletion`（当前唯一 server-streaming RPC），断言（start/end 日志、`sent=N`、panic→Internal、超时快速失败）逐条保留。

## 5. 未做 / 待办（诚实声明）

- **T-24 CI 接线**：本文件撰写时新守卫已可独立跑；接入 `e2e-guards.yml` 后须复验"仍能红"（🟢 可做）。
- **T-21/T-23 全栈冒烟**：清除后全栈可起（Nacos `count:6`）+ §2.4 数据契约 smoke —— **🔴 需 dev 栈**，按契约 §3.1 排队在 A/B 之后，**未取证**。
- **T-22 接真端到端**：**N/A**（裁定后本阶段无"接真"条目，Phase 4 整体 N/A）。
- **T-20 来源计划迁移**：留到阶段收口时做。
- **`gofmt -l` 在本机对未改动文件同样报红**（CRLF 工作区）⇒ 本次以"只对改动文件看 `gofmt` 输出 + `git diff` 只含预期行"判定，未把它当门禁。
- **归档 ≠ 可调用**：`MentalHealthTrigger`/`Trend` 的 server 是**真实现**，`StreamMessages` 仍返 `Unimplemented`；归档只表示"proto 保留 + 明确不做 + 状态可查"，**不表示可用**。

## 6. T-24 实测：守卫接入 CI 后**真能红**（2026-10-14）

**为什么必须实测**：本仓 5 个 workflow 的触发条件都是 `push/PR → main` ⇒ **单纯推分支不会跑 CI**，
所以"接进 CI 后仍生效"只能靠一次真实的 PR 运行来证；否则就是"门禁只报不拦"（anti-patterns 之一）。

**做法**：临时分支 `test/cl-02-guard-red-proof`（从本轨 HEAD 切出）把一条 `live` 行的**证据符号**
改成不存在的名字（`NoSuchMethod_CI_PROOF`），开 **draft PR #218**（标注"勿合并"）触发 CI。

**结果**（PR #218 head `363023e`，`GET /commits/363023e/check-runs`）：

```console
completed    success    文档守卫总闸
completed    success    静态守卫（合并）
completed    failure    static-guards      <<< 只有它红
completed    success    web-test-frontend
completed    success    shared-test
completed    success    llm-service-pytest
completed    success    test (emotion-echo-{user,chat,assessment,web-bff,ai,analytics}-svc)   # 6 个全 success
```

`GET /check-runs/115676826454/annotations` 返回：`annotation_level=failure`，
`message="Process completed with exit code 1."`。

**证据链（如实说明强度）**：
- 该 PR 的**唯一改动**就是那一行 TSV；同一输入在本地跑守卫 **rc=1**（`FAIL: A2 live 但调用方证据不足`）；
- CI 上只有 `static-guards` 红、其余 11 个 job 全绿 ⇒ 失败**隔离在那条守卫步骤**，非连带故障；
- **未取到步骤级日志**（job log 需鉴权，本 token 无）⇒ 本行标注为"**job 级失败 + 隔离证据**"，
  **不宣称**"日志里逐字出现 `守卫 27/27 FAIL`"。
- 对照：本轨正式分支（head `4134a27`）的 `static-guards` = **success** ⇒ 同一守卫
  "清单正确则绿、清单错则红"，两侧都实测过。

**清理**：PR #218 已关闭（未合并）+ 源分支已删（本地 + 远端）。

