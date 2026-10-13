---
status: planned
priority: medium
owner: TBD
created: 2026-10-11
type: implementation-plan
source: 2026-10-11 会话（用户："Redis 没真正用到位，落地双层缓存"）
depends-on: []
related-plans:
  - frontend-message-cache-offline-resilience.md（L1 前端缓存层，本计划的"第一层"）
---

# Plan — Redis 读缓存：与前端组成真正的"双层读缓存"

> **只描述要做的事，不含代码改动。**
> 动机：当前 Redis **只做锁 + 限流**（无读缓存），面试问"Redis 用到位了吗"很难答。本计划给 Redis 补一层**真正的 cache-aside 读缓存**，并与前端 IndexedDB（L1）组成"双层读缓存"。

> **⚠︎ 与 E2E 路线图的关系**：本计划**独立于 E2E 路线图**——30 阶段已定稿，**不登记账本、不归属阶段**。（追溯到的 D-27/D-28、E2E-18/20 引用仅作历史来源。）

---

## §0 上下文（★ 功课）

### 0.1 已读代码（真实文件）

| 文件 | 关键事实 |
|---|---|
| `emotion-echo-web-bff/internal/authlock/redis_store.go` | Redis 现用于**登录失败锁定**（Hash `authlock:fails:{user}` + TTL） |
| `emotion-echo-shared/pkg/middleware/redis_backend.go` | Redis 现用于**限流**（Lua token bucket，多实例共享配额） |
| `emotion-echo-web-bff/main.go`（buildRedisClient / redisHealthProbe） | BFF 已持有 Redis client；**无任何"读缓存"用法** |
| `emotion-echo-shared/pkg/skywalking/init.go`（`InitRedis`） | 埋点 helper 存在但**无生产调用**（死代码） |
| `emotion-echo-analytics-svc/migrations/a001_create_views.sql` | 报表基于 **视图/物化视图聚合**（`msg_summary_v` / `daily_emotion_v` / `assessment_v`）——**读密集、计算贵** |
| `emotion-echo-web-bff/internal/downstream/personality.go` | 人格画像扫描"我的结果"取最新——**读频繁、写极少** |

### 0.2 历史（为什么现在 Redis 没做缓存）

- **E2E-18「缓存层」**：其"缓存"指 **ai-svc 进程内 LRU**（fusion 去重），不是 Redis；Redis 当时**零业务引用**（实测 #6/#7），决策 **D-27**「保留并接入业务」= **保留待接入**。
- **E2E-20（D-28）**：才真正接入 Redis——**登录锁定 + 限流**（**仍非读缓存**）。
- 结论：**至今没有任何"DB → Redis 读缓存"链路**。

### 0.3 双层缓存的正确分层（**关键认知**）

两层**不是"同一份数据缓存两遍"**，而是**服务不同场景**：

| 层 | 位置 | 服务场景 | 缓存对象 |
|---|---|---|---|
| **L1** | 前端 IndexedDB（Dexie） | **同一用户反复看同一段数据**（秒开、离线可用） | 消息列表（见 `frontend-message-cache-offline-resilience.md`） |
| **L2** | **Redis**（服务端，跨实例共享） | **同一份数据被多个请求/实例反复读**（省掉昂贵的聚合/回源） | **报表聚合、人格画像**（本节重点） |
| 真源 | PostgreSQL | — | — |

> **这条认知是面试的加分点**：能说清"两层分工不同"，比"前端缓存 + Redis 缓存同一条消息"更像真做过。

---

## §1 目标与非目标

### 目标
- **G1** 在服务端加 **cache-aside 读缓存（Redis）**，键在"读多写少、计算贵"的对象上（报表/画像）。
- **G2** 设计与实现**失效**（TTL + 写时主动删）与**三大防护**（穿透/击穿/雪崩）。
- **G3** **降级**：Redis 不可用直连 PG，不让 Redis 成新单点（与锁/限流同款纪律）。
- **G4** **可观测**：加缓存命中率指标（hit/miss）——这是"真做了缓存"的硬证据。
- **G5** 与 L1（前端）形成**分层协同**（不是串联同一份数据）。

### 非目标
- **不缓存消息历史**（append 频繁、失效成本高、一致性风险大）——消息的缓存归 L1 前端。
- 不改锁/限流（E2E-20 的产物，不动）。
- 不引入 Redis 高可用/集群（单实例够用；降级已覆盖单点）。
- 不登记 E2E-F、不挂阶段。

---

## §2 设计

### 2.1 缓存对象选择（**先选对对象，再谈缓存**）

| 候选 | 读/写比 | 计算成本 | 结论 |
|---|---|---|---|
| **报表/分析聚合**（daily/trend/emotion 分布） | 读多写少（写入来自事件流，非用户高频） | **高**（聚合/视图） | ✅ **首选**（收益最大） |
| **人格画像**（personality profile） | 读频繁、写极少（一次测评） | 中 | ✅ 次选 |
| 消息历史 | 写频繁（每条消息都写） | 低（主键/索引查） | ❌ **不缓存**（失效复杂） |
| 会话列表 | 写较频繁（新消息改 `updated_at`） | 低 | ❌ 暂不缓存 |

### 2.2 缓存模式：cache-aside

