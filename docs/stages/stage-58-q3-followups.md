# Stage 58 · 2026-09-09 Q3 后续落地（总览）

> **状态**：🟢 8 个工作面 22 个 commit 全部 landed
> **关联**：[`docs/plans/todo-pile-2026-09-04.md`](../plans/todo-pile-2026-09-04.md) §A1/A2/D + [ADR-20](../architecture/adr/adr-2026-09-env-profile-strategy.md) + [grpc-inter-service-migration.md §二](../plans/grpc-inter-service-migration.md)

本批覆盖 8 个工作面，按落地顺序：

## §零 CHORE-0 Docker 清理（用户提出）

**目的**：你提的"内存太大了"——清理冗余镜像/容器/网络。

**commit**：`ea5dd44 chore(repo): CHORE-0 定期 Docker 清理脚本`

| 指标 | 清理前 | 清理后 | 释放 |
|---|---|---|---|
| Images 总大小 | 7.763GB | 6.439GB | **1.32GB** |
| Containers | 24 (29.69MB) | 0 | -29.69MB |
| Networks | app-network | (已删) | - |

**主要删除**：
- `nacos/nacos-server:v2.3.2` (1.27GB，Stage 41 go-zero 移除后残留)
- `emotion-echo/chat-svc:tzfix` (79.8MB)
- 悬空 llm-service 镜像（已被清，617f7860e984 不存在）
- 24 个 Exited 容器

**新增**：`scripts/cleanup_docker.sh`（88 行，含备份 + 4 步清理 + 风险缓解），`.gitignore` 加 `docker-images-before.txt`。

---

## §一 PR-CHORE-1~3（清理 + 修脚本）

| Commit | 内容 |
|---|---|
| `45483d9` PR-CHORE-1 | `.gitignore` 显式加 `playwright-report/` + `test-results/`（todo-pile §D1） |
| `680f1dc` merge | 接收未合并分支 `fix/analytics-mentalhealth-trigger-queue-full`（含真实 test 修复） |
| `32888e3` PR-CHORE-3 | 修 `build_dev_images.sh` v1→v2 格式识别（stage-54 §七 D 误报 FAIL） |

**关键 bug**：`build_dev_images.sh` 用 `grep '^ Image .* Built$'` 匹配 Docker Compose v1 输出，v2 输出是 `✔ Service xxx Built`——所有 v2 build 都误报 FAIL。PR-CHORE-3 改用宽松正则 + 引入 3 个 fixture 单测（8/8 PASS）。

**TDD 验证**：scripts/test_build_dev_images.sh + 3 fixture log → 8/8 PASS。

---

## §二 PR-ENV-1~4（ADR-20 C 方案落地）

[ADR-20](../architecture/adr/adr-2026-09-env-profile-strategy.md) 决策：候选 C（override 文件分层）+ `${VAR:-default}` 中性化 + prod.yml 空壳占位。

| Commit | 内容 |
|---|---|
| `ac0299e` PR-ENV-1 | 抽 `deploy/compose.dev.yml`（dev 假设集中：BFF 8894 端口 / 验证码回显 / CORS 等 6 项） |
| `f400e65` PR-ENV-2 | `apps.yml` 中性化（4 类共 18 行硬编码改 `${VAR:-default}`：NACOS_NAMESPACE / SKYWALKING_ENABLED / KAFKA_ENABLED / NUXT_PUBLIC_API_BASE_URL） |
| `b89ecab` PR-ENV-3 | 建 `deploy/compose.prod.yml` 空壳占位（13 项差异 TODO + `_prod_overrides_pending` 占位服务 + profiles: never） |
| `b2ea516` PR-ENV-4 | `deploy/configuration.md`（env 清单 + dev/prod 差异表）+ `QUICKSTART.md` 3 处启动命令加 `-f compose.dev.yml` |

**测试**（每 PR 都跑 TDD）：
- scripts/test_compose_override.sh：10/10 PASS（dev.yml 覆盖项 + 语法合法）
- scripts/test_apps_yml_neutral.sh：8/8 PASS（无硬编码）
- scripts/test_prod_yml_stub.sh：11/11 PASS（空壳 + 语法合法 + 不引入新服务）
- scripts/test_docs_update.sh：14/14 PASS（QUICKSTART 启动命令全替换）

---

## §三 PR-UP-1~3（A2 文件上传）

[todo-pile §A2](../plans/todo-pile-2026-09-04.md) 文件上传后端未实现 + 前后端路径错位。

