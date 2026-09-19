---
stage: e2e-11
title: 我的空间
executed: 2026-09-19
status: done
environment: dev 模式（17 容器 healthy，docker-compose.infra.yml + docker-compose.apps.yml + compose.dev.yml + .env.local）
---

# E2E-11 执行记录（report）

> 详档 [plan.md](plan.md)。填写规则见 [RUNBOOK.md](../../RUNBOOK.md) §10。

## 1. 环境基线

- 启动命令：
  ```bash
  cd deploy && docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml \
    -f compose.dev.yml --env-file .env.local up -d
  ```
- 容器状态：17 个容器（infra 8 + 业务 6 + web + apisix + seed）全部 healthy。
  注意：验收期间 Docker Desktop 曾整体假死（build 中断、`docker ps` 空返回），
  重启 Docker Desktop 后容器自动恢复；三服务镜像随后重建（build-e2e11.log /
  build2~5.log，BUILD_EXIT=0）。
- 声明的配置差异：dev overlay 为 `NUXT_PUBLIC_API_BASE_URL=http://localhost:19080/api/v1`、
  各 svc `CORS_ALLOW_ORIGINS=http://localhost:3000`。**验收发现的额外事实**：
  `:3000` 实际由宿主机残留的 `nuxt dev` 进程（PID 19808，`node nuxt dev --port 3000`）
  服务（Windows 把 `localhost` 解析到 `::1`，Docker 映射的 `0.0.0.0:3000` 被绕开）——
  该进程热加载了本轮源码，前端改动实时可见；web 容器内为旧构建产物，不影响结论。

## 2. 测试点结果

| # | 测试点 | 判定 | 结果 | 证据 | 备注 |
|---|--------|------|------|------|------|
| 1 | `/user/profile` 返回真实用户数据（非硬编码"体验用户"） | `[A]` | **PASS** | curl：`{"id":"1","username":"echo","nickname":"Echo User","avatar":"http://localhost:9000/avatars/avatars/1-2ec01835.png","createdAt":"2026-09-04T19:38:56+08:00"}` | 修复前恒返 `"demo"/"体验用户"/avatar:""` |
| 2 | 修改昵称成功 + 页面显示新昵称 | `[A]`+`[V]` | **PASS** | PATCH 200 + 重拉 profile 一致 + 页面 `.nickname` 断言新值 | Playwright #2 含复原步骤 |
| 3 | 昵称格式校验（1 字符 / 13 字符被拦截，不发请求） | `[A]`+`[V]` | **PASS** | 网络监听 PATCH 计数 = 0；截图 [t03-nickname-validation.png](screenshots/t03-nickname-validation.png) | 修复前弹框按钮在 DOM 中不存在 |
| 4 | 年龄校验（-1 / 131 被拦截，不发请求） | `[A]`+`[V]` | **PASS** | 网络监听 PATCH 计数 = 0；截图 [t04-age-validation.png](screenshots/t04-age-validation.png) | 年龄落库缺口另见 E2E-F-80 |
| 5 | 头像上传成功 + MinIO 存储 + 页面头像更新 | `[A]`+`[V]` | **PASS** | POST /user/avatar 200；DB `avatar_url` 非 NULL；MinIO 匿名 GET 200（image/png）；截图 [t05-avatar-uploaded.png](screenshots/t05-avatar-uploaded.png)、[t05b-avatar-on-page.png](screenshots/t05b-avatar-on-page.png) | 经历 3 层修复，见 §3 发现 F5 |
| 6 | 头像 >2MB 被前端拦截 | `[A]`+`[V]` | **PASS** | 网络监听 /user/avatar 请求计数 = 0 | |
| 7 | 昼夜使用模式饼图有数据可渲染 | `[V]` | **PASS** | 截图 [t07-09-three-charts.png](screenshots/t07-09-three-charts.png)（4 时段 35/46/21/14=116，与 DB 逐小时桶聚合一致） | 修复前 API 400 / 塌缩成单桶 |
| 8 | 近 30 天对话频次折线图有数据可渲染 | `[V]` | **PASS** | 同上截图；API `dates[7]`+`messageCount[4,9,15,13,62,11,2]`=116 与 DB 一致 | |
| 9 | 互动深度指标柱状图有数据可渲染 | `[V]` | **PASS** | 同上截图；API `totalMessages=116`（DB 116）、`totalConversations=34` | SQL 语义与文档注释的 1 条偏差见 E2E-F-82 |
| 10 | 图表空态可读（不与图表同时出现） | `[V]` | **PASS** | Playwright #10：`chartCount===0 || emptyCount===0` 且骨架屏归零 | 修复前空态恒渲染 |
| 11 | 退出登录弹框确认 → 跳转登录页 | `[A]`+`[V]` | **PASS** | URL 断言 `/login`；截图 [t11-logout-dialog.png](screenshots/t11-logout-dialog.png) | 修复前"确认退出"按钮不存在 |
| 12 | 侧边栏"我的空间"导航可进入 | `[V]` | **PASS** | URL `/chat/user` + `.user-info-card` 可见；截图 [t12-sidebar-nav.png](screenshots/t12-sidebar-nav.png) | |

