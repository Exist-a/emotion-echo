---
status: planned
priority: high
owner: TBD
created: 2026-09-16
last-refresh: 2026-09-16
type: data-contract-validation
depends-on:
  - stage-103-dev-mode-launch-2026-09-16.md (dev 模式 14 容器 healthy)
  - stage-108-sender-architecture-debt-fix-2026-09-16.md (sender 修复)
  - sprint-109a-apisix-jwt-401-2026-09-16.md (A7 修复)
  - sprint-109b-end-to-end-chat-2026-09-16.md (端到端真通)
related-stages:
  - AGENTS.md §2.4 数据契约验收清单
  - AGENTS.md §〇 第一性原则（合并前必跑契约 smoke）
related-issues:
  - docs/plans/test-coverage-tracker-2026-09-16.md §三 6 条契约状态
blocking:
  - "整体落地验收" (AGENTS.md §〇 §2.4 硬门槛)
  - Sprint 113 全量契约 smoke（剩余 §3 §4）
---

# Sprint 109c — 数据契约 smoke（§1 §2 §5 §6）

> **目的**：跑 AGENTS.md §2.4 业务数据契约验收清单中的 §1 §2 §5 §6（依赖 chat 链路端到端通的 4 条），全部 6/6 PASS 是"整体落地"硬门槛。
>
> **Sprint 109c 范围**：chat 链路相关的 4 条契约（§1 §2 §5 §6）。§3 analytics_reader GRANT（独立单元测已绿）+ §4 chartData 非空（reports E2E，sprint-112）独立排期。

---

## 一、§2.4 数据契约清单（本 sprint 范围）

### §契约 1：user_behavior_events 行数 = 业务事件数

**断言**：
```sql
psql -c "SELECT COUNT(*) FROM emotion_echo_analytics.user_behavior_events"
```
**期望**：`≥ 最近 1 分钟 message.created + conversation.created + conversation.closed 数`

**实施**：
1. 在浏览器实测发消息（Sprint 109b 步骤 1 已包括）
2. 等 30 秒（chat-svc outbox + Kafka + ai-svc consumer 写 user_behavior_events）
3. psql 查 user_behavior_events 行数
4. 对比 message.created / conversation.created 行数

**捕获 bug 类型**：A1（dev 模式 outbox 永远 pending）

### §契约 2：event_type enum 细分

**断言**：
```sql
psql -c "SELECT event_type, COUNT(*) FROM emotion_echo_analytics.user_behavior_events GROUP BY 1"
```
**期望**：出现 `≥ 2 种 event_type`（不能全 `conversation`）

**实施**：
1. 同 §1 流程
2. 跑完后查 event_type 分布

**捕获 bug 类型**：A3（conversation.created/closed 都映成 'conversation'）

### §契约 5：schema 与写入端一致性

**断言**：对每个 VARCHAR(32) NOT NULL 枚举列，至少 1 个 integration test 断言写入值 ∈ enum 集合

**实施**：
1. 列出所有 VARCHAR(32) NOT NULL 枚举列（user_behavior_events.event_type, target 等）
2. 检查现有 integration_test 是否覆盖每列
3. 缺的补测试

**捕获 bug 类型**："SQL DDL 定义正确但写入端用错值"（如 event_type / target 列）

### §契约 6：KAFKA_ENABLED=false 路径不空跑

**断言**：
```bash
# 启动 dev 模式（KAFKA_ENABLED=false）
docker compose -f deploy/docker-compose.apps.yml -f deploy/docker-compose.infra.yml up -d
# 触发业务事件
# 等 30s
# 跑 §契约 1+2
```

**期望**：§1+2 通过（即使 Kafka off，dev fallback 也让数据落库）

**实施**：
1. 确认 dev 模式 KAFKA_ENABLED=false（compose.yml 默认）
2. 跑 §1+2 流程
3. 如果 dev 模式已经触发业务事件，直接复用数据

**捕获 bug 类型**："dev 模式走通但 prod 走通是巧合"（如 outbox publisher=nil 永远失败）

---

## 二、调研依据（AGENTS.md §〇 硬规则）

| 文件 | 用途 |
|---|---|
| `AGENTS.md §2.4` | 6 条契约原文 + 抓的 bug 类型 |
| `scripts/smoke_data_layer.py` | 现有 smoke 脚本（AGENTS.md 标注"待建"但已存在）|
| `emotion-echo-analytics-svc/migrations/004_security_test.go` | §契约 3 已绿（参考模式）|
| `emotion-echo-analytics-svc/migrations/` | schema 来源 |
| `emotion-echo-chat-svc/internal/repository/outbox_test.go` | outbox 写入测试参考 |
| `emotion-echo-ai-svc/internal/consumer/consumer_test.go` | Kafka consumer 测试参考 |
| `docs/plans/test-coverage-tracker-2026-09-16.md §三` | 6 条契约当前状态矩阵 |
| `docs/stages/stage-36-smoke-report.md` | 历史 smoke 报告参考 |

### 架构假设清单

