# ADR · 2026-09 · 环境配置分层策略（dev 本地调试 vs prod 远端部署）

> **决策状态**：✅ **Accepted**（2026-09-11 owner sign-off）
> **实施状态**：✅ 完成（2026-09-09 PR-ENV-1~4 全 landed + 43/43 TDD PASS）
> **关联**：
> - [stage-55-roadmap-z-status.md §九 P3](../../stages/stage-55-roadmap-z-status.md)（todo-pile §B1 BFF dev/prod 端口分离 半天）
> - [stage-57-b3-merged.md §三](../../stages/stage-57-b3-merged.md)（批 3 收口发现 SKYWALKING_ENABLED 真 bug 触发本次讨论）
> - [todo-pile-2026-09-04.md §B1](../../plans/todo-pile-2026-09-04.md)
> - [architecture/positioning.md](../positioning.md)（项目"单机多节点 + docker-compose"定位）

---

> **🔧 2026-09-10 决策 18 §4.4 就地更正块（登记于 doc-drift-registry #23）**：
>
> 上面头 4 行的"🟡 Proposed / ⏸ 未开始"描述**与事实严重失真**——候选 C 已在 2026-09-09
> 由 PR-ENV-1~4 全部落地：
>
> | Commit | 日期 | 内容 | 验证 |
> |---|---|---|---|
> | `ac0299e` | 2026-09-09 06:31 | PR-ENV-1 抽 `deploy/compose.dev.yml`（61 行） | `wc -l deploy/compose.dev.yml` → 61 |
> | `f400e65` | 2026-09-09 06:32 | PR-ENV-2 `apps.yml` 中性化 `${VAR:-default}`（18 处硬编码 → 中性） | `git show f400e65 --stat` → 2 files / 155+/16- |
> | `b89ecab` | 2026-09-09 06:33 | PR-ENV-3 `deploy/compose.prod.yml` 空壳占位（66 行） | `wc -l deploy/compose.prod.yml` → 66 |
> | `b2ea516` | 2026-09-09 06:35 | PR-ENV-4 `deploy/configuration.md`（179 行）+ QUICKSTART.md 同步 | `wc -l deploy/configuration.md` → 179 |
>
> **测试记录**（stage-58-q3-followups.md §二）：
> - `scripts/test_compose_override.sh`：10/10 PASS（dev.yml 覆盖项 + 语法合法）
> - `scripts/test_apps_yml_neutral.sh`：8/8 PASS（无硬编码）
> - `scripts/test_prod_yml_stub.sh`：11/11 PASS（空壳 + 语法合法 + 不引入新服务）
> - `scripts/test_docs_update.sh`：14/14 PASS（QUICKSTART 启动命令全替换）
>
> **结论修正**：
> - **决策状态**：✅ **Accepted**（候选 C 已落定，2026-09-09）
> - **实施状态**：✅ **PR-ENV-1~4 landed**（2026-09-09）
> - **唯一遗留**：本文档头 4 行未与事实同步——属"未复跑即记录"型失真（决策 18 §三 类型 2）
> - **整改要求**：当 owner 拍板 ADR-20 §5.1/5.2/5.3 三个待决问题时，一并把头 4 行状态字段更新为 Accepted（commit 形式追溯）
> - **同步动作**：本次会话已在 `decisions.md` 决策 20 区块与 `todo-pile §G` 反映此更正

---

## 一、上下文（Context）

Stage 57 批 3 收口时为修 `SKYWALKING_ENABLED` 默认 false 的真 bug，在 `deploy/docker-compose.apps.yml` 直接写死 `SKYWALKING_ENABLED: "true"`（6 处）。修复后引发一次内部讨论——**项目到底要不要做 dev / prod 环境分层？**

讨论暴露三个不一致：

1. **历史轨迹偏向"无区分"**：Stage 30 / Stage 33 / Stage 35 多次提到"dev compose = `infra.yml + apps.yml`"，从未实际建过 `compose.dev.yml` / `compose.prod.yml` 拆分文件。
2. **代码注释里有"dev only"标签**：`chat-svc/etc/chat-api.yaml:27-33` 注释"Kafka Enabled 默认 true"但 compose dev 默认 `KAFKA_ENABLED=false`；`BFF_DEV_RETURN_CODE` 在 `compose apps.yml` 默认 1 + todo-pile H.2.4 警告"prod 必须删掉"。
3. **项目定位是"单机多节点 docker-compose"**，但 roadmap（`roadmap.md`）规划"后续部署到远端服务器"——dev 与 prod 是**不同部署目标**而非同一台机器的两种模式。

