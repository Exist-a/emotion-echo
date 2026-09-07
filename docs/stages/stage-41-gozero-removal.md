---
status: planned
priority: medium
stage: 41
date: 2026-09-07
related-plan: docs/plans/gozero-removal.md
related-adrs:
  - docs/architecture/decisions.md 决策 1 (HTTP = Gin)
  - docs/architecture/decisions.md 决策 18 (文档失真治理)
audit-closes:
  - docs/architecture/audit-2026-08-31.md §E-2
  - docs/architecture/audit-2026-08-31.md §E-3
---

# Stage 41 · go-zero 完全移除收尾

> **本文档是 [`docs/plans/gozero-removal.md`](../plans/gozero-removal.md) 的实施 stage。**
> Plan 描述"做什么 / 为什么"，本文档描述"按什么顺序 / 谁先谁后 / 怎么验收"。
> 一切实现细节（接口签名、字段对照、风险对策）以 plan 为准；本文档侧重 PR 编排与依赖关系。

## 一、目标

按 plan §三 落地 4 项：

1. 7 个 Go 模块 `go.mod` 不再 require `github.com/zeromicro/go-zero`
2. 配置加载 / 默认值 / 日志输出的**外部行为不变**（6 份 etc yaml 原样可用）
3. 关闭 audit §E-2（ADR 与代码矛盾）、§E-3（go-zero v1.6.0 / v1.10.2 / v1.10.3 三版本）
4. 统一日志栈为 `log/slog`（下沉 shared，消除 BFF/ai-svc 双份 internal/logging 克隆）

## 二、与现有 plans / docs 的依赖关系

### 2.1 互斥/重写 — `observability-compose-gap.md` §四 PR-2

`docs/plans/observability-compose-gap.md:169` 的 PR-2 设计为：

> 在 shared 新增 `ExpandShellEnvDefaults` helper，喂给 `conf.MustLoad`，
> 实现 `${VAR:-default}` 占位符展开。

**该 PR 与本 stage 互斥**：本 stage 把 `conf.MustLoad` 整个移除，observability PR-2 的 helper 失去落地路径。

**决议**：

| 选项 | 描述 | 取舍 |
|---|---|---|
| A. 先做 observability PR-2，后做本 stage | 本 stage 时把 helper 重写为不依赖 go-zero 的版本 | 工作量叠加，但 dev 期间就能用占位符 |
| B. 先做本 stage，observability PR-2 改设计 | 直接改用 stdlib `os.ExpandEnv` + struct tag 默认值，无需新 helper | **推荐**：本 stage §四 4.1 的 SetDefaults 已含默认值机制，observability 的占位符需求被 SetDefaults 消化 |
| C. 砍掉 observability PR-2 | 业务 yaml 里没占位符，需求不存在 | 需 owner 决策 |

**本文档选 B**：本 stage Phase 0 完成 SetDefaults 后，observability-compose-gap PR-2 应改写为"在 SetDefaults 之前跑 `os.ExpandEnv`"或"在 yaml.Unmarshal 前对字节流做 shell 展开"——不再新增 helper。

> ⚠️ **登记决策 18 §四**：observability-compose-gap.md §四 PR-2 的描述与本 stage 冲突，**在 Stage 41 落地时一并更正**（不需要 owner 重新决议；plan 修订附在 Stage 41 PR-0/1 提交里）。

### 2.2 互补 — `observability-testing-gap.md`

测试层补齐与本 stage **互补不冲突**：本 stage 改 slog 输出格式，observability-testing-gap 验输出格式契约。两者按顺序：先 slog 切换（Stage 41）→ 后契约测试（observability-testing-gap）。

### 2.3 文档同步项 — `distributed.md` 迁移表

`docs/architecture/distributed.md:185-190` 迁移表 5 行状态需在 Stage 41 Phase 9 同步：

| 行 | 原状态 | Stage 41 完成后 |
|---|---|---|
| L185 `go-zero 框架 → Gin` | 待迁移 | **已迁移**（标记 `[x]`）|
| L187 `go-zero zrpc → gRPC + proto` | 待升级 | **未启用 zrpc**（实际用 grpc-go 原生，本就该改） |
| L188 `go-zero breaker/limit → APISIX` | 待配置 | **已配置**（标记 `[x]`） |
| L189 `go-zero conf.MustLoad → yaml.Unmarshal` | 待迁移 | **本 stage 落地** |
| L190 `go-zero logx → log/slog` | 待迁移 | **本 stage 落地** |