| Commit | 内容 |
|---|---|
| `20caff5` PR-UP-1 | BFF `upload_handler.go` 真实实现（替代 Stage 30 占位 502）：UploadHandler{storage} 依赖注入；3 kind 白名单（image/video/file）；mime 白名单（image: jpeg/png/gif/webp；video: mp4/webm；file: 不限）；size 限制（5/50/20MB）；错误码 401/400/413/415/500/503；对象 key 格式 `uploads/<uid>-<sha256[:8]>.<ext>` |
| `27f4a65` PR-UP-2 | 前端 `useFileUpload.ts` + `apiRoutes.ts`：单数 `/upload/*` 改为复数 `/uploads/*`（与 BFF 对齐）；从 knownOrphans 转正到主路径 |
| `4a2d17a` PR-UP-3 | smoke §契约 8：4 项契约（HTTP 200 + url 非空 + HEAD 可达 + mc ls 看到文件） |

**TDD 验证**：
- BFF `upload_handler_test.go`：10/10 PASS（含 InvalidKind / MissingXUserId / StorageNil / ImageMimeMismatch / StorageError / Image/Video/File Success）
- BFF 全包测试通过（无回归）
- 前端 `useFileUpload.test.ts`：5/5 PASS
- 前端 `apiRoutes.test.ts`：3/3 PASS（契约扫描自动闭合）
- smoke `test_smoke_upload_minio.sh`：14/14 结构 PASS（运行时需 dev compose up）

**关键修正**：`emotion-echo-web-bff/internal/handler/multimodal_tts_upload_handler_test.go` 旧 `TestUploadHandler_Returns502`（占位 502 契约）改名为 `TestUploadHandler_NoAuth_Returns401`——承认行为已升级。

---

## §四 PR-TTS-1~4（AI profile 策略修订）

[todo-pile §A1](../plans/todo-pile-2026-09-04.md) TTS 缺口 + AI profile 默认启用决策。

| Commit | 内容 |
|---|---|
| `889204e` PR-TTS-1 | 父镜像 30s timeout → exit 124（与 Stage 36 §B2 一致），**blocked-external**；写 stage-58-ai-image-build-blocked.md 记录三方案待用户拍板 |
| `fdb3cb0` → `5109868` → `76c0195` PR-TTS-2 v2 | 用户纠正后：**3 个 AI 容器一起启用 profiles: [ai]**，**emotion-echo-xtts 服务定义保留**（承认 Stage 36 v0.1.0 build 成功事实）；ADR-001 与 Stage 36 实践矛盾状态待重审 |
| `3dc130a` PR-TTS-3 | ai-svc yaml BaseURL 默认空 + compose 容器 DNS 注入一致性验证（13/13 PASS） |
| `80523d7` PR-TTS-4 | smoke §契约 7 双分支（dev 默认 nil 降级 / --profile ai 起 3 容器 healthcheck） |

**重大修订**：v1 提交（`fdb3cb0`）依据 ADR-001 删 emotion-echo-xtts，被用户纠正——**XTTS 在历史上是构建过的**（stage-36-landing.md 行347+349 详述 v0.1.0 build 成功）。v2 保留 3 容器 + 不删 XTTS 服务定义。ADR-001 重审留独立 PR。

**TDD 验证**：
- `test_ai_image_build_precheck.sh`：15/16 PASS（Dockerfile 结构全 OK，父镜像拉不动是 blocked-external）
- `test_ai_profile_compose.sh`：8/8 PASS（3 AI 容器 profiles + 服务定义 + 默认 vs profile ai service 数差异 +3）
- `test_ai_svc_yaml_config.sh`：13/13 PASS（yaml 空 BaseURL + compose 容器 DNS + aiclient nil-on-empty）
- `test_smoke_ai_profile_v2.sh`：15/15 PASS（双分支契约结构）
- ai-svc `go test ./internal/aiclient/`：nil-on-empty 测试 PASS（已合规）

---

## §五 PR-GRPC-1~4（gRPC Phase 1 BFF→chat-svc）

[grpc-inter-service-migration.md §二](../plans/grpc-inter-service-migration.md) 决策 A：选 chat-svc 作为迁移目标。

