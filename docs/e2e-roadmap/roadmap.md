---
status: active
priority: high
created: 2026-09-17
last-refresh: 2026-09-17
type: e2e-stage-roadmap
---

# E2E 阶段式测试路线图（长期）

## 当前激活阶段

**E2E-01 登录会话持久化**（status: in-progress，2026-09-17 起）

## 排期总表（由浅入深）

| 阶段 | 功能块 | 目标 | 边界（不做） | 状态 |
|------|--------|------|--------------|------|
| E2E-01 | 登录会话持久化 | cookie 存储/刷新恢复/过期/登出/remember-me 全周期验证 | 注册、找回密码 | 🔄 in-progress |
| E2E-02 | 找回/重置密码 | 三步向导 + 验证码 + 重置 + 重新登录 | 真实邮箱服务 | ⏳ pending |
| E2E-03 | 历史会话管理 | 会话列表/删除/pin/重命名/分组 | 消息内容同步 | ⏳ pending |
| E2E-04 | 注册流程 | 邮箱验证码注册全流程 | — | ⏳ pending |
| E2E-05 | 聊天核心链路 | 发送/SSE 流式/错误处理/中断重试 | 多模态 | ⏳ pending |
| E2E-06 | 我的空间 | 资料修改/头像上传（MinIO 链路） | — | ⏳ pending |
| E2E-07 | 设置页 | 字体/主题切换与持久化 | — | ⏳ pending |
| E2E-08 | 心理测验 | 列表→答题→提交→结果查看 | — | ⏳ pending |
| E2E-09 | 报表 Dashboard | 数据内容正确性/日期切换/历史 chartData=[] 复查 | — | ⏳ pending |
| E2E-10 | 多模态 | 语音/表情/TTS/文件上传/数字人 | — | ⏳ pending |
| E2E-11 | 支撑：可观测 | SkyWalking trace UI / OAP 9.x queryDuration bug | — | ⏳ pending |
| E2E-12 | 支撑：消息链 | outbox→Kafka→ai-svc→入库；数据契约 §1/2/6 | — | ⏳ pending |
| E2E-13 | 支撑：基础设施 | Nacos 注册/配置、APISIX 路由/限流、MinIO | — | ⏳ pending |
| E2E-14 | 横切：异常与安全 | JWT 过期刷新/IDOR/限流/CORS | — | ⏳ pending |
| E2E-15 | 收口 | §2.4 六项数据契约 smoke 全绿 | — | ⏳ pending |

## 依赖声明

- E2E-02 依赖 E2E-01（验证码弹窗位于登录页，会话状态影响流程验证）
- E2E-09 依赖 E2E-05（报表数据来自聊天产生的行为事件）
- E2E-10 依赖 E2E-05（多模态入口位于聊天页）
- E2E-12 / E2E-13 依赖前面链路稳定（浅层问题会污染深层观测）

## 每阶段标准流程

见全局 skill `e2e-stage-testing`（6 步：取阶段卡 → 环境准备 → IAB 实测 → 发现分类 → TDD 修复 → 收口回归钉）。

## 历史与背景

- 前身：`docs/plans/test-coverage-tracker-2026-09-16.md`（业务路径覆盖追踪，Sprint 109-115）
- 动机：逐块测试发现真实环境问题频发——单元绿 ≠ 端到端绿（历史：16/16 smoke 全绿但 dev 模式 chartData=[]、event_type 全错）
- 排期依据：用户 2026-09-17 提出功能块顺序——"登录 cookie 存储 → 找回重置密码 → 历史会话管理"，后续块按"用户可见功能 → 支撑模块"由浅入深展开