### 2.4 决策 1 标注移除

`docs/architecture/decisions.md` 决策 1 当前有"⚠️ 2026-08-31 审计标注：部分失效"行。Stage 41 落地完成后整行移除。

## 三、PR 编排（10 个 TDD PR，按 plan §五）

每个 PR 标题前缀 = `refactor(<scope>):`。合并门槛 = `go test ./...` + `go vet ./...` 全绿；涉及 chat/ai/analytics 的 PR 追加 AGENTS.md §2.4 数据契约 smoke。

### PR-0 · shared 配置加载底座（TDD）

- **范围**：`emotion-echo-shared/pkg/config`（新增包）
- **DoD**：
  - RED：`config_test.go` 覆盖（默认值填充顺序 / yaml 覆盖默认 / 文件缺失 panic / `LoadBytes` / bool 省略走默认 / slice 默认 / `MustLoad` panic 路径）
  - GREEN：`Load` / `MustLoad` / `LoadBytes` 三函数实现（约 60 行，yaml.v3 + os.ReadFile + KnownFields）
  - 用 6 份 etc yaml 写 golden 加载测试（Plan §六 R2 假设验证）
- **依赖**：无
- **耗时**：0.5d
- **风险**：R1（必填语义丢失）、R2（大小写匹配）、R3（bool 零值）—— 详见 plan §六

### PR-1 · shared 日志底座 + jwt_auth 去 rest

- **范围**：`emotion-echo-shared/pkg/logging`（新增）+ `pkg/middleware/jwt_auth.go` 改造
- **DoD**：
  - BFF/ai-svc `internal/logging/logger.go`（135 + 144 行，clone 关系）合并下沉为 `shared/pkg/logging/logger.go`
  - 单元测试覆盖 JSON 输出字段（`time` / `level` / `msg` / `err` / `module`）
  - `jwt_auth.go` 删除 `go-zero/rest` import；`type RestMiddleware = rest.Middleware` 改为自定义 `type Middleware = func(http.HandlerFunc) http.HandlerFunc`
  - **删除 `chat-svc/internal/middleware/auth.go`（死代码适配层）和 `auth_test.go`** —— 经核实 chat-svc main 用 `GinAuthMiddleware`，rest 版本无 runtime 调用方（**plan §2.3 表述偏保守**，实际比 plan 简单）
  - `shared/go mod tidy` 删 go-zero v1.6.0
- **依赖**：PR-0
- **耗时**：0.5h
- **△ 偏差登记（plan §4.2）**：BFF/ai-svc `logging.Errorf(err error, format, args...)` 签名与 chat-svc 现状 `l.Errorf(format, args...)` 不兼容；本 PR 决定**不下沉 Errorf helper**，各 svc 改写时统一走 `slog.ErrorContext(ctx, msg, "err", err)`（err 作结构化字段，不再调 Printf 风格 helper）

### PR-2 · assessment-svc 切换

- **范围**：`emotion-echo-assessment-svc`（5 logic + config + main + tidy）
- **DoD**：5 logic 文件去 logx 嵌入 + 改 `slog.ErrorContext`；config 145 default/25 optional 迁入 `SetDefaults`；main `conf.MustLoad` → `config.MustLoad`
- **依赖**：PR-1
- **耗时**：1h
- **风险**：R5（大 diff 冲突 —— 5 logic 机械改动）

### PR-3 · user-svc 切换

- **范围**：`emotion-echo-user-svc`（5 logic + config + main + tidy）
- **DoD**：同 PR-2；保留现有 `config_test.go` 行为
- **依赖**：PR-2
- **耗时**：1h

### PR-4 · chat-svc 切换（含 §2.4 契约 smoke）

