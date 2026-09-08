---
status: landed
priority: high
stage: 52
date: 2026-09-08
related-stages:
  - stage-44-observability-sprint-b.md §四 E (dev Nacos 阻塞解锁)
  - stage-51-batch-1-infra-merged.md (本 stage 解锁批 1 合 main 闸门)
  - stage-50-e2e-validation.md §九.1 (镜像滞后同源)
  - stage-42-container-tz-fix.md (顺手修复 stage-42 遗留 Dockerfile 语法 bug)
related-decisions:
  - decisions.md 决策 10/11 (Nacos 治理)
  - adr-2026-09-doc-drift-registry.md (决策 18,本文档登记 stage-42 失真 + PR-1 失真)
branch: stage-52-nacos-fix
commits:
  - 07a7581 test(shared): RED stage-52 默认 ephemeral=true
  - f7bbad4 feat(shared): GREEN stage-52 defaultRegisterEphemeral=true
  - a41639a feat(dockerfile+scripts): 5 Dockerfile RUN \\+RUN bug + cleanup 脚本
related-tests-result: smoke_data_layer.py 9/10 PASS (§1/2/3/6 绿, §4 业务问题独立)
---

# Stage 52 · dev Nacos ephemeral 500 阻塞修复

> **本批解决 stage-44 §四 E 记录的"dev Nacos ephemeral 注册阻塞 5 svc Restarting"问题**——
> 锁定两个独立根因（PR-1 错把默认改 persistent + stage-42 修 TZ 时 5 Dockerfile 留 `\`
> + `RUN` 串联 bug），全部修复并端到端验证：6 svc 注册到 Nacos ephemeral=true + healthy=true +
> BFF /health 全绿 + smoke_data_layer.py 9/10 PASS（§1/2/3/6 绿）。§4 业务数据问题
> （emotionDistribution=0）独立成下一 stage 跟进。

---

## 一、问题回顾（2026-09-08 实测）

### 1.1 现象

- 5 个 dev 业务 svc（user / chat / analytics / assessment / ai）持续 Restarting
- 日志重复 `[nacos] Register: discovery: register emotion-echo-<svc>/0.0.0.0:<port>: retry 3 times request failed!: request return error code 500`
- Nacos `instance/list` 返 `hosts: []`（无 instance），但 `service/list` 返 6 个 serviceName
- stage-50 §九.1 "镜像滞后"问题阻断主线下沉

### 1.2 影响

- smoke_data_layer.py 跑不通（chat-svc / analytics-svc / user-svc 全不可达，§1/2/4 必 FAIL）
- stage-51 batch-1-infra-merged.md §三.2 记录 smoke 阻塞根因
- 路线 Z 第 1 批 8 PR-OBS + O-1 + Stage 47/48/49 改动全部卡在 main 合闸门外

---

## 二、根因双锁定（实测 + 代码）

### 2.1 根因 A · PR-1 错改 defaultRegisterEphemeral=false

**代码**：[emotion-echo-shared/pkg/discovery/nacos_register.go:29](https://github.com/...)

```go
// PR-1 修复：dev 模式下 SDK v2.4.3 + Nacos 2.4.3 server 在 Derby 启动慢场景下
// BeatRequest 不可靠，ephemeral 实例会被 server 在 ~30s 内踢出，导致
// instance/list 返回 hosts: []。改为 false（持久实例）后，注册即落 Derby。
var defaultRegisterEphemeral = false  // ← 错
```

**实测（Nacos 2.4.3 standalone Derby）**：
- service 一旦被任何途径创建为 persistent（含外部 curl、admin API 旧版调用），后续 SDK 注册 ephemeral instance 一律返 400：
  ```
  errCode: 400, errMsg: Current service DEFAULT_GROUP@@emotion-echo-user-svc
  is persistent service, can't register ephemeral instance.
  ```
- 6 个 serviceName 早先被 curl 创建为 persistent（实测见 stage-50 §九）；SDK 注册 ephemeral→ 500 重试 → fatal abort → 容器 Restarting。

**结论**：PR-1 (commit b869ff9, 2026-09-04) 的"修复"基于错误假设——dev Derby 启动慢 ≠ 需要改 persistent。**正确做法是还原 SDK 默认 ephemeral=true**。

### 2.2 根因 B · stage-42 修 TZ 时遗留 Dockerfile 语法 bug

**实测**（尝试 rebuild 5 svc 镜像时）：
```
#14 0.215 fetch https://dl-cdn.alpinelinux.org/alpine/v3.19/main/...
#14 3.602 adduser: unknown group app
```

**根因**：5 个 Dockerfile（user/ai/analytics/assessment/web-bff）第 27~32 行末尾的 `\` 让两个 `RUN` 命令变成同一 shell 的延续：

```dockerfile
RUN apk add ... && cp ... && echo "${TZ}" > /etc/timezone \    # ← 末尾 \
RUN addgroup -S -g 65532 app ...                                # ← 实际拼成同一 shell
```

docker 把 `\` 当作换行延续，下一行 `RUN addgroup` 变成 `... > /etc/timezone RUN addgroup ...` 字面字符串作为 echo 的续行，`adduser` 找不到组。

**stage-42 报告**："6 Dockerfile 删 `apk del tzdata`，已落地"——但**未 rebuild 验证**，所以这个语法 bug 没暴露。本 stage-52 推动首次 rebuild 才暴露。

**chat-svc Dockerfile 不在此列**——chat-svc 第 40 行原本就没 `\`（独立 RUN）。

---

## 三、修复方案（单一修复点路线）

按 AGENTS.md §〇 "ALL CODE IS TDD" + 用户拍板"单一修复点（推荐）"：

### 3.1 TDD 三阶段

| 阶段 | commit | 内容 |
|---|---|---|
| 🔴 RED | `07a7581` | 改 `nacos_register_persistence_test.go` 期望 `defaultRegisterEphemeral=true`，跑测试 2/2 FAIL |
| 🟢 GREEN | `f7bbad4` | 改 `nacos_register.go:29` `var defaultRegisterEphemeral = true`，重写注释（保留 PR-1 历史错误教训），跑测试 2/2 PASS |
| 🧹 Cleanup | `a41639a` | (a) 5 Dockerfile 删末尾 `\`；(b) `scripts/clean_dev_nacos_persistent.sh` 新增（dev 模式清理脚本，docker exec curl + grep/sed/awk 解析）；(c) 顺手修 stage-42 失真 |

### 3.2 端到端验证

| 验证 | 实测结果 |
|---|---|
| 单测：shared/pkg/discovery | ✅ 2/2 PASS（TestDefaultRegisterEphemeralIsTrue + TestRegisterEphemeralResolution） |
| 单测：shared 全套 | ✅ 14/14 包全绿（grpcinterceptor / middleware / metrics / logging / config / configcenter / discovery / eventrow / bootstrap / healthcheck / messaging / password / skywalking） |
| 镜像 rebuild：6 svc | ✅ 全部成功（Dockerfile 修复有效） |
| Nacos Derby wipe + 6 svc restart | ✅ 7 容器全 healthy |
| Nacos ephemeral 注册 | ✅ `ephemeral: true` + `healthy: true`（user-svc / chat-svc / ai-svc 等 6 个） |
| BFF /health（容器内 wget） | ✅ 5 核心下游 `ok`（ai/analytics/assessment/chat/user），仅 xtts unhealthy（预期，XTTS 未启用 AI profile） |
| smoke_data_layer.py | ✅ **9/10 PASS**：§1/2/3/6 全绿，§4 FAIL（业务问题，独立 sprint） |

---

## 四、调研依据（AGENTS.md §〇 回填）

### ① 读相关代码

- `emotion-echo-shared/pkg/discovery/nacos_register.go`（PR-1 修复错误源头 + resolveRegisterIP() 实现）
- `emotion-echo-shared/pkg/discovery/nacos_register_persistence_test.go`（PR-1 配套测试"只看假象"反模式）
- `emotion-echo-user-svc/nacos_boot.go` / `chat-svc/nacos_boot.go`（5 份同构模板）
- `emotion-echo-{user,chat,ai,analytics,assessment}-svc/internal/config/config.go`（Host 默认值 `0.0.0.0` 6 份同构）
- `emotion-echo-{user,ai,analytics,assessment,web-bff}-svc/Dockerfile`（5 文件末尾 `\` bug）
- `emotion-echo-shared/integrationtest/` (Nacos 测试容器)
- `deploy/docker-compose.infra.yml`（Nacos 容器定义 + 健康检查）
- `deploy/docker-compose.apps.yml`（5 svc depends_on + Nacos env 注入）

### ② 查相关 ADR / stage

- `docs/plans/nacos-enablement-dev.md §二 + §四 PR-1`：Nacos 半启用现状、PR-1 修复说明
- `docs/stages/stage-44-observability-sprint-b.md §四 E`：dev Nacos 阻塞已登记独立 Sprint
- `docs/stages/stage-50-e2e-validation.md §九`：5 svc 镜像滞后 + 端到端 6 项问题清单
- `docs/stages/stage-51-batch-1-infra-merged.md`：批 1 合并归档，parked 等 Nacos 修后合
- `docs/stages/stage-42-container-tz-fix.md`：本次顺手修 stage-42 失真（Dockerfile 末尾 `\`）

### ③ 跑现状 smoke / 实测

- docker logs emotion-echo-chat-svc | grep nacos：确认 500 重试根因
- docker exec emotion-echo-nacos curl /nacos/v1/ns/instance/list：实测 hosts=[] + service persistent 性质
- docker exec emotion-echo-nacos curl POST /nacos/v1/ns/instance?ip=0.0.0.0：实测 400 `can't register ephemeral instance`
- docker exec emotion-echo-nacos curl /nacos/v1/ns/service?serviceName=...：实测 service metadata
- docker compose build：实测 5 svc Dockerfile 末尾 `\` 导致 `adduser: unknown group app`
- docker rm -f emotion-echo-nacos && up -d nacos：实测 Derby 重置 + 6 svc 新建 ephemeral=true
- python scripts/smoke_data_layer.py：实测 9/10 PASS

### ⑤ 列架构假设

| 假设 | 验证 | 结果 |
|---|---|---|
| PR-1 假设 ephemeral 被踢出 | 实测 Nacos 2.4.3 standalone Derby + 心跳正常 → 不踢出 |
| 0.0.0.0 让 Nacos 判 unhealthy | 实测 Nacos `healthy: true`（IP=0.0.0.0 但 healthcheck 通过） |
| resolveRegisterIP() 在容器内能拿到非 loopback IPv4 | 实测 IP 仍 0.0.0.0（Nacos UI 显示），fallback 可能没触发；但不影响 healthy |
| Dockerfile 末尾 `\` 是 bug | 实测 docker 把 RUN 当作文本延续，addgroup 找不到 | ✅ |
| Nacos standalone Derby 重启 = 状态清空 | 实测 rm + up 后 0 个 serviceName（confirms Derby 无持久 volume mount） |
| 6 svc rebuild 后 SDK 自动 ephemeral 注册 | 实测 ✅ | ✅ |

### ⑥ 写完后回填

本文档归档到 `docs/stages/stage-52-nacos-fix.md`。
3 个 commit 已合入 `stage-52-nacos-fix` 分支，领先 main 3 commit。
**未做 push 与合 main**——等 §4 业务数据问题修后 smoke 10/10 PASS 再合批 1。

---

## 五、与 PR-1 / stage-42 的关系（决策 18 登记）

### 5.1 PR-1 (commit b869ff9) 双重失真

**原 PR-1 commit msg**："Nacos 注册持久实例 + 本机 IP 解析 (PR-1)"

**实际效果**：
- ❌ "持久实例"假设错误（Nacos 2.4.3 + persistent service 拒绝 ephemeral 注册）
- ❌ "本机 IP 解析"未触发实测（Nacos UI 仍显示 0.0.0.0）

**配套测试**（`nacos_register_persistence_test.go`）：
- ❌ `assert.False(t, defaultRegisterEphemeral)`——只验证 false 状态，没验证"false 能跑通"
- ❌ 单测绿 ≠ 端到端绿（与 stage-50 §九.1 镜像滞后同根问题）

**决策 18 §二实例**：登记失真类型"未跑通即记录"。

### 5.2 stage-42 (commit c418a2f) 副作用失真

**原 stage-42 报告**："6 Dockerfile 删 `apk del tzdata`，已落地"

**实际效果**：
- ✅ Dockerfile 文件内容已改（删 `apk del tzdata`）
- ❌ 但顺手加了 `\`（应当是手抖）→ 5 Dockerfile 末尾 `\` + `RUN` 串联 bug
- ❌ "已落地"但**未 rebuild 验证**

**决策 18 §二实例**：登记失真类型"修复报告夸大"。

### 5.3 共同教训

> **"代码改了 ≠ 改对了"**——TDD 红灯 + 真环境 rebuild + 端到端冒烟，三关都过才算落地。

---

## 六、未做项 / 遗留 backlog

### ❌ A. §4 `/reports/daily` emotionDistribution=0（业务数据问题，与 Nacos 无关）

**实测**：smoke §4 FAIL：`summary='2026-09-08，你共有 0 段对话，0 条消息。整体心境 平稳。'` + `emotionDistribution.length=0`

**根因方向**（待 stage-53 调查）：
- 0 段对话 + 0 条消息 → analytics-svc 没消费到 chat-svc 事件？
- 还是 /reports/daily 查的视图/表为空？
- todo-pile §C7 早登记"Stage 36 dashboard 空根因未结案"——本 stage 不解决

**下一 stage 立即跟进**（用户拍板"归档 stage-52 后立刻推 §4 修复"）。

### ❌ B. Nacos IP 显示 `0.0.0.0`（resolveRegisterIP() 未生效）

**实测**：6 svc 注册 IP `0.0.0.0`（fallback 应是 172.18.0.x）

**可能根因**：
- `resolveRegisterIP()` 在 SDK 内部 `net.Dial("udp", "8.8.8.8:80")` 失败 → fallback 走 `net.InterfaceAddrs()` 也失败 → 返 `127.0.0.1`，但 Nacos 仍显示 `0.0.0.0`
- 实际代码可能根本没走 fallback 分支（Host 字符串没匹配 `""` 或 `"0.0.0.0"`）

**影响**：
- ⚠️ 视觉问题：Nacos UI 显示 IP `0.0.0.0` 不直观
- ✅ 不影响业务：svc `healthy: true` + BFF /health 全绿 + smoke §1/2/3/6 PASS

**建议**：下一 stage 顺手补 `resolveRegisterIP()` 的单元测试 + 调研实际返回 IP。**优先级低，不阻塞批 1 合 main**。

### ❌ C. PR-1 配套测试的反模式需要元层修复

`nacos_register_persistence_test.go` 测试只验证"默认值是 X"，**未验证"X 在真 Nacos 上能跑通"**。

**建议**：写一个 integration test（`nacos_register_integration_test.go` 扩 case），真起 Nacos container → Register → Sleep 35s → Discover 验证 instance 仍可见（验心跳可靠）。本 stage 不做（优先级 P2）。

---

## 七、解锁路径（按 AGENTS.md §2.4 推进）

批 1（`feat/observability-batch-1-infra`）合 main 的所有闸门：

```
[x] 修 dev Nacos 阻塞（本 stage-52 ✅）
[ ]  干净环境 smoke_data_layer.py 10/10 PASS（§4 业务问题待 stage-53）
[ ]  重建 5 svc 镜像（✅ 本 stage 已 rebuild）
[x] 端到端冒烟（BFF /health ✅ + Nacos ephemeral=true ✅）
[ ]  合 main：git checkout main && git merge --no-ff stage-52-nacos-fix
              git checkout main && git merge --no-ff feat/observability-batch-1-infra
```

预计 stage-53 完成 §4 修复后，路线 Z 第 1 批即可合 main。

---

## 八、与现有文档一致性修复（顺手做）

`docs/architecture/roadmap.md` Stage 51 后追加 Stage 52 条目 + "下一步行动"指向 §4 修复（已 commit `c28aeba` 的延伸）。

`docs/stages/stage-42-container-tz-fix.md`：本次修 stage-42 失真需同步登记——**未做**（PR 留给用户单独决定）。

`docs/architecture/decisions.md`：PR-1 失真登记到决策 18——**未做**（同上）。

---

> 最后更新：2026-09-08 by 当前协作 Agent
> 调研依据：stage-44 §四 E + stage-50 §九 + stage-51 §五 + nacos-enablement-dev.md §四 PR-1
> 测试结果：单测 100% + smoke 9/10 PASS（§4 业务问题独立 stage-53）