现状是**"无明确分层 + 隐含假设 + 一堆 dev-only hack"**——既不是干净的无分层，也不是工程化的分层。

### 1.1 决策驱动

- Stage 57 收口的 `SKYWALKING_ENABLED` 默认值是否合理
- `BFF_DEV_RETURN_CODE=1` 默认值是否应该保留
- `KAFKA_ENABLED` 默认值（dev 默认 false、prod 应该 true）如何承载
- 远端部署时端口映射 / JWT secret / TLS / 资源限制 如何落地
- 是否要在 `apps.yml` 之上加 `compose.dev.yml` / `compose.prod.yml` 覆盖层

### 1.2 不决策的代价

- **不做分层**：dev 假设散落在 compose 文件、yaml 默认值、env 变量默认值、代码注释里，新人 / 远端部署者难以搞清哪些配置可以改、哪些必须改、哪些改了会爆
- **做过度分层**（如嵌套 `dev: {...} / prod: {...}` 配置结构、配置中心动态推环境差异）：单机场景下是过度抽象，引入复杂度但收益有限

---

## 二、候选（Options）

### 候选 A：**保持现状，不做显式分层**

继续把 dev/prod 差异混在 `compose apps.yml` + `etc/*.yaml` + 代码注释里，靠注释和部署文档口头约定。

| 维度 | 评估 |
|---|---|
| 实施成本 | 0（不动） |
| 远端部署可读性 | ❌ 差——`BFF_DEV_RETURN_CODE=1`、`KAFKA_ENABLED=false` 等 dev 假设散落各处 |
| 新人 onboarding | ❌ 差——需要通读 compose + 所有 etc yaml + 多个 stage 文档才能搞清 |
| 维护成本 | 高——任何配置项要改都要考虑对两套部署的影响 |
| 与本项目场景契合度 | **误判风险**：本项目实际是"dev 本地 + prod 远端"两种部署目标，不是不分场景的纯本地项目 |

### 候选 B：**单一 compose + env 变量驱动 + 配置分层判断清单**

保持 `compose apps.yml` 单一文件，所有差异用 `${VAR:-default}` 形式的 env 变量承载。**不建独立 dev/prod 覆盖文件**。写一份 `deploy/configuration.md` 列出所有 env 变量和默认值。

| 维度 | 评估 |
|---|---|
| 实施成本 | 低（半小时改 env 变量 + 写文档）|
| 远端部署可读性 | 🟡 中——env 变量一目了然，但端口映射 / 资源限制难用 env 表达 |
| 新人 onboarding | 🟡 中——读 `deploy/configuration.md` 即可 |
| 维护成本 | 低 |
| 与本项目场景契合度 | 🟡 部分契合——适合"配置差异纯粹是 env 值"的场景，但 compose 层差异（端口映射、deploy.resources）难以纯 env 化 |

### 候选 C：**compose override 文件分层 + dev/prod 明确拆分**

```
deploy/
├── docker-compose.infra.yml      # 中间件（dev/prod 共用）
├── docker-compose.apps.yml       # 业务 svc容器定义（中性）
├── docker-compose.dev.yml        # dev override（本地开发）
└── docker-compose.prod.yml       # prod override（远端部署）
```

启动命令：

```bash
# dev
docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml -f docker-compose.dev.yml up -d

# prod
docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml -f docker-compose.prod.yml up -d
```

`apps.yml` 保持中性默认，所有"dev 假设"或"prod 假设"都搬到 override 文件。

| 维度 | 评估 |
|---|---|
| 实施成本 | 中（1-2 小时抽 dev override + 写 prod override + 文档）|
| 远端部署可读性 | ✅ 好——`compose prod.yml` 一目了然看到 prod 期望 |
| 新人 onboarding | ✅ 好——compose 文件三层结构清晰 |
| 维护成本 | 中（两个 override 文件需同步基线变更）|
| 与本项目场景契合度 | ✅ 契合——dev 和 prod 是两套部署目标，override 层正好对应 |
| 工程化程度 | 与 K8s overlays / helm values 一致 |

