---

status: landed
superseded-by: stage-41-gozero-removal.md
landed-at: 2026-09-07
priority: medium
owner: TBD
created: 2026-09-07

related-adrs:
  - docs/architecture/decisions.md（决策 1：HTTP 框架 = Gin，不再用 go-zero）

related-docs:
  - docs/architecture/audit-2026-08-31.md（E-2 ADR 与代码矛盾 / E-3 go-zero 三版本并存）
  - docs/architecture/distributed.md（迁移表：conf→yaml.Unmarshal、logx→log/slog 标 "待迁移"）
  - docs/stages/stage-41-gozero-removal.md（实施 stage）

---

# Plan — go-zero 完全移除（conf /logx/rest 三零件清零）✅ Landed 2026-09-07

# Plan — go-zero 完全移除（conf /logx/rest 三零件清零）

## 一、结论

**go-zero 可以完全移除。** 2026-07-14 决策 1 已把 HTTP server 迁到 Gin，但迁移只做了一半：`core/conf`、`core/logx`、`rest`（仅一个类型别名）仍是硬依赖。经全仓代码调查：



1. go-zero 的核心能力（zrpc /breaker/limit /discov/stores/sqlx）**本来就没用**——gRPC 走原生 grpc-go、限流走 APISIX + shared 令牌桶、服务发现走 Nacos 自研封装、ORM 走 GORM；

2. 残留使用面**封闭且有限**（见 § 二清单），无第三方库反向依赖 go-zero（`go mod graph` 实测：需求方只有主模块与 shared）；

3. 替代物已部分存在：BFF 与 ai-svc 已有基于 `log/slog` 的 `internal/logging`，BFF `nacos_boot.go` 已直接使用 `gopkg.in/yaml.v3`。

本计划是决策 1 的收尾，落地后审计 E-2（ADR 与代码矛盾）、E-3（go-zero v1.6.0 /v1.10.2 /v1.10.3 三版本并存）同步关闭。

## 二、现状（与代码事实对齐，2026-09-07 实测）

### 2.1 core/conf —— 配置加载（6 个 Go 模块全部在用）



| 使用点                                 | 数量    | 证据                                                            |
| ----------------------------------- | ----- | ------------------------------------------------------------- |
| `conf.MustLoad(path, &c)` 启动加载      | 6 处   | 6 个服务 `main.go`（user/chat/ai/analytics/assessment/web-bff）    |
| `conf.Load`（返回 error）               | 1 处   | `emotion-echo-web-bff/internal/config/config_test.go:24`      |
| `conf.LoadConfigFromYamlBytes`      | 4 处   | `emotion-echo-ai-svc/internal/config/config_override_test.go` |
| `json:",default=X"` tag             | 145 处 | 6 份 `internal/config/config.go`（ai-svc 34、BFF 39 最多）          |
| `json:",optional"` tag              | 25 处  | 同上                                                            |
| slice 默认值 `default=["chat-events"]` | 2 处   | ai-svc /analytics-svc 的 `Kafka.Topics`                        |
| env /range/map 默认值                  | 0 处   | 未使用                                                           |

配套现状：



* 6 个服务都已有 `applyEnvOverrides` / `ApplyEnvOverrides`，在 MustLoad 之后用 OS env 覆盖 —— 因为 go-zero conf **不展开&#x20;**`${VAR:-default}`**&#x20;占位**（chat-svc main.go:40-44、Stage 26-Q 记录），且 list 字段不支持 env 占位（Kafka brokers 因此从 list 改成 CSV 字符串）。

* 配置文件：6 份 `etc/*.yaml`，字段名用大写风格（`Name:`/`Port:`，无 yaml tag）。

* 已被测试锁死的契约（迁移时必须保持绿）：


  * ai-svc `config_override_test.go`：yaml 省略 bool 字段 → 走 struct 默认值（`Kafka.Enabled=true`、`Nacos.Enabled=false`）；

  * analytics-svc `config_test.go`：yaml 加载后 `GroupID`/`Enabled` 默认值、Topics 显式给出；

  * BFF `config_test.go`：端口 8894、5 个下游默认地址非空、env 覆盖 / 空 env 不覆盖；

  * 各服务 `yaml_env_test.go`：yaml 非注释行不得含 `${` 占位（纯文本断言，与 conf 无关）。

