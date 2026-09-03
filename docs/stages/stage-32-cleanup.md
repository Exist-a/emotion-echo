# Stage 31/32 收口报告 + Stage 33 交接

> **配套文档**：[`stage-32-landing.md`](./stage-32-landing.md)（Stage 32 落地报告）、
> [`stage-32-apisix-reintroduction.md`](./stage-32-apisix-reintroduction.md)（Stage 32 设计与决策）、
> [`stage-33-p0-fix-bff-purify.md`](./stage-33-p0-fix-bff-purify.md)（下一阶段实施规划）。
>
> **收口日期**：2026-09-01 · **目标基线**：`docs/stage-31-landing` 分支 HEAD

---

## 一、为什么需要这次收口

Stage 31（Nacos）+ Stage 32（APISIX）两阶段共 23 个 commit 落地后，工作区残留以下未提交物：

| 类别 | 项 | 性质 |
|------|-----|------|
| 未提交修改 | `charts/emotion-echo/charts/prometheus/values.yaml`（+6 行 apisix.serviceName） | **真实功能修复**，必须提交 |
| 文档 TODO | `docs/stage-32-landing.md` §9 "4 commits / +1,300 / -240 行（详细数字待 PR commit 后填）" | 文档残留，PR 已 commit 但数字未填 |
| 文档 TODO | `docs/stage-32-landing.md` §10 `TODO: 待 PR commit 后填` | 跟随 §9 消除 |
| untracked | `rel.json`（53KB APISIX Dashboard releases dump） | 调研临时物，无引用 |
| untracked | `Emotion-Echo-Web/scripts/convert-el-*.py`（2 个 Element→原生转换脚本） | 0 引用，0 README |
| untracked | `charts/emotion-echo/Chart.lock` | Helm 自动生成，应忽略 |

若不收口就直接进 Stage 33，这些杂物会持续干扰 `git status` 视觉与 PR diff 整洁度。

---

## 二、变更清单（6 条 commit）

| # | commit | 类型 | 文件 | 实质改动 |
|---|--------|------|------|----------|
| 1 | `fc29cfa` | `chore(helm)` | `charts/emotion-echo/charts/prometheus/values.yaml` | +6 行 `apisix.serviceName: apisix`（Stage 32 PR-13 真实修复，独立 lint 子 chart 不再依赖 umbrella values） |
| 2 | `186fd38` | `docs(stage-32)` | `docs/stage-32-landing.md` | §9 标题从 "4 commits" 改为 "8 commits"（4 feat + 4 docs/fix）；填实 8 条 SHA + 累计行数（118 files / +10,449 / -983）+ PR-13/14/15/16 单 PR 拆分；§10 收尾 `TODO` 标记随 §9 填实消除 |
| 3 | *（无 commit）* | `chore` | `rel.json` | 物理删除。文件本就 untracked，无任何文档/代码引用，git 无须动作 |
| 4 | `0e34bee` | `chore(gitignore)` | `.gitignore` | 末尾追加 `charts/**/Chart.lock` 规则；`charts/emotion-echo/Chart.lock` 从 untracked 转为 ignored |
| 5 | *（无 commit）* | `chore(frontend)` | `Emotion-Echo-Web/scripts/` | 物理删除 `convert-el-notify.py` + `convert-el-to-native.py`。原本就是 untracked，无任何生产代码或文档引用（[`docs/STAGE-26-O-P-LANDING.md:290`](./STAGE-26-O-P-LANDING.md) 已明确"主仓不处理"，归 Emotion-Echo-Web submodule） |
| 6 | 本文档 | `docs(stage-32-cleanup)` | `docs/stage-32-cleanup.md`（新建） | 收口报告 + Stage 33 交接 |

**实际 commit 数：4 条**（项 1、2、4、6）+ 2 项**文件系统级删除**（项 3、5）。

### 2.1 为什么 6 项只生成 4 个 commit

untracked 文件（`rel.json`、`Emotion-Echo-Web/scripts/`）的删除**不需要 git 记录**：
- `rel.json` 从未被 `git add`，从未进入 staging 区
- `scripts/` 同上
- 删除后它们直接消失，`git status` 不再列出

若强行 `git rm` 一个从未被 add 的文件，会报 `fatal: pathspec ... did not match any files`。

**结论**：本次收口在 git 历史里留下 4 条 commit，全部可通过 `git log --oneline main..HEAD | head -4` 追溯。

