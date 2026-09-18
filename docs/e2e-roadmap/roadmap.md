---
status: active
priority: critical
created: 2026-09-17
last-refresh: 2026-09-18 (第二方核对：R-01 未完成，R-02/R-03 部分完成)
type: e2e-stage-roadmap
---

# E2E 阶段式测试路线图（长期）

## R 系列核对结论（2026-09-18 第二方核对，非执行者自证）

> **结论：R-01 未完成，E2E-07 仍被阻塞。** 七项中 #2 实测**未通**：BFF 与 user-svc 两端都已实现 `/api/v1/auth/verify-security-answer`，但 APISIX 白名单只注册了 110~115（login / register / verification-code / refresh / logout / reset-password），该路径会落到 route 100（`/api/v1/*`，挂 jwt-auth）⇒ 未登录态的找回密码调用必然 401。前端经 APISIX `:19080` 调用，故该链路实际不可达。
>
> **核对为真的部分**（已独立复现，非引用他人结论）：`go vet ./...` 7/7 模块通过；`pnpm test` 366/366；`pnpm typecheck` 退出码 0；`migrate.sh` checksum 负向测试通过；密保按 D-05 改为可选；演示账号与 initdb 解耦；`phone`/`email` 已移出权威 DDL。
>
> **CI 精确表述**：main HEAD 上实际只运行了 `go-test` + `doc-drift-check`（均绿）；`web-test` 最后一次运行在 `e31f419`、`llm-test` 在 `30d6b66`（09-17），因 `paths` 过滤未覆盖 R 系列提交。故"4 workflow 全绿"在 HEAD 上**不成立**（HEAD `48ac75b` 尚未 push，无任何 CI 记录）。
>
> **R-02 / R-03 为"部分完成"而非"基本完成"**：账本未回填（AP-04 复发）、SSR 无 ADR、E2E-05 三处 status 仍不一致。逐条证据见下方 R 表与 [remediation.md](remediation.md)。

**当前激活阶段：R-01 🔴 补完 #2（APISIX 路由）** → 详档 [remediation.md](remediation.md) §R-01

## 下一阶段（被 R-01 阻塞）

**E2E-07 找回密码**（status: ⏳ pending，**仍阻塞**）→ 详档 [stages/e2e-07-password-recovery/plan.md](stages/e2e-07-password-recovery/plan.md)

> E2E-07 正是"密保找回"流程——在 #2 的网关路由补齐前开工，等于把新阶段建在 401 的路径上。

> 🔧 = 改造阶段（不是纯测试，含代码/schema/目录/配置变更）

## R 系列：补救与约束机制（插入前置，优先于一切新阶段）

> 详见 [remediation.md](remediation.md)。R 系列不占用 E2E-NN 编号，以示"欠债归还"区别于"路线图推进"。

| 阶段 | 功能块 | 目标 | 阻塞关系 | 状态 |
|------|--------|------|---------|------|
| **R-01** 🔴 | **阻断项修复** | BFF 密保校验 fail-open（安全）+ 找回密码端点缺失 + 注册链路断裂 + 测试编译失败 + Register 非事务 + migrate.sh checksum 死代码 + CI 转绿 | 无依赖 → **可立即启动** | ⛔ **未完成**（2026-09-18 第二方核对：6/7 项为真；**#2 不可达**——APISIX 缺 `/api/v1/auth/verify-security-answer` 白名单路由；#5 Register 非事务降级） |
| **R-02** 🔧 | **收口补账** | 15 项契约补齐：report 重写 / 截图 / 账本对账 / 状态三处对齐 / 撤 E2E-06 done / SSR 补 ADR + 更正 7 处失效文档 / 孤儿产出物 / 演示账号解耦 | 依赖 R-01 | 🟡 **部分完成**（核对：#5/#7~#14 为真；**#4 不实**（E2E-05 plan=partial vs roadmap/report=done）；**#6 缺 ADR**；**#15 处置违背 D-06**；#1~#3 未做；账本未回填） |
| **R-03** 🔧 | **约束机制建设** | 把 14 类反例机械化：`e2e_stage_audit.py` 收口审计器 + 证据有效性校验 + TDD 门禁 + ADR 门禁 + 门禁真能拦 + 孤儿检测 + 脚本负向用例 + CI 严格化剩余 14 项 | 依赖 R-02 | 🟡 **部分完成**（#1/#3/#4/#6/#8/#9 为真；**#5 未接通**：`required_status_checks` 实测为空集；**⚠️ 且 branch protection 已自锁死**——强制 PR + 需 1 approve + 单协作者 ⇒ main 完全不可写，见 E2E-F-68；#7/#10 未做；§13.3 的 #8/#16 两条断言无实现也无人工理由；TDD 门禁不覆盖 `.sh`/`.py`） |