### 2.2 core/logx —— 日志（goctl 骨架惯性）



| 使用点                                                     | 数量                                         | 证据                                                                                |
| ------------------------------------------------------- | ------------------------------------------ | --------------------------------------------------------------------------------- |
| Logic 结构体内嵌 `logx.Logger` + 构造器 `logx.WithContext(ctx)` | 30 个文件                                     | 5 个 svc 的 `internal/logic`（user 5 / chat 6 / ai 4 / analytics 10 / assessment 5）  |
| 真实日志输出调用                                                | **7 处，全部&#x20;**`l.Errorf`**，全在 chat-svc** | createconversationlogic ×2、sendmessagelogic ×3、deleteconversationlogic ×2         |
| 死代码                                                     | 1 处                                        | `analytics-svc/.../reports_trend_logic.go:91` `var _ = logx.WithContext`（嵌入后从未调用） |
| 特殊构造                                                    | 1 处                                        | `ai-svc/.../consumehandler.go:38` 用 `context.Background()`                        |

替代物已存在：`emotion-echo-web-bff/internal/logging/logger.go` 与 `emotion-echo-ai-svc/internal/logging/logger.go`（注释写明 clone 关系），均为 Go 1.21+ 标准库 `log/slog` 的 JSON 封装（`Init/Printf/Infof/Warnf/Errorf/Fatalf`，支持 `LOG_FORMAT`/`LOG_LEVEL`）。BFF main 层已经在用，logic 层是残留。

### 2.3 rest —— 仅一个类型别名



* 唯一 import：`emotion-echo-shared/pkg/middleware/jwt_auth.go:22`，用法是 `type RestMiddleware = rest.Middleware`（即 `func(http.HandlerFunc) http.HandlerFunc`）与 `AuthMiddleware() rest.Middleware`；

* 下游：`chat-svc/internal/middleware/auth.go` re-export 该类型，`auth_test.go:16` 有类型断言 `var _ sharedmw.RestMiddleware = mw`；

* **rest 版&#x20;**`AuthMiddleware()`**&#x20;全仓无运行时调用方**—— 各服务 main 实际挂的是 `GinAuthMiddleware()`（同目录 `gin_auth.go`，纯 net/http + Gin）。它是 go-zero rest server 时代的遗留适配层；

* shared 其余中间件（`gin_auth.go`、`gin_skywalking.go`、`limiter.go` 令牌桶）均不依赖 go-zero。

### 2.4 goctl 遗留物（生成器已废弃，产物还在）



* 6 份 `.api` 定义（5 个服务，assessment 目录有 2 份）：`user.api` / `chat.api` / `ai.api` / `analytics.api` / `assessment.api` / `emotion-echo-assessment-svc.api`；

* `internal/types/types.go` 文件头仍是 `Code generated by goctl. DO NOT EDIT. goctl 1.10.1`，但类型早已手写演进；

* 22 个 goctl 生成文件（17 个 logic + 5 个 `internal/svc/servicecontext.go`）文件头留 `Code scaffolded by goctl. Safe to edit.`；

* **构建链零引用**：全仓 Makefile / Dockerfile /scripts/compose / Helm 中 grep 不到 `goctl` 或 `.api` 消费。

### 2.5 依赖与版本



* 7 个 go.mod：shared `v1.6.0`、5 个业务 svc `v1.10.2`、BFF `v1.10.3`（审计 E-3）；全部 `go 1.26.1`；

* `go mod graph`（chat-svc 实测）：go-zero 的需求方只有主模块与 shared，无第三方反向依赖；go-zero v1.10.2 同时拖入 etcd client、k8s client、OpenTelemetry、mongo/redis driver、gopher-lua 等大批传递依赖，移除后 `go.sum` 预期明显瘦身（具体数以 tidy 后为准，不预设）。

## 三、目标与非目标

### 目标



