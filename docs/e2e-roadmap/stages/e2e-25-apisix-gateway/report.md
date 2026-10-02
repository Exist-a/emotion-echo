---
stage: e2e-25
title: 网关 APISIX（上游健康检查/重连 + seed↔admin 治理 + JWT/限流/CORS）
executed: 2026-10-02
status: partial
environment: dev 模式（30 容器 healthy，compose.dev.yml + .env.local；worktree ../Emotion-Echo-e2e25 执行）
---

# E2E-25 执行记录

## 0. 未完成清单（唯一真相源 · 收口时必须逐条销账）

**1 项未完成**：

| # | 事项 | 责任人 | 可核验的完成判据 | 状态 |
|---|------|--------|------------------|------|
| **T-1** | M2 漂移处置策略裁定（fail-closed vs 覆盖+报告，建议后者，见 §6） | 用户 | `docs/e2e-roadmap/decisions.md` 出现 D-36 处置策略决议；若选 fail-closed 则 `check_apisix_drift.sh verify` 的 exit 2 改为拦截并补测试；裁定后本阶段翻 done | 🔴 待裁定 |

## 1. 环境基线

- 启动命令：`cd deploy && docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml -f compose.dev.yml --env-file .env.local --profile dev up -d`（错峰：先核心 infra 后全量）+ `--profile obs` 补观测栈
- 容器状态：30 容器全 healthy，`db-migrate`/`apisix-seed`/`minio-init`/`kafka-init` 均 Exited(0)，Nacos `count:6`
- **worktree 教训**（已记忆归档，防复发）：gitignored 的 `deploy/.env.local` 与 `deploy/tls/*` 不随 worktree 检出——.env.local 必须手动复制；TLS 文件缺失时 Docker 会把单文件 bind mount 源自动建成**空目录**，导致 llm-service `IsADirectoryError` 崩溃循环（修复 = 拷文件后 `--force-recreate`）
- 声明的配置差异：BFF_DEV_RETURN_CODE=1、BFF_TRUST_APISIX=true（dev 覆盖，compose.dev.yml）

## 2. 测试点结果

| # | 测试点 | 判定 | 结果 | 证据 | 备注 |
|---|--------|------|------|------|------|
| 1 | upstream `checks.active` TDD 落地 | [A] | PASS | seed_test.js RED 6 FAIL → GREEN 72/72；admin GET /upstreams/1 返回 checks 段 | PR #137 |
| 2 | **M1：checks × nacos 动态节点兼容性** | [M] | PASS | control API `/v1/healthcheck`：动态节点被健康检查管理器登记并双向翻转 | **实证成立，无需 fallback**（D-35） |
| 3 | 故障摘除时窗 | [A] | PASS | 停 user-svc → 节点 ~7s 翻 `unhealthy`（对照 D-30 的 75s 滞后基线）；`/user-health` 503 快速失败；重启 → ~10s 翻回 healthy（success=2）+ `/user-health` 200×3 | |
| 4 | BFF 重启自愈窗口 | [A] | PASS | 10:41:00 重启 web-bff → 10:41:19 login 200（≤19s，零手工干预；对照 F-137 事发时需手工 restart apisix） | |
| 5 | 网关自身 healthcheck 语义（F-f） | [A] | PASS | etcd 停 → 容器 **90s 转 unhealthy**（3×30s）；恢复 → **15s 回 healthy** + login 200；admin 状态码对照 503↔200 | PR #139 |
| 6 | IAB 故障注入可见行为 | [V] | PASS | screenshots/06-chat-during-chat-svc-outage.png（骨架完整/无白屏/无踢登录/空态+spinner）+ 06-chat-recovered.png（列表恢复） | 前端把 503 静默处理为空态，记观察项 O-1 |
| 7 | seed 幂等性 | [A] | PASS | `check_apisix_drift.sh verify` → no drift + no extras（24 行快照） | PR #138 |
| 8 | 漂移检测两形态 | [A] | PASS | 篡改 route 100 → seed 覆盖回（§0.2.1）；PUT 探针 299 → extras rc=1 精确定位 → 删除后 rc=0 | |
| 9 | **M2：漂移处置策略（fail-closed vs 报告）** | [M] | BLOCKED | 工具已落地（exit 1=extras / 2=篡改类），**处置策略需用户裁定**（见 §6） | 唯一阻塞项，1/20 ≤ 1/3 |
| 10 | 路由集合回归钉 + CI 兜底 | [A] | PASS | lib 契约测试 15 断言 + wrapper 离线测试 8 断言进 `apisix-seed-structure` job | PR #138 |
| 11 | 弱断言修复 | [A] | PASS | `set -euo pipefail` 断言改为匹配非注释行 `^set -eu$`；全量复核无"被注释满足"型 | PR #137 |
| 12 | check_routes_alignment 保持全绿 | [A] | PASS | 开工基线 PASS=2 FAIL=0（8/8 auth action 对齐） | |
| 13 | jwt 验签 401/200 + X-User-Id 注入 | [A] | PASS | 有效 token → /users/me 200 userId=1；篡改签名 → 401 | |
| 14 | 伪造 X-User-Id 被覆盖 | [A] | PASS | 伪造 header=999 + 有效 token → 返回 userId=1（无条件覆盖）；无 token + 伪造 → 401 | |
| 15 | 白名单 9 条免 jwt | [A] | PASS | register/reset-password/verify-security-answer → 400（业务）；其余 → 200；login 错密码 → **业务层 JSON** 401；对照 /conversations 无 token → APISIX 401 | 回归钉 spec 钉死 |
| 16 | 浏览器端到端 | [V] | PASS | screenshots/16-login-page-iab.png + 16-chat-page-after-login.png + 16-daily-report-page.png（登录→聊天→报表全通，无 401 踢回） | |
| 17 | 单节点 429 + 死变量清除 | [A] | PASS | 修复前 60×200+10×**503** → 修复后 60×200+10×**429**；PLUGINS_JSON 死变量删除 + 断言锁死 | N2 + F-d，PR #137 |
| 18 | **M3/F-145：双节点配额不放大** | [M] | PASS | 临时第二 APISIX 节点（同 etcd/redis）交替打 70 发 → **合计 60×200+10×429**（=单节点配额，共享 redis 键） | 实测资源可行（+400MiB），压完即撤 |
| 19 | CORS preflight + 单源回归 | [A] | PASS | 六头齐全 + origin 精确回显；重跑 seed 后仍成立 | 回归钉 spec 钉死 |
| 20 | api-breaker 真实触发/恢复 | [M] | PASS | 可控 5xx 探针 upstream：500×3 → **503 打开** → 200 恢复；连接拒绝 502 不计数（同官方文档）；**N3**：原配置三字段非 schema 字段被静默忽略 → 已换真实字段 | seed_test 72/72 |

