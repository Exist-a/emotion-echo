---
status: landed
priority: high
stage: 54
date: 2026-09-08
related-stages:
  - stage-44-observability-sprint-b.md §四 B/C/D (基础设施层)
  - stage-52-nacos-fix.md (上一 stage, 解锁 Nacos 阻塞)
  - stage-53-smoke-section4-fix.md (smoke_data_layer 11/11)
  - stage-50-e2e-validation.md §九.1 (镜像滞后同源)
related-decisions:
  - adr-2026-09-doc-drift-registry.md (决策 18, 失真登记)
branch: feat/observability-batch-1-infra
commit:
  - 4cd6b45 fix(infra+prom): 端口失配修复
related-tests-result: smoke_data_layer.py 11/11 + smoke_observability.py 12/12 全绿
---

# Stage 54 · prometheus / sw-oap 端口失配修复 → smoke_observability 12/12 PASS

> **本 stage 闭环 stage-44 §四 B/C/D 全部基础设施层**——
> rebuild 6 svc 镜像 + Wipe Nacos + 重启 7 容器后，**smoke_data_layer.py 11/11 + smoke_observability.py 12/12 全绿**。
> 两个端口失配（prometheus analytics-svc 端口 + OAP telemetry env）一并修复。

---

## 一、问题回顾

### 1.1 阶段前置（批 1 + stage-52 落地后）

| 项 | 状态 |
|---|---|
| 批 1 含 8 PR-OBS + O-1 + Stage 43 Kafka Sprint A | ✅ 全部 merge 进批 1 分支 |
| 6 svc 镜像 rebuild | ✅ 全部成功（含 stage-52 Dockerfile bug fix） |
| Wipe Nacos + 重启 7 容器 | ✅ 7 容器 healthy（nacos + 6 svc + apisix） |
| smoke_data_layer.py | ✅ **11/11 PASS** |
| smoke_observability.py | ⚠️ **11/12 PASS**（1 FAIL：prometheus 2 target DOWN）|

### 1.2 唯一 FAIL 根因

`prometheus /api/v1/targets?state=active` 实测：

```
DOWN emotion-echo-analytics-svc:8904
   Get "http://emotion-echo-analytics-svc:8904/metrics":
   dial tcp 172.18.0.18:8904: connect: connection refused

DOWN emotion-echo-sw-oap:1234
   Get "http://emotion-echo-sw-oap:1234/metrics":
   dial tcp 172.18.0.3:1234: connect: connection refused
```

**两个独立失配**：
1. `prometheus.yml` 写 analytics-svc port = 8904，但 `analytics-api.yaml` 实际 port = 8893
2. `docker-compose.infra.yml` sw-oap env `SW_TELEMETRY=prometheus` 已加（O-1），但 OAP 9.7 在 standalone 模式下没启 :1234 listener

---

## 二、修复方案（commit 4cd6b45）

### 2.1 三处改动（1 commit）

| 文件 | 改动 |
|---|---|
| `deploy/prometheus/prometheus.yml:30` | `emotion-echo-analytics-svc:8904` → `emotion-echo-analytics-svc:8893` |
| `deploy/docker-compose.infra.yml` sw-oap env | 加 `SW_TELEMETRY_PROMETHEUS_HOST: 0.0.0.0` + `SW_TELEMETRY_PROMETHEUS_PORT: "1234"` |
| `scripts/smoke_observability.py` EXPECTED_TARGETS | `emotion-echo-analytics-svc:8904` → `emotion-echo-analytics-svc:8893` |

### 2.2 sw-oap telemetry 调研

WebFetch `https://skywalking.apache.org/docs/main/next/en/setup/backend/backend-telemetry/`：
- `SW_TELEMETRY=prometheus` 是默认 selector
- `SW_TELEMETRY_PROMETHEUS_HOST` 默认 0.0.0.0
- `SW_TELEMETRY_PROMETHEUS_PORT` 默认 1234
- 但 OAP 9.7 standalone 模式下默认值不可靠（实测 :1234 没 LISTEN）

**修法**：显式设 `SW_TELEMETRY_PROMETHEUS_HOST=0.0.0.0` + `SW_TELEMETRY_PROMETHEUS_PORT=1234`。

