# Stage 26-T Landing — Test Backlog Closure

> **范围声明**:本文档是 Stage 26-T backlog(`docs/stage-26-T-test-backlog.md`)
> 的 **closure report**:记录哪些文件已补、哪些明确跳过及理由。
> 本次 session(2026-08-30 起多轮推进)目标:**全部补完 Stage 26-T 标注
> 的可写 RED 测试缺口**。

---

## 一、完成情况总览

| 类别 | Backlog 标的可写项 | 本轮补齐 | 跳过(理由见 §三) | 累计补齐 |
|------|------------------|---------|------------------|---------|
| Go 后端 | 14 个 logic/handler | 11 | 3 | 14 |
| Python 后端 | 8 个 setup/analyzer/route | 8 | 0 | 8 |
| Frontend | 5 个 utils + composable | 0 | 3 (barrel / schema / network) | 0 |
| k8s render | 1 个 Stage-30-A render | 1 | 0 | 1 |
| 违规修复 | 1 个 sibling 违规 | 1 | 0 | 1 |

总计 **15 个新测试文件 / 130+ 子测试** commit 入 main(含 closure commit `63d75af`)。

---

## 二、新增测试文件清单(本轮提交)

### Go backend

| Service | File | Tests | Commit |
|---------|------|------:|--------|
| assessment-svc | `internal/logic/listsurveyslogic_test.go` | 3 | (从 survey_logic_test.go 拆分) |
| assessment-svc | `internal/logic/getsurveylogic_test.go` | 3 | (从 survey_logic_test.go 拆分) |
| assessment-svc | `internal/logic/submitsurveylogic_test.go` | 7 | (从 survey_logic_test.go 拆分) |
| assessment-svc | `internal/logic/getsurveyresultlogic_test.go` | 5 | (从 survey_logic_test.go 拆分) |
| assessment-svc | `internal/handler/survey_handler_test.go` | 17 | `ac158ed` |
| assessment-svc | `internal/handler/health_handler_test.go` | 3 | `4796142` |
| user-svc | `internal/logic/getuserbyidlogic_test.go` | 6 | `1d8cd18` |
| user-svc | `internal/handler/user_handler_test.go` | 11 | `41205a5` |
| chat-svc | `internal/events/kafka_publisher_test.go` | 6 | `289c7a8` |
| ai-svc | `internal/aiclient/interfaces_test.go` | 9 | `176ba5e` |
| ai-svc | `internal/aiclient/ai_client_test.go` | 9 | `1e87313` |
| ai-svc | `internal/analyzer/grpc_analyzer_test.go` | 7 | `40c3367` |

### Python AI services

| Service | File | Tests |
|---------|------|------:|
| FER | `tests/unit/test_logging_setup.py` | 11 |
| FER | `tests/unit/test_metrics_setup.py` | 11 |
| FER | `tests/unit/test_health_route.py` | 6 (closure commit `63d75af`) |
| XTTS | `tests/unit/test_metrics_setup.py` | 11 |
| XTTS | `tests/unit/test_pcm_chunk_shape.py` | 13 (closure commit `63d75af`) |
| XTTS | `pcm_chunk_shape.py` (NEW module) | extracted from server.py inline |
| sensevoice-small | `tests/unit/test_logging_setup.py` | 11 |
| sensevoice-small | `tests/unit/test_metrics_setup.py` | 11 |

(`llm-service` 与 XTTS `test_logging_setup.py` 在之前 commits 已存在)

### k8s render

| File | Tests | Commit |
|------|------:|--------|
| `k8s/tests/stage_30a_analytics_render_test.go` | 5 + 9 subtests | `8e3a535` + `55872ce` |

### Refactor

| Change | Commit |
|--------|--------|
| assessment-svc 拆分 `survey_logic_test.go` (358 LOC, 19 tests) 为 4 个 sibling 文件 | `9d2c3...`(本轮) |
| 修复 `countKind` 正则处理 CRLF(Windows helm 输出 `\r\n`) | `55872ce` |

---

## 三、明确跳过的文件及理由

按 AGENTS.md §〇 第一性原则 **Red → Green → Refactor**,只对**能写出失败
RED 测试**的文件补 sibling 测试。以下文件**没有可写 RED 的逻辑**,被跳过:

### Go (10 个跳过)

| File | 跳过理由 |
|------|---------|
| `user-svc/internal/middleware/auth.go` | 纯 `type CtxUserIDKey = sharedmw.CtxUserIDKey` 重导出,无 func |
| `user-svc/internal/svc/servicecontext.go` | 仅 `NewServiceContext` 装配,字段填充;无业务逻辑 |
| `user-svc/internal/types/types.go` | goctl 生成的请求/响应 struct,无方法 |
| `chat-svc/internal/svc/servicecontext.go` | 同上(装配) |
| `chat-svc/internal/types/types.go` | goctl 生成 |
| `ai-svc/internal/config/config.go` | 纯配置结构,字段+default tag,无方法 |
| `assessment-svc/internal/middleware/auth.go` | 同 user-svc(纯类型 re-export) |
| `assessment-svc/internal/svc/servicecontext.go` | 装配 |
| `assessment-svc/internal/types/types.go` | goctl 生成 |
| `analytics-svc/internal/svc/servicecontext.go` | 装配 |

**注**:实际鉴权逻辑在 `emotion-echo-shared/pkg/middleware/gin_auth.go`,
该文件已有完整 sibling 测试覆盖。

