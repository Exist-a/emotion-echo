> 日期：2026-09-12
> 来源：Stage 73 收口报告 §五 open 清单 → 用户指示"技术债收尾"
> 计划：[docs/legacy-plans/landed/tech-debt-closure-2026-09-12.md](../legacy-plans/landed/tech-debt-closure-2026-09-12.md)

## 〇、范围变更（计划期修正）

计划初期按 3 组技术债（lag 面板 / dev web 容器 / Helm 残余全量 7 PR）设计。计划评审阶段用户指出 **决策 3（K8s 备好不部署，decisions.md:73）**——据此 Helm 残余 5 项改判 **冻结**（新登记为决策 23），7 个 Helm PR 全部取消，改为纯文档处置。A（Grafana）/ B（web 容器）为 compose 路径照做。

## 一、§A Grafana lag 面板确认（4 commit）

探索结论颠覆了"面板待查"的记录：面板 JSON（uid `kafka-consumer-lag`，3 panel）、provider、双 volume 挂载、smoke 断言 8-10 早在 Stage 44 PR-OBS-7 就全落地了。Stage 74 实测揪出**两个真缺口 + 一个连带 bug**：

| # | 问题 | 根因 | 修复 |
|---|------|------|------|
| 1 | dashboard 3 panel 硬编码 datasource uid=`prometheus`，datasource.yaml 未写 uid | Grafana 生成随机 uid（实测 `PBFA97CFB590B2093`）→ 面板 datasource not found | datasource.yaml 显式 `uid: prometheus` / `uid: loki`；smoke 新增断言 10b 守护（RED→GREEN） |
| 2 | **Grafana 与 web 前端同抢宿主 :3000**（infra.yml:365 vs apps.yml:765） | 两服务从未同时起过，冲突一直潜伏 | Grafana 迁宿主 :13000（web 保留 3000：CORS 默认/QUICKSTART/入口约定）；smoke GRAFANA const + runbook 3 处联动 |
| 3 | apisix `worker_events.sock` EADDRINUSE crash 循环 | docker restart 保留容器可写层，残留 socket 不清理 | 启动 command 前置 `rm -f /usr/local/apisix/logs/*.sock`（docker restart 复测存活） |

**验收**：`smoke_observability.py` 13/13 PASS（uid='prometheus' 断言在列）；浏览器 GUI 截图确认 3 panel 渲染正常（ai-svc/analytics-svc lag 时序有数据线，DLQ stat 显示 No data = 无死信健康态）。

## 二、§B dev 栈 web 前端容器（历史首次 build 成功 + 连修 8 bug）

web 前端镜像**此前从未成功构建**（Stage 73 记录"node:20-alpine 拉取受限"只是第一层）。镜像源 retag（daocloud → `node:20-alpine`）后逐层炸出 8 个埋藏 bug：

| # | 层 | bug | 修复 |
|---|---|-----|------|
| 1 | 构建 | Docker Hub 匿名限流 | daocloud 镜像源 retag（不改 Dockerfile）；build_dev_images.sh 默认纳入 web + node:20-alpine 预拉（TDD，测试 10/10） |
| 2 | 构建 | npm lockfile（Windows 生成）缺 linux-x64-musl optional 原生依赖 → "Cannot find native binding" | Dockerfile 构建期 `rm -f package-lock.json`（npm/cli#4828 官方 workaround） |
| 3 | 构建 | `chartsCard.vue` import `radarChart.vue`，实际文件 `RadarChart.vue`（Windows 大小写不敏感掩盖） | import 改正大小写 |
| 4 | 构建 | 18 处相对路径 `../../lib/apiRoutes` 层级错误（页面/组件/composables/stores 全中招，dev 惰性加载掩盖） | 统一改 Nuxt 别名 `~/lib/apiRoutes` |
| 5 | 运行 | **生产构建白屏**（资源全 200 但 Vue 挂载 0 节点） | `nuxt.config.ts` 移除 `manualChunks`（手动分包打断 naive-ui/element-plus 类初始化顺序，经典 prod-only 坑） |
| 6 | 网关 | **登录全链路坏**：应用请求全部 `Failed to fetch` | seed.sh cors 字段名 `allow_credentials`（复数）→ APISIX schema 实际是 `allow_credential`（单数，cors.lua:237）——字段被静默忽略，`Access-Control-Allow-Credentials` 头从未发出（TDD：seed_test.js RED→GREEN） |
| 7 | 网关 | seed 修复后 PUT 400 | APISIX schema：`allow_credential=true` 时禁止其它字段用 `"*"`，auth 白名单路由 `allow_headers` 改显式列表 |
| 8 | 工具 | （过程性）注释误入 heredoc JSON 致 PUT 失败 | 注释移出 heredoc，`bash -n` 校验入流程 |