1. 7 个 Go 模块 `go.mod` 不再 require `github.com/zeromicro/go-zero`，全仓 grep `zeromicro` 零命中（`legacy/` 除外）；

2. 配置加载、默认值、日志输出的**外部行为不变**：6 份 etc yaml 原样可用，现有 config /logic 测试全部保持绿；

3. 关闭审计 E-2 / E-3，决策 1 的 "部分失效" 标注移除；

4. 统一日志栈为 `log/slog`（下沉 shared，消除 BFF /ai-svc 双份 internal/logging 克隆）。

### 非目标



* 不改 HTTP 路由、handler 签名、业务逻辑与请求 / 响应 types 结构；

* 不引入 viper 等重型配置框架；不做 Nacos 配置热加载改造（另见 `nacos-enablement-dev.md`）；

* 不动 `legacy/` 目录；不借机重写 middleware（limiter /skywalking 保持原样）；

* 不调整 etc yaml 的字段命名风格（大写保留）。

## 四、技术方案

### 4.1 配置加载：shared/pkg/config 薄封装 + 代码默认值

新增 `emotion-echo-shared/pkg/config`（TDD，先写测试）：



```
// Load 读取 yaml → 先填默认值 → 再反序列化（yaml 值覆盖默认值，等价 go-zero default 语义）

func Load(path string, dst any, defaults func()) error

// MustLoad 同 Load，失败即 panic（复刻 go-zero Must 语义，供 main 使用）

func MustLoad(path string, dst any, defaults func())

// LoadBytes 等价 conf.LoadConfigFromYamlBytes（供测试）

func LoadBytes(b \[]byte, dst any, defaults func()) error
```

实现要点（约 60 行）：`os.ReadFile` → `defaults()` → `yaml.v3` Decoder；默认开启 `KnownFields(true)` 加严未知字段（go-zero 对拼写错误同样不友好，加严属于改进，若与现有 yaml 冲突则在 Phase 0 golden 测试中暴露并回退为宽松）。

每个服务把 tag 默认值迁移为 `SetDefaults()`：



```
func SetDefaults(c \*Config) {

&#x20;   c.Name = "emotion-echo-chat-svc"

&#x20;   c.Host = "0.0.0.0"

&#x20;   c.Port = 8890

&#x20;   c.Postgres.MaxOpenConns = 10

&#x20;   c.Kafka.BrokersCSV = "localhost:9092"

&#x20;   c.Kafka.GroupID = "chat-svc"

&#x20;   c.Kafka.Enabled = true

&#x20;   // ...145 处 default 逐一迁入；25 处 optional 直接删除 tag（yaml.v3 天然允许缺失）

}
```

main.go 改为 `config.MustLoad(*configFile, &c, func() { config.SetDefaults(&c) })`，其后的 `applyEnvOverrides` 顺序与逻辑不变。

**语义对照（必须在 Phase 0 用测试钉死）**：



| go-zero conf 行为                         | 新方案                                       | 备注                         |
| --------------------------------------- | ----------------------------------------- | -------------------------- |
| `default=X` 加载时填默认                      | SetDefaults 在 Unmarshal 前填零值字段            | yaml 显式值覆盖默认，顺序不能反         |
| `optional` 允许缺失                         | 删除 tag，天然允许                               | 25 处纯删除                    |
| 无 tag 字段必填，缺失报 `missing required field` | 不做全量必填；对关键字段（Port/DSN）写 `Validate()` 显式校验 | 行为差异，见 § 六风险 R1            |
| slice `default=["x"]`（且有保留字面引号怪癖）       | SetDefaults 直接赋 `[]string{"chat-events"}` | 仅 2 处，顺带消除怪癖               |
| `${VAR:-default}` 不展开                   | 维持现状：继续靠 applyEnvOverrides                | 行为不变                       |
| MustLoad 文件缺失 / 语法错即 panic              | MustLoad 复刻                               | main 启动语义不变                |
| 字段名大小写宽松匹配                              | yaml.v3 对导出字段同样大小写不敏感                     | Phase 0 golden 测试验证（假设 A2） |