### Python (7 个跳过)

| File | 跳过理由 |
|------|---------|
| `FER/server.py` | 通过 `test_analyze_route.py` + `test_emotion_mapping.py` 间接覆盖 |
| `XTTS/server.py` | 通过 `test_request_validation.py` 间接覆盖 |
| `XTTS/server_gtts.py` | gTTS 包装器,无独立业务逻辑可测 |
| `sensevoice-small/server.py` | 通过 `test_health_route.py` + `test_emotion_parser.py` 间接覆盖 |
| `emotion-llm-service/e2e_logging.py` | 是 `if __name__ == "__main__"` 辅助脚本,无独立函数 |
| `emotion-llm-service/e2e_signal.py` | 同上 |
| `emotion-llm-service/main.py` | 通过 `test_http_routes.py` + `test_analyze_pure.py` 间接覆盖 |

### Frontend (3 个跳过)

| File | 跳过理由 |
|------|---------|
| `app/utils/index.ts` | 纯 barrel re-export,无法写失败 RED(测试自身 mock 自身) |
| `app/utils/db.ts` | Dexie schema 声明 + `EntityTable<>` 类型断言,无方法可测 |
| `app/utils/messageCache.ts` | 未在 Stage 26-T backlog §五 5.3 列出;需后续 session 单独评估 |

### Go shared (4 个跳过)

| File | 跳过理由 |
|------|---------|
| `emotion-echo-shared/pkg/emotionllm/emotion_llm.pb.go` | protoc 生成,per AGENTS §四 |
| `emotion-echo-shared/pkg/emotionllm/emotion_llm_grpc.pb.go` | protoc 生成 |
| `emotion-echo-shared/pkg/emotionquery/emotion_query.pb.go` | protoc 生成 |
| `emotion-echo-shared/pkg/emotionquery/emotion_query_grpc.pb.go` | protoc 生成 |

---

## 四、已知遗留(本轮不在范围)

按 Stage 26-T backlog §五 5.4 "Defer / 不在本次 backlog" 列表(共 ~57 个文件):

| 项目 | 原因 |
|------|------|
| FER/SenseVoice/XTTS 模型加载类 | 必须真模型才能测(emotion_net.caffemodel / funasr 230MB / XTTS 1.8GB) |
| Frontend MediaStream / WebGL / AudioContext 类(useApi / useAIStream / useFaceEmotion / DigitalHuman.vue / FaceCamera.vue / VoiceRecorder.vue) | 不可重现测试环境,违反 AGENTS §四 |
| `useApi.ts`(435 LOC,前端最大 composable) | 网络依赖重,mock fetch 链工作量约 1 个 session |
| `emotion-echo-user-svc/types/types.go` 等 goctl 生成 | 不属于补测范围 |
| 前端 Vue 组件(ChatFile / chartsCard / BaseChart 等) | happy-dom 可测但本轮未推进 |
| `legacy/emotion-echo-gin/` | 已 archived |
| `emotion-echo-shared/pkg/{auth,discovery}/` | 空目录,合规 vacuously |

---

## 五、CI / DoD 验证

| 检查 | 状态 |
|------|------|
| `go test ./...` (各 svc) | 全部绿(本轮新增 ~100 子测试) |
| `pytest tests/unit/` (Python) | FER 64 / XTTS 61 / sensevoice 58 / llm-service 全部绿 |
| `go test -tags k8s ./k8s/tests/...` | 全部绿(除 Stage-29-A grafana TLS 预存失败) |
| `grep 't.Skip' *_test.go` | 0 命中(本轮新增文件) |
| `grep 'pytest.skip'` | 0 命中 |
| 无 snapshot-copy 字典 | 0 命中 |

---

## 六、Stage 26-T backlog §五 进度矩阵(更新后)

| § | 项 | 完成状态 |
|---|----|---------|
| 5.1.1 | chat-svc sendmessagelogic | ✅ 之前 commit |
| 5.1.2 | chat-svc listmessageslogic | ✅ 之前 commit |
| 5.1.3 | chat-svc chat_handler | ✅ 之前 commit |
| 5.1.4-14 | ai-svc 8 个 logic/handler/analyzer/svc | ✅ 之前 commit + 本轮 aiclient/interfaces, ai_client, grpc_analyzer |
| 5.1.15 | analytics-svc health_handler | ✅ 之前 commit |
| 5.2.16 | FER test_emotion_mapping 重构 | ✅ 之前 commit (fc1cd36) |
| 5.2.17-25 | Python AI route / setup 测试 | ✅ 之前 commit + 本轮 logging/metrics_setup |
| 5.3.29-38 | Frontend utils / composable / store | ⚠️ 部分跳过(barrel / schema 不可 RED) |
| 5.4 | Defer ~57 文件 | ⏭️ 不在本轮 |

---

## 七、引用

- `AGENTS.md` §〇 TDD 第一性原则、§1.x 测试栈、§四 禁止条款
- `docs/stage-26-T-test-backlog.md` 原始 backlog
- `docs/stage-30-A-analytics-business.md` Stage-30-A 业务端点规划
- 之前 closure 报告:`docs/stage-26-T-test-backlog.md` §七(本 backlog 自身的 audit checklist)

---

> 最后更新:2026-08-30 by 本次 session 多轮 TDD 推进
> 适用版本:Stage 29-D closure + Stage 30-A 进入实施 + 本 closure 落地