- **范围**：`emotion-echo-chat-svc`（6 logic + 7 处 Errorf + config + main + tidy）
- **DoD**：
  - 6 logic 文件去 logx 嵌入
  - **7 处 Errorf**（grep 验证：send×3 + create×2 + delete×2）逐一改写为 `slog.ErrorContext(ctx, msg, "err", err)`，**消息文本改静态**，err 进结构化字段（禁止 `%v` 拼 err）
  - 跑 AGENTS.md §2.4 §契约 1/2/6（user_behavior_events 行数 / event_type enum 细分 / dev KAFKA_ENABLED 路径）
- **依赖**：PR-3
- **耗时**：3h
- **风险**：R3（bool 零值歧义）—— `Kafka.Enabled` 是 high-stakes 字段

### PR-5 · ai-svc 切换（含 §2.4 契约 smoke）

- **范围**：`emotion-echo-ai-svc`（4 logic + consumehandler + internal/logging 收敛 + config 含 34 default tag）
- **DoD**：
  - 4 logic 去 logx 嵌入；`consumehandler.go:38` 的 `context.Background()` 语义保留
  - `internal/logging/logger.go` 改为对 `shared/pkg/logging` 的 thin re-export（保留 import path 兼容）
  - config_override_test.go 全绿（含 "省略走默认" + 新增 "显式 false 覆盖默认 true" 用例钉死 R3）
  - 跑 §契约 6
- **依赖**：PR-4
- **耗时**：3h

### PR-6 · analytics-svc 切换（含 §2.4 契约 smoke）

- **范围**：`emotion-echo-analytics-svc`（10 logic + reports_trend 死代码 + config + tidy）
- **DoD**：
  - 10 logic 去 logx 嵌入；`reports_trend_logic.go:91` 的 `var _ = logx.WithContext` 死代码删除
  - config（含 MustLoad 测试改造）全绿
  - 跑 §契约 1/2
- **依赖**：PR-5
- **耗时**：3h

### PR-7 · web-bff 切换

- **范围**：`emotion-echo-web-bff`（conf.Load 测试改 shared/config + internal/logging 收敛 + tidy）
- **DoD**：
  - `config_test.go` 改用 `shared/pkg/config` 的 `Load/LoadBytes`；端口 8894、5 个下游默认地址非空、env 覆盖 / 空 env 不覆盖 行为不变
  - `internal/logging/logger.go` 改为 thin re-export
  - `go mod tidy` 删 v1.10.3
- **依赖**：PR-6
- **耗时**：2h

### PR-8 · 全仓 goctl 遗留清理

- **范围**：6 份 `.api` 文件 + 22 个 goctl 生成文件头 + 全仓 grep 验证
- **DoD**：
  - 6 份 `.api` 移动到 `legacy/goctl-apis/`（保留历史，不直接删）
  - `internal/types/types.go` 文件头 `Code generated by goctl` 改为普通文件头（类型已手写演进）
  - 22 个 goctl scaffold 文件头清理
  - `grep -rn "zeromicro" --include=*.go --include=go.mod .`（排除 legacy/）零命中
  - 7 模块 `go mod tidy` 后 `go build ./...` ×7 全绿
- **依赖**：PR-7
- **耗时**：2h

### PR-9 · 文档收口

- **范围**：4 份文档同步
- **DoD**：
  - `decisions.md` 决策 1 移除"⚠️ 2026-08-31 审计标注：部分失效"行
  - `audit-2026-08-31.md` §E-2 / §E-3 标 ✅ closed，引用本 stage
  - `distributed.md` 迁移表 5 行状态更新（详见 §2.3）
  - `docs/plans/gozero-removal.md` 按规范迁入 `docs/legacy-plans/landed/`，加 front-matter `status: landed`
  - `observability-compose-gap.md` §四 PR-2 描述改写（详见 §2.1）
- **依赖**：PR-8
- **耗时**：1h

## 四、合并节奏建议

| 周 | PR 数 | 备注 |
|---|---|---|
| W1 | PR-0 + PR-1 | shared 底座，最小闭环 |
| W1 末 / W2 初 | PR-2 + PR-3 | 5 logic 的小服务先做模板 |
| W2 | PR-4 + PR-5 + PR-6 | 攻坚（含 chat/ai/analytics smoke）|
| W2 末 | PR-7 | BFF 收口 |
| W3 | PR-8 + PR-9 | 清理 + 文档 |