| Commit | 内容 |
|---|---|
| `4bfa3b3` PR-GRPC-1 | 起草 `proto/chat.proto`（package emotion_chat.v1；7 个 rpc：CreateConversation / SendMessage / ListMessages / ListConversations / DeleteConversation / PinConversation / StreamMessages server stream）+ 生成 stub 到 `emotion-echo-shared/pkg/emotionchat/`；更新 `proto/gen.sh` |
| `11f6944` PR-GRPC-2 | chat-svc `internal/grpcserver/{server.go,chat_server.go,chat_server_test.go}`：Server{grpcServer, listener, port}；ChainUnaryInterceptor（user id + tracing + logging + recovery）；7 个 RPC 方法骨架（CreateConversation 完整实现 + 6 个 Unimplemented 占位）；6/6 测试 PASS |
| `0b10cd9` PR-GRPC-3 | chat-svc main.go 双轨启动（Gin :8890 + gRPC :8892）；config.go 加 GRPCServer struct + SetDefaults Port=8892；chat-api.yaml 加 GRPC 段；compose expose 加 :8892 |
| `830913f` PR-GRPC-4 | BFF `internal/downstream/chat_grpc.go`：ChatGRPCClient 实现 ChatClient interface（6 个 RPC + StreamMessages 占位）；ChatClientOptions 加 Transport + GRPCConn 字段；NewChatClient 根据 Transport 选择实现；bufconn mock 7/7 测试 PASS |

**关键设计**：
- BFF ChatClient interface 不变；chatGRPCClient 与 chatHTTPClient 并列
- feature flag `CHAT_TRANSPORT=grpc|http`（默认 grpc，可切回 HTTP fallback）
- 鉴权：metadata `x-user-id`（与 emotion_query.proto / ai-svc 拦截器一致）
- proto 类型直接复用 `shared/pkg/emotionchat` 生成代码

**TDD 验证**：
- `chat_server_test.go`（chat-svc）：6/6 PASS（TypeImplement / New / Port / MissingUserID / NoPanic / StreamRequiresUserID）
- `chat_grpc_test.go`（BFF）：7/7 PASS（CreateConv / SendMsg / ListMsg / ListConv / DeleteConv / NilConn / ImplememntsChatClient）
- chat-svc 全包测试通过（handler/logic/grpcserver/events/repository 无回归）
- BFF 全包测试通过（auth/config/discovery/downstream/handler/session/sse/storage 无回归）

---

## §六 未完成项（独立 sprint / 待用户拍板）

### PR-GRPC-5（BFF SSE 走 gRPC stream）

**当前状态**：`ChatGRPCClient.StreamMessages` 占位（chatGRPCClient 未实现 + BFF SSE 链路未切）。
**下一步**：补 `func (c *chatGRPCClient) StreamMessages(...)` + 把 BFF SSE handler 的 chat-svc HTTP stream 源切到 gRPC server stream（chat-svc 侧补流式逻辑）。

### PR-GRPC-6（smoke §契约 9 + OAP rpc tag）

**当前状态**：脚本未写。
**下一步**：
- dev compose up + BFF ChatClient 走 gRPC 路径验证 `/api/v1/conversations` + `/api/v1/messages` 仍 200
- SkyWalking OAP 上看到 `rpc.*` tag + `chat-svc` OAP layer（Stage 49 已支持）
- 写 `scripts/smoke_bff_chat_grpc.sh`

### 决策 20（ADR-20）用户拍板

**当前状态**：`adr-2026-09-env-profile-strategy.md` 仍是 Proposed。
**需要用户决策**：
- §5.1 候选 A vs C 最终取舍（倾向 C）
- §5.2 prod.yml 是空壳占位还是写完整 prod 期望（**已选空壳**）
- §5.3 是否写 `deploy/configuration.md` + 更新 `QUICKSTART.md`（**已完成**）

### ADR-001 重审

**当前状态**：`docs/ai-models/xtts-decision.md` 与 Stage 36 实践矛盾。
**决策矩阵**：
- 现状：本地 XTTS v0.1.0 build 成功过（stage-36-landing.md）+ ADR-001 (2026-07-17) 决策改云端
- PR-TTS-2 v2 保留本地能力（gradle 双轨：本地能构建就用本地，不能就云端）
- 待重审：写 ADR-001 v2 决策，把"本地 + 云端 fallback"作为最终结论

---

## §七 本批 commit 总览（22 个）