**执行路径（核对后修正）**：`R-01（补 #2）→ R-02 收尾（#1~#3 + 账本回填 + SSR ADR）→ R-03 收尾（#5/#7/#10）→ 解除 E2E-07 → … → E2E-30`。R-02/R-03 可与路线图后续阶段并行，但须先于"下一批阶段的收口"。


## 排期总表（30 阶段）

### 第一批：基础改造与门槛

| 阶段 | 功能块 | 目标 | 边界（不做） | 状态 |
|------|--------|------|--------------|------|
| E2E-01 | 登录会话持久化 | cookie 存储/刷新恢复/过期/登出/remember-me 全周期 | 注册、找回密码 | ⚠️ done**（契约欠账）**：`[V]` 截图 0 张、测试点 #10 无实质证据、report 误引账本编号（写 F-23 实为 F-38）→ [R-02](remediation.md) |
| E2E-02 🔧 | 项目目录清理 | 清理无关文件/目录、补 gitignore、归档测试证据 | 不动业务代码 | ✅ done（最接近合规；唯一瑕疵：#6 可验证却标 BLOCKED） |
| E2E-03 🔧 | **CI/CD 门槛** | 落地可跑的 `.github/workflows/`（现全仓零 CI，模板从未执行） | 复杂流水线/部署自动化 | ⚠️ done**（契约欠账）**：report 非模板、**21 点中 13 点无结果**、无汇总行、账本未闭环、`-race` 被降级当已解决 → [R-02](remediation.md) / [R-03](remediation.md) |
| E2E-04 🔧 | **前端工程化门槛** | ESLint/Prettier 引入 + typecheck 103→0 + SPA 产物 smoke + Playwright mobile project + browserslist + a11y 基线 | 全量代码重构 | ⚠️ done**（契约欠账）**：**4 个测试点假 PASS**（其中 #4 与代码事实相反）、账本一行未改、回归钉从未运行 → [R-02](remediation.md) |
| E2E-05 🔧 | **文档与代码一致性收口** | 11 个校验脚本接入 CI + 更正 3 处实测失真 + digest 假绿修复 + 2 个 migration 脚本 | 通用文档检查器 | ⚠️ done**（契约欠账）**：`plan.md` 至今 `status: pending`、汇总行留占位符 `PASS x`、账本未更新、**其接入的 `doc-drift-check` 在 main 上持续红** → [R-02](remediation.md) |

### 第二批：数据库与认证

| 阶段 | 功能块 | 目标 | 边界（不做） | 状态 |
|------|--------|------|--------------|------|
| E2E-06 🔧 | 数据库改造 | 删死字段（phone/email/status）+ 加密保问题字段（D-01）+ 加 schema_migrations 版本表 + 统一软删除 + 修 db README | 连接池调优 | ⚠️ **partial**：R-01 修复了安全漏洞/注册断裂/测试编译；剩余：integration test 未补、演示账号解耦（R-02 #9~10） |
| E2E-07 | 找回/重置密码 | 三步向导改造为**密保问题**流程（D-01=C）+ 端到端跑通 | 短信/邮件服务 | ⏳ pending（R-01 已解除阻塞） |
| E2E-08 | 历史会话管理 | 会话列表/删除/pin/重命名/分组 | 消息内容同步 | ⏳ pending |
| E2E-09 | 注册流程 | 注册全流程（含密保问题设定步骤） | — | ⏳ pending（密保录入 UI 待实现） |

### 第三批：聊天与周边

