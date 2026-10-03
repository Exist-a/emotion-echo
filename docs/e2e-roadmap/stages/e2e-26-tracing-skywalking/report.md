---
stage: e2e-26
title: 链路追踪 SkyWalking（sw8 传播 + OAP 查询 + UI 可视化）
executed: 2026-10-03
status: done
environment: dev 模式（30 容器，compose -f infra -f apps --env-file .env.local --profile dev --profile ai；OAP/UI 9.7.0；宿主内存 9.7G）
---

# E2E-26 执行记录

## 1. 环境基线

- 启动命令：`bash scripts/dev-up.sh`（错峰分批；**rc=0 且漂移段真实执行** `[apisix-drift] no extras`——F-181 修复后首验）
- 中途经历：宿主内存两次击穿 → 用户重启电脑 → WSL/Docker 恢复 → 再次全栈 dev-up + 手动补起 xtts/sensevoice/fer（readiness 含 xtts 依赖，dev-up 批 3↔批 4 顺序倒挂在冷启动场景会卡，report 备查）
- 关键容器：6 应用 svc + sw-oap/sw-ui 9.7.0 + apisix + loki/promtail 全 healthy；Nacos `count=6`
- **OAP H2 内存库**：期间 OAP 重启 1 次（#19/#20 故障注入），历史 trace 清空为预期行为，证据链完整
- 服务 tracer：6/6 `tracer initialized`；OAP `listServices` 6 服务
- 截图渲染：IAB 及 Playwright 首轮均出现**渲染帧与 DOM 不同步**（F-184），处置 = 截图证据改由 Playwright 独立栈产出 + 视口抖动强制重绘，全部截图人工目视验收

## 2. 测试点结果