汇总：PASS 19 / FAIL 0 / BLOCKED 1 / N/A 0

## 3. 发现与分类

| 发现 | 分类 | 处理 |
|------|------|------|
| N1：route 205 /apisix-health 无 upstream ⇒ 命中即 503（自健康路由本身是坏的） | 范围内 | 修复 PR #139（serverless exit(200)）；账本 E2E-F-176 ✅ |
| N2：AUTH_WHITELIST limit-count 缺 rejected_code ⇒ 限流拒绝 503 与 catch-all 429 不一致 | 范围内 | 修复 PR #137；账本 E2E-F-177 ✅ |
| N3：api-breaker 配置 min_requests/error_threshold_ratio/open_time 非 schema 字段被静默忽略（实际跑默认值） | 范围内 | 修复本 PR；账本 E2E-F-178 ✅ |
| O-1：上游 503 时前端对话列表静默显示空态+spinner（无错误提示） | 范围外（前端 UX） | 只记账不修，归前端体验轮 |
| O-2：seed_test.js 曾断言被注释满足的 `set -euo pipefail`（弱断言实例） | 范围内 | #11 修复（PR #137） |
| O-3：`echo` 插件不终止请求（body=ok 仍 503） | 范围内（实现取舍记录） | 改用 serverless exit(200)，取舍写入 seed.sh 注释 |
| O-4：守卫提取 sed 的 `[[:space:]]*[a-z]` 截止锚会截断多行块标量探针 | 范围内（守卫自省） | 修复锚为 `/^  [a-z]/`（本 PR） |
| O-5：本地守卫循环曾从主 checkout 运行（shell cwd 重置）造成假绿假红双误判 | 过程教训 | 已写入 commit + 记忆；全程改用 worktree 绝对路径复跑 |
| O-6：MSYS 吞多行 `node -e` 参数（`-e requires an argument` rc=9，静默） | 过程教训 | CLI 入口进 lib 不用 -e；已记忆归档 |
| O-7：Git Bash tampered-token 末字符替换是等价签名（base64 填充位）⇒ 测试偶发 200 假失败 | 范围内（测试构造缺陷，非安全漏洞） | 回归钉改篡改签名首字符 |

## 4. 修复清单（TDD 记录）

| commit | 内容 | 先行的失败测试 |
|--------|------|---------------|
| da5f972→#137 | C1/C4：checks + rejected_code + 删 PLUGINS_JSON | seed_test.js RED 6 FAIL |
| 72d022d→#138 | C2：apisix_drift_lib + wrapper + CI | lib/wrapper 契约测试 RED（模块/脚本不存在） |
| 4e81c0c→#139 | C3：route 205 + healthcheck 改造 | seed_test.js RED 3 FAIL |
| fd0bfdc | N3：api-breaker 真实字段 | seed_test.js RED 6 FAIL |
| d42b165/#138 | check_apisix_drift.sh 补执行位 | CI（ubuntu）直接执行 Permission denied |
| 本 PR | O-4 守卫锚修复 + 回归钉 spec | test_obs_healthchecks.sh RED 2 FAIL（worktree 内复现） |

## 5. 回归钉

- 新增 spec：`emotion-echo-web/e2e/apisix-gateway.spec.ts`（6 用例 × chromium+mobile = 12，本地连跑 5 轮 12/12 全绿；含 token 缓存防限流自伤 + 随机用户名防登录锁定自伤）
- CI 静态钉：seed_test.js 72 断言（apisix-seed-structure job）+ drift lib/wrapper 测试

## 6. 待决策 / 升级项

| # | 决策 | 建议 |
|---|------|------|
| M2（#9） | seed 漂移处置策略：**A. fail-closed**（seed 检测到漂移即退出，要求人工确认）vs **B. 覆盖+报告**（现状：seed 覆盖后 drift 工具报告，exit 2 只报不拦） | 建议 **B+CI 兜底**：dev 环境保持"seed 是唯一真理源"的覆盖语义（可预期、可自动化），篡改类漂移由 `check_apisix_drift.sh verify` 在 dev-up 后报告 + 账本留痕；A 会让 compose up 卡死在交互确认，违反无人值守启动。若用户选 A，verify 的 exit 2 改为直接拦截 |
| M1（#2） | checks × discovery 实证成立（D-35），无需 fallback | 无需用户动作，登记备查 |
| M3（#18） | 双节点验证已实测完成（临时容器压完即撤） | 无需用户动作 |

## 7. 收口自检

- [ ] git status 干净
- [ ] main 与 origin 无 ahead/behind
- [ ] 无残留已合并分支

> 状态 partial 的原因：#9（M2）需用户裁定 + RUNBOOK §13.3 第二方核对未做。裁定与核对完成后翻 done。