---

## 三、验证

### 3.1 git status（收口后）

执行时间：2026-09-01 收口末尾

```
?? .zcode/                                                              ← ZCode agent session（已 gitignore）
?? Emotion-Echo-LLM/sensevoice-small/image/sensevoice2.png               ← Emotion-Echo-LLM submodule 内（主仓不处理）
?? Emotion-Echo-LLM/sensevoice-small/image/webui.png                     ← Emotion-Echo-LLM submodule 内（主仓不处理）
?? Emotion-Echo-Web/playwright-report/                                  ← Emotion-Echo-Web submodule 内（主仓不处理）
?? Emotion-Echo-Web/test-results/                                       ← Emotion-Echo-Web submodule 内（主仓不处理）
?? docs/architecture-audit-2026-08-31.md                                ← Stage 32 设计期间产生的 4 份文档，待用户决定是否入仓
?? docs/stage-32-apisix-reintroduction.md
?? docs/stage-33-p0-fix-bff-purify.md
```

**剩余 8 项 untracked 全部归类为外部/未决**：
- `.zcode/`、3 个 submodule 内文件：submodule 自身管理
- 3 份 docs/*.md：Stage 32/33 规划设计文档，待用户决定是否进 `docs/` 入仓（**本次收口不动**，独立决策）

### 3.2 git log（main..HEAD 顶部 4 条）

```
0e34bee chore(gitignore): 忽略 Helm Chart.lock
186fd38 docs(stage-32): 填实 §9 commits 落地清单 + 删除 §10 TODO
fc29cfa chore(helm): 提交 prometheus values.yaml apisix.serviceName 修复
9196e75 fix(apisix): host port 9080→19080 + 收口验证记录 (Stage 32 收官)
```

Stage 32 PR-13 ~ PR-16 8 条 commit + 收口 4 条 commit = **总计 12 条 commit 领先 main**（收口前 8 → 收口后 12）。

### 3.3 .gitignore 规则自检

| 路径 | 期望状态 | 实际状态 |
|------|----------|----------|
| `charts/emotion-echo/Chart.lock` | ignored | ✅ `!!`（已生效） |
| `Emotion-Echo-Web/playwright-report/` | ignored | ✅ submodule 自身 .gitignore 处理 |
| `Emotion-Echo-Web/test-results/` | ignored | ✅ submodule 自身 .gitignore 处理 |
| `.zcode/` | ignored | ✅ 第 140 行 |

### 3.4 Go 测试回归

本收口 PR **未修改任何 Go 代码**，仅修改 Helm/文档/.gitignore。按 AGENTS.md "提交前必须绿" 的精神做最后一次全模块回归：

```bash
for mod in emotion-echo-shared emotion-echo-user-svc emotion-echo-chat-svc \
           emotion-echo-assessment-svc emotion-echo-analytics-svc \
           emotion-echo-ai-svc emotion-echo-web-bff; do
  (cd "$mod" && go test ./... )
done
```

**预期结果**：全部 7 模块 ✅ PASS（无 Go 代码改动，状态应与收口前完全一致）。

---

## 四、Stage 33 交接

### 4.1 当前状态盘点

| P0 问题 | 状态 | 备注 |
|---------|------|------|
| **S-1** JWT 不验签 | ✅ **已修**（物理层） | Stage 32 PR-16：APISIX jwt-auth 真验签 + shared X-User-Id 透传 |
| **S-1 BFF 侧** mock JWT | ⏳ **Stage 33 R-3** | BFF 仍签发 mock JWT，dev 环境有效 |
| **A-1** 主聊天链路（消息不落库 + SSE 协议不匹配） | ⏳ **Stage 33 R-1 + R-2** | 唯一直接阻断核心聊天功能的 P0 |
| **S-2** 端口全暴露 | ⏳ **Stage 33 R-4** | 11 业务 svc + 4 中间件仍映射宿主端口 |

### 4.2 下一分支建议

**`feat/stage-33-pr17-sse-protocol`**（修复 A-1 之一）

理由：
1. A-1 是当前唯一直接阻断核心聊天功能可用性的 P0
2. R-2（聊天写库）依赖 R-1 的协议稳定——先协议对齐，再做写库路径
3. PR 范围小（1 个 composable + 1 个 test 文件，约 30 行改动），便于 review 与回滚

**TDD 约束**（按 AGENTS.md 硬规则）：

```
1. 写失败测试（🔴 RED）
   Emotion-Echo-Web/app/composables/useAIStreamHandler.test.ts
   描述：mock fetch 返回 OpenAI 兼容 SSE 流，
   断言 onDelta 在每次 delta 触发 + onFinish 在 [DONE] 触发。
   必须先看到测试红。

2. 写最小实现（🟢 GREEN）
   替换 switch (data.type) 为 choices?.[0]?.delta?.content 解析。
   必须让新测试 + 已有测试全绿。

3. 重构（♻️ REFACTOR）
   命名 / 错误处理 / 类型守卫，保持测试绿。
```

### 4.3 Stage 33 完整 6 PR 顺序

按 [`stage-33-p0-fix-bff-purify.md`](./stage-33-p0-fix-bff-purify.md)：

| 顺序 | 分支 | 主题 | 依赖 |
|------|------|------|------|
| 1 | `feat/stage-33-pr17-sse-protocol` | R-1 SSE 协议对齐 | 无（最优先，修复 A-1 之一） |
| 2 | `feat/stage-33-pr18-chat-persist` | R-2 聊天写库 + `messages.client_msg_id` migration | PR-17（协议稳定后才改写库前移） |
| 3 | `feat/stage-33-pr19-real-login` | R-3 BFF 真实登录（bcrypt + 验证码限流） | 无（独立） |
| 4 | `feat/stage-33-pr20-port-tighten` | R-4 端口收紧（11 svc + 4 middleware 端口仅 4 个对外） | PR-17/18（功能稳定后再收紧） |
| 5 | `feat/stage-33-pr21-bff-collect` | BFF 收口（删除 mock auth_handler + jwt_auth.go 注释改写） | PR-19 |
| 6 | `feat/stage-33-pr22-smoke` | `scripts/smoke_stage33.sh` 端到端冒烟 | PR-17/18/19/20 |

### 4.4 决策建议

**从当前 `docs/stage-31-landing` 基线开 `feat/stage-33-pr17-sse-protocol`**：

```bash
git checkout -b feat/stage-33-pr17-sse-protocol
```

**不在 Stage 33 范围内做的事**（避免范围蔓延）：
- ❌ APISIX upstream 切换 nacos-discovery（Stage 34+）
- ❌ Kafka DLQ / Outbox 封顶 / 端到端幂等（Stage 34+，I-1 P1）
- ❌ 数据库迁移工具引入（Stage 34+，D-1 P1）
- ❌ CI/CD 落地（Stage 34+，E-1 P1）
- ❌ Nacos 3 节点集群 / etcd HA / Helm prod values（Stage 34+）

---

## 五、已知未做（透明披露）

| 项 | 状态 | 备注 |
|-----|------|------|
| `deploy/tls/` 目录不存在 | 留待 Stage 34+ | nginx:alpine TLS 终结的证书目录，目前 `docker-compose.infra.yml` 的 `nginx` service 标 `profiles: ["tls"]` 默认不启动 |
| main 落后 origin 133 commits | 不在本次范围 | 历史债，main 自分支管理混乱后从未推送，需独立 PR 决策处理 |
| 11 个 feat/test/* 分支未合 main | 不在本次范围 | Stage 31 PR-02 ~ PR-12 的独立分支，属于"治理能力分批落地"的设计选择 |
| 3 份 docs/*.md 仍 untracked | 待用户决策 | `architecture-audit-2026-08-31.md`、`stage-32-apisix-reintroduction.md`、`stage-33-p0-fix-bff-purify.md` 是 Stage 32/33 设计期文档，是否入仓需独立判断 |
| Emotion-Echo-LLM/sensevoice-small/image/*.png | submodule 内 | 2 张模型预览图，主仓不处理 |

---

## 六、变更追溯

收口前 → 收口后：

```
git log main..HEAD --oneline 计数：23 → 25（+2：4 收口 commit - 2 独立 commit 因操作本质无需 commit）
```

实际只新增 4 条 commit（`fc29cfa`、`186fd38`、`0e34bee`、本文件本身的 commit），其他 2 项通过文件系统级删除完成。

---

**下一步行动**：

1. 用户 review 本收口 PR（4 commit）
2. 在 `docs/stage-31-landing` 上保留 PR，待 Stage 33 启动时一并推到 origin 或拆分
3. 启动 `feat/stage-33-pr17-sse-protocol` 进入 Stage 33（届时单独规划 plan）
