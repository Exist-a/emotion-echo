# Stage 76 — Nacos PR-3 dev e2e 验收 + 文档销账

> 日期：2026-09-12
> 来源：Stage 75 收口后排期核查 → 用户指示"先做首选"
> 计划：[docs/legacy-plans/landed/nacos-enablement-dev.md](../legacy-plans/landed/nacos-enablement-dev.md) §四 PR-3
> 性质：**验收批次（0 业务代码改动）**——PR-3 实现已由 Stage 39（2026-09-04）落地，本批补跑
> 其从未执行过的 dev e2e 验收（stage-39 §七.2），并办理计划文档迁移销账。

## 〇、排期核查结论（Stage 74/75 open 表文档漂移更正）

Stage 74/75 open 表把 "Nacos PR-3（APISIX upstream 切 nacos-discovery）" 列为未启动（半天），
经代码核实 **PR-3/PR-4/PR-5 已在 Stage 39 全部落地**：

| 项 | Stage 39 证据 |
|---|---|
| PR-3 APISIX nacos-discovery | `deploy/apisix/seed.sh` `put_nacos_upstream()`（6 upstream）+ `deploy/apisix/test_seed_nacos.sh` + chart configmap 顶层 `discovery.nacos` |
| PR-4 HotReload | web-bff `nacos_boot.go` ListenConfig + HotReloadLimiter（commit `7e7d59a`） |
| PR-5 fail-fast | `shared/pkg/discovery/failfast.go`（commit `86e570e`）+ 6 svc main.go 接入 |

漂移根因：stage-74/75 收口时只核对了计划文档 §四，未回查 stage-39（决策 18 的同类复发）。
真实残余只有 **PR-3 的 dev e2e 验收从未跑过**，即本批内容。

## 一、验收执行记录（dev 栈真实 e2e）

| # | 验收项 | 结果 |
|---|---|---|
| 1 | `seed.sh` 重跑 | 6 个 nacos upstream OK（`discovery_args` 嵌 namespace/group）+ 12 routes + jwt-auth consumer |
| 2 | `test_seed_nacos.sh` 契约 1（upstream 形状） | 6/6 PASS：discovery_type=nacos、service_name 长名、discovery_args 正确、顶层无 namespace_id、无静态 nodes |
| 3 | 契约 2（Nacos 注册表有实例） | 6/6 PASS：每个 svc hosts≥1 |
| 4 | 契约 3（经 APISIX 登录） | PASS：accessToken 返回——APISIX→BFF→user-svc 全链路走 nacos-discovery |
| 5 | 契约 4（受保护端点） | PASS：/users/me /conversations /surveys 经 APISIX 200 |
| 6 | 契约 5（X-User-Id 不可伪造） | PASS：伪造 999 被覆盖为实际 userId=1 |
| 7 | 契约 6（未鉴权 401） | PASS ×2 |
| 8 | **不健康实例摘除** | `docker stop user-svc` → t+15s 超时 → **t+30s 起 /user-health 稳定 503**（符合计划"30s 内摘除"验收） |
| 9 | **恢复** | `docker start user-svc` → 重注册 healthy=true → `/user-health` 回到 200 |
| 10 | §契约 7 `smoke_nacos_registry.sh` | 6/6 svc registered |
| 11 | 回归 `smoke_data_layer.py` | **11/11 PASS**（含 §契约 4 经 BFF→analytics gRPC 链路） |

**PR-3 验收结论：全项通过。** Nacos 治理层（注册 / 发现 / 网关发现 / 配置中心骨架 / fail-fast）
在 dev 栈端到端可用。

## 二、验收过程中发现并处置的环境/栈问题（3 起）

1. **APISIX 容器重启循环**：容器内残留 `worker_events.sock` 未清理（同容器 restart 后
   `bind() failed (98: Address already in use)`）。处置：`--force-recreate` 重建容器。
   日志同时确认 `use discovery: nacos` 配置生效。
2. **Docker Desktop 端口转发卡死 ×2**（宿主机侧 `Empty reply from server`，容器内正常）：
   Nacos :8848、BFF :8894 先后中招，`docker restart` 对应容器重建端口映射即恢复。
   纯环境问题，非项目代码缺陷；smoke 脚本遇到 000/空回复时可先怀疑此症状。