**备选方案（不推荐，仅记录）**：引入 `github.com/creasty/defaults`，把 `json:",default=X"` 机械换成 `default:"X"`，迁移量最小；但该仓库 2026 年仍挂 "Looking for a new maintainer"，为 145 个标量默认值引入反射依赖与维护风险不划算。若 Phase 中发现手写 SetDefaults 工作量超预期，可再评估。

### 4.2 日志：slog 下沉 shared，logic 去嵌入



1. 把 BFF `internal/logging` 上移为 `shared/pkg/logging`（slog JSON，API 不变），补齐单测；BFF、ai-svc 的 internal/logging 改为对 shared 的 thin re-export 或直接换 import（避免一次性大 diff，可分两步）；

2. 30 个 logic 文件机械改造：



```
// before

type SendMessageLogic struct { logx.Logger; ctx context.Context; svcCtx \*svc.ServiceContext }

func NewSendMessageLogic(ctx context.Context, svcCtx \*svc.ServiceContext) \*SendMessageLogic {

&#x20;   return \&SendMessageLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}

}

l.Errorf("SendMessage persist err: %v", err)

// after

type SendMessageLogic struct { ctx context.Context; svcCtx \*svc.ServiceContext }

func NewSendMessageLogic(ctx context.Context, svcCtx \*svc.ServiceContext) \*SendMessageLogic {

&#x20;   return \&SendMessageLogic{ctx: ctx, svcCtx: svcCtx}

}

slog.ErrorContext(l.ctx, "SendMessage persist failed", "err", err)
```



1. chat-svc 的 7 处 `Errorf` 逐一改为 `slog.ErrorContext`（消息文本改为静态、err 进结构化字段，禁止 `%v` 拼 err）；

2. `reports_trend_logic.go:91` 的 `var _ = logx.WithContext` 死代码随嵌入一并删除；`consumehandler.go` 的 `context.Background()` 语义保留。

### 4.3 rest 类型清零

`shared/pkg/middleware/jwt_auth.go`：



```
// 删除 go-zero/rest import，类型自定义

type Middleware func(http.HandlerFunc) http.HandlerFunc

// 旧别名 RestMiddleware 保留一个版本周期并加 Deprecated 注释，或直接全局改名（仅 3 处引用）
```

同步改 `chat-svc/internal/middleware/auth.go` 返回类型与 `auth_test.go` 的类型断言。rest 版 `AuthMiddleware()` 无运行时调用方（假设 A5），本计划**只换类型不删函数**（删函数属于死代码清理，另开 PR 决策，避免混淆移除范围）。

### 4.4 goctl 遗留处置



* 6 份 `.api` 文件移动到 `legacy/goctl-apis/`（保留历史，不直接删）；

* `types.go` 文件头 `Code generated ... DO NOT EDIT` 改为普通文件头（类型已手写维护，"DO NOT EDIT" 注释本身就是失真文档）；

* 22 个 goctl 生成文件（17 logic + 5 servicecontext.go）的 `Code scaffolded by goctl` 文件头在各服务 PR 中顺手清除。

## 五、分阶段实施（每阶段一个 PR，严格 TDD）

> 分支命名 
>
> `refactor/<svc>-gozero-removal`
>
> ；每 PR 内按 Red→Green→Refactor 拆 commit；合并门槛：
>
> `go test ./...`
>
>  \+ 
>
> `go vet ./...`
>
>  全绿，涉及 chat/ai/analytics 的 PR 追加 AGENTS.md §2.4 数据契约 smoke。



