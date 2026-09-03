# AGENTS.md — Emotion-Echo 开发协作约定

> 本文件是 **约束文件**，对未来所有 AI / 人类 Agent 的代码改动都有约束力。  
> 当规则冲突时：**业务类型强约束优先于本文件**，但工程、测试、提交约定一律以本文件为准。

---

## 〇、第一性原则（必读）

### 🔴 ALL CODE IS TDD 🌱🔴 🟢 ♻️

**从此刻起，对本项目任何一行新代码（含 AI 自动生成 / 人类提交），都必须先写测试。**  
这一条是项目硬规则，没有例外。

#### 什么是 TDD（在我们这个上下文）
TDD = **Red → Green → Refactor** 三个动作的严格循环：

| 阶段 | 必须做什么 | 禁止做什么 |
|------|----------|----------|
| 🔴 RED | 写一个**会失败**的测试，描述"代码应该做什么" | 写实现 |
| 🟢 GREEN | 写**最少的**实现让测试通过 | 写超出测试范围的代码 |
| ♻️ REFACTOR | 改进实现（命名 / 结构 / 抽象），保持测试通过 | 改测试的行为 |

#### 何时算违反 TDD
- ❌ 写完 `service.go` 再补 `service_test.go`（这是"测试后置"，不是 TDD）
- ❌ 让 Copilot/AI 一次性吐完整段实现再补测试（违反 Red-Green 节奏）
- ❌ 测试只验证 happy path，不验证边界与失败
- ❌ 测试需要网络/磁盘/数据库才能跑（必须便于 `go test ./...` 一键跑过）
- ❌ 测试运行超过 5 秒不分层

### 📚 文档撰写前必须做的功课（AI / 人类通用）

**背景**：本项目长期受 AI 写错文档之害——典型症状：
- roadmap 写"chat-svc 加 devFallbackRepo 同步写库"，**与实际架构（outbox + Kafka + consumer）不符**
- ADR 写"HTTP 框架 = Gin（不再用 go-zero）"，**代码却硬依赖 go-zero**
- 决策 2（APISIX）被退役又被复职，**状态未在文档里同步**

**硬规则**：写任何文档（roadmap / ADR / stage landing / plan / docs/plans/）之前，必须先做以下功课，**未做禁止动笔**：

| 步骤 | 必须做什么 | 输出 |
|------|----------|------|
| ① 读相关代码 | 用 Read / Grep 工具读至少 3 个关键实现文件 + 1 个测试文件，列文件名 | "已读：emotion-echo-chat-svc/internal/events/events.go 等 N 个文件" |
| ② 读相关 ADR | 用 Grep 搜现有 ADR / stage / decisions.md，看是否有相关决策已被记录 | "已查：ADR-XX §X，stage-Y §Z" |
| ③ 跑现状 smoke（如适用） | 涉及业务契约的文档必须先跑 [scripts/smoke_data_layer.py](/scripts/smoke_data_layer.py) | smoke 输出截图 / 退出码 |
| ④ 网上信息（如涉及外部依赖） | 涉及 Kafka / Postgres / 第三方库版本号等，必须 WebFetch / WebSearch 官方文档 | 引用 URL |
| ⑤ 列架构假设清单 | 在文档 §A "上下文" 写"本文假设 X / Y / Z"，与现状对比 | 假设清单 |
| ⑥ 写完后回填 | commit message 末尾列 "调研依据：文件 A、文件 B、ADR-XX §Y、smoke 输出 N 项 FAIL" | commit 末尾 |

**违反此规则的典型后果**（实际发生过的）：
- A1 修复方向定错 → 走 Kafka 配置而不是 chat-svc dev fallback
- A4 修 GRANT 但视图根本没建 → 白修
- ADR 决策与代码矛盾 → 误导后续 PR

