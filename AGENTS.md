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
| 合并前 | `go test ./...` + `go vet ./...` + (前端) `npm run lint` 必须过 |

### 2.3 覆盖率底线

| 类型 | 底线 |
|------|------|
| 核心业务包（service、handler、repository） | 80% |
| pkg 工具包 | 90% |
| 三方适配层（database、messaging、skywalking 钩子） | 70%（因依赖真实外部组件） |

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
