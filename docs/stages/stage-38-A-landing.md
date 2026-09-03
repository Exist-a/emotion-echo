---
status: landed
date: 2026-09-03
branch: fix/stage-36-post-test-cleanup
supersedes: stage-37-fixes-roadmap.md §B1 / §PR-B1（QQ OAuth 方向废弃）；[plans/wechat-qq-login-and-upload.md](../plans/wechat-qq-login-and-upload.md) 标 superseded
---

# Stage 38-A · 登录方式从手机号改 username+password（Landing Report）

> 状态：**全绿** · 日期：2026-09-03
> Smoke：[scripts/smoke_data_layer.py](/scripts/smoke_data_layer.py) 10/10 PASS · [scripts/smoke_bff_t5.py](/scripts/smoke_bff_t5.py) 16/16 OK

---

## 一、目标

用户决策：dev 模式登录从 "手机号 + 密码（13800138000 / abc123）" 改为 "用户名 + 密码（echo / echo123）"。放弃微信/QQ OAuth 方向。

---

## 二、关键真因（roadmap 描述与代码失真）

roadmap §B1 / §PR-B1 / [docs/plans/wechat-qq-login-and-upload.md](/docs/plans/wechat-qq-login-and-upload.md) §1.1 写：

> 微信 OAuth 已完成（代码 + 路由 + 数据模型 + config）

**实测反驳**：

```bash
$ find emotion-echo-user-svc/internal -name "*oauth*"
# 0 个文件
```

代码上 OAuth 从未实施过。user-svc 实际从 Stage 33 PR-19a 起就是 username+password 登录（[auth_handler.go:24](emotion-echo-user-svc/internal/handler/auth_handler.go) 接 `{username, password}`）。BFF 也早就透传到 user-svc（不是 mock）。

**改动范围比预期小**：

| 类别 | 范围 |
|---|---|
| 后端代码 | 0 行（已 username+password） |
| 数据 schema | 0 行（`users.username` 字段已存在） |
| Seed | 1 行改 username + 1 个 bcrypt hash |
| 文档/脚本 | 6 处引用改 + 1 处 plan 标 superseded + 1 处 roadmap 修订 |

---

## 三、落地清单（4 commits）

| Commit | 类型 | 文件 | 内容 |
|--------|------|------|------|
| `seed` | chore | `deploy/db/03-seed-default-users.sql` | username 13800138000 → echo，phone 改 NULL，bcrypt hash 改为 echo123 的新 hash |
| `seed` | chore | `scripts/check_seed_users.sh` | 默认 EXPECTED_USERNAME/PASSWORD 改 echo/echo123，输出文案改 |
| `smoke` | test | `scripts/smoke_data_layer.py` + `scripts/smoke_bff_t5.py` | LOGIN_BODY 改 echo/echo123 |
| `docs` | docs | `QUICKSTART.md` | 测试账号文案改 |
| `front` | feat | `Emotion-Echo-Web/app/pages/login/index.vue` | placeholder "邮箱"→"用户名"，quickLogin demo@emotion-echo.com/Demo12345 → echo/echo123，注册表单 placeholder 一致 |
| `plan` | docs | `docs/plans/wechat-qq-login-and-upload.md` | status: planned → superseded，加 superseded-reason 说明 |
| `roadmap` | docs | `docs/stages/stage-37-fixes-roadmap.md` | §B1 加删除线，加 Stage 38-A 修订说明 |

### 配套改动（未 commit）

- dev PG `emotion_echo_user.users` 表手动 UPDATE：
  - DELETE `username='13800138000'`
  - UPDATE `smoke_user` 的 password_hash 为 echo123 的 hash
  - INSERT `username='echo'` with echo123 hash

> 注：seed SQL 已改（commit），但 migration 006 / initdb.d 没自动跑（Stage 37-A landing 留的 backlog），
> dev 重建 PG 时需手动重跑 seed SQL 或挂 init script。本轮只手动 UPDATE 当前 dev PG。

### Bcrypt hash 生成

```python
$ python -c "import bcrypt; h = bcrypt.hashpw(b'echo123', bcrypt.gensalt(rounds=10, prefix=b'2a')); print(h.decode())"
$2a$10$x/oarv7WP0HJBNTiJGJBSeBMCvqIS.jMndnYasMS.O2SLzm7pqQnC
```

（用 `$2a$` 前缀与原 hash 一致，避免 Go `bcrypt.Verify` 不同 scheme 兼容问题）

---

## 四、Smoke 实证（2026-09-03）

### smoke_bff_t5.py（16/16 OK）