**AI 自检清单**（每次写文档前在内部确认）：
1. 我读了实际代码文件吗？还是只在脑子里"以为"架构是这样？
2. 我查了所有相关 ADR / stage 文档吗？还是只看了 roadmap 顶部？
3. 我跑过 smoke 看现状吗？还是基于"上次跑过 16/16 绿"的旧印象？
4. 我引用的版本号 / 配置名是从代码里 grep 出来的，还是凭记忆写的？
5. 如果文档是修另一份文档的修订，我在 commit message 引用原文档了吗？

---

## 一、测试栈与工具

### 1.1 Go（emotion-echo-gin / go-zero 后续项目）

**统一使用 `stretchr/testify`**（`assert`、`require`、`suite`、`mock`）。  
风格参照 `t.Run` 子测试 + 表驱动 + setup/teardown。

| 工具 | 用途 | 引入时间 |
|------|------|----------|
| `testing` | 标准库基础 | 已有 |
| `github.com/stretchr/testify/assert` | 友好断言 | **本约定生效后必加** |
| `github.com/stretchr/testify/require` | 失败立刻终止 | 同上 |
| `github.com/stretchr/testify/mock` | 接口 mock | 仅在需要模拟外部组件时引入 |
| `github.com/alicebob/miniredis/v2` | Redis 单元测试 | 引入 Redis/分布式组件时 |
| `github.com/IBM/sarama` | Kafka 客户端（含 mock broker） | 引入 Kafka 时 |

#### 文件与目录

```
xxx.go            ← 实现
xxx_test.go       ← 单元测试（同包）
xxx_integration_test.go  ← 集成测试（build tag: //go:build integration）
mock_xxx_test.go  ← 仅 mock 文件时使用，平时 inline 即可
```

> 集成测试默认不参与 `go test ./...`，需 `go test -tags integration ./...`。

#### 命名约定

| 元素 | 风格 | 示例 |
|------|------|------|
| 测试函数 | `TestXxx_MethodName_Scenario` | `TestProducer_Publish_ReturnsError_WhenBrokerDown` |
| 子测试 | `t.Run("场景名", func(t *testing.T) {...})` | `"return error on empty topic"` |
| 表驱动切片名 | `tests` / `cases` | `tests := []struct{...}{}` |
| Fixture 包 | `testdata/` | 静态文件 / golden data |

### 1.2 Vue / Nuxt（emotion-echo-front）

**统一使用 Vitest + Vue Test Utils + Pinia Testing**。  
测试位于组件同目录 `__tests__/` 或同名 `.spec.ts`。

```ts
import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import MyComponent from './MyComponent.vue'

describe('MyComponent', () => {
  it('renders greeting when prop is set', () => {
    const wrapper = mount(MyComponent, { props: { name: 'A' } })
    expect(wrapper.text()).toContain('Hello A')
  })
})
```

E2E 用 **Playwright**（与 Nuxt 天然集成）。

### 1.3 Python（AI 服务）

**统一使用 pytest + pytest-asyncio + httpx**。  
FastAPI 用 `TestClient` / `AsyncClient` 测路由；算法逻辑单独 unit test。

---

## 二、TDD 工作流（强制）

### 2.1 提交流程

```
1. 先跑测试，确认当前为绿
   go test ./...          # 必须绿
   
2. 写失败测试（RED）
   go test ./pkg/foo     # 必须红
   git diff               # 看测试文件改了就行
   git commit -m "test: add failing test for Foo.Bar"
   
3. 写最小实现（GREEN）
   go test ./pkg/foo     # 必须转绿
   git commit -m "feat: implement Foo.Bar to satisfy test"
   
4. 重构（REFACTOR）
   go test ./...         # 必须保持绿
   git commit -m "refactor: simplify Foo.Bar logic"
```

### 2.2 分支与 PR