| Phase | 范围             | 内容                                                                                                                                                       | 预估   |
| ----- | -------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------- | ---- |
| **0** | shared         | RED：pkg/config 与 pkg/logging 的单测（默认值填充顺序、yaml 覆盖、文件缺失 panic、LoadBytes、bool 省略走默认、slice 默认、日志 JSON 字段）→ GREEN 实现；用 6 份 etc yaml 写 golden 加载测试钉住大小写 / 字段映射 | 0.5d |
| **1** | shared         | jwt\_auth.go 去 rest 类型 + chat-svc adapter/test 同步；shared `go mod tidy` 删除 go-zero v1.6.0；shared 全测试绿（最小闭环，先解放底座）                                         | 0.5h |
| **2** | assessment-svc | 5 logic 去 logx + config SetDefaults + main 换 loader + 测试改造 + tidy                                                                                        | 1h   |
| **3** | user-svc       | 同上（5 logic）                                                                                                                                              | 1h   |
| **4** | chat-svc       | 6 logic、**7 处 Errorf 全部在此**、config、main；完成后跑 § 契约 1/2/6 smoke                                                                                            | 3h   |
| **5** | ai-svc         | 4 logic 嵌入 + consumehandler；internal/logging 收敛为 re-export shared；config（34 tag，最多之一）                                                                    | 3h   |
| **6** | analytics-svc  | 10 logic + reports\_trend 死代码 + config（含 MustLoad 测试改造）                                                                                                  | 3h   |
| **7** | web-bff        | conf.Load 测试改 shared/config；internal/logging 收敛 shared；tidy 删 v1.10.3                                                                                    | 2h   |
| **8** | 全仓             | `.api` 移入 legacy/goctl-apis/；types 与 22 个 goctl 文件头清理；全仓 `grep -r zeromicro --include=*.go .`（排除 legacy）零命中；7 模块 `go mod tidy` 后 `go build ./...` ×7     | 2h   |
| **9** | 文档             | decisions.md 决策 1 去 "部分失效" 标注并补记本计划落地；audit E-2/E-3 标关闭；distributed.md 迁移表 "待迁移" 改 "已完成"；本文件按规范迁入 legacy-plans/landed/                                   | 1h   |

顺序理由：先 shared（底座）→ 按 logic 文件数从小到大（assessment/user 练手验证模板）→ chat/ai/analytics 攻坚 → BFF 收口 → 清理与文档。每个业务 PR 独立可回滚，不跨服务夹带。

## 六、风险与对策



| #  | 风险                                                                                            | 影响     | 对策                                                                                           |
| -- | --------------------------------------------------------------------------------------------- | ------ | -------------------------------------------------------------------------------------------- |
| R1 | **必填语义丢失**：go-zero 对无 default/optional 的字段缺失报 `missing required field`，yaml.v3 不校验，漏配会以零值静默启动 | 中      | Phase 0 loader 提供 `Validate()` 钩子；每服务对 Port / DSN / GroupID 等关键字段补显式校验单测；保留现有默认值断言测试         |
| R2 | **字段映射差异**：yaml.v3 与 go-zero 的大小写 / 嵌套匹配若不完全一致，现有大写风格 yaml 可能加载出零值                            | 中      | Phase 0 用 6 份真实 etc yaml 做 golden 全字段快照；若失配再补 `yaml:"Name"` tag，不提前大面积加 tag                  |
| R3 | **bool 零值歧义**：SetDefaults→Unmarshal 顺序写反会导致 yaml 的 `false` 被默认值覆盖                             | 高（但易测） | ai-svc `TestConfig_YAMLFieldMapping` 已锁 "省略走默认" 契约，再补 "显式 false 覆盖默认 true" 用例（Kafka.Enabled） |
| R4 | **日志格式变化**：logx 文本格式 → slog JSON，若有日志采集按旧格式 grep 会失配                                          | 低      | BFF/ai-svc main 层早已是 slog JSON 输出，采集侧已适配；msg 静态化、err 字段名统一为 `err`                            |
| R5 | **大 diff 冲突**：35 个 logic 机械改动与并行分支冲突                                                          | 中      | 严格按服务拆 10 个 PR；logic 改动是纯机械删除嵌入，冲突易解；不与功能 PR 同批合并                                            |
| R6 | **传递依赖误删**：tidy 后发现某间接依赖实际被业务代码使用                                                             | 中      | 每 PR 都跑 `go build ./... + go test ./...`；go.sum 瘦身结果在 PR 描述列前后行数对比                           |
| R7 | creasty/defaults 备选的维护者风险                                                                     | 低      | 默认选手写 SetDefaults，不新增依赖                                                                      |

## 七、验收清单（DoD）