| 阶段 | 功能块 | 目标 | 边界（不做） | 状态 |
|------|--------|------|--------------|------|
| E2E-10 | 聊天核心链路 | 发送/SSE 流式/错误处理/中断重试 | 多模态 | ⏳ pending |
| E2E-11 | 我的空间 | 资料修改/头像上传（MinIO）+ 现有 3 个对话行为图表有数据可渲染、空态可读 | 测评图表（归 E2E-14） | ⏳ pending |
| E2E-12 | 设置页 | 字体/主题切换与持久化 | — | ⏳ pending |

### 第四批：决策支持与多模态

| 阶段 | 功能块 | 目标 | 边界（不做） | 状态 |
|------|--------|------|--------------|------|
| E2E-13 | 心理测验链路修复 | 列表→答题→提交→结果查看 跑通（三层契约错位见 E2E-F-02）+ 补量表种子数据 + `/question` 页区分两类量表 | 人格量表内容设计 | ⏳ pending |
| E2E-14 🔧 | 人格量表与 AI 提示词定制 | D-02：新增人格量表（设计+种子+维度评分器）→ 结果模型扩展 → 注入 AI prompt → user 页测评图表 | — | ⏳ pending |
| E2E-15 | 报表 Dashboard | 数据内容正确性/日期切换/历史 chartData=[] 复查 | — | ⏳ pending |
| E2E-16 | 多模态（语音/表情/文件上传） | 语音输入/表情识别/文件上传链路 | 数字人、TTS | ⏳ pending |
| E2E-17 🔧 | 数字人 + TTS | D-03：做真口型同步（接 `/tts_with_phonemes` 时间戳）+ 排查段间播放断点 | — | ⏳ pending |

### 第五批：数据与缓存

| 阶段 | 功能块 | 目标 | 边界（不做） | 状态 |
|------|--------|------|--------------|------|
| E2E-18 | 缓存层 | 本地缓存（ai-svc LRU）行为 + Redis 是否启用/下线的决策与验证 | — | ⏳ pending |
| E2E-19 | 数据库层验证 | 连接池/迁移幂等重放/分区裁剪/视图可读/软删除行为 + **备份→破坏→恢复演练**（只验证不改 schema） | schema 变更 | ⏳ pending |
| E2E-20 🔧 | **多实例并发正确性** | 修 in-memory 限流/登录锁定/验证码防枚举的多实例失效 + APISIX limit-count 跨实例 + 双实例并发验证 | 分布式事务 | ⏳ pending |

### 第六批：可观测性

| 阶段 | 功能块 | 目标 | 边界（不做） | 状态 |
|------|--------|------|--------------|------|
| E2E-21 | 日志体系 | 结构化日志 + traceId 注入（含 gRPC 侧）+ Loki 采集链路（Go 日志现未进 Loki） | 日志平台选型 | ⏳ pending |
| E2E-22 | 监控告警 | Prometheus 抓取/Grafana 面板/Alertmanager 通知渠道 | — | ⏳ pending |
| E2E-23 | 健康检查与服务发现 | /health 与 gRPC health 语义 + Nacos 注册/配置中心/热更新 | — | ⏳ pending |

### 第七批：消息与网关

| 阶段 | 功能块 | 目标 | 边界（不做） | 状态 |
|------|--------|------|--------------|------|
| E2E-24 | 消息链路 | outbox→Kafka→consumer→DLQ 全链 + 重试/死信/回放 | — | ⏳ pending |
| E2E-25 | 网关 APISIX | 路由注册/JWT 插件/限流/CORS | — | ⏳ pending |
| E2E-26 | 链路追踪 SkyWalking | sw8 传播 + OAP 查询 + UI 可视化（OAP 9.x queryDuration bug） | — | ⏳ pending |
| E2E-27 | 对象存储 MinIO | 头像上传/下载/匿名读权限 | 其他文件类型接入 | ⏳ pending |

### 第八批：质量属性与收口

| 阶段 | 功能块 | 目标 | 边界（不做） | 状态 |
|------|--------|------|--------------|------|
| E2E-28 | **性能与延迟基线** | 首次建立基线：/analyze p50/p95、SSE TTFT、TTS 单段与段间 gap + 小规模阶梯 | 全站压测 | ⏳ pending |
| E2E-29 | 横切：异常与安全 | JWT 过期刷新/IDOR/限流/CORS/越权 + **JWT 密钥轮换机制**（BFF+APISIX 原子性） | 渗透测试 | ⏳ pending |
| E2E-30 | 数据契约收口 | §2.4 六项数据契约 smoke 全绿 + helm template/lint 渲染回归 | — | ⏳ pending |