| 元素 | 约定 |
|------|------|
| 分支 | `feat/<scope>-<desc>` / `fix/<scope>-<desc>` / `test/<scope>-<desc>` |
| Commit 前缀 | `feat:` / `fix:` / `test:` / `refactor:` / `docs:` / `chore:` |
| 单 PR 范围 | 一个 TDD 循环（一个或一组相关测试 + 它们的实现） |
| 合并前 | `go test ./...` + `go vet ./...` + (前端) `npm run lint` **+ §2.4 数据契约 smoke 全绿** |

### 2.3 覆盖率底线

| 类型 | 底线 |
|------|------|
| 核心业务包（service、handler、repository） | 80% |
| pkg 工具包 | 90% |
| 三方适配层（database、messaging、skywalking 钩子） | 70%（因依赖真实外部组件） |

### 2.4 业务数据契约验收清单（合并前必跑）

> **目的**：smoke 只验"管道通不通"（HTTP 200、key 存在），**不验"数据对不对"**。
> 本节列出的数据契约必须在 dev 模式（`docker compose -f deploy/docker-compose.apps.yml up -d`）下全绿才允许合并。

**触发场景**：PR 涉及以下任一范围必须跑本清单：

| 范围 | 必须跑的契约 |
|------|------------|
| 修改 chat-svc 事件发布链 | §契约 1（user_behavior_events 行数） + §契约 2（event_type enum 细分） |
| 修改 analytics-svc 写入或 SQL | §契约 3（analytics_reader 视图可读） |
| 修改 BFF 报表端点 | §契约 4（chartData 实际有数据） |
| 修改 schema / migration / GRANT | §契约 5（schema 与写入端一致性） |
| dev 模式相关改动 | §契约 6（KAFKA_ENABLED=false 路径不空跑） |

**契约清单**：

```
§契约 1  user_behavior_events 行数 = 业务事件数
        psql -c "SELECT COUNT(*) FROM emotion_echo_analytics.user_behavior_events"
        断言 ≥ 最近 1 分钟 message.created + conversation.created + conversation.closed 数
        抓 A1（dev 模式 outbox 永远 pending）类 bug

§契约 2  event_type enum 细分
        psql -c "SELECT event_type, COUNT(*) FROM ... GROUP BY 1"
        断言出现 ≥ 2 种 event_type（不能全 'conversation'）
        抓 A3（conversation.created/closed 都映成 'conversation'）类 bug

§契约 3  analytics_reader role 能查所有 *_v 视图
        psql -U analytics_reader -c "SELECT * FROM daily_emotion_by_modality_v LIMIT 1"
        断言无 permission denied
        抓 A4（GRANT 缺失）类 bug

§契约 4  /api/v1/reports/daily 返回 summary 非空 + chartData.length > 0
        curl -H "Authorization: Bearer $TOKEN" .../reports/daily?user_id=1
        断言 response.data.summary != "" && emotionDistribution.length > 0
        抓 G4（Kafka off 时情绪分析无数据）类 bug

§契约 5  schema 与写入端一致性
        对每个 VARCHAR(32) NOT NULL 枚举列，至少 1 个 integration test 断言写入值 ∈ enum 集合
        抓"SQL DDL 定义正确但写入端用错值"类 bug（如 event_type / target 列）

§契约 6  KAFKA_ENABLED=false 路径不空跑
        docker compose -f deploy/docker-compose.apps.yml -f deploy/docker-compose.infra.yml \
          up -d 启动 → 触发业务事件 → 等 30s → 断言 §契约 1+2 通过
        抓"dev 模式走通但 prod 走通是巧合"类 bug（如 outbox publisher=nil 永远失败）
```

**smoke 脚本**：`scripts/smoke_data_layer.py`（待建），按本清单跑全绿才算合并。

**为什么必须写死**：Stage 36-FU closure 报告 §三 smoke 16/16 全绿，但 dev 模式 4 dashboard `chartData.length === 0`、event_type 全 'conversation'、analytics_reader 视图读不出——**单纯 HTTP 200 smoke 永远抓不到**这些契约 bug。本节把"数据对不对"列入硬门槛，强制 PR 提交方自己先验证。



---

## 三、可测试性设计原则