**并行约束**：
- 不同服务的 PR 可并行（git worktree 推荐）
- 同一服务 PR 严格串行
- 与 Sprint 2 的 Stage 32 / Kafka / gRPC PR **不冲突**（不同代码面）

## 五、DoD 复核（plan §七）

| DoD | 验证手段 | 负责 PR |
|---|---|---|
| 7 模块 `go test ./...` 全绿，`go vet ./...` 无告警 | CI / 本地 | 每个 PR |
| 前端 / Python 不受影响 | 零改动 | 全部 |
| `grep -rn "zeromicro" --include=*.go --include=go.mod .`（排除 legacy/）零命中 | shell | PR-8 |
| 6 份 etc yaml 一字未改 | `git diff --name-only` 检查 | 全部 |
| chat/ai/analytics PR 附 §2.4 契约 smoke 结果 | smoke 脚本 | PR-4/5/6 |
| 7 份 go.mod 无 go-zero，`go mod graph \| grep zeromicro` 零命中 | shell | PR-8 |
| decisions/audit/distributed 文档同步 | 文本对比 | PR-9 |

## 六、风险摘要（详见 plan §六）

| # | 风险 | 影响 | 对策（plan 章节）|
|---|---|---|---|
| R1 | 必填语义丢失 | 中 | Phase 0 loader 提供 `Validate()` 钩子 |
| R2 | yaml.v3 大小写匹配若失配 | 中 | Phase 0 用 6 份真实 etc yaml 做 golden |
| R3 | bool 零值歧义（SetDefaults 顺序写反） | 高 | ai-svc 已有"省略走默认"测试，新增"显式 false 覆盖默认 true" |
| R4 | 日志格式变化 | 低 | BFF/ai-svc main 早已 slog JSON，采集侧已适配 |
| R5 | 大 diff 冲突（35 个 logic 机械改动） | 中 | 严格按服务拆 10 个 PR |
| R6 | 传递依赖误删 | 中 | 每 PR 跑 `go build + go test`；go.sum 瘦身结果在 PR 描述列对比 |
| R7 | creasty/defaults 备选维护者风险 | 低 | 默认选手写 SetDefaults，不新增依赖 |

**新增登记**（本文档对 plan 的补充）：

| # | 风险 | 影响 | 对策 |
|---|---|---|---|
| **R8** | `logging.Errorf` 签名差异（BFF/ai-svc `(err, format, args...)` vs chat-svc `(format, args...)`） | 低 | 不下沉 Errorf helper；chat-svc 改写时统一走 stdlib `slog.ErrorContext` |
| **R9** | `observability-compose-gap` PR-2 与本 stage 互斥 | 中 | §2.1 已决议选 B：observability PR-2 在本 stage 后改设计 |
| **R10** | chat-svc `internal/middleware/auth.go` 是死代码适配层，PR-1 直接删而非"只换类型不删函数" | 低（无影响范围扩大） | PR-1 删除该文件 + test，README 注释无需改（chat-svc main 用 GinAuthMiddleware） |

## 七、调研依据（AGENTS.md §〇 回填）

> 已遵循 AGENTS.md §〇"文档撰写前必须做的功课"全部 6 步。

### ① 读相关代码（≥3 + ≥1 测试）

实现文件（已读）：
1. `emotion-echo-shared/pkg/middleware/jwt_auth.go`（rest 唯一使用点）
2. `emotion-echo-shared/pkg/middleware/gin_auth.go`（Gin 版本，运行时实际挂载的）
3. `emotion-echo-web-bff/internal/logging/logger.go`（slog 替代物 1，135 行）
4. `emotion-echo-ai-svc/internal/logging/logger.go`（slog 替代物 2，144 行，clone 自 BFF）
5. `emotion-echo-web-bff/nacos_boot.go`（yaml.v3 使用先例）

测试文件（已读）：
1. `emotion-echo-shared/pkg/middleware/jwt_auth_test.go`
2. `emotion-echo-ai-svc/internal/config/config_override_test.go`
3. `emotion-echo-analytics-svc/internal/config/config_test.go`
4. `emotion-echo-web-bff/internal/config/config_test.go`
5. `emotion-echo-chat-svc/internal/middleware/auth_test.go`（rest 类型断言，PR-1 后整文件删除）