## 改造项决议状态

完整选项分析与影响面见 **[decisions.md](decisions.md)**。

| 项 | 归属阶段 | 状态 | 决议 |
|----|---------|------|------|
| **D-01 找回密码方式** | E2E-07（+E2E-06 供字段） | ✅ **密保问题** | A 运维脚本体验不好、B 要接额外 API、C 只需改页面 + 数据库 → 选 **C** |
| **D-02 心理测验定位与人格画像** | E2E-13 / E2E-14 | ✅ **两种量表并存** | 新增人格量表（产心理画像→驱动 AI 提示词）+ 保留症状量表（风险预警）；`/question` 页区分两类；user 页新增测评图表 |
| **D-03 数字人口型同步与 TTS 断点** | E2E-17 | ✅ **真口型同步 + 排查断点** | 接 XTTS `/tts_with_phonemes` 字符级时间戳驱动 BlendShape + 解决段间播放间隙 |
| **D-04 多语言支持（i18n）** | 待定（候选） | 🟡 **候选未决** | 项目完全单语硬编码（无 vue-i18n、无 locales 目录，UI 文案硬编码中文）。是否做属产品决策，暂不列为 E2E 阶段 |

## 改造阶段明细

### E2E-02 🔧 项目目录清理

**范围**：清理仓库中无关文件/目录、补 .gitignore、归档测试证据。**不动业务代码。**

| 对象 | 现状 | 处置建议 |
|------|------|---------|
| `.mimosa/`（根、deploy/apisix、emotion-echo-web 三处） | hook 运行时状态，**未被 gitignore**，持续污染 `git status` | 加入 `.gitignore` |
| `emotion-echo-web;D` | **空目录（0 字节）**，shell 误建残留 | 删除 |
| `docker-images-before.txt` | 一次性快照产物（2026-09-09） | 删除 |
| `gui-test-screenshots/`（根） | **25 个文件已被 git 跟踪** | 归档到 `docs/evidence/` |
| `tmp/` | 已 gitignore | 确认可清空 |
| 根 `node_modules/` | 已 gitignore，根目录无 `package.json` | 确认可删 |

### E2E-03 🔧 CI/CD 门槛

**范围**：把 `docs/ci-workflows/` 的 3 份模板落地为真实可跑的 `.github/workflows/`。

| 现状 | 目标 |
|------|------|
| `.github/` 目录**不存在**，3 份模板（go-test/llm-test/web-test）从未执行 | `.github/workflows/` 真实注册 + 首条绿 run 证据 |
| 285 个 Go 测试、47 个前端测试全凭自觉运行 | push/PR 自动触发 |
| AGENTS.md §2.2 合并门槛（`go test` + `go vet` + lint + smoke）**无人机械执行** | 门槛机械化 |

**阻塞原因**（已记录在 `docs/ci-workflows/README.md`）：PAT 只有 `repo` scope，GitHub 拒绝 push `.github/workflows/*.yml`，需换 token 或手工在网页创建。

### E2E-04 🔧 前端工程化门槛

**范围**：给前端重度阶段（E2E-01/07~17）建立机械防护。

| 项 | 现状 | 目标 |
|----|------|------|
| ESLint | ❌ 无配置、devDependencies 无 eslint | 引入 + `lint` script + 接入 CI |
| Prettier | ❌ 无 | 引入 + 格式化基线 |
| typecheck | ⚠️ 脚本存在但**96 处历史错误基线** | 清零，或落"仅新增文件零错"基线并写入 CI |
| 构建产物 | ❌ 无 `nuxt build` 后 smoke（项目是 **SPA 模式** `ssr: false`） | 产物 smoke 脚本 |
| Playwright project | ⚠️ 仅 `chromium-headless-shell` | 加 `mobile`（Pixel 5）+ `firefox`（可选） |
| browserslist | ❌ 无支持范围声明 | 补声明 |
| a11y | ❌ 零工具链（仅零散手工 aria-label） | `@axe-core/playwright` 对 6 主页跑基线，只修 critical/serious |

