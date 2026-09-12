# Stage 75 — Nacos PR-2 收口：BFF gRPC 拨号走 Nacos Discover

> 日期：2026-09-12
> 来源：Stage 74 收口报告 §五 open 表 → 用户指示"按推荐推进 + 修复 roadmap 文档漂移"
> 计划：[docs/legacy-plans/landed/nacos-enablement-dev.md](../legacy-plans/landed/nacos-enablement-dev.md) §四 PR-2

## 〇、调研修正（计划文档过期部分）

nacos-enablement-dev.md §1.2 "业务代码 0 处调用 Discover" 与 Stage 72 后的代码事实不符：
**PR-2 的 HTTP 路径已全部落地**（`web-bff/internal/discovery/resolver.go` 的 NacosResolver、
`downstream/resolver_integration_test.go` 契约测试、main.go 对 ai/chat/analytics 三个
HTTP client 的 Resolver 注入均已存在）。真正的缺口在 **gRPC 路径**：

1. dev 默认 `Transport=grpc`（决策 4，Stage 63 收口），5 处 `dialGRPC` 全部用 compose
   注入的容器 DNS（`*_SVC_GRPC_ADDR`）——gRPC 流量从未经过 Nacos
2. 根因：仅 ai-svc 注册 `metadata.grpc_port`（Stage 32 先例），user/chat/assessment/analytics
   都没带——BFF 即使想解析也无端口可查
3. 顺带发现潜伏 bug：main.go EmotionQuery 段用**无 portHint 的 HTTP resolver** 解析
   ai-svc，拿到 HTTP 端口 8891 覆盖正确的 env `AI_SVC_GRPC_ADDR=:8892`

## 一、落地内容（TDD RED→GREEN，2 commit）

### BFF 侧（web-bff）

- `main_grpc_discovery_test.go`（RED）：`resolveGRPCAddr` 四态契约（Nacos 有实例优先 /
  Discover 失败兜底 env / 空 host 兜底 / nil resolver 保持 Stage 63 行为）+
  `buildServiceContext` 5 处拨号地址全部来自 grpcResolver 的接线断言
- `main.go`（GREEN）：
  - 新增 `resolveGRPCAddr(resolver, fallbackAddr, svcName)`：Nacos 优先、env GRPCAddr
    兜底（Nacos 抖动不阻断启动），每次解析打来源日志
  - `buildServiceContext` 签名加 `grpcResolver`（`NewNacosResolver(...).WithPortHint("grpc_port")`），
    user/chat/assessment/analytics/ai 五处 `dialGRPC` 全部接入
  - EmotionQuery 段统一走 `resolveGRPCAddr`，修复 :8891 覆盖 :8892 的潜伏 bug

### svc 侧（user / chat / assessment / analytics）

- `nacos_boot_test.go`（RED）：注册 metadata 断言 `grpc_port` 正/负用例（对齐 ai-svc）
- `nacos_boot.go`（GREEN）：`cfg.GRPC.Enabled` 时写 `metadata.grpc_port`
  （user 8887 / chat 8892 / assessment 8886 / analytics 8885）

### dev 栈语义变化

compose 注入的 `*_SVC_GRPC_ADDR` 从主路径**降为 Nacos 故障兜底**——正是计划 §三.2
"服务间调用走 Discover（不再是 env 硬编码 DNS）"的目标形态，且保留故障韧性。

## 二、SDK 陈旧发现 30s 窗口评估（Stage 72 §3.3 残余销账）

结论：**不构成阻塞**。BFF 采用 boot 期一次性解析（非每请求重解析），陈旧窗口的影响仅限
启动瞬间；Discover 空/失败时回退 env GRPCAddr，栈照常启动。注销后 30s 内重启 BFF 的
极端场景由兜底地址覆盖。若未来引入运行期动态重解析，需先解决 SDK 空列表 push 延迟。

## 三、验收（dev 栈真实 e2e）

- 镜像重建（build_dev_images.sh 5 svc）+ 滚动重启（4 svc 先、BFF 后）
- Nacos 服务端：`instance/list?serviceName=emotion-echo-chat-svc` 返回 hosts[0].metadata
  `grpc_port: "8892"`（healthy=true）
- BFF 日志：5 处 `[nacos] resolve ... -> 172.18.0.x:<grpc端口> (grpc)` + `[grpc] ... connected`
  全部命中容器 IP + gRPC 端口；**ai-svc 解析到 :8892**（潜伏 bug 修复的真实环境确认）
- `smoke_data_layer.py` **11/11 PASS**（含 §契约 4 /reports/daily 经 BFF→analytics gRPC 链路）
- 6 模块 `go vet` + `go test ./...` 全绿

## 四、本批未做（open）

> ⚠️ **2026-09-12 排期核查更正**（详见 stage-76）：本表前两行的 PR-3/PR-4/PR-5 实现已在
> **Stage 39（2026-09-04）** 落地（seed.sh nacos-discovery / HotReloadLimiter `7e7d59a` /
> fail-fast `86e570e`），当时误列为 open；真实残余是 PR-3 的 dev e2e 验收，已在 Stage 76 补跑。

| 项 | 说明 |
|---|---|
| ~~Nacos PR-3~~ | ~~APISIX upstream 切 nacos-discovery~~ → 实现已在 Stage 39；e2e 验收 Stage 76 完成 |
| ~~Nacos PR-4/PR-5~~ | ~~HotReload 真启用 / BootNacos fail-fast~~ → 均已在 Stage 39 落地 |
| Subscribe 动态感知 | 当前 boot 期解析一次；实例变更（扩缩容）需重启 BFF 或后续接 Subscribe |
| Kafka P3 / consumer 进程级指标 | Stage 74 §五 残余，不变 |

## 五、调研依据

- 已读：emotion-echo-web-bff/{main.go,nacos_boot.go,main_grpc_test.go,internal/discovery/resolver.go,
  internal/downstream/{ai,chat,resolver_integration_test}.go}、emotion-echo-{user,chat,assessment,analytics}-svc/
  {nacos_boot.go,nacos_boot_test.go,internal/config/config.go}、emotion-echo-ai-svc/{nacos_boot.go,nacos_boot_test.go}、
  emotion-echo-shared/pkg/discovery/{registry.go,servicenames.go}、deploy/docker-compose.apps.yml（web-bff 段）、
  scripts/build_dev_images.sh
- 已查：docs/plans/nacos-enablement-dev.md（§1.2/§三/§四 PR-2）、stage-72 §3.3、stage-74 §五、
  backlog-order-2026-09-12.md 项 3、decisions.md 决策 4/10/11
- 命令证据：RED 5 组（web-bff 编译红 + 4 svc 断言红）→ GREEN 全绿；`go vet` 6 模块 0 err；
  smoke_data_layer 11/11；Nacos open-api JSON（grpc_port 字段实证）；BFF 容器日志 grep（5 处 resolve）
- 关联：决策 4（gRPC）/ 决策 10（Nacos 演进引入）/ nacos-enablement-dev PR-2 / stage-72 §3.3

---

> 最后更新：2026-09-12 by Stage 75 实施 session
> 关联：nacos-enablement-dev.md §四 PR-2、backlog-order-2026-09-12.md 项 3、stage-74 §五