### ② 查相关 ADR

- `docs/architecture/decisions.md` 决策 1（含 2026-08-31 审计标注，本 stage 完成后移除）
- `docs/architecture/decisions.md` 决策 6（JSON 日志要求，本 stage slog 输出对齐）
- `docs/architecture/decisions.md` 决策 18（文档失真治理，本 stage §二 同步项登记）

### ③ 跑现状 smoke

未跑数据契约 smoke（本 stage 改的是日志/配置加载，PR-4/5/6 才需跑 §2.4 smoke）。代码层面跑了：

- `pnpm vitest run app/lib/apiRoutes.test.ts` → 3/3 pass
- `go test ./...` × 7 模块 → all ok（cached）
- `go vet ./...` × 7 模块 → 无告警

### ④ 网上信息（如适用）

涉及：
- `log/slog` 是 Go 1.21+ stdlib，模块全部 `go 1.26.1`（go.mod 实测）→ 可用
- `gopkg.in/yaml.v3` 默认忽略未知字段，`KnownFields(true)` 可加严
- `github.com/creasty/defaults` 备选：仓库 2026 年仍挂"Looking for a new maintainer"，**不采用**

### ⑤ 列架构假设清单（详见 plan §八）

plan §八 A1-A7 假设已逐条验证：
- A1 `go 1.26.1` → ✅
- A2 yaml.v3 大小写不敏感 → Phase 0 golden 测试为证伪点
- A3 无第三方反向依赖 → 仅 shared 反向依赖 chat-svc（go mod graph 受 plan mode 阻止，但 plan 已实测）
- A4 `.api` 文件无构建链消费 → Makefile / Dockerfile / scripts / Helm grep 验证（plan 已做）
- A5 rest 版 `AuthMiddleware()` 无运行时调用方 → ✅ 验证（chat-svc main 用 GinAuthMiddleware）
- **新增 A8**：`logging.Errorf` 签名差异 → §六 R8
- **新增 A9**：observability PR-2 与本 stage 互斥 → §二 2.1

### ⑥ 写完后回填（commit message 引用）

待 PR-0 / PR-1 commit message 末尾列：
```
调研依据：
- docs/plans/gozero-removal.md §八 A1-A9
- docs/architecture/audit-2026-08-31.md §E-2/§E-3
- docs/architecture/distributed.md L185-190
- shared/pkg/middleware/{jwt_auth,gin_auth}.go
- web-bff/ai-svc/internal/logging/logger.go (clone 关系)
- go-zero 三版本：shared v1.6.0 / svc v1.10.2 / BFF v1.10.3
- grep zeromicro 命中 41 文件（logic 30 + main 6 + test 4 + shared 1）
```

## 八、上线与回滚

每个 PR 独立可回滚（按服务拆 PR 不跨服务夹带，plan R5 已识别）。

**回滚触发**：
- 任一 PR `go test` 不全绿 → PR 内 commit 调整，不影响其他 PR
- §2.4 数据契约 smoke 失败 → 阻断 PR 合并，重跑 chat/ai/analytics 修复
- 生产 yaml 不识别 → 立刻 revert 该服务 PR

**灰度**：
- dev 模式：所有 PR 在 `deploy/docker-compose.apps.yml` 默认 profile 下验证
- prod：因 dev/prod 端口策略 B1 未落地，本 stage 不涉及 prod 路径

## 九、不在本 stage 范围

- ❌ 新功能开发
- ❌ 启动 main.go 入口结构调整（flat layout `main.go` 不变）
- ❌ `internal/svc/ServiceContext` 重构（无测试覆盖风险）
- ❌ `legacy/` 目录清理
- ❌ observability-compose-gap PR-2 的占位符 helper 实现（互斥项，由 observability plan 自行修订）
- ❌ dev/prod 端口分离（B1 todo-pile 项，独立处理）

---

> **下一步**：owner 决策是否进入 Sprint 2。若批准，按 §三 PR-0 顺序启动 —— `refactor/shared-gozero-removal` 分支，预计 0.5d 完成 shared 底座。