### E2E-05 🔧 文档与代码一致性收口

**范围**：让 ADR-18 的防线真正生效。

| 项 | 现状 | 目标 |
|----|------|------|
| 10 个校验脚本（路由对齐/视图一致性/env 变量/迁移契约/JWT secret 一致/digest/布局/TLS…） | 全部 CI-shaped，**全部无人在跑** | 接入 E2E-03 的 CI |
| `check_docker_digests.sh` | **假绿**：只校验 FROM 格式，而 `Dockerfile.digests.lock` 7 个 digest 是 `sha256:000...000` 占位 | 校验 digest 非占位值 |
| 3 处实测失真 | 见下方账本 E2E-F-20~22 | 就地更正 + 登记 ADR-18 表 |

### E2E-06 🔧 数据库改造

**(a) 删除死字段**（已核实使用情况）：

| 字段 | 现状 | 结论 |
|------|------|------|
| `users.email` | 仅 model tag 声明，**无任何读写** | 死字段，可删 |
| `users.phone` | 仅在 API 响应里回显（4 处），**无任何写入点** → 恒为 NULL | 死字段，可删 |
| `users.status` | 仅 model tag，无逻辑读写 | 待确认（可能有意软禁用），无使用则删 |

**(b) 新增密保问题字段**（供 D-01=C）：`security_question` + `security_answer_hash`（bcrypt），或独立 `user_security_answers` 表支持多问题

**(c) 迁移治理**：加 `schema_migrations` 版本表（现靠"幂等 + 每次重放"）

**(d) 软删除统一**：chat 的 `deleteconversation` 是物理删，与 users/ai 域不一致

**(e) 文档修正**：`deploy/db/README.md` 仍列不存在的 `03-migrate-data.sql`

### E2E-20 🔧 多实例并发正确性

**范围**：修 3 处多实例下静默失效的防护（项目决策 3 是"本地 Docker 单机多实例"，故必须正确）。

| 缺陷 | 位置 | 多实例后果 |
|------|------|-----------|
| BFF 登录失败锁定 in-memory | `auth_handler.go:14,66` | 5 次错密码锁定**可被绕过**（打不同实例） |
| 验证码 60s 防枚举 in-memory | `auth_handler.go:15` | 防枚举**失效** |
| APISIX 限流 `policy: local` | `seed.sh:318-325` | 每节点各自计数，**总配额放大 N 倍** |

**修复路径已铺好**：`shared/pkg/middleware/limiter.go:133` 的 `LimiterBackend` 抽象接口已存在，:137-140 的 `RedisLimiterBackend: TODO` 待实现；Redis 容器现成。

## 依赖声明

- E2E-04 建议排在所有前端阶段之前（E2E-01/07~17 全是前端重度）
- E2E-05 强依赖 E2E-03（脚本要挂进 CI 才有意义）；E2E-05 与 E2E-03 可视为同一议题的两面
- E2E-07 依赖 E2E-06（密保问题字段就位）与 E2E-01（会话状态）
- E2E-14 依赖 E2E-13（人格画像是量表链路的延伸）
- E2E-15 依赖 E2E-10（报表数据来自聊天产生的行为事件）
- E2E-16 / E2E-17 依赖 E2E-10（多模态与数字人入口位于聊天页）
- E2E-20 依赖 E2E-18（缓存层先决定 Redis 用不用）；两者**应串行不可并行**
- E2E-28 依赖 E2E-10（SSE 已通）、E2E-17（TTS 改造后复测）、E2E-22（Prometheus 抓取已通）
- E2E-24 / E2E-25 / E2E-26 建议在浅层用户流程稳定后再测（浅层问题会污染深层观测）

## 每阶段标准流程与详档约定

**执行协议**：**开工前必读 [RUNBOOK.md](RUNBOOK.md)** —— 状态机、环境准备（含 `--env-file .env.local` 红线）、执行循环、测试点判定分级（`[A]`自动/`[V]`视觉/`[M]`需裁定）、账本写入契约、收口契约（8 项必做）、阻塞与升级协议、report 模板、迭代护栏、命令速查。

**方法论**：全局 skill `e2e-stage-testing`（为什么这么做）；RUNBOOK 是项目内的可执行版本，不依赖 skill 是否加载。

