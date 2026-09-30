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
| `doc-drift-check.yml` | push / PR | 文档/代码漂移守卫（ADR 门禁、TDD 门禁、孤儿产出物、残留物等） | `docs/**` + `scripts/check_*.sh` |
| `e2e-guards.yml` | push / PR | 静态守卫（E2E-23 新增） | 7 个 `scripts/test_*.sh` 静态守卫，**见下方"门禁生效边界"** |

`e2e-guards.yml` 跑的这 7 个守卫（均为纯静态检查，不需要运行中的容器）：

| 脚本 | 断言 |
|------|------|
| `scripts/test_healthcheck_readiness.sh` | compose + Helm 两侧 readiness 探针必须是 `/health/ready`；反向断言 liveness 仍是浅探针 `/health` |
| `scripts/test_grpc_health_shutdown.sh` | 5 个 gRPC 服务 `MarkShuttingDown()` 先翻 NOT_SERVING 再 GracefulStop |
| `scripts/test_nacos_required_declared.sh` | 各服务在 compose 中声明了它实际依赖的 Nacos 服务 |
| `scripts/test_migrate_pg_wait.sh` | `migrate.sh` 的 `PG_WAIT_MAX_SECS` + 递增退避，且失败非零退出 |
| `scripts/test_obs_healthchecks.sh` | 16 个观测/网关容器都有 healthcheck 且真的探得到东西 |
| `scripts/test_devup_batch_waits.sh` | `dev-up.sh` 的分批等待没有"起完就当就绪"的空等 |
| `scripts/test_route_contract.sh` | APISIX ↔ BFF ↔ 前端三方路由集合无漂移（早于 E2E-23 存在，**本轮才接进 CI** —— 见下方"为什么补这一条"） |

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