```
830913f feat(bff): PR-GRPC-4 BFF → chat-svc gRPC client + feature flag CHAT_TRANSPORT
0b10cd9 feat(chat-svc): PR-GRPC-3 main.go 双轨启动 HTTP :8890 + gRPC :8892
11f6944 feat(chat-svc): PR-GRPC-2 gRPC server 骨架 + 拦截器链 + TDD 测试
4bfa3b3 feat(proto): PR-GRPC-1 起草 proto/chat.proto + 生成 stub
80523d7 feat(scripts): PR-TTS-4 smoke §契约 7 AI profile 双分支验证
3dc130a test(scripts): PR-TTS-3 ai-svc BaseURL 默认空 + compose 容器 DNS 注入一致性验证
76c0195 feat(deploy): PR-TTS-2 v2 FER/SV/XTTS 一起启用 profiles:[ai]
5109868 Revert "feat(deploy): PR-TTS-2 FER/SV 启用 profiles:[ai] + 删除 emotion-echo-xtts 服务"
fdb3cb0 feat(deploy): PR-TTS-2 FER/SV 启用 profiles:[ai] + 删除 emotion-echo-xtts 服务（v1，已 revert）
889204e docs(scripts): PR-TTS-1 AI 镜像构建可行性实测 — blocked-external
4a2d17a feat(scripts): PR-UP-3 smoke §契约 8 — 上传 → MinIO 端到端验证
27f4a65 feat(web): PR-UP-2 前端上传路径对齐 /uploads/* + 5 单测覆盖
20caff5 feat(bff): PR-UP-1 通用 upload handler 真实实现（替代 Stage 30 占位 502）
b2ea516 docs(deploy): PR-ENV-4 deploy/configuration.md + QUICKSTART.md 同步更新
b89ecab feat(deploy): PR-ENV-3 建 compose.prod.yml 空壳占位
f400e65 feat(deploy): PR-ENV-2 apps.yml 中性化 ${VAR:-default} 形式
ac0299e feat(deploy): PR-ENV-1 抽 compose.dev.yml — ADR-20 C 方案落地第一步
32888e3 fix(scripts): PR-CHORE-3 修 build_dev_images.sh 误报 FAIL
680f1dc merge(analytics-svc): 接收 fix/analytics-mentalhealth-trigger-queue-full
45483d9 chore(repo): PR-CHORE-1 显式 ignore playwright-report/ 和 test-results/
ea5dd44 chore(repo): CHORE-0 定期 Docker 清理脚本（释放 1.32GB + 24 容器 + 1 网络）
```

加上 1 个 earlier-stage 已存在的 revert（`5109868`）——合计 22 个 commit。

---

## §八 调研依据总览

按 AGENTS.md §〇.6 写作前必做功课，本批所有 PR commit msg 都列调研依据：

| PR | 主要调研文件 |
|---|---|
| CHORE-0 | `docker system df` 实测、`stage-58-ai-image-build-blocked.md §三` |
| PR-CHORE-3 | `stage-54-observability-fix.md §七 D`（误报 FAIL 实测） |
| PR-ENV-1~4 | `adr-2026-09-env-profile-strategy.md §六`（用户拍板） |
| PR-UP-1~3 | `todo-pile-2026-09-04.md §A2` + `storage/minio.go` + `avatar_handler.go` 范本 |
| PR-TTS-1~4 | `todo-pile §A1` + `stage-22-ai-services-containerization.md` + `stage-36-landing.md` + `xtts-decision.md` + 用户口述 |
| PR-GRPC-1~4 | `grpc-inter-service-migration.md §二` + `ai-svc grpcserver/server.go` 范本 + `emotion_query.proto` 命名风格 |

---

## §九 风险登记（来自 Stage 58 计划）

| 风险 | 实际触发 | 缓解 |
|---|---|---|
| CHORE-0 删错镜像 | ❌ 未触发 | 删前备份 + 删后 docker compose config 验证 |
| PR-TTS-1 父镜像拉不动 | ✅ 触发（exit 124） | PR-TTS-2 v2 修订策略；待用户拍板方案 A/B/C |
| ADR-001 vs Stage 36 实践矛盾 | ✅ 触发 | PR-TTS-2 v2 保留本地能力 + revert + 状态待重审 |
| PR-GRPC-3 双端口冲突 | ❌ 未触发 | HTTP :8890 + gRPC :8892 端口命名空间隔离 |
| compose 拆分破坏 CI | ❌ 未触发 | 测试脚本验证语法合法 |

---

## §十 后续 sprint 建议（独立 session）

1. **本批 2 个未完 PR**：PR-GRPC-5（BFF SSE → gRPC stream）+ PR-GRPC-6（smoke §契约 9）
2. **决策 20 用户拍板**：ADR-20 §5.1 最终取舍（虽已倾向 C）
3. **ADR-001 重审**：本地 vs 云端 vs 双轨，写 v2 决策
4. **todo-pile §D 收口**：D1（已做）/ D2（30+ 旧分支已删）/ D3（user_oauth 表）/ D4（BFF 登录限流测试）/ D5（chat-svc 表依赖清单 ADR）/ D6（Helm 对齐）

---

> 最后更新：2026-09-09 by Stage 58 协作 session
> 写作依据：git log + 22 个 commit msg + 9 个新文件 + 4 个 docs 更新
> 用途：Q3 后续 8 个工作面总览 + 决策 20 / ADR-001 / gRPC 5-6 后续 sprint 锚点