### 候选 D：**单一 compose + 启动脚本分流**

写一个 `scripts/start.sh` 脚本，识别 `MODE=dev|prod` 环境变量，按模式注入不同 env 到 compose。**不用 compose override 文件**。

| 维度 | 评估 |
|---|---|
| 实施成本 | 中（脚本逻辑 + 文档）|
| 远端部署可读性 | 🟡 中——脚本里能看见差异，但 docker-compose 自身不显式 |
| 新人 onboarding | ❌ 差——必须先看脚本才能知道差异 |
| 维护成本 | 中（脚本与 compose 双源同步）|
| 与本项目场景契合度 | 🟡 部分契合——本质是候选 C 的脚本化变体，但失去 compose 原生优势 |

---

## 三、关键差异清单（无论选哪个方案都要梳理清楚）

| 维度 | dev（本地开发） | prod（远端部署） | 当前实现 |
|---|---|---|---|
| 部署目标 | 本机（开发机/笔记本）| 远端服务器 | dev：本地起容器；prod：未实现 |
| BFF 直连端口（8894）| 暴露（直连调试）| 不暴露（仅 APISIX 网关可达）| apps.yml 写了端口映射（dev 友好）|
| `BFF_DEV_RETURN_CODE` | 默认 1（验证码回显方便调试）| 默认 0（生产必须关）| apps.yml 默认 1 + 注释"prod 必须删" |
| `SKYWALKING_ENABLED` | true | true | apps.yml 写死 true（Stage57 加）|
| `KAFKA_ENABLED` | false（dev 跑不起 Kafka 是常态）| true（生产必须 Kafka）| apps.yml 写死 false（chat-svc）|
| AI profile（FER/SenseVoice/XTTS）| 默认关（dev 镜像可能没构建）| 启用（按需）| etc yaml 默认空 + 注释"启用 AI profile 后 compose 注入" |
| 日志级别 | DEBUG | INFO | 未配置（默认 INFO）|
| JWT secret | 测试值（`dev-test-secret-32bytes-min`）| 真随机 | etc yaml 写死测试值 |
| Nacos 命名空间 | `dev` | `prod` | compose 写死 `dev` 命名空间 |
| `STARTUP_STRICT_DEPS` | 0（warn 模式）| 1（fail-fast）| 未配置（默认 0）|
| 数据持久化卷 | docker 默认卷 | 命名卷 + 备份 | 默认卷 |
| 资源限制（CPU/内存）| 不限 | 显式 limit | 未配置 |
| TLS / HTTPS | HTTP | HTTPS（APISIX ssl 配置）| 未配置 |

---

## 四、讨论过程（Decision Log）

### 4.1 内部讨论初始判断

第一次内部讨论倾向"无分层"——理由是"项目是单机多节点 docker-compose，强行分 dev/prod 是过度抽象"。判断错误地把"单机"等同于"单部署目标"。

### 4.2 用户纠正

用户指出："**项目最终部署到远端服务器**"——也就是说 dev（本地）和 prod（远端）是两种部署目标。**只要有两种部署目标，就有分层需求**。

### 4.3 用户取舍

用户对四个候选的态度：

- **A 保持现状**：❌ 不接受（承认现状有"散落 dev 假设"问题）
- **B 单一 compose + env**：🟡 部分接受（认为可作候选）
- **C override 文件分层**：✅ **更倾向于保留**（认为是工程化、有价值）
- **D 启动脚本分流**：🟡 部分接受（视为 C 的变体）

但用户也强调：**不想要过度抽象**——不接受嵌套 `dev: {...} / prod: {...}` 配置结构、不接受配置中心动态推环境差异。

### 4.4 倾向方案

**用户倾向 C（override 文件分层）**，但保留 B（env 变量驱动）作为实施细节——即 C 方案内部也大量使用 `${VAR:-default}` 形式让 `apps.yml` 保持中性。

---

## 五、待决策点（Open Questions）

### 5.1 候选 A vs C 的最终取舍

