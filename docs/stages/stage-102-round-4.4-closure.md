---
stage: 102
title: Round 4.4 PR-1+PR-2 收口 — PG 池配置化 + SkyWalking gorm/redis 接入
status: landed
date: 2026-09-15
commits: [ce0d7d1, 168e1f5, d6884b5, 38075eb]
related-plan: ../plans/multi-round-iteration-2026-09-15.md#十六
related-roadmap: ../architecture/roadmap.md (line 1101-1115, rows 10-11)
---

# Stage 102 — Round 4.4 PR-1+PR-2 收口

## 一、目标

收口 plan §十四.3 最后 2 项真未落：
- **Round 4.4 PR-2**：PG 连接池配置化（`dbconnect.ApplyPoolEnv` 0 caller → 5 svc 接入）
- **Round 4.4 PR-1**：SkyWalking gorm/redis 接入（`InitGORM` / `InitRedis` 0 caller → 5 svc 接入 + InitRedis 隐性 bug 修）

## 二、实际落地（3 commits + 1 docs commit）

### commit `ce0d7d1` — test(dbconnect): RED→GREEN 钉死 ApplyPoolEnv env→SetMax* 副作用契约

- `emotion-echo-shared/pkg/dbconnect/pool_test.go` 新增 3 个测试：
  - `TestApplyPoolEnv_AppliesEnvToSetters` — env=33 → MaxOpenConnections 真值=33
  - `TestApplyPoolEnv_FallsBackToDefaults` — env 全空回落到 10/5/1h（向后兼容 Stage 50）
  - `TestApplyPoolEnv_BadEnvPropagatesError` — env=typo 立即 fail-fast
- 实现无改动（函数已定义，本轮只钉测试契约）
- 注：`sql.Open("postgres")` 在沙箱返回 nil → panic；改用注册本地 `dbconnect-test-stub` 空 driver 拿真 `*sql.DB`（test-only，副作用隔离到本包 `init()`）

### commit `168e1f5` — fix(skywalking): InitRedis 真接 *redis.Client + InitGORM caller-wiring 测试

**隐性 bug 修**：原 `InitRedis(client interface{})` 用本地 `hookable` interface 做 type assertion：
```go
type hookable interface { AddHook(any) }
```
`*redis.Client.AddHook` 签名是 `func(redis.Hook)` → `*redis.Client` 不实现 `hookable` → **永远返回 false**（hook 从未注册）。本次改为：
```go
func InitRedis(client *redis.Client) bool {
    if client == nil { return false }
    InstrumentRedis(client)
    return true
}
```

- `init_test.go` 新增 2 个测试：
  - `TestInitGORM_NonNilPositive` — 非 nil `*gorm.DB` 必须返回 ≥1（InstrumentGORM 调用信号）
  - `TestInitRedis_RealClientRegistersHook` — 真 `*redis.Client` 必须返回 true（修复后才通过）

### commit `d6884b5` — feat(svc): Round 4.4 PR-1+PR-2 — 5 svc main.go 接入

5 svc `openPostgres` 全部改：
1. 先按 yaml 配置 `SetMax*`（向后兼容 Stage 50 行为，`Postgres.MaxOpenConns` / `MaxIdleConns` 仍生效）
2. 再调 `dbconnect.ApplyPoolEnv(sqlDB)` 让 env (`PG_MAX_CONNS` / `PG_MIN_IDLE_CONNS` / `PG_MAX_LIFETIME_SECONDS`) 覆盖 yaml；非法 env 立即 fail-fast
3. 调 `sharedskywalking.InitGORM(db)` 注册 GORM 5 个 opType 的 Trace 回调

**顺手修**：`emotion-echo-assessment-svc/nacos_boot_test.go` fakeRegistry 缺 `BeatHeartbeat` 方法（Stage 101 commit `d33e9e1` 漏了 assessment-svc，本轮跑 `go test ./...` 暴露）。

### commit `38075eb` — docs(roadmap+plan): §十四.3 标 ✅ + §十六 登记

- `roadmap.md` 行 10/11 标 ✅ + 头注更新
- `multi-round-iteration-2026-09-15.md` §十四.3 + front-matter + 新增 §十六 收口段
- `docs/plans/README.md` 索引摘要更新（backlog 9→7）

## 三、回归结果

```
go test ./... -count=1 -short (6 svc + shared) → 0 FAIL / 0 编译错
```

## 四、§十四.3 修订后状态

- **真未落**：0 项（全部落地 + 移触发条件型）
- **roadmap 剩余触发条件 backlog**：7 项（详见 roadmap.md line 1103）

## 五、教训

1. **Stage 101 "全绿" 是漂移**：assessment-svc fakeRegistry 缺 BeatHeartbeat，说明 Stage 101 的"6 svc 全绿"说法不准确——4 业务 svc 只修了 3 个，漏了 assessment-svc。
2. **0 caller 函数必须有 caller-wiring test**：`ApplyPoolEnv` / `InitGORM` / `InitRedis` 定义但 0 caller 的状态潜伏了数月，本轮 TDD 循环才暴露。
3. **InitRedis type assertion 隐性 bug**：interface{} + 本地 `hookable` interface 模式在类型签名不匹配时静默失败（返回 false 而非 panic），只有 caller-wiring test 才能暴露。