汇总：**PASS 12 / FAIL 0 / BLOCKED 0 / N/A 0**

> 判定依据：`[V]` 项的 8 张截图全部被逐张查看；`[A]` 项附 curl / 网络监听 / DB 查询输出。

## 3. 发现与分类

| # | 发现 | 分类 | 处理 |
|---|------|------|------|
| F1 | BFF `/user/profile` 恒返硬编码 mock + 头像在 4 层链路逐层丢弃（user-svc types→proto→BFF downstream→ProfileVM） | 范围内 | commit `6ddc21d` |
| F2 | analytics-svc 3 个 UserBehavior gRPC RPC 是"返空 + 注释 PR-3.4 阶段补"的桩 ⇒ 图表恒空态 | 范围内 | commit `5c9886a` |
| F3 | BFF behavior 响应形状与前端 chartData 期望字段完全错位（pattern vs periods 等） | 范围内 | commit `11962f8` |
| F4 | 前端 6 缺陷：notify 未 import / `<Plus/>` 未定义 / 空态骨架屏恒渲染 / el-upload action="" / 表单不回填 / 错误消息传空 | 范围内 | commit `b9a9cf6` |
| F5 | **头像上传 3 层丢弃点**（实测 200 但 DB 恒 NULL）：① avatar_handler 传 c.Request.Context() ⇒ gRPC Unauthenticated；② BFF UpdateMe 漏映射 AvatarUrl；③ user-svc UpdateProfile proto→types 漏组装 | 范围内 | commit `50ff3a6` |
| F6 | **Element Plus 未安装**（el-dialog/el-upload 未解析）⇒ 弹框内联恒渲染 + footer 按钮被静默丢弃（实测 DOM 中不存在） | 范围内 | commit `0b535ad` |
| F7 | **Pinia store getter 快照化**：`const x = store.getX` 拿到普通字符串 ⇒ 页面恒显兜底值、弹框回填空 | 范围内 | commit `0b535ad` |
| F8 | **nav 布局无 NotifyHost** ⇒ 聊天区所有 notify() 静默 | 范围内 | commit `0b535ad` |
| F9 | behavior 三端点不传日期时 BFF/analytics-svc 双重 400（前端本来就不传） | 范围内 | commit `27e40e3` |
| F10 | **hour 桶塌缩**：`h*3600 % 24 ≡ 0` ⇒ 24 桶全塌到 hour 0 且互相覆盖（116 事件只剩 1 桶 14 条） | 范围内 | commit `27e40e3` |
| F11 | 深度指标 proto 只有 buckets+average_length，BFF 只读后者 ⇒ 3/4 指标恒 0 | 范围内 | commit `27e40e3` |
| F12 | **年龄不落库**：前端 PATCH {age} 被 Go json.Unmarshal 静默丢弃（BFF/proto 无 age 字段；表列是 birthday） | 范围外 | 账本 [E2E-F-80](../../discovered-unresolved.md) |
| F13 | `.ee-field`/`.ee-input` 全仓无样式定义，data-label 不渲染 | 范围内（顺手修） | commit `0b535ad` |
| F14 | `GetInteractionDepth` 文档注释写"totalConversations = DISTINCT session_id 数"，实际 SQL `GROUP BY session_id` 把 NULL session 也算一组（34 vs 33） | 范围外 | 账本 [E2E-F-81](../../discovered-unresolved.md) |