| # | 测试点 | 判定 | 结果 | 证据 | 备注 |
|---|--------|------|------|------|------|
| 1 | tracer 初始化与服务注册 | [A] | PASS | 6/6 svc 日志 `tracer initialized`；`query_oap.sh services` 返回 6 个 emotion-echo 服务（id/name/normal=true）；OAP 重启后 reporter 自动重连再注册（`send keep alive error` 停机窗口日志 → 恢复后 listServices 回满） | — |
| 2 | HTTP 入口 span 四 tag | [A] | PASS | `queryTrace(161080f9/cb39e879)`：web-bff Entry span type=**Entry**，tags=`http.method=GET, http.url=/api/v1/users/me, user_id=1, http.status_code=200` 原文回读 | StartEntry 旧实现产 Exit 型 span，已随 #3 修复 |
| 3 | gRPC 跨进程 ≥3 服务同 trace | [A] | PASS | `queryTrace(161080f9)` = 3 服务（web-bff Entry+3Exit / chat-svc Entry×2 refs=1 / ai-svc Entry refs=1，CROSS_PROCESS）；`cb39e879` 同形 7 跨度 | **途中修复 4 组真缺陷**（见 §4）：bff 拨号顺序 / 5 svc SetTracer / server EntrySpan+承载 ctx / client injector 空串覆盖 |
| 4 | Kafka 跨进程（producer↔consumer 同 trace） | [A] | PASS | 4 对全等：chat-svc `kafka-publish` traceId == analytics-svc + ai-svc `kafka-consume` traceId（cc675984 / 549a8411 / 16a36b05 / e3fa0ec），refs CROSS_PROCESS + `event.type=message.created` tags | publish 为独立根（outbox 异步）→ F-183 记账；消费日志 join 依赖 F-185 修复（见 #17） |
| 5 | APISIX access log → OAP | [A] | PASS | `query_oap.sh logs 30` = 4 条 APISIX access JSON（含 request headers/upstream/latency）；**负向**：health 路径条目 = 0（`seed.sh:543` 探针豁免生效） | batch flush 5-60s，首查过早为空属时序非缺陷 |
| 6 | OAP 宕机降级两向 | [A] | PASS | warn 向：`tracer init failed (warn mode, continue): dial ... i/o timeout`（真实启动日志）+ svc 照常服务；strict 向：`docker run` 死端口 + `STARTUP_STRICT=true/DEPS=skywalking` → `strict mode + required dep, refusing to start` | STARTUP_STRICT 默认 false（deps.go:16） |
| 7 | span 错误语义 | [A] | PASS | 504 注入（停 user-svc→直连 BFF）→ trace `2c84e50d`/`669d6ef7` **isError=True**；恢复后 200 → isError=False；UI 列表红行 5011ms 可见 | buildSpanError（status≥500） |
| 8 | OAP 查询工具契约（TDD） | [A] | PASS | `test_query_oap.sh` C0-C4 全绿（rc=0）；运行时 `services`/`traces`/`logs`/`trace`/`dry-run` 子命令真查询回读 | RED 先红（工具不存在）→ GREEN |
| 9 | 官方格式查询 | [A] | PASS | `query_oap.sh traces emotion-echo-web-bff` 等返回真实 trace 数据（SECOND=yyyy-MM-dd HHmmss 无冒号） | 多次运行时回读 |
| 10 | 带冒号格式反向对照 | [A] | PASS | 容器网实发带冒号查询 → `Invalid format: "2026-10-03 00:30:00" is malformed at ":30:00"`（与 2026-09-14 stage-92/93 记录的 "malformed at :00:00" 同型） | 两向对照留档 |
| 11 | queryDuration 定性 [M] | [M] | PASS | **用户裁定：判客户端格式错**（2026-10-03）；D-37 登记；stage-92/93 顶部 + known-issues-backlog Item 4 三处回填并关闭 | 备选①采纳；证据=官方格式无冒号+实测同型报错+上游 issues 零命中 |
| 12 | smoke 契约 9 修复 | [A] | PASS | `test_smoke_oap_contract.sh` 5/5 绿（禁宿主直连/伪签名/best-effort）+ 旧结构守卫仍绿；**运行时 smoke**：契约 9a（listServices 含 chat-svc）+ 9b（近 5min 非健康 RPC span）双 PASS | 契约 4/7 恒 401（缺 Bearer）= 范围外 → F-182 归 E2E-29 |
| 13 | sw-ui 宿主入口 [M] | [M] | PASS | **用户授权执行者按部署考量评判 → D-38：127.0.0.1:18080 限定映射**（业界惯例/不扩大暴露面/生产走 k8s charts 解耦）；infra.yml 落地 + Stage 33 注记回填 + 实测 `GET 127.0.0.1:18080 → 200` | OAP 12800 保持不映射（工具容器网封装） |
| 14 | IAB/浏览器查聊天 trace 树 | [V] | PASS | `screenshots/26-14-trace-tree-cross-service.png`（目视验收：服务=bff、traceId 04dedf12、跨度 5、树结构跨进程展开、多服务 chips） | IAB 渲染失真改 Playwright 产出（F-184）；3 服务完整版（cb39e879 跨度 7 chips×3）有 09:45 运行时 queryTrace 文字留档，末版截图因当次 GetEmotion 未触发为 2 服务（bff↔chat 已跨进程） |
| 15 | 服务拓扑/完整调用链 | [V] | PASS | `screenshots/26-15-service-topology.png`（目视验收：User→web-bff→user-svc/chat-svc→ai-svc/analyti… 深度 2，Healthy 图例） | backlog Item 4 DoD 后半达成 |
| 16 | 错误 trace UI 可见 | [V] | PASS | `screenshots/26-16-error-trace-detail.png`（目视验收：追踪ID=669d6ef7、红行 /api/v1/users/me 5011ms、详情跨度 2） | 504 注入轮产出 |
| 17 | OAP trace ↔ 日志 join 分路径 | [A] | PASS | A（HTTP）：BFF 日志 `trace_id=38dee19f...`（APISIX request_id）**∉** OAP traceIds（669d6ef7 等）——不等成立；B（Kafka）：**F-185 修复后** unknown-type 探针消费日志 `trace_id=7f7ae3a8bece11f19f8fdab0016b00cf` = sw8 base64 解码明文 = **OAP kafka-publish traceId 三方同值** | 途中修复 F-185（见 §4）；Kafka 成功路径 INFO 静默无日志行，探针以 unknown-type 触发 `:389` 带 ctx 日志 |
| 18 | 双 trace ID 处置 [M] | [M] | PASS | **用户授权执行者评判 → D-39：保留双 ID + 文档化**（真一致需 APISIX 进 trace 树=网关专项，单改应用侧只是挪动不一致位置、净收益假 + 动 E2E-21 已验语义）；D-39 含互查路径口径 | APISIX 接入 trace 传播列候选账本（D-39 内文） |
| 19 | 存储生命周期（H2 重启丢数据） | [A] | PASS | OAP stop→start 后：重启前 traceId `7f7ae3a8...` → `queryTrace spans:[]`（数据清空）；重启前 30min 窗 5 条 → 重启后查无 | 持久化改造按 plan 范围外记账不改 |
| 20 | OAP/UI healthcheck 三态 | [A] | PASS | 停 OAP：查询输出 `query_oap: curl 请求失败: Could not resolve host: skywalking-oap` + **退出码 rc=1**（C4 负向对照，失败即红不静默）+ 容器 3s 翻 **unhealthy**（telemetry :1234 探测日志）+ sw-ui **仍 healthy**（healthcheck 只探自身 8080 静态页，不反映后端依赖——如实记录为探针语义局限）；恢复：9s healthy + 服务重注册 + 新 trace `dd90fca7` 查询输出采集恢复 | E2E-23 #26 healthcheck 回归 |