**详档约定（just-in-time）**：每个阶段的详细规划文档写在 `stages/<e2e-NN-slug>/plan.md`，**在轮到该阶段前 1 个阶段时撰写**，不提前批量写完全部 30 份。原因：本项目长期受"文档与代码漂移"之害（ADR-18），提前写出的详档会随前面阶段的发现而失效，反而制造新的失真。已写详档：

| 阶段 | 详档 |
|------|------|
| E2E-01 登录会话持久化 | [stages/e2e-01-login-session/plan.md](stages/e2e-01-login-session/plan.md) |
| E2E-02 项目目录清理 | [stages/e2e-02-directory-cleanup/plan.md](stages/e2e-02-directory-cleanup/plan.md) |
| E2E-03 CI/CD 门槛 | [stages/e2e-03-ci-gate/plan.md](stages/e2e-03-ci-gate/plan.md) |
| E2E-04 前端工程化门槛 | [stages/e2e-04-frontend-engineering/plan.md](stages/e2e-04-frontend-engineering/plan.md) |
| E2E-05 文档与代码一致性 | [stages/e2e-05-doc-code-consistency/plan.md](stages/e2e-05-doc-code-consistency/plan.md) |
| E2E-06 数据库改造 | [stages/e2e-06-db-transformation/plan.md](stages/e2e-06-db-transformation/plan.md) |
| E2E-07 找回/重置密码 | [stages/e2e-07-password-recovery/plan.md](stages/e2e-07-password-recovery/plan.md) |
| E2E-08 历史会话管理 | [stages/e2e-08-conversation-management/plan.md](stages/e2e-08-conversation-management/plan.md) |
| E2E-09 注册流程 | [stages/e2e-09-registration/plan.md](stages/e2e-09-registration/plan.md) |

模板见 [stages/_TEMPLATE.md](stages/_TEMPLATE.md)，执行记录模板见 [stages/_REPORT_TEMPLATE.md](stages/_REPORT_TEMPLATE.md)。批次三及以后在轮到前补写。E2E-01 已按判定分级标注，其余已写详档在启动前补齐标记。

## 决策门（开工前核对，未落定不得开工）

| 阻塞项 | 阻塞阶段 | 需谁决定 | 现状 |
|--------|---------|---------|------|
| D-04 i18n 是否立项 | 不阻塞任何阶段 | 用户 | 🟡 候选 |

**已解除**：`users.status` → 删；注册验证码步骤 → 删；密保可否跳过 → 不可跳过、注册必设；GitHub token `workflow` scope → 已具备（实测真实 push 成功）；**存量未设密保用户 → 选 C 不处理**（数据库现有数据全是无用信息、未上线、测试阶段），改为**新建可重跑、可删除的演示账号**（带密保）；**main 分支保护 → 安全子集已开**（防强推 + 防删除 + `enforce_admins=true`，实测强推被拒 `GH006`；status checks / PR 要求待 CI 落地后再开）。

> E2E-03 已拆为两段：**阶段 1 落地**（模板 → 真 CI）+ **阶段 2 严格化**（18 处缺陷逐条加固，清单见 [E2E-03 plan](stages/e2e-03-ci-gate/plan.md) §2.3）。详细规则见 [RUNBOOK.md](RUNBOOK.md) §9。

## 历史与背景

- 前身：`docs/plans/test-coverage-tracker-2026-09-16.md`（业务路径覆盖追踪，Sprint 109-115）
- 建档预探查：2026-09-17 三大块代码级探察（人格测试/AI 提示词、数字人、横切模块），发现 19 项登记 `discovered-unresolved.md`
- 覆盖盲区排查：2026-09-17 对 12 个候选面向做覆盖性排查，发现 5 处缺口并补为阶段（CI/CD、前端工程化、文档一致性、多实例并发、性能基线）；其中 CI/CD 与文档一致性属于"治理基础设施"，是其余阶段的防退化机制
- 改造决议：D-01 密保问题 / D-02 两种量表并存 / D-03 真口型同步，详见 [decisions.md](decisions.md)
- 排期依据：用户指定前三顺序（登录 cookie → 密码找回 → 历史会话），改造项排到前面，并追加缓存层/数据库/日志等横切切分要求