## 4. 修复清单（TDD 记录）

| commit | 内容 | 先行的失败测试 |
|--------|------|---------------|
| `6ddc21d` | profile 透传真实数据（4 层映射） | `TestToProfileVM_MapsAvatarURL`、`TestUserHandler_Profile_ReturnsRealData`（实测 RED：avatar=""、nickname="demo"） |
| `5c9886a` | 3 个 UserBehavior gRPC 对接 logic | `TestAnalyticsServer_UserBehavior{DayNight,Depth,Frequency}_ReturnsData`（实测 RED：空数组） |
| `11962f8` | BFF 前端契约变换层 | `TestAnalyticsHandler_{DayNight,Frequency,Depth}_ReturnsFrontendShape`（实测 RED：字段全空） |
| `b9a9cf6` | 前端 6 缺陷 + 静态契约钉 12 项 | `e2e-11-my-space-contract.architecture.test.ts`；对 HEAD 版本跑同断言 11/11 缺陷被捕获（非空证明） |
| `27e40e3` | 默认日期窗口 + hour 槽编码 + 深度指标还原 | `TestAnalyticsHandler_BehaviorEndpoints_DefaultDateWindow`、`TestAnalyticsServer_UserBehaviorDayNight_HourSlotEncoding`、`TestAnalyticsGRPCClient_InteractionDepth_MapsAllMetricsFromBuckets` |
| `50ff3a6` | 头像链路 3 层透传 | `TestAvatarHandler_PassesUserIDToDownstream`、`TestUserGRPCClient_UpdateMe_PassesAvatarURL`、`TestUserServer_UpdateProfile_PersistsAvatarURL`（每层 RED 有实证） |
| `0b535ad` | Element Plus 替换 + computed getter + NotifyHost | `e2e-11-my-space-contract.architecture.test.ts`（21 项，其中 el-dialog/el-upload/按钮/getter 各断言均先红后绿）+ `nav.test.ts` NotifyHost 契约 |

## 5. 回归钉

- 新增 spec：`emotion-echo-web/e2e/my-space.spec.ts`（10 用例覆盖 12 测试点，最终运行 **10/10 绿**，57.7s）
- 新增静态契约钉：`emotion-echo-web/app/pages/chat/user/e2e-11-my-space-contract.architecture.test.ts`（21 项，全绿）
- nav 布局契约：`emotion-echo-web/app/layouts/nav.test.ts` 新增 1 项（全绿）
- Go 侧：BFF handler/downstream、user-svc grpcserver 新增契约测试 7 条，`go test ./...` 三服务全绿 + `go vet` 全绿
- 前端全量：vitest 409/409 绿

## 6. 待决策 / 升级项

- **E2E-F-80 年龄落库**：age↔birthday 语义需决策（age 是派生值、birthday 是源值，直接存 age 会随时间失真），且需 proto 变更 + 两端重新生成。留账，不在本阶段动。
- 无其他升级项。

## 7. 收口自检

- [x] `git status` 干净（改动均有意保留/已提交）
- [x] `git status -sb` main 与 origin/main 无 ahead/behind
- [x] `git branch --merged main` 除 main 外为空

## 附：验收期间的环境事故记录

Docker Desktop 在验收中途整体假死（所有 `docker` 命令空返回、build 挂起 45 分钟、
容器端口全断）。处置：`taskkill` 强制结束 com.docker.backend.exe 后重启 Docker
Desktop，容器自动恢复（restart: unless-stopped）。此后所有镜像重建/重启均正常。
事故根因未深挖（范围外），仅记录：三服务镜像均已在本轮 rebuild 且 BUILD_EXIT=0，
验收结论基于重建后的镜像。