**验收**：`emotion-echo/web:v0.1.0` build OK；容器 healthy（:3000）；演示账号登录 e2e 浏览器实测通过（进入 /chat/conversation 主界面，侧边栏/对话区完整渲染）；页面内 fetch 实测 200 + accessToken。

> ⚠️ 值得注意：bug 5+6+7 意味着**容器化前端 + 网关 CORS 链路在本批之前从未端到端通过**。决策 11"网关是唯一业务入口"的承诺自 Stage 32 起实际只对 curl 成立，对浏览器（带 credentials 的请求）一直不成立。本批补齐。

## 三、§C Helm 冻结处置（决策 23）

- `decisions.md` 登记**决策 23**：Helm 残余 5 项（NACOS_ADDR 硬编码 / web 绕过网关 / xtts·sensevoice 镜像源 / 缺 MinIO·db-migrate·apisix-seed chart / values-prod 停更）在 K8s 不启用期内冻结，重启条件 = 多机迁移启动；决策变更记录表已登记
- 过期注释纠偏：`Chart.yaml`（"BFF 替代 APISIX"）、`values.yaml:30`（"APISIX + etcd 已退役"）、`values.yaml:62`——均指向决策 11（Stage 32 网关回归）
- `values-prod.yaml` 头部加停更声明（停更于 Stage 28-F，指向决策 3/23）
- 回写指向：todo-pile §D6、backlog-order 项 4 标注冻结，防后续 session 误捡
- 验收：`helm lint`（values-dev）0 failed

## 四、回归与销账

- `smoke_data_layer.py` **11/11 PASS**（web + grafana 共存态下复跑）
- `smoke_observability.py` **13/13 PASS**
- `seed_test.js` 38/38、`test_build_dev_images.sh` 10/10
- Stage 73 §五 open 表已逐项销账（Kafka P3 仍 open；eventrow 项已闭）

## 五、本批未做（open）

| 项 | 说明 |
|---|---|
| Kafka P3 | outbox relay dead 机制已有，告警走 alertmanager（计划 §3.6），仍 open |
| §1.4 可选增强 | consumer 进程级指标（消费速率/处理耗时）未埋 |
| nacos SDK 陈旧发现 30s 窗口 | backlog 项 3 残余（BFF PR-2 走 Discover 时需注意） |
| Nacos dev 全链路 PR-2（BFF 走 Discover） | Stage 72 完成 PR-1 后仍 open |
| Grafana k8s chart 侧 kafka dashboard | 随决策 23 冻结 |

## 六、调研依据

- 已读：deploy/grafana/provisioning/datasources/datasource.yaml、deploy/grafana/dashboards/*.json、scripts/smoke_observability.py、scripts/{build_dev_images.sh,test_build_dev_images.sh}、emotion-echo-web/{Dockerfile,nuxt.config.ts,package.json,app/components/report/chartsCard.vue,app/composables/useApi.ts,app/stores/user.ts,app/pages/login/index.vue}、deploy/{docker-compose.infra.yml,docker-compose.apps.yml,apisix/seed.sh}、deploy/apisix/seed_test.js、容器内 apisix/plugins/cors.lua（:237 allow_credential 单数字段实证）、charts/emotion-echo/{Chart.yaml,values.yaml,values-prod.yaml}、docs/architecture/decisions.md 决策 3/11
- 已查：kafka-reliability-gaps.md §1.4、stage-73 §五、stage-72 项 4、stage-59/36 先例、npm/cli#4828
- 命令证据：smoke_observability 13/13、smoke_data_layer 11/11、seed_test 38/38、test_build_dev_images 10/10、helm lint 0 failed、RED 三轮（uid 断言 / build_dev_images 断言 / seed_test 断言）
- 用户决策：Helm 冻结（决策 23）· nacos/apisix 命名空间问题随冻结不再展开 · 镜像源 retag

---

> 最后更新：2026-09-12 by Stage 74 实施 session
> 关联：决策 3 / 决策 11 / 决策 23、stage-73 §五、kafka-reliability-gaps §1.4、backlog-order 项 4