- 读：`GET key` → 命中返回；未命中 → 查 PG → `SET key TTL value` → 返回。
- Key 设计：`cache:{domain}:{entity}:{user_id}:{param_hash}`（如 `cache:analytics:daily:42:2026-10-11`）。
- 新增 **`shared/pkg/cache`** 包：`GetOrLoad(ctx, key, ttl, loader)` 统一封装（Redis 不可用 → 直接 `loader()`）。

### 2.3 失效

- **TTL**（报表可短 TTL 如 30~60s；画像可长 TTL 如 10min）。
- **写时主动删**：报表刷新型操作 / 画像更新 → `DEL` 相关 key（避免脏读超过 TTL）。
- TTL **加随机抖动**（如 TTL×(0.9~1.1)）防**雪崩**。

### 2.4 三大防护

| 问题 | 手段 |
|---|---|
| **穿透**（查不存在的 key） | **空值缓存**（缓存 null，短 TTL，如 30s）；高频恶意 key 可上布隆过滤器 |
| **击穿**（热点 key 过期瞬间并发回源） | **singleflight / 互斥重建**：同一 key 的并发未命中只放一个回源，其余等待其结果 |
| **雪崩**（大量 key 同时过期 / Redis 挂） | **TTL 抖动** + **降级直连 PG** + 限流（已有）兜底 |

### 2.5 降级

- Redis 不可达 → `GetOrLoad` 内部吞错、直接 `loader()`（回源 PG），**不 fail-closed**（与 `authlock` / `limiter` 的降级一致）。
- 指标记录降级次数。

### 2.6 与 L1 的分工（双层协同）

- **L1（前端）**：管消息列表——用户本地秒开 + 离线可用（属 `frontend-message-cache-offline-resilience`）。
- **L2（Redis）**：管报表/画像——省服务端昂贵的聚合回源、跨实例共享。
- **不同数据、不同场景**，不强行串联；面试时按这个分工讲。

---

## §3 分阶段实施（TDD）

### Phase 0 — 选点与基线
- 确认报表/画像的**回源耗时基线**（改前先量，证明"缓存有意义"）。

### Phase 1 — `shared/pkg/cache` 通用包（RED→GREEN）
- RED：`GetOrLoad` 单测（miniredis）——命中不调 loader / 未命中调一次并回写 / Redis 挂时直接 loader（降级）/ 空值缓存 / singleflight 并发只回源一次。
- GREEN：实现。

### Phase 2 — 接入报表（analytics-svc 或 BFF 聚合层）
- RED：报表读命中缓存（第二次不查 PG）；写/刷新后 key 被删。
- GREEN：接线 + TTL。

### Phase 3 — 接入人格画像
- 同上（读频繁、写极少，最适合）。

### Phase 4 — 可观测
- 加 `emotion_echo_cache_hit_total{layer,cache}` / `miss` / `degrade`；Grafana 面板 + 命中率。

### Phase 5 — 回归钉 + 文档
- 单测 + 端到端（第二次读明显更快 / 不查 PG）；runbook 增"缓存对象清单 + TTL + 失效点"。

---

## §4 验收与回归钉

- **单测（miniredis）**：命中/未命中/降级/空值/singleflight 全绿。
- **命中率**：指标可见且 >0（证明缓存真在用）。
- **降级**：停 Redis → 功能不挂（回源 PG），有 degrade 计数。
- **收口**：落地后迁 `docs/legacy-plans/landed/`。

---

## §5 风险与坑

| 风险 | 说明 | 规避 |
|---|---|---|
| **缓存一致性** | 报表数据变了但缓存没过期 → 脏读 | TTL 短 + 写时主动删 key |
| **为面试而缓存制造 bug** | 缓存了不该缓存的对象（如消息历史），引入一致性 bug | **只缓存读多写少**；消息历史坚决不缓存 |
| **Redis 成单点** | 缓存挂了拖垮服务 | 降级直连 PG（必修项） |
| **雪崩** | 大量 key 同时过期 | TTL 抖动 |
| **击穿** | 热点 key 过期并发回源把 PG 打满 | singleflight |
| **测试环境** | 单测需 Redis | 用 **miniredis**（项目已在用 `alicebob/miniredis` 约定） |

---

## §6 待拍板

| # | 事项 | 说明 |
|---|---|---|
| C1 | 缓存对象范围 | 建议"报表 + 画像"起步；是否加会话列表看收益 |
| C2 | 缓存位置 | 报表缓存在 **BFF 聚合层**还是 **analytics-svc**？（建议：谁拥有数据谁缓存，报表→analytics-svc；画像→BFF，因为它在 BFF 组装） |
| C3 | TTL 取值 | 需按数据时效性定（报表 30~60s / 画像 10min 起步） |

---

## §7 调研依据（本次实读）

- `emotion-echo-web-bff/internal/authlock/redis_store.go`、`emotion-echo-shared/pkg/middleware/redis_backend.go`、`emotion-echo-web-bff/main.go`（Redis 现状）
- `emotion-echo-shared/pkg/skywalking/init.go`（InitRedis 死代码）
- `emotion-echo-analytics-svc/migrations/a001_create_views.sql`（报表聚合形态）
- `emotion-echo-web-bff/internal/downstream/personality.go`（画像读取）
- 账本/决策：`docs/e2e-roadmap/decisions.md`（D-27 / D-28）、`docs/e2e-roadmap/stages/e2e-18-cache-layer/report.md`（Redis 零引用实测 #6/#7）

**未能确认**（需实机）：报表/画像的**实际回源耗时**（决定缓存收益大小）；各报表的**数据时效要求**（决定 TTL）。