- [ ] 用户是否最终确认选 C（override 文件分层）？
- [ ] 是否接受 `compose.dev.yml` + `compose.prod.yml` + `compose.apps.yml` 三文件结构？
- [ ] 是否要求 `apps.yml` 完全中性（无任何"假设"），所有 env 默认值都用 `${VAR:-default}` 形式？

### 5.2 实施边界

- [ ] `compose.prod.yml` 是先建**空壳占位**还是**写完整 prod 期望**？（远端部署未启动，先写空壳等真部署时填具体）
- [ ] Stage 57 加的 `SKYWALKING_ENABLED: "true"` 是否在 C 方案下保持 apps.yml 默认？还是挪到 dev.yml？（根据用户回答"保持 true 承认是默认推荐"——倾向保留 apps.yml 默认）
- [ ] `BFF_DEV_RETURN_CODE` 默认 1 vs 0 如何在 compose 文件里分布？（用户回答"保留现状默认 1，文档警告 prod 改"——倾向 dev.yml override）
- [ ] KAFKA_ENABLED、LOG_LEVEL、AI profile、JWT secret 等差异是否在 apps.yml 用 `${VAR:-default}`、在 dev/prod override 文件覆盖？
- [ ] 端口映射是否 dev.yml override 完全去掉宿主端口映射？（prod 不暴露 BFF 直连端口）

### 5.3 文档配套

- [ ] `deploy/configuration.md` 是否写？列出哪些 env 变量、默认值、dev/prod 差异
- [ ] 现有 `docs/QUICKSTART.md` 是否更新反映 dev/prod 启动命令差异？
- [ ] 是否要写一份 `deploy/prod-deployment.md` 描述远端部署步骤？

### 5.4 时机

- [ ] 当前是否落地？还是等真要远端部署时再做？
- [ ] 如果现在落地，候选 C 实施工作量约 1-2 小时，是否值得在该工作流停顿点做？

---

## 六、决策（Decision）

> **🟡 Proposed — 待用户最终拍板**

**倾向结论**（待用户确认）：

- 选 **C 方案（override 文件分层）**，但**只用 `${VAR:-default}` 形式让 apps.yml 保持中性**
- 立即落地：`compose.dev.yml` 抽出当前 apps.yml 的 dev 假设 + `compose.prod.yml` 建空壳
- 文档：`deploy/configuration.md` + 更新 `QUICKSTART.md`
- Stage 57 加的 `SKYWALKING_ENABLED: "true"` 改成 `${SKYWALKING_ENABLED:-true}` 形式

**否决理由**（为什么不选其他）：

- A 不可接受：现状散落 dev 假设是已知问题，不分层是逃避
- B 不够：compose 层差异（端口映射、deploy.resources）纯 env 难表达
- D 没必要：候选 C 的变体，丢失 compose 原生可读性

---

## 七、调研依据

| 调研项 | 来源 |
|---|---|
| Stage 30 / 33 / 35 历史 | 多次提到"dev compose = `infra.yml + apps.yml`"未建独立 override |
| Stage 57 触发本次讨论 | `stage-57-b3-merged.md` §三"SKYWALKING_ENABLED 默认 false"修复时引发 |
| 项目定位 | `architecture/positioning.md` "单机多节点 docker-compose" + roadmap "后续远端部署" |
| 当前 dev 假设散落 | `compose apps.yml` + `etc/*.yaml` + 代码注释（详见 §三清单）|
| 候选 C 工程化对照 | K8s overlays / helm values 范式（业内通用）|

---

## 八、ADR 元信息

- **ADR 编号**：待登记（建议 决策 20 或后续序号）
- **创建日期**：2026-09-08
- **创建触发**：Stage 57 批 3 收口讨论
- **关联决策**：决策 3（单机多节点 docker-compose）+ roadmap 远端部署规划
- **影响范围**：deploy/*（compose 文件结构）+ 文档（configuration.md / QUICKSTART.md）

---

> 最后更新：2026-09-08 by 当前协作 Agent
> 调研依据：Stage 30/33/35/55/57 + architecture/positioning.md + roadmap.md + 4 候选比较
> 用途：环境配置分层策略决策记录（待用户最终拍板）+ 落地后实施依据