### 2.3 analytics-svc port 失配来源

未深查——`analytics-api.yaml` 显示 `Port: 8893`（Stage 26-P "避开 ai-svc 占用的 8892"），
而 `prometheus.yml` 第 30 行写 8904——推测是早期 author 写配置时**凭记忆写错**，
未与实际 svc port 对照（决策 18 §4.4 "凭记忆写" 失真类型）。

---

## 三、测试结果（实测）

| 验证 | 结果 |
|---|---|
| `docker exec emotion-echo-analytics-svc wget :8893/metrics` | ✅ 返回 Prometheus metrics 文本 |
| `docker exec emotion-echo-sw-oap sh` 看 :1234 LISTEN | ✅ OAP 启动后 :1234 起来了 |
| `prometheus /api/v1/targets` UP 列表 | ✅ **10/10 UP**（apisix + 6 svc + sw-oap + prometheus self）|
| `python scripts/smoke_data_layer.py` | ✅ **11/11 PASS** |
| `python scripts/smoke_observability.py` | ✅ **12/12 PASS** |
| 单测：shared + 6 svc go test | ✅ 全绿 |

---

## 四、决策 18 登记

### 4.1 prometheus.yml 端口失真

**类型**：凭记忆写（类型 5）
**位置**：`deploy/prometheus/prometheus.yml:30` analytics-svc 端口
**来源**：早期 author 写配置时**凭记忆写 8904**，未与 `analytics-api.yaml` 实际 port 对照
**教训**：port 失配类配置应自动生成（从 yaml 读 svc port），不要硬编码

### 4.2 sw-oap telemetry env 缺失

**类型**：env 不全（类型 4）
**位置**：`deploy/docker-compose.infra.yml` sw-oap env
**来源**：O-1 commit 296de5c 加 `SW_TELEMETRY=prometheus`，但 OAP 9.7 standalone
模式还需 `SW_TELEMETRY_PROMETHEUS_HOST/PORT` 才启 telemetry receiver
**教训**：查 OAP 文档时只看"默认值"够不够，standalone 模式默认值不可靠

---

## 五、与路线 Z / stage-44 关系

| 阶段 | 状态 | 与本 stage 关系 |
|---|---|---|
| Stage-44 §四 A 分支未合并 main | ✅ 本批 1 推进合并 | 批 1 merge main 含 stage-52/53/54 |
| Stage-44 §四 B 6/6 步 | ✅ 本 stage 闭环 | smoke 11/11 + 12/12 + 镜像 rebuild + 端到端验证 |
| Stage-44 §四 C logging helper | ✅ Stage 47 已收口 | 含在批 1 |
| Stage-44 §四 D sw-oap telemetry | ✅ O-1 + 本 stage 完整收口 | 加 SW_TELEMETRY_PROMETHEUS_HOST/PORT |
| Stage-44 §四 E PR-OBS-4/5 干净环境 | ✅ 本 stage 闭环 | smoke_observability 12/12 PASS |

剩余 backlog（与本 stage 无关）：
- Stage-44 §四 F Kafka Sprint A §1.5 Protobuf 迁移（独立 Sprint C）
- Stage-44 §四 G Kafka Sprint A §3 历史数据迁移 SQL（运维窗口）
- todo-pile §A1/B4 TTS/多模态 AI 决策
- todo-pile §A2 文件上传
- todo-pile §C8 BFF 路由三方契约收口

---

## 六、调研依据（AGENTS.md §〇 回填）

### ① 读相关代码

- `deploy/prometheus/prometheus.yml`（PR-OBS-4 落地，O-1 commit 296de5c 已更新 §44-51）
- `deploy/docker-compose.infra.yml` skywalking-oap env（O-1 commit 296de5c 加 SW_TELEMETRY）
- `emotion-echo-analytics-svc/etc/analytics-api.yaml`（Port: 8893 实际值）
- `scripts/smoke_observability.py` EXPECTED_TARGETS（PR-OBS-4 写 8904 错误）

### ② 查相关 ADR / stage

