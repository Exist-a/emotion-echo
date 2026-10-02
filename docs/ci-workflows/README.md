---
purpose: CI workflow 的权威说明（.github/workflows/ 现状 + 门禁生效边界）
status: active
related-stage: stage-97, e2e-03, e2e-22, e2e-23
---

# CI Workflows 说明

> **本文件的唯一事实源是 `.github/workflows/` 下的实际文件**。这里是说明与索引，
> 不是模板存放处——workflow 已经直接落在 `.github/workflows/`（见下方"历史"）。

## 一、当前实有 workflow（5 个）

**为什么补 `test_route_contract.sh` 这一条**：该脚本早于 E2E-23 就存在，但从未接进任何
执行路径。E2E-23 给 BFF 新增 `GET /health/ready` 路由后，它立刻变红
（`BFF route GET /health/ready NOT covered by APISIX`），而**没有任何机制发现**——
这正是 anti-patterns **AP-10 的变体：守卫写了 ≠ 守卫在跑**。

| 文件 | 触发 | 用途 | 覆盖 |
|------|------|------|------|
| `go-test.yml` | push / PR | 6 Go svc + shared 跑 `go test` + `go vet` | ai / analytics / chat / web-bff / assessment / user / shared |
| `llm-test.yml` | push / PR | emotion-llm-service pytest | /analyze auth + gRPC + file_context + Nacos |
| `web-test.yml` | push / PR | emotion-echo-web vitest + lint | composables + apiRoutes + auth.global + renderMarkdown |
| `doc-drift-check.yml` | push / PR | 文档/代码漂移守卫（23 项）+ **汇总门禁** | `docs/**` + `scripts/check_*.sh` |

> `doc-drift-check.yml` 里除 23 个检查 job 外还有两个**元 job**（E2E-23 收口轮新增，见 [ADR](../architecture/adr/adr-2026-09-e2e-23-gate-aggregation.md)）：
>
> - `文档守卫总闸`（`doc-drift-gate`）：`needs` 全部 23 个检查 + `if: always()`，任一失败即失败。
>   **分支保护只填这一条**即可覆盖全部文档守卫 —— 避免 23 条中文 job 名被逐条维护、
>   避免有人改 `name:` 导致门禁**静默失效**（精确匹配不上、不报错、不拦）。
> - `汇总门禁 needs 覆盖校验`（`doc-drift-needs-sync`）：跑 `scripts/check_doc_drift_gate_needs.sh`，
>   断言 gate 的 `needs` 覆盖全部检查 job 且无失效引用。**它必须独立于 gate 跑** ——
>   校验汇总门禁的 job 不能被汇总门禁覆盖，否则 gate 漏掉自己就没人发现。 |
| `e2e-guards.yml` | push / PR | 静态守卫（E2E-23 新增） | **15 个**静态守卫（14 个 `scripts/test_*.sh` + 1 个 `scripts/check_*.sh`），**见下方"门禁生效边界"** |

`e2e-guards.yml` 跑的这 15 个守卫（均为纯静态检查，不需要运行中的容器；计数 2026-10-02 对账——此前写"7 个"且清单只列 8 行，属"守卫清单漂移"，已修）：

| 脚本 | 断言 |
|------|------|
| `scripts/test_healthcheck_readiness.sh` | compose + Helm 两侧 readiness 探针必须是 `/health/ready`；反向断言 liveness 仍是浅探针 `/health` |
| `scripts/test_grpc_health_shutdown.sh` | 5 个 gRPC 服务 `MarkShuttingDown()` 先翻 NOT_SERVING 再 GraceStop |
| `scripts/test_nacos_required_declared.sh` | 各服务在 compose 中声明了它实际依赖的 Nacos 服务 |
| `scripts/test_migrate_pg_wait.sh` | `migrate.sh` 的 `PG_WAIT_MAX_SECS` + 递增退避，且失败非零退出 |
| `scripts/test_obs_healthchecks.sh` | 16 个观测/网关容器都有 healthcheck 且真的探得到东西（E2E-25 更新：apisix 基准为 admin API 9180 原始 GET，非 TCP 9080） |
| `scripts/test_devup_batch_waits.sh` | `dev-up.sh` 的分批等待没有"起完就当就绪"的空等 |
| `scripts/test_devup_drift_check.sh` | **（E2E-25 D-36 新增）** `dev-up.sh` 接入 `check_apisix_drift.sh extras` 漂移报告且只报不拦（`if !` 保护），调用位置在 apisix-seed 之后 |
| `scripts/test_route_contract.sh` | APISIX ↔ BFF ↔ 前端三方路由集合无漂移（早于 E2E-23 存在，**本轮才接进 CI** —— 见下方"为什么补这一条"） |
| `scripts/test_stage_todo_section.sh` | 每个 partial 阶段的 report 必须有 §0 未完成清单，且声明条数与实际条目一致（棘轮：历史阶段计入 legacy，基线只降不升） |
| `scripts/test_integration_tag_compiles.sh` | `//go:build integration` 的代码在 `-tags integration` 下也能编译（E2E-23，防编译盲区） |
| `scripts/test_audit_ledger_parser.sh` | 审计器账本解析器无盲区（跨行 Markdown / 转义竖线不丢行） |
| `scripts/test_healthcheck_no_dead_server.sh` | healthcheck 无生产死代码（E2E-23 F-163） |
| `scripts/test_health_nilrepo_truthful.sh` | 降级启动（repo=nil）时健康探针必须说假话，不许 dbOK=true（E2E-23 F-96） |
| `scripts/test_go_test_exitcode_gate.sh` | go test 结果判定必须用退出码，禁止 grep（E2E-23 ugrep 假绿教训） |
| `scripts/test_check_doc_drift_gate_result.sh` | **（E2E-F-173 新增）** 文档总闸聚合判定：success/skipped=通过、failure/cancelled=失败——防 push 事件下 adr-gate skip 被判红致 main 总闸恒红 |