汇总：PASS 20 / FAIL 0 / BLOCKED 0 / N/A 0

## 3. 发现与分类

| 发现 | 分类 | 处理 |
|------|------|------|
| web-bff main 中 SkyWalking 初始化排在 buildServiceContext 之后 ⇒ packageTracer 恒 nil、全部下游 gRPC 不挂 tracing 拦截器（OAP 无 client: exit span） | 范围内 | 修复 C1（静态守卫 main_skywalking_wiring_test.go + 重排） |
| `skywalking.Init()` 全仓零调用 ⇒ 5 个 gRPC server 的全局 tracer 恒 nil（服务端 span/sw8 提取全灭） | 范围内 | 修复 C2（SetTracer + 5 svc 装配守卫） |
| server 拦截器用 StartEntry（不提取 sw8、返原 ctx）+ gin 中间件丢弃承载 ctx ⇒ 每跳各起新 trace、span type=Exit | 范围内 | 修复 C3（CreateEntrySpan+md extractor / StartEntry 承载 ctx / gin 装回） |
| client injector 忽略 key ⇒ go2sky 的空 sw8-correlation 覆盖 sw8 为 "" ⇒ 下游按无 sw8 起 root（bufconn RED 实锤） | 范围内 | 修复 C3 附属（key 过滤 + 拒空） |
| `/health/ready` 未进 SKIP_PATH_LIST 默认值 ⇒ 每 30s 一条探针 span 污染 OAP | 范围内 | 修复 C4（默认加 `/health/` 前缀） |
| TraceIDFromSW8 未 base64 解码（F-146 半修）⇒ 消费日志与 OAP 永远 join 不上 | 范围内 | 修复 F-185（TDD + 重建 analytics 运行时三方复证） |
| dev-up 漂移检查路径在 cd 后不可解析 + 127 误报漂移（F-181） | 记账（owner=E2E-25） | **顺手修复**（A5 对 e2e-25 红灯阻塞审计）：TDD 6/6 + 运行时 rc=0 真执行，F-181 翻 ✅ |
| smoke 契约 4/7 缺 Bearer 恒 401（F-182） | 范围外 | 记账归 E2E-29/E2E-03 |
| outbox 发布 span 独立根，入站 RPC trace 断在 outbox 边界（F-183） | 跨阶段设计 | 记账归 E2E-30/专项（需 ADR；#4 按 plan 判据 PASS 不阻收口） |
| IAB/Playwright 渲染帧与 DOM 不同步，截图证据失真（F-184） | 工具/环境 | 记账 + 处置：截图改 Playwright 栈 + 视口抖动 + 全部人工目视验收 |
| dev-up 批 3（业务 svc）依赖 web-bff healthy，而 web-bff readiness 依赖 xtts、xtts 在批 4 —— 冷启动全停场景死锁 | 范围外（编排） | 本阶段手动补起绕过；report 备查（与 F-181 同族，归 E2E-25 编排域，未另登账避免碎片化） |

## 4. 修复清单（TDD 记录）

| commit 类型 | 内容 | 先行的失败测试 |
|--------|------|---------------|
| test→fix | web-bff SkyWalking 初始化顺序（C1） | `main_skywalking_wiring_test.go`（RED：8130 ≮ 7271） |
| test→fix | shared SetTracer + 5 svc main 装配（C2） | `settracer_test.go`（undefined: SetTracer）+ `svc_wiring_test.go`（5 svc 缺调用） |
| test→fix | server CreateEntrySpan/sw8 提取 + StartEntry 承载 ctx + gin 装回 ctx（C3） | `tracing_server_entry_test.go` / `ReturnsSpanBearingCtx` / `gin_skywalking_ctx_test.go` 三 RED；`client_sw8_bufconn_test.go` RED（md.Get("sw8")=[$空串]）钉根因 |
| test→fix | client injector key 过滤拒空（C3 附属） | 同上 bufconn RED→GREEN |
| test→fix | SKIP_PATH_LIST 默认加 `/health/`（C4） | `TestShouldSkipPath_DefaultFallback` 扩用例（RED got=false） |
| test→fix | `scripts/query_oap.sh` 查询工具（#8） | `test_query_oap.sh` C0-C4（RED：工具不存在） |
| test→fix | smoke 契约 9 三重空转修复（#12） | `test_smoke_oap_contract.sh` 5 断言（RED 5/5） |
| test→fix | dev-up 漂移路径固化 + 缺失≠漂移（F-181） | `test_devup_drift_check.sh` 新 2 断言（RED 2/2） |
| test→fix | TraceIDFromSW8 base64 解码 + hex 校验（F-185/#17） | `TestTraceIDFromSW8_Base64EncodedSegment`（RED：got=b64 want=明文） |