* [ ] 7 个模块 `go test ./...` 全绿，`go vet ./...` 无告警；

* [ ] 前端与 Python 服务不受影响（本计划零改动）；

* [ ] `grep -rn "zeromicro" --include=*.go --include=go.mod .`（排除 legacy/）零命中；

* [ ] 6 份 etc yaml 一字未改，服务可用原配置启动；

* [ ] chat/ai/analytics PR 附 §2.4 契约 smoke 结果（契约 1/2/6）；

* [ ] 7 份 go.mod 无 go-zero，`go mod graph | grep zeromicro` 零命中；

* [ ] decisions.md/audit /distributed.md 文档同步，本文件落地后归档 legacy-plans/landed/。

## 八、§A 上下文与假设



* A1：全部模块 `go 1.26.1`（go.mod 实测），`log/slog`（1.21+）与 yaml.v3 可用；

* A2：yaml.v3 对导出字段按名称大小写不敏感匹配，现有 `Name:/Port:` 大写风格 yaml 无需改名、无需补 tag——**Phase 0 golden 测试为该假设的证伪点，若不成立则改为补 yaml tag 方案，工作量增加约 2h**；

* A3：go-zero 无第三方反向依赖（chat-svc `go mod graph` 实测；其余模块在各自 Phase tidy 时复验）；

* A4：6 份 `.api`（5 服务，assessment 含 2 份）无任何工具链消费（Makefile/Dockerfile/scripts/compose/Helm grep 零命中）；

* A5：rest 版 `AuthMiddleware()` 无运行时调用方（全仓 grep：仅 chat-svc adapter 包装与其测试），本计划只换类型不删函数；

* A6：logx.WithContext 未承载 trace 透传 —— 链路追踪由 SkyWalking middleware 从 ctx 独立完成，移除 logx 不影响 trace；

* A7：`legacy/` 为归档目录，不在移除范围，其 go-zero/zap 使用保持原样。

## 九、调研依据（AGENTS.md 文档前置功课回填）

已读实现文件（≥3）：



1. `emotion-echo-chat-svc/main.go`（conf.MustLoad + applyEnvOverrides + Gin 装配全文）

2. `emotion-echo-chat-svc/internal/config/config.go`、`emotion-echo-ai-svc/internal/config/config.go`、`emotion-echo-web-bff/internal/config/config.go`（tag 全集：145 default / 25 optional / 2 slice）

3. `emotion-echo-chat-svc/internal/logic/sendmessagelogic.go`（logx 嵌入模式样板）

4. `emotion-echo-shared/pkg/middleware/jwt_auth.go`（rest.Middleware 唯一使用点）、`limiter.go`（确认不依赖 go-zero）

5. `emotion-echo-web-bff/internal/logging/logger.go`（slog 替代物）、`emotion-echo-web-bff/nacos_boot.go`（yaml.v3 使用先例）

6. `emotion-echo-chat-svc/internal/middleware/auth.go`（rest 类型 re-export 链）

已读测试文件（≥1）：`ai-svc/internal/config/config_override_test.go`、`analytics-svc/internal/config/config_test.go`、`web-bff/internal/config/config_test.go`、`chat-svc/internal/config/{config_test,yaml_env_test,go_mod_replace_test}.go`、`chat-svc/internal/middleware/auth_test.go`。

已查文档：`docs/architecture/decisions.md` 决策 1（含 2026-08-31 审计标注）、`docs/architecture/distributed.md` 迁移表、`docs/architecture/audit-2026-08-31.md` E-2/E-3、`docs/plans/README.md` 写入规范。

命令证据：全仓 `grep zeromicro/go-zero`（40 文件 import 清单）、`grep conf./logx./rest.` 使用面统计、`go mod why`、`go mod graph`（反向依赖）、构建链 `.api/goctl` 引用扫描、各服务 logic/logx 文件计数。

外部查证：`log/slog` 为 Go 1.21+ 标准库；`gopkg.in/yaml.v3` 默认忽略未知字段、`KnownFields(true)` 可加严；`github.com/creasty/defaults` 能力与维护状态（备选，不采用）。