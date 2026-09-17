# stages/ — E2E 阶段详档与执行记录

每个阶段一个目录：`e2e-NN-<kebab-topic>/`，内含：

| 文件 | 用途 | 撰写时机 |
|------|------|---------|
| `plan.md` | 阶段详档：目标/范围/边界/前置条件/测试点清单/DoD/风险/产出物 | 轮到该阶段前 1 个阶段时 |
| `report.md` | 执行记录：IAB 实测结果、发现分类、修复 commit 清单 | 阶段执行后 |
| `screenshots/` | IAB 截图证据（按测试点编号命名） | 阶段执行中 |

模板：[_TEMPLATE.md](_TEMPLATE.md)

## 已建目录

| 阶段 | 目录 | plan.md |
|------|------|---------|
| E2E-01 登录会话持久化 | `e2e-01-login-session/` | ✅ |
| E2E-02 项目目录清理 | `e2e-02-directory-cleanup/` | ✅ |
| E2E-03 CI/CD 门槛 | `e2e-03-ci-gate/` | ✅ |
| E2E-04 前端工程化门槛 | `e2e-04-frontend-engineering/` | ✅ |
| E2E-05 文档与代码一致性 | `e2e-05-doc-code-consistency/` | ✅ |
| E2E-06 数据库改造 | `e2e-06-db-transformation/` | ✅ |
| E2E-07 找回/重置密码 | `e2e-07-password-recovery/` | ✅ |
| E2E-08 历史会话管理 | `e2e-08-conversation-management/` | ✅ |
| E2E-09 注册流程 | `e2e-09-registration/` | ✅ |

E2E-10 及以后：轮到前补写。

## 收口约定

阶段收口后，按 `docs/README.md` 的生命周期把阶段记录迁移/归档到 `docs/stages/`，本目录保留 `plan.md` 作为规划留档。