镜像重建：bff + 6 svc 两轮（共享库变更），全部 `go build` / `go vet` / `go test` 绿；shared `./pkg/... -count=1` exit=0。

## 5. 回归钉

- 新增 spec：`emotion-echo-web/e2e/skywalking-trace.spec.ts`（3 用例：跨进程流量自造 / Trace 查询+3服务 chips+树截图 #14 / Topology 拓扑截图 #15）
- 首次全量运行：`BASE_URL=http://127.0.0.1:18080 npx playwright test e2e/skywalking-trace.spec.ts --project=chromium` → **3 passed (21.6s)**，截图落盘并人工目视验收
- 自包含：spec 内先经 APISIX 造 login/users-me/ai-stream 流量，不依赖历史数据

## 6. 待决策 / 升级项

无未决项（M1/M2a/M2b 均已裁定/授权评判并落地 D-37/D-38/D-39）。

跨阶段转挂（不阻本阶段）：F-182（E2E-29）、F-183（E2E-30/专项，需 ADR）、F-184（账本治理/上游）、dev-up 批序倒挂（E2E-25 编排域，report 备查未登账）。

## 7. 收口自检

- [x] git status 干净（提交后）
- [x] main 与 origin 无 ahead/behind（合并后）
- [x] 无残留已合并分支（合并后删）
- [x] `e2e_stage_audit.py --all` 0 FAIL（见收口轮）
- [x] 20/20 四值判定、BLOCKED=0、N/A=0
- [x] 名下账本：F-185 ✅ 已解决（owner=E2E-26）；F-181 顺手修复翻 ✅；F-182/183/184 归属他阶段

## 8. 第二方核对

按 RUNBOOK §13.3 由**独立子代理**执行（执行者不自证；核对方全部结论来自其亲自运行的命令与亲自读取的文件/图片，未引用本 report 的声称）。

## 9. 第二方核对结论（2026-10-03）

**总结论：同意判 done；阻塞问题：无**（12 项 11 PASS，1 项为本节回填本身）。

| 项 | 结论 | 核对方独立证据 |
|----|------|----------------|
| 机器门槛 | PASS | `e2e_stage_audit.py --all` → 30 阶段 0 FAIL（EXIT=0）；`--stage e2e-26` ✅ done；A3 对 e2e-26 真实执行（无 skip WARN）且 20⊆20 |
| A3 编号对账 | PASS | plan={1..20}、report={1..20}，双向差集为空 |
| A2 汇总计数 | PASS | 20 行数据行，`Counter({PASS: 20})` 与汇总行逐项相等 |
| 必填章节 | PASS | §1~§7 齐全 |
| 证据抽查 | PASS | #3/#12/#17/#19 均为命令+输出型；**亲跑** `test_smoke_oap_contract.sh`(5/5)、`go test TraceIDFromSW8`(全绿)、`test_query_oap.sh`(C0-C4)、`query_oap services/traces`(真数据) |
| [V] 截图 | PASS | 3 文件非空（67400/42509/65877 B），逐张目视与 report 描述一致（含 #14 末版 2 服务的如实披露） |
| 账本对账(A5) | PASS | owner=E2E-26 仅 F-185 且 ✅；`test_devup_drift_check.sh` 亲跑 6/6 GREEN（F-181 修复复证） |
| A9 三处状态 | PASS | plan/report=done、roadmap ✅ done + last-refresh 同步 |
| 回归钉 | PASS | 亲跑 spec → **3 passed (21.2s)** |
| 裁决落地 | PASS | D-37/38/39 行结构与 D-36 同构；stage-92/93 注记在文；infra.yml 映射在位 + 宿主 200 |
| 行号回读 | PASS | seed.sh:543 探针豁免、deps.go:16 STARTUP_STRICT 默认 false 均精确吻合 |

**核对方建议（不阻 done）及处置**：
1. ~~plan front-matter 预先写死"核对通过"属先于事实的自证措辞~~ → **本轮已修正**（改为以本节结论为准）
2. 18080 端口潜在冲突：`skywalking-ui`（默认 profile）与 `obs-mock-receiver`（profile obs，infra.yml L472）同宿主端口，同时启用 `--profile obs` 时绑定撞车 → **已在 infra.yml D-38 注记处标注**；如未来启用 obs profile 需改 mock 宿主端口
3. 残留容器 `e2e26-sw-ui-proxy`（M2 备选③试验遗留）→ **已删除**
4. 账本 F-180 坏行（8 格）致全阶段 A8 WARN——owner=账本治理/审计器（既有 F-180 修法条目），非本阶段，留其修法执行
5. spec 用例名"3 服务树"与末版截图（2 服务）口径落差——report #14 已如实披露 + 09:45 三服务版有 queryTrace 文字留档；后续可让用例名与断言口径对齐（记入下阶段顺手项）