### 3.1 依赖反转（必须）

所有跨包/跨外部组件的依赖（DB、Redis、Kafka、HTTP 客户端），**必须**通过**接口**注入：

```go
// 反例：直接用具体实现，无法 mock
func NewService() *Service {
    db, _ := gorm.Open(...)
    return &Service{db: db}
}

// 正例：依赖接口
type Service struct {
    userRepo repository.UserRepository  // 接口！
}
```

### 3.2 时钟、UUID、随机数

**必须**通过接口暴露，禁止 `time.Now()` / `uuid.New()` 直接调用：

```go
type Clock interface { Now() time.Time }
type IDGen interface { New() string }
```

便于在测试里固定时间戳、断言 ID。

### 3.3 副作用与异步

- DB/Redis/Kafka 等副作用 → 必须用 mock 接口 + 测试替身
- 异步逻辑 → 显式注入 `done` channel 或断言回调被调用
- 永不睡眠 `time.Sleep` —— 测试用 `clock.Step(1*time.Second)` 或 wait loop

---

## 四、禁止事项

| ❌ 禁止 | 原因 |
|--------|------|
| 提交没测试的业务代码 | 违反本约定第一性原则 |
| 在测试里调真实 DB / Redis / Kafka | 测试不可重现 |
| 用 `t.Skip` 跳过写不出的测试 | 跳过即承认失败 |
| 修改实现但偷偷改测试行为通过 | 失去测试价值 |
| 写在 `_test.go` 里的 `init()` | 易引入全局状态 |
| 在测试包里导出 API / 写生产逻辑 | 测试代码不可被打进 binary 但仍污染仓库 |
| 跳过 §2.4 数据契约 smoke 直接合并 | smoke 16/16 全绿但 dev 模式 `chartData=[]` / `event_type='conversation'` / `permission denied` 类 bug 永远抓不到 |
| dev 模式改动只测 `KAFKA_ENABLED=true` 路径 | outbox publisher=nil、Kafka fallback 等 dev-only 路径 bug 潜伏到下次拉数据才暴露 |
| 写文档前不读代码 / 不查 ADR / 不跑 smoke 直接动笔 | roadmap / ADR / plan 与实际架构不符，修代码时按错文档走（如 A1 修复方向定错、A4 修 GRANT 但视图没建） |

---

## 五、参考资源

- [Test-Driven Development by Example (Kent Beck)](https://www.amazon.com/dp/0321146530)
- [Go Testing (官方)](https://pkg.go.dev/testing)
- [stretchr/testify 文档](https://pkg.go.dev/github.com/stretchr/testify)
- [Vitest 文档](https://vitest.dev/)
- [pytest 文档](https://docs.pytest.org/)

---

## 六、违反约定的代价

PR review 时若发现违反 TDD：
- **首次**：Reviewer 留言指出并要求补测试
- **二次**：合并被 reject，需拆 PR 重做
- **第三次以上**：视为违反协作约定，列入协同黑名单

---

> 最后更新：2026-07-15 by 当前协作 Agent  
> 适用版本：本约定生效后的所有 PR

---

## 七、文档写入约定（2026-09-03 起生效）

未来新增功能计划请写到 [`docs/plans/`](docs/plans/)（不再使用 `.trae/documents/` 目录——已于 2026-09-03 重构删除）。完整规则见 [`docs/README.md`](/docs/stages/README.md) 与 [`docs/_meta/doc-migration-map.md`](docs/_meta/doc-migration-map.md)。

简要：

- **新功能计划** → `docs/plans/<kebab-case>.md` + front-matter `status: planned`
- **架构决策** → `docs/architecture/adr/adr-YYYY-MM-<topic>.md` + 在 `docs/architecture/decisions.md` 索引登记
- **演进记录** → `docs/stages/stage-XX-<topic>.md`
- **计划落地后** → 从 `docs/plans/` 迁入 `docs/legacy-plans/landed/` 加 front-matter