3. **5 业务 svc 带 nil repo 启动（dev 宽容策略的固化效应，⚠️ 遗留风险登记）**：
   13:01 重建 APISIX 时 Docker DNS 抖动，5 个 svc 恰好重启，`openPostgres` 瞬时
   `no such host` → 按"dev 不阻断"策略带 nil repo 启动 → user-svc Login RPC 在
   `authlogic.go:62`（`l.svcCtx.UserRepo.GetByUsername`）panic，BFF 兜底为 502。
   处置：统一 `docker restart` 5 svc 后恢复。**根因不是代码 bug，而是"Postgres 连接失败
   静默降级"与"瞬时 DNS 故障"叠加后无自愈**——已登记 open 项（见 §四），候选修法：
   启动期 Postgres 重试退避 / 或按 PR-5 模式将 DB 列为 required 依赖 fail-fast（dev 亦然）。

## 三、文档销账

- `docs/plans/nacos-enablement-dev.md` → **迁入 `docs/legacy-plans/landed/`**，
  front-matter 改 `status: landed`（landed-stages 列 39/72/75/76，residuals 列 3 项）
- stage-75 §四 open 表就地更正（PR-3/4/5 行划掉 + 指向本报告）
- roadmap「下一步行动」快照刷新（前一批 commit `0e431ee` 已预排，本批勾销第 1 项）

## 四、本批未做（open）

| 项 | 说明 |
|---|---|
| **Postgres nil repo 无自愈**（本批新登记） | 启动期 DB 连接失败静默降级 + 瞬时 DNS 故障 → svc 长期带 nil repo 跑、RPC panic。候选：启动重试退避 / DB 纳入 fail-fast required 依赖 |
| Kafka P3 | outbox relay dead 告警接 alertmanager（kafka-reliability-gaps.md §3.6，小） |
| Kafka §1.4 可选 | consumer 进程级指标（消费速率/处理耗时埋点，小） |
| 业务功能排期 | ai-response-structured / intent-classification-6-types / file-upload-message-extension（均 status: planned，未排期） |
| Nacos 深水区（可选） | SDK 升级 v2.4.x（ListenConfig 偶发不回调）/ Subscribe 动态感知（暂缓）/ llm-service Python 端注册 |

顺带观察（不在本批范围）：ai-svc→llm-service gRPC 健康检查间歇 `server preface: EOF`
（llm-service 容器 healthy，业务链路未受影响），留观。

## 五、调研依据

- 已读：deploy/apisix/{seed.sh,test_seed_nacos.sh}、deploy/docker-compose.{infra,apps}.yml
  （apisix/apisix-seed 段）、emotion-echo-user-svc/{main.go,internal/logic/authlogic.go,
  internal/grpcserver/user_server.go,internal/svc/servicecontext.go,
  internal/repository/user_repository.go}、docs/stages/stage-39-nacos-enablement.md
  （§2.2/§三/§七）、docs/stages/stage-75-nacos-pr2-grpc-discovery-2026-09-12.md §四
- 已查：docs/legacy-plans/landed/backlog-order-2026-09-12.md、docs/_meta/doc-migration-map.md、
  git log（7e7d59a PR-4 / 86e570e PR-5）、Nacos open-api `instance/list` 实测 JSON
  （metadata.grpc_port 字段实证）
- 命令证据：seed.sh 6 upstream OK；test_seed_nacos.sh **23 项断言全 PASS（EXIT=0）**；
  stop/start user-svc 摘除-恢复实测（503@t+30s → 200@recover）；smoke_nacos_registry 6/6；
  smoke_data_layer **11/11 PASS**；docker logs 三起环境问题的原始报错已摘录 §二
- 关联：决策 10（Nacos 演进引入）/ 决策 11（网关唯一入口）/ stage-39 / stage-72 / stage-75

---

> 最后更新：2026-09-12 by Stage 76 实施 session
> 关联：nacos-enablement-dev.md（已迁 landed）、stage-39 §七.2、stage-75 §四（已更正）