| 假设 | 验证 |
|---|---|
| smoke_data_layer.py 已实现 | 🟡 存在但未实测跑过（之前 session 标注 AGENTS.md 说"待建"，但实际有）|
| dev 模式 KAFKA_ENABLED 默认 false | ✅ compose.yml 默认 |
| user_behavior_events 表在 emotion_echo_analytics schema | ✅ migrations 已建 |
| chat-svc outbox 写库后 Kafka consumer 同步写 user_behavior_events | 🟡 端到端待 Sprint 109b 验证 |

---

## 三、TDD 实施步骤

### Step 1 · 评估 smoke_data_layer.py 是否真能跑

```bash
# 读脚本头部
head -50 scripts/smoke_data_layer.py

# 看它跑什么契约
grep -E "契约|user_behavior_events|event_type|analytics_reader" scripts/smoke_data_layer.py
```

**可能结果**：
- (a) 脚本已实现 §1 §2 §5 §6，直接跑
- (b) 脚本只覆盖 §3 §4，需补充 §1 §2 §5 §6
- (c) 脚本不存在，按 AGENTS.md §2.4 重新写

### Step 2 · RED：写 §1 §2 §5 §6 契约测试（如果脚本不存在或不完整）

**§契约 1 测试**：
```python
def test_user_behavior_events_count():
    # 触发 1 次 message.send
    # 等 30s
    # 断言 user_behavior_events 行数 ≥ 1
    count = query("SELECT COUNT(*) FROM emotion_echo_analytics.user_behavior_events")
    assert count >= 1
```

**§契约 2 测试**：
```python
def test_event_type_diversity():
    # 触发 message.created + conversation.created
    # 等 30s
    types = query("SELECT DISTINCT event_type FROM emotion_echo_analytics.user_behavior_events")
    assert len(types) >= 2
```

**§契约 5 测试**：
```python
def test_enum_columns_write():
    # 对每个 VARCHAR(32) 枚举列，写一个合法值，断言通过
    # 写一个非法值（如 enum 不存在的字符串），断言失败
    for col, valid_values in ENUM_COLUMNS.items():
        for v in valid_values:
            insert_ok(row={col: v})  # 成功
        with pytest.raises(IntegrityError):
            insert(row={col: 'INVALID_VALUE'})
```

**§契约 6 测试**：
```python
def test_kafka_disabled_path():
    # 假设 KAFKA_ENABLED=false（dev 默认）
    # 触发业务事件
    # 等 30s
    # 断言 §1 + §2 通过（即使 Kafka off）
    test_user_behavior_events_count()
    test_event_type_diversity()
```

### Step 3 · GREEN：跑 §1 §2 §5 §6 全绿

如果 Step 2 写的测试已绿 → 完成。
如果有失败 → 修对应代码（chat-svc outbox / ai-svc consumer / analytics-svc write 等）。

### Step 4 · 文档化

写 `docs/stages/stage-109c-data-contract-smoke-2026-09-16.md`：
- §一 起点 + §二 6 条契约跑结果 + §三 抓到的 bug（如有）+ §四 经验

---

## 四、DoD

- [ ] scripts/smoke_data_layer.py 实测能跑 §1 §2 §5 §6
- [ ] 或新建 scripts/test_data_contracts.py 补齐
- [ ] §1 §2 §5 §6 全 PASS（chat 链路相关 4 条）
- [ ] docs/stages/stage-109c-data-contract-smoke-2026-09-16.md 落地
- [ ] test-coverage-tracker §三 6 条契约状态更新（§1 §2 §5 §6 从 🟡 → 🟢）
- [ ] §2.5 收口 + push

---

## 五、工作量估计

- Step 1 评估脚本: 30 min
- Step 2 RED: 1-2 hour（如果脚本不存在需重写）
- Step 3 GREEN: 30 min - 1 hour（如果发现 bug 需要修）
- Step 4 文档: 30 min

**总计**: 2.5-4 hour

---

## 六、风险

| 风险 | 影响 | 应对 |
|---|---|---|
| smoke_data_layer.py 缺失或陈旧 | 补齐工作量大 | 按 AGENTS.md §2.4 原文重写 |
| §5 发现 schema 漂移（写入端用错 enum 值） | 需修 schema 或写入端 | 拆独立 sprint 修，本 sprint 只登记 |
| KAFKA_ENABLED=false dev fallback 路径有 bug | §6 FAIL | 排查 chat-svc events/dev_fallback |
| Sprint 109b 端到端不真通 | §1+2 跑不出数据 | 等 109b 真通后再跑 109c |

---

## 七、commit 计划

| # | Commit | 文件 |
|---|---|---|
| 1 | `test(smoke): RED 钉住 §1 §2 §5 §6 数据契约` | `scripts/test_data_contracts.py` (新建或补 smoke_data_layer.py) |
| 2 | `fix(svc): 根据契约测试失败修对应代码（如有）` | 待 Step 3 决定 |
| 3 | `docs(stage-109c): 数据契约 smoke 跑通 + 经验` | `docs/stages/stage-109c-data-contract-smoke-2026-09-16.md` |
| 4 | `docs(plans): test-coverage-tracker §三 契约状态更新` | `docs/plans/test-coverage-tracker-2026-09-16.md` |