- `docs/plans/observability-sprint-b.md §〇`（16 PR-OBS-X 定义）
- `docs/stages/stage-44-observability-sprint-b.md §四 D`（sw-oap telemetry 收口）
- `docs/stages/stage-52-nacos-fix.md §四 E`（解锁 Nacos 阻塞）
- `docs/stages/stage-53-smoke-section4-fix.md §三`（smoke §4 7 天窗）
- `docs/stages/stage-50-e2e-validation.md §九.1`（镜像滞后同源）

### ③ 跑现状 smoke / 实测

- `docker exec emotion-echo-prometheus wget /api/v1/targets?state=active` 拿 UP/DOWN 列表
- `docker exec emotion-echo-analytics-svc wget :8893/metrics` 验证端口实际暴露
- `docker exec emotion-echo-sw-oap /proc/net/tcp` 验证 :1234 listener 起来
- `python scripts/smoke_observability.py` 12/12 PASS
- `python scripts/smoke_data_layer.py` 11/11 PASS

### ⑤ 列架构假设

| 假设 | 验证 | 结果 |
|---|---|---|
| analytics-svc :8893/metrics 暴露 | `wget :8893/metrics` | ✅ Prometheus metrics 文本 |
| prometheus.yml 写 8904 是错的 | 实测 8904 connect refused | ✅ 失配 |
| OAP 9.7 standalone 默认不启 telemetry | `/proc/net/tcp` 没 1234 | ✅ 假设命中 |
| SW_TELEMETRY_PROMETHEUS_HOST/PORT 显式可启动 telemetry | 重启 sw-oap 后 :1234 listener | ✅ |
| prometheus.yml 与 smoke EXPECTED_TARGETS 一致才 PASS | 实测 | ✅ 同步改才 12/12 |

### ⑥ 写完后回填

本文档归档到 `docs/stages/stage-54-port-mismatch-fix.md`。
1 commit（4cd6b45）已合入 `feat/observability-batch-1-infra` 分支。
**批 1 分支领先 main 53 commit**——下游阶段可合 main。

---

## 七、未做项 / 遗留

### ❌ A. §4 summary "0 段对话 0 条消息"（继承 stage-53 backlog）

本 stage 不解决，stage-53 §六 A 已登记 event_type='conversation' 精确字符串失配细分 enum。

### ❌ B. 业务链路"今天无数据"（继承 stage-53 backlog）

llm-service 未配 LLM 凭据，dev 模式 ai-svc dial llm-service 失败 → fallback to noop。
本 stage 不解决，独立 Sprint 配 LLM 凭据。

### ❌ C. prometheus.yml 应改为自动生成

**当前问题**：每个 svc 端口硬编码在 prometheus.yml，svc port 变更时易失配（本次失配就是）。
**修复路径**：写脚本从各 svc `*/etc/*.yaml` 读 port，生成 prometheus.yml。
**预估**：半天。
**本 stage 不做**：聚焦当前端口失配修复。

### ❌ D. build_dev_images.sh 脚本误报 FAIL

实测镜像全部 build 成功（`Image emotion-echo/web-bff:v0.1.0 Built`），但脚本报"build failed after 3 retries"。
**根因**：脚本 line 75 `grep -q "^ Image .* Built$" /tmp/build_${svc}.log` 可能因
tee 流或 stderr 干扰判定失败。
**修复路径**：修脚本判定逻辑 + 加 `--no-cache` 选项。
**本 stage 不做**：已手动验证 6 镜像都建成。

---

## 八、下一步建议

按 stage-51 §七规划 + 本 stage 闭环：

```
1. ✅ 批 1 merge main (含 stage-52/53/54)
2. 🔜 批 1 合 main (53 commit, smoke 11/11 + 12/12 全绿)
3. 推路线 Z 第 2 批 (测试护栏 OBS-9~16)
4. 推路线 Z 第 3 批 (业务 tag OBS-17/18/19/23)
5. Stage 47/48/49 镜像重建已通过本批验证
```

预计剩余路线 Z 工作 3-5 天合 main。

---

> 最后更新：2026-09-08 by 当前协作 Agent
> 调研依据：prometheus.yml + analytics-api.yaml 端口失配实测 + OAP 9.7 standalone telemetry 默认值不可靠
> 测试结果：smoke_data_layer 11/11 + smoke_observability 12/12 全绿