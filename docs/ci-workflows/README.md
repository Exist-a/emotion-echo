---
purpose: GitHub Actions workflow 模板（绕过 PAT workflow scope 限制的产物存放处）
status: Stage 97 PR-9e 闭环
related-stage: stage-97
---

# docs/ci-workflows · Stage 97 PR-9e 落地物

> **背景**：Round 2 §P1-R2-16 要求仓库根有 `.github/workflows/` CI，
> AGENTS.md §2.2 合并前门槛要求 `go test ./...` + `go vet ./...` + (前端) `npm run lint`。
>
> **阻塞**：当前 PAT 仅含 `repo` scope，GitHub 拒绝 push 修改
> `.github/workflows/*.yml`（refusing to allow a Personal Access Token to create
> or update workflow）。
>
> **解决方案**：把 workflow 文件存放在 `docs/ci-workflows/`（git 可跟踪位置，
> 不受 workflow scope 限制），用户后续 3 选 1 启用：
>
> 1. **PAT 加 workflow scope**（GitHub Settings → Personal access tokens → 编辑
>    现有 token → 勾 `workflow`）后：
>    ```bash
>    mkdir -p .github/workflows
>    cp docs/ci-workflows/*.yml .github/workflows/
>    git add .github/workflows/ && git commit -m "ci: 启用 GitHub Actions"
>    git push
>    ```
>
> 2. **GitHub UI 粘贴**：Settings → Actions → New workflow → set up a workflow
>    yourself → 把 `docs/ci-workflows/{go,llm,web}-test.yml` 内容粘贴进去。
>
> 3. **改用 SSH key**（推荐长期方案）：
>    ```bash
>    ssh-add ~/.ssh/id_ed25519
>    git remote set-url origin git@github.com:Exist-a/emotion-echo.git
>    cp docs/ci-workflows/*.yml .github/workflows/
>    git add .github/workflows/ && git commit -m "ci: 启用 GitHub Actions"
>    git push
>    ```

## 文件清单

| 文件 | 用途 | 覆盖 |
|------|------|------|
| `go-test.yml` | 6 Go svc + shared 全跑 go test + go vet | ai-svc / analytics-svc / chat-svc / web-bff / assessment-svc / user-svc / shared |
| `llm-test.yml` | emotion-llm-service pytest | /analyze auth (P0-R2-3) + gRPC + file_context + Nacos |
| `web-test.yml` | emotion-echo-web vitest + lint | composables + apiRoutes + auth.global + renderMarkdown |

## 启用后效果

- `main` push / PR 自动触发
- 任何 test 失败 → PR 不可 merge
- 与 AGENTS.md §2.2 "合并前 go test ./... + go vet ./... + npm run lint" 挂钩

## 调研依据

- Round 2 plan §P1-R2-16
- AGENTS.md §2.2 合并前门槛
- Stage 97 收口报告 §6.1 PR-9e 阻塞说明
- 实际 push 错误：`refusing to allow a Personal Access Token to create or update workflow .github/workflows/go-test.yml without workflow scope`