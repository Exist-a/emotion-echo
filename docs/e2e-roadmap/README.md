# docs/e2e-roadmap — 长期阶段式 E2E 测试路线图

> 建立时间：2026-09-17。本目录承载"按功能块分阶段、由浅入深"的长期 E2E 测试治理。
> 每阶段流程见全局 skill `e2e-stage-testing`（用户级 `~/.agents/skills/e2e-stage-testing/SKILL.md`）。

## 目录结构

| 文件 | 用途 |
|------|------|
| [roadmap.md](roadmap.md) | 23 阶段总表 + 改造讨论项 + 当前激活阶段指针 |
| [discovered-unresolved.md](discovered-unresolved.md) | 已发现未解决账本（E2E-F-xx 编号） |
| [findings/](findings/) | 建档/阶段预探查的代码级发现全景（带文件行号证据） |
| [stages/](stages/) | 每阶段执行记录（IAB 报告/修复清单/回归 spec/截图证据） |

### 已有 findings

- [2026-09-17-pretest-panorama.md](findings/2026-09-17-pretest-panorama.md) — 建档预探查：人格测试→AI 提示词链路（结论：不存在）、数字人（结论：假口型未同步）、横切模块（缓存/数据库/日志/监控/健康检查）

## 与现有体系的关系

| 现有体系 | 关系 |
|---------|------|
| docs/plans/known-issues-backlog-runtime-bugs-2026-09-17.md（R-xx 编号） | 本目录账本独立追踪"E2E 阶段中发现"的问题；确认为运行时 bug 且修复落地后回填 R 系 |
| docs/plans/test-coverage-tracker-2026-09-16.md | 业务路径覆盖追踪的前身，其 Sprint 111-115 排期已被本 roadmap 吸收对齐 |
| AGENTS.md §2.4 数据契约 | E2E-15 收口阶段目标 = 六项契约全绿 |
| docs/stages/ | 阶段收口记录最终按现有生命周期迁移到 docs/stages/ |

## 核心纪律

1. 每阶段开始必须先做 IAB 内置浏览器实测，不预设"有没有问题"
2. 范围外问题只记入账本不修，防止解决顺序混乱
3. 每阶段收口必须写 Playwright 回归钉
4. 修复遵循 AGENTS.md TDD 全流程（Red → Green → Refactor）
