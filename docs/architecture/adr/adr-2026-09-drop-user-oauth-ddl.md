# ADR · 2026-09 · 清理 emotion_echo_user.user_oauth 表 DDL 残留（OAuth 路径已主动废弃）

> **状态**：✅ **Accepted**（2026-09-10）
> **关联**：
> - [stage-62-cleanup-and-grpc-plan.md §二.5](../../plans/stage-62-cleanup-and-grpc-plan.md)
> - [wechat-qq-login-and-upload.md](../../plans/wechat-qq-login-and-upload.md) (`status: superseded by Stage 38-A`)
> - [stage-33-landing.md §七](../../stages/stage-33-landing.md) PR-19a（username + password 落地）
> - 决策 18（doc-drift-registry 失真台账 #25 关联）

---

## 一、上下文（Context）

`emotion_echo_user.user_oauth` 表是项目早期设计微信/QQ 第三方登录时遗留的 DDL：

| 历史轨迹 | 来源 |
|---|---|
| legacy Gin 单体阶段（2026 早期） | `oauth_handler.go` + `oauth_service.go` 处理 `WechatOpenID` / `WechatUnionID` |
| Stage 33 PR-19a（2026-09-XX） | user-svc 切到 username + password 登录，**OAuth 代码路径下线** |
| Stage 38-A（2026-09-XX） | 用户明确决策"改用 username + password 登录，去掉微信/QQ OAuth 路径" |
| 现状（2026-09-10） | DDL 表残留 + `.env.example` WECHAT/QQ 注释残留 |

**实测零引用**（[Stage 62 PR-5 契约测试脚本](../../../../scripts/test_user_oauth_zero_ref.sh) 5/5 PASS）：

| 维度 | 结果 |
|---|---|
| Go svc 零引用 user_oauth | ✅ 0 命中 |
| 前端 emotion-echo-web 零引用 | ✅ 0 命中 |
| Python emotion-llm-service 零引用 | ✅ 0 命中 |
| legacy/ 目录豁免（决策 19 归档不动） | ✅ 0 命中 |
| emotion-echo-user-svc model 无 OAuth 字段 | ✅ grep WechatOpenID/UnionID/provider = 0 |
| .env.example 中 WECHAT_APP_ID/WECHAT_REDIRECT_URI/QQ_REDIRECT_URI | ✅ 清理后 0 行 |

---

## 二、决策（Decisions）

### §A. 删除 `emotion_echo_user.user_oauth` 表

执行路径：`deploy/db/05-drop-user-oauth.sql`（挂 `initdb.d`，仅在**全新 dev 环境**数据卷为空时执行），
对已存在 dev 环境通过 `migrate.sh` 单独 PR 收口（不在本 ADR 范围）。

### §B. 删除 `deploy/db/01-create-schemas.sql` 与 `02-create-tables-in-schemas.sql` 中的 OAuth DDL 块

仅删除对应 DDL 块 + 加注释说明"已删"，避免新人误读 OAuth 已落地。

### §C. 清理 `emotion-echo-web/.env.example` 中 OAuth 模板注释

替换为"第三方登录配置（已废弃）"小节 + ADR 21 引用 + 未来撤销流程指针。

### §D. 决策 19 规范保留：`legacy/` 目录 oauth_handler.go 不动

已归档的 legacy Gin 单体代码按决策 19 不主动改动；`legacy/emotion-echo-gin/internal/handler/oauth_handler.go`
文件保留作为"该路径曾存在过"的历史快照。

---

## 三、撤销流程（什么时候能恢复 OAuth）

满足以下**全部**条件才能恢复：

1. **用户明确决策**"重新启用 OAuth 登录"（owner 签字）
2. **新建 ADR**撤销本决策（明确写明原因 + 触发场景）
3. **新建 migration**重建表 + user-svc model 加 OAuth 字段 + BFF 加 OAuth 端端
5. **新 plan**描述完整 OAuth 实施步骤（参考 [wechat-qq-login-and-upload.md](../../plans/wechat-qq-login-and-upload.md)）

**否决条件**：
- 仅凭"功能想加"或"未来可能需要"不构成撤销理由（避免反向决策 18 类型 5 自报告）
- 撤销必须基于**真实业务需求**（如：与第三方心理咨询平台对接、监管要求境内账号体系等）

---

## 四、未做项（不在本 ADR 范围）

- **已存在 dev 环境的 user_oauth 残留**：dev 数据卷非空，`initdb.d` 不执行，需要走 `migrate.sh`
  单独 PR 收口。本 ADR 暂不开 `06-drop-user-oauth-existing.sql`（避免引入未知副作用）
- **legacy/emotion-echo-gin oauth_handler.go 删除**：决策 19 豁免
- **legacy/emotion-echo-gin/docs/DESIGN.md 中的 OAuth 设计**：已归档，不动
- **frontend 任何 OAuth 路由 / Vue 组件**：实测零引用，无需清理

---

## 五、调研依据（commit message 末尾格式）

| 项 | 来源 |
|---|---|
| Stage 38-A 主动弃 OAuth | `docs/plans/wechat-qq-login-and-upload.md:9-13`（supersede-reason） |
| wechat-qq-login-and-upload.md 已 superseded | 同上 `status: superseded` + `superseded-at: 2026-09-03` |
| Stage 33 PR-19a username+password 落地 | `stage-33-landing.md §七` |
| user_oauth 表 DDL 残留 | `grep -n "user_oauth" deploy/db/*.sql` 实测 4 处命中 |
| `.env.example` 残留 | `grep "WECHAT\|QQ_" emotion-echo-web/.env.example` 实测 4 行 |
| 决策 19（废弃件处置） | `decisions.md` 决策 19 |
| 契约测试通过 | `bash scripts/test_user_oauth_zero_ref.sh` 5/5 PASS |
| model 无 OAuth 字段 | `grep -i "wechat\|union\|provider\|oauth" emotion-echo-user-svc/internal/model/user.go` 0 命中 |

---

> 最后更新：2026-09-10 by Stage 62 PR-5 session
> 用途：OAuth DDL 残留清理决策 · 撤销流程显式化
> 关联：决策 18 #25（doc-drift-registry OAuth DDL 残留 type 2 陈旧结论）→ 本 ADR closure