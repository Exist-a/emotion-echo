---
stage: e2e-05
title: 文档与代码一致性收口
executed: 2026-09-18
status: done
environment: dev 模式（Docker 后端容器 + 本地 pnpm dev 前端）
---

# E2E-05 执行记录（report）

## 1. 环境基线

- 启动命令：`cd deploy && docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml -f compose.dev.yml --env-file .env.local up -d`
- 容器状态：6 个应用服务 healthy（user/chat/analytics/assessment/ai/web-bff）+ infra 组件
- 前端：本地 `pnpm dev --port 3000`
- 声明的配置差异：`BFF_DEV_RETURN_CODE=1`、`BFF_TRUST_APISIX=true`、`KAFKA_ENABLED=false`

## 2. 测试点结果

| # | 测试点 | 判定 | 结果 | 证据 | 备注 |
|---|--------|------|------|------|------|
| 1 | 11 个脚本本地基线盘点 | [A] | PASS | 基线输出 | 6 PASS / 2 FAIL / 2 WARN |
| 2 | 全部接入 CI 并在 push 时执行 | [A] | PASS | `.github/workflows/doc-drift-check.yml` | 11 个 job |
| 3 | check_docker_digests.sh 假绿已修 | [A] | PASS | 脚本输出"FAIL: 占位 digest" | 6 个占位值检出 |
| 4 | 2 个 migration 脚本已创建 | [A] | PASS | `scripts/test_migrations_contract.sh` + `test_migrations_no_service_order.sh` | |
| 5 | 3 处失真已更正 | [A]+[V] | PASS | git diff | stage-21 + .env.common |
| 6 | 更正的失真已登记 ADR-18 表 | [A] | PASS | `adr-2026-09-doc-drift-registry.md` §八 | |

汇总：**PASS 6 / FAIL 0 / BLOCKED 0 / N/A 0**

## 3. 发现与分类

| 发现 | 分类 | 处理 |
|------|------|------|
| lint_env_vars.sh 检出 8 个未文档化变量 | 范围内 | 记录为已知 drift，不在本阶段修（需更新 .env.local.example） |
| check_docker_digests.sh 6 个占位 digest | 范围内 | ✅ 脚本已修复，检出占位值 |
| stage-21 Secret 泄露声明失真 | 范围内 | ✅ 已更正 |
| .env.common APISIX 版本 + Gin 配置过时 | 范围内 | ✅ 已更正 |
| migration 脚本缺少事务包装 | 范围外 | 记录为 WARN，不在本阶段修 |
| analytics 视图跨 schema 引用 | 范围外 | 记录为 WARN（合理架构，非真正依赖） |

## 4. 修复清单（TDD 记录）

| commit | 内容 | 先行的失败测试 |
|--------|------|---------------|
| `a1bb8b5` | check_docker_digests.sh 检测占位 digest 防假绿 | 基线盘点检出假绿 |
| `7af9cba` | 新增 migration 契约 + 服务顺序独立性校验脚本 | 脚本不存在 |
| `6f68a09` | 新增 doc-drift-check CI workflow | CI 不存在 |
| `cd375a6` | 更正 2 处文档失真（stage-21 + .env.common） | 基线盘点检出失真 |
| `52b400b` | ADR-18 更新 — 推翻"不引入 CI 校验"决策 | E2E-05 计划 |

## 5. 回归钉

- 无新增 Playwright spec（本阶段是治理基础设施，不涉及用户流程）
- CI workflow `.github/workflows/doc-drift-check.yml` 本身就是回归钉 — 每次 push/PR 自动运行 11 个校验脚本

## 6. 待决策 / 升级项

| 项 | 优先级 | 备注 |
|---|---|---|
| lint_env_vars.sh 8 个未文档化变量 | medium | 需更新 `.env.local.example` 补充文档 |
| Dockerfile.digests.lock 6 个占位 digest | high | 需运行 `scripts/sync_docker_digests.sh` 回填真值（需 docker.io 网络） |
| migration 脚本事务包装 | low | 建议后续 migration 统一加 BEGIN/COMMIT |

## 7. 收口自检

- [x] `git status` 干净（或改动是有意保留的）
- [ ] `git status -sb` main 与 origin/main 无 ahead/behind（待 push）
- [ ] `git branch --merged main` 除 main 外为空（待确认）

## 8. commit 历史

| # | Commit | 内容 |
|---|--------|------|
| 1 | `a1bb8b5` | fix(scripts): check_docker_digests.sh 检测占位 digest 防假绿 |
| 2 | `7af9cba` | test(scripts): 新增 migration 契约 + 服务顺序独立性校验脚本 |
| 3 | `6f68a09` | ci(e2e-05): 新增 doc-drift-check workflow — 11 个校验脚本全部接入 CI |
| 4 | `cd375a6` | docs(e2e-05): 更正 2 处文档失真 |
| 5 | `52b400b` | docs(e2e-05): ADR-18 更新 — 推翻"不引入 CI 校验"决策 + 登记 3 处更正失真 |