> **E2E-25 另在 `doc-drift-check.yml` 的 `apisix-seed-structure` job 加了 2 步**（属该 job 内步骤，不改上方 job 计数）：
> `scripts/test_apisix_drift_lib.js`（漂移检测纯函数契约 15 断言）+ `scripts/test_check_apisix_drift.sh`（wrapper 离线 6 断言）。

需要运行中的容器栈的 `scripts/smoke_health_discovery.py` **不在 CI**（CI 无 docker 栈），
只在本地/dev 模式跑。

## 二、门禁生效边界（必读，防 AP-11 误判）

> 原则来自 [`docs/e2e-roadmap/anti-patterns.md`](../e2e-roadmap/anti-patterns.md) **AP-11**：
> **"workflow 能跑红" ≠ "门禁生效"**。真正的门禁 =
> **分支保护的 `required_status_checks` + `required_pull_request_reviews`**；
> workflow 本身只是报告器。

**本仓库当前的事实（2026-09-30 实测）**：

- `required_status_checks` 与 `required_pull_request_reviews` **未启用**
  （用户 2026-09-28 在网页端关闭过 `main` 的 require status checks）
- 因此：**CI 全绿或全红都不会自动阻止合并**，需要人工看
- `e2e-guards.yml` 文件头已**如实标注**"本 workflow 当前仅报告，不拦合并"

**因此本文件不写、也不得写**"任何 test 失败 → PR 不可 merge"这类表述，
除非 `required_status_checks` 已启用且**经 API 实测确认非空**。

要启用门禁（需仓库管理员权限，本仓无 admin token 时只能由用户操作）：

```
GitHub 仓库 → Settings → Branches → Branch protection rules → main
  ☑ Require status checks to pass before merging
      勾选：go-test / llm-test / web-test / doc-drift-check / e2e-guards
  ☑ Require approvals: 1
```

启用后请回本文档与 `discovered-unresolved.md`（E2E-F-162）同步翻状态。

## 三、查询 CI 结果的正确姿势

`GET /repos/{owner}/{repo}/pulls/{n}/statuses`（旧 Statuses API）**对本仓恒返回空**，
用它判断"CI 还没跑"会误判。用下面任一：

```bash
# 1. Actions API（推荐）
curl -s "https://api.github.com/repos/Exist-a/emotion-echo/actions/runs?branch=<branch>&per_page=10" \
  | python -c "import sys,json;[print(r['head_sha'][:7],r['name'],r['status'],r['conclusion']) for r in json.load(sys.stdin)['workflow_runs']]"

# 2. 失败时再看哪个 job 的哪个 step 红了
curl -s "https://api.github.com/repos/Exist-a/emotion-echo/actions/runs/<run_id>/jobs?per_page=50"
```

注意 `api.github.com` 在本机会**间歇性返回 HTTP 000**（网络抖动），脚本需重试。
`/actions/jobs/<id>/logs` 下载需要 token，无 token 时只能靠本地复现（`go vet` / `go test`）。

## 四、历史（Stage 97 PR-9e 的绕行方案，已作废）

Stage 97 当时因 **PAT 只有 `repo` scope**、GitHub 拒绝 push 修改
`.github/workflows/*.yml`（`refusing to allow a Personal Access Token to create or
update workflow`），把 workflow 存放在本目录等待用户三选一启用。

**该阻塞已于 2026-09-24 解除**（见 `docs/e2e-roadmap/roadmap.md` 记："GitHub token
`workflow` scope → 已具备，实测真实 push 成功"），workflow 已直接落在
`.github/workflows/`。本目录现只保留本说明文件。

## 调研依据

- `.github/workflows/` 实有 5 个文件（2026-09-30 `ls` 核实）
- `docs/e2e-roadmap/roadmap.md` E2E-23 阶段行（token workflow scope 已具备）
- `docs/e2e-roadmap/discovered-unresolved.md` E2E-F-162（门禁未拦，记账）
- `docs/e2e-roadmap/anti-patterns.md` AP-11 硬规则（required_status_checks 未验证前禁止写"会拦"）
- Round 2 plan §P1-R2-16 / AGENTS.md §2.2（workflow 存在的原因）
