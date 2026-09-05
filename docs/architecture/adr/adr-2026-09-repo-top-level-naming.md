# ADR · 仓库顶层命名规范与废弃部署件处置（Repo Top-Level Naming）

- **编号**：决策 19
- **日期**：2026-09-05
- **状态**：✅ 生效
- **相关**：[decisions.md](/docs/architecture/decisions.md) ·
  [git-layout.md](/docs/deployment/git-layout.md) ·
  [doc-migration-map.md](/docs/_meta/doc-migration-map.md) ·
  [.gitignore](/.gitignore)

---

## 一、上下文

仓库顶层目录长期存在三类"杂乱"，均有实测证据：

1. **命名大小写与语义分裂**：
   - 前端一个应用三种叫法：目录 `Emotion-Echo-Web`（Pascal）、陈旧称呼 `emotion-echo-front`
     （散布于 AGENTS.md / decisions.md / microservices.md）、部署容器名/服务键 `emotion-echo-web`。
   - `Emotion-Echo-LLM/` 名不符实：内容为 FER（人脸）、sensevoice（语音识别）、XTTS（语音合成）
     三个模型推理服务，并非 LLM；真正的文本情绪 LLM 是 `emotion-llm-service/`，两者极易混淆。
   - `emotion-echo-web-bff` 与前端目录共享 "web" 词根且大小写不同，常被误认为前端；实际其
     目录名 = compose 服务键 = Nacos 注册名 = 网关上游名，三者自洽。
   - llm-service 的 compose `container_name: emotion-echo-llm-service` 与目录/服务键/注册名
     `emotion-llm-service` 前缀不一致。
2. **根目录 20 个已跟踪残留 json**：18 个 `apisix-*.json`（手工 Admin API PUT 载荷，其中
   `apisix-ai-up`/`apisix-up-ai`、`apisix-chat-up`/`apisix-up-chat` 字节级重复）+ `msg1/msg2/query.json`
   （curl 请求体 dump）。全部自初始提交 `7abae10` 入库、从未被修改/删除、全仓零引用，且已被
   `deploy/apisix/seed.sh`（catch-all → web-bff + Nacos discovery）整体取代。
3. **废弃部署件滞留根目录**：根 `docker-compose.yml` 头部自述 superseded（frontend 还 depends_on
   一个未定义的 kafka，文件本身不可用）；`charts/emotion-echo/Chart.lock` 引用已删除的
   apisix-routes/apisix-ingress 子图、缺 web-bff/nacos/etcd，属漂移产物。

## 二、决策

| 维度 | 选择 |
|------|------|
| 顶层目录命名 | **一律小写 kebab-case**；PascalCase 仅作 prose 展示名（对照表见 git-layout §六） |
| 前端目录 | `Emotion-Echo-Web` → **`emotion-echo-web`** |
| AI 模型推理仓 | `Emotion-Echo-LLM` → **`emotion-echo-models`** |
| BFF 目录 | `emotion-echo-web-bff` **保持不改**（自洽；仅文档澄清"非前端"） |
| 曾用名 | `Emotion-Echo-Web` / `Emotion-Echo-LLM` / `emotion-echo-front` / `emotion-echo-llm-service`（容器名）退役 |
| 根残留 json | 20 个 `git rm`（内容被 seed.sh 取代，git 历史可找回） |
| 废弃部署件 | 根 `docker-compose.yml` 归档 `legacy/dev-root-compose/`；`Chart.lock` 可再生物且已被忽略，直接清理 |
| npm 包名 | `Emotion-Echo-Web` 是身份标识（package.json / lockfile / gh-pages 仓库），非路径，**不改** |

## 三、候选方案

| 候选 | 结论 |
|------|------|
| 仅文档称呼对齐、目录不动 | ❌ 放弃：目录 Pascal/语义分裂依旧，clone 视图与文档长期不一致 |
| **目录物理改名 + 引用同步**（本决策） | ✅ 采用：目录名 = 唯一路径标识，与容器/服务键/注册名对齐 |
| 分层 monorepo（apps/ services/ shared/ models/ 分组） | ⏸️ 暂缓：牵动全部 Dockerfile build context、`go.mod replace ../emotion-echo-shared`、seed.sh、自检脚本，属高风险大改，单独立计划分批做 |

## 四、落地清单（2026-09-05 一次性提交链）

| 动作 | 说明 |
|------|------|
| `docs(deploy)` | git-layout §一树/§六命名规范先更新（git-layout §五 流程要求 doc-first） |
| `chore(repo)` 清理 | 根 20 json `git rm` + 3 处 stage/map 文档同步 |
| `chore(deploy)` 归档 | 根 compose → `legacy/dev-root-compose/`；契约脚本 §3 改指归档路径 |
| `chore(repo)` 改名 | 前端/模型目录 `git mv` + 全仓活动文档/脚本/compose/自检引用同步 |
| `chore(repo)` 对齐 | llm 容器名 → `emotion-llm-service`；消灭 `emotion-echo-front` |
| `chore(repo)` ignore | 修复 .gitignore 行尾注释失效的 7 条规则（.zcode/ playwright-report/ 等） |

**保留原样**（历史记录，git-layout §六 曾用名清单已登记）：`docs/stages/`、`docs/legacy-plans/`、
`docs/flatten-snapshot-*.json`、`docs/architecture/audit-2026-08-31.md`、`scripts/_round5-fix-links.py`、
`scripts/pre_flatten_snapshot.py`、npm 包名。历史文档中的旧路径是"当时状态"的记录，不追溯改写。

## 五、验收门禁

- `python scripts/check_git_layout.py --strict`：7/7 PASS（KEY_DIRS 换新名 + 布局文档路径修正）。
- `git grep` 断言：活动文件（非历史档）中旧名残留为 0；git-layout §六 的"曾用名"行有意保留。
- 前端 WIP（19 个未提交文件）全程保持未暂存，改名提交不含任何 WIP 内容。

## 六、调研依据

- 已读代码/文件：`deploy/docker-compose.apps.yml`（build context / container_name）、
  `deploy/apisix/seed.sh`（取代旧静态路由）、`scripts/check_git_layout.py`（KEY_DIRS 硬编码）、
  `emotion-echo-shared/pkg/discovery/servicenames.go`（Nacos 注册名）、`charts/emotion-echo/Chart.lock`、
  `.gitignore`（行 140/143-147 行尾注释失效）。
- 已查文档：`docs/deployment/git-layout.md` §五（布局修改 doc-first 流程）、§六（命名规范，本决策落地时新增）、
  `docs/_meta/doc-migration-map.md` §三（根残留登记）、decisions.md 决策 9/12（web-bff 角色）、决策 18（失真治理）。
- git 证据：`git log --diff-filter=D` 证明 20 个 json 自 `7abae10` 从未被删；全仓 grep 证明零引用；
  `git check-ignore -v` 逐条验证 ignore 规则失效与修复。