```
[OK  ] BFF /health: status=ok downstream_ok=6/6
[OK  ] BFF /api/v1/auth/login: user_id=3 token_len=181    ← echo 登录成功
[OK  ] BFF /api/v1/users/me: user_keys=['userId', 'account', 'phone', 'nickname']
[OK  ] BFF POST /api/v1/conversations: conv_id=30
[OK  ] BFF GET /api/v1/conversations
[OK  ] BFF POST /api/v1/conversations/{id}/messages: msg_id=29
[OK  ] BFF /api/v1/ai/stream (mock LLM fallback): has_sse=True
[OK  ] BFF /api/v1/reports/daily: data_keys=['date', 'summary', 'emotionDistribution', ...]
[OK  ] BFF /api/v1/surveys
[OK  ] BFF /metrics: 201 series
... (其余 OK)
```

### smoke_data_layer.py（10/10 PASS）

```
[OK  ] §1 行数 ≥ 1: actual=33
[OK  ] §2 event_type enum 细分: 4 种
[OK  ] §3 analytics_reader 读 msg_summary_v / daily_emotion_v / assessment_v / user_behavior_events: 全 OK
[OK  ] §4 /reports/daily 数据真有: summary='2026-09-03，你共有 0 段对话，0 条消息。主要情绪是 平静（1 次）...'
汇总: 10/10 PASS, 0 FAIL
```

### 手动验证

```
$ curl -X POST http://localhost:8894/api/v1/auth/login \
    -H "Content-Type: application/json" \
    -d '{"username":"echo","password":"echo123"}'
{"code":0,"data":{"accessToken":"...","user":{"id":"3","username":"echo","nickname":"Echo User",...}}}

$ curl -X POST http://localhost:8894/api/v1/auth/login \
    -H "Content-Type: application/json" \
    -d '{"username":"13800138000","password":"abc123"}'
{"code":1,"data":null,"message":"invalid username or password"}    ← 旧账号已失效
```

---

## 五、隐藏发现（前端 quickLogin 调了一个不存在的接口）

[Emotion-Echo-Web/app/pages/login/index.vue:181](/Emotion-Echo-Web/app/pages/login/index.vue) `quickLogin` 函数调用 `userStore.login({ username: 'demo@emotion-echo.com', password: 'Demo12345' })` ——**`demo@emotion-echo.com` 这个用户从未存在过**，seed 表里没有。

[Emotion-Echo-Web/e2e/login-flow.spec.ts](/Emotion-Echo-Web/e2e/login-flow.spec.ts) 注释里写 "由于 dev mode 下后端 API (localhost:18080) 未启用，quickLogin 异步调用失败"——**E2E 测试本来就预期失败**，从未真正验证 quick login 路径。

本轮修复：quickLogin 直接发 echo/echo123（标准 login 路径，不依赖任何后端 quick-login 端点）。
**真正的 quick-login 端点从来没实现过**，e2e 测试也从来没真正通过。这是另一个隐藏的 roadmap 失真点，不在本轮修复范围（文档失真待 Stage 38 单独 ADR 化）。

---

## 六、未在本轮关闭（明确留 backlog）

| # | 项目 | 留待原因 |
|---|------|--------|
| 1 | `user_oauth` 表（Stage 19/22 设计的 OAuth 预留）至今未用 | user-svc 里 0 处引用，schema 仍在；本轮不动，由 ADR 决定删/留 |
| 2 | 前端注册表单 placeholder 改"用户名"但 `getVerificationCode` 按钮仍要 6 位验证码 | dev 模式验证码固定 6 位 + 终端打印，与 username/password 模式无冲突；prod 上线再评估 |
| 3 | `quick-login` 后端端点从未实现（前端 quickLogin 实际走标准 login） | Stage 38+ 单独 ADR 决定删前端 quickLogin 按钮 或 实现真 quick-login |
| 4 | 登录失败限流：BFF auth_handler.go 有 `isLocked` + `recordFailure` 但未测试 | dev 不阻塞，留 Stage 38 |
| 5 | 跨阶段 ADR 失真清单（Stage 1-37 累计多少处 roadmap/ADR 描述与代码不符） | 单独任务，Stage 38 起 ADR-20 立项 |

---

## 七、引用

- 路线图：[stage-37-fixes-roadmap.md](/docs/stages/stage-37-fixes-roadmap.md)
- 前置 stage：[stage-37-A-landing.md](/docs/stages/stage-37-A-landing.md) · [stage-37-B-landing.md](/docs/stages/stage-37-B-landing.md)
- 废弃计划：[plans/wechat-qq-login-and-upload.md](/docs/plans/wechat-qq-login-and-upload.md)（superseded）
- Smoke 脚本：[scripts/smoke_data_layer.py](/scripts/smoke_data_layer.py) · [scripts/smoke_bff_t5.py](/scripts/smoke_bff_t5.py)
- AGENTS.md §2.4 数据契约验收：[AGENTS.md §2.4](/AGENTS.md)
- ADR-17：[adr-2026-09-chart-contract-alignment.md](/docs/architecture/adr/adr-2026-09-chart-contract-alignment.md)
