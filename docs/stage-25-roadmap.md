# Stage 25 · Roadmap（接下来还有哪些没做）

**日期**：2026-07-16
**目标**：列出当前 Emotion-Echo 项目**还没完成**的事，按价值/工作量比排序。

---

## 一、当前项目状态总结

### 已完成（✅）
- **Stage 0-19**：后端微服务化（grpc / mTLS / 双协议 / 拦截器 / 重试 / 流式）
- **Stage 20**：emotion-llm-service + ai-svc 容器化（Dockerfile / JSON 日志 / metrics / graceful）
- **Stage 22**：FER / SenseVoice / XTTS 三个 AI 模型容器化 + Go 客户端 + MultiModalAnalyzer
- **Stage 23**：ai-svc 3 个 HTTP endpoint（multimodal / TTS / ai-health）
- **Stage 24**（本次）：端到端冒烟通过 + 修复 6+ 个 bug

### 项目健康度
- ai-svc 镜像 build 成功，运行正常
- 4 个 Stage 23 endpoint 3/4 工作（第 4 个是降级预期）
- 单元测试 18 个（aiclient 11 + multimodal 7）已写但**还没在 CI 里跑过**
- **没有 git 仓库**（所有改动都是 IDE 工作区的）
- **没有 K8s 化**（只在 docker compose）

---

## 二、按价值排序的待办事项

### 🥇 优先级 P0：项目主线 + 演示

#### A. FER / SenseVoice / XTTS 三个模型实际拉起 + 端到端测试

**问题**：当前 AI profile 没启用，3 个模型容器没起。`verify` 脚本只能验证降级路径，**没有跑过完整链路**。

**要做**：
| 任务 | 工作量 |
|------|--------|
| `docker compose --profile ai up -d --build` | 5 min |
| 等模型加载（FER ~30s, SenseVoice ~60s, XTTS ~120s） | 5 min |
| 跑完整 verify：上传图片 → FER 返回 emotion | 30 min |
| 上传音频 → SenseVoice ASR + 情绪 | 30 min |
| 文本 → XTTS 合成 → 浏览器播放 | 30 min |
| **小计** | **3-4 h** |

**价值**：项目核心能力闭环，演示价值高

---

#### B. Nuxt 前端集成 3 个 endpoint

**问题**：emotion-echo-web 现在没用 multimodal/tts/health 接口。

**要做**：
| 任务 | 工作量 |
|------|--------|
| `composables/useMultimodal.ts`：上传 file + kind → emotion | 1 h |
| `composables/useTTS.ts`：text → base64 WAV → audio 标签播放 | 1 h |
| `pages/upload.vue`：上传图像/音频 UI | 2 h |
| `pages/chat.vue`：LLM 回复 + 自动 TTS 播放 | 2 h |
| 单元测试 + 联调 | 1 h |
| **小计** | **7 h** |

**价值**：演示完整链路（拍照→分析→播放回复）

---

#### C. 把所有改动 commit 到 git

**问题**：当前整个项目没有 `.git/`，所有改动都在 IDE 工作区，**任何一次 IDE 崩溃都会丢失工作**。

**要做**：
| 任务 | 工作量 |
|------|--------|
| `git init` | 1 min |
| 写 `.gitignore`（node_modules / 证书 / docker volumes / 临时文件） | 15 min |
| 分批 commit（按 Stage 切分）| 30 min |
| push 到 Gitee / GitHub（用户之前提到 Gitee） | 10 min |
| **小计** | **1 h** |

**价值**：避免丢失工作 + 版本管理

---

### 🥈 优先级 P1：工程化质量

#### D. proto 文件规范化

**问题**：
```
emotion-echo-shared/pkg/emotionllm/   ← 应该的位置 ✅
仓库根:                              ← 错位 ❌
  ├─ emotion_llm.pb.go
  └─ emotion_llm_grpc.pb.go
emotion-llm-service/emotion_llm_pb2.py
emotion-llm-service/emotion_llm_pb2_grpc.py
```

**要做**：
| 任务 | 工作量 |
|------|--------|
| 找 emotion.proto 源文件 | 15 min |
| 写 `proto/gen.sh`（protoc 一键生成） | 1 h |
| 删散落 pb 文件 | 10 min |
| 改 shared 包引用，go build + python test 验证 | 30 min |
| **小计** | **2 h** |

---

#### E. 其他 svc 接 /metrics（chat-svc / user-svc / assessment-svc / analytics-svc）

**问题**：这 4 个业务服务没有 `/metrics`，ARMS Prometheus 抓不到它们的 QPS。

**要做**：
| 任务 | 工作量 |
|------|--------|
| 抽 `emotion-echo-shared/pkg/metrics`：PromMiddleware + PromHTTPHandler | 1 h |
| 4 个 svc 在 main.go 加 2 行（`r.Use(metrics.PromMiddleware)` + `/metrics` route）| 1 h |
| 测试 + 验证 | 30 min |
| **小计** | **2.5 h** |

---

#### F. Kafka consumer trace 上报

**问题**：ai-svc Kafka consumer 已经启动但没接 SkyWalking span。

**要做**：
| 任务 | 工作量 |
|------|--------|
| consumer.go 接 go2sky tracer | 30 min |
| 处理事件前开 span，处理后结束 | 30 min |
| 验证 SkyWalking UI 能看到 consumer→LLM→DB 的链路 | 15 min |
| **小计** | **1.5 h** |

---

#### G. 限流中间件

**问题**：ai-svc 没限流，恶意客户端可以打爆 LLM 调用。

**要做**：
| 任务 | 工作量 |
|------|--------|
| `emotion-echo-shared/pkg/middleware/limiter.go`：令牌桶 per-user | 1 h |
| ai-svc gin 接入 | 30 min |
| 测试（高并发打一个 user） | 30 min |
| **小计** | **2 h** |

---

### 🥉 优先级 P2：生产 / 部署

#### H. Helm chart（K8s 化）

**问题**：当前只有 docker compose，不能直接上 ACK。

**前置**：stage-21-k8s-strategy.md 已写完设计。

**要做**：
| 任务 | 工作量 |
|------|--------|
| 写 `deploy/helm/Chart.yaml` + `values.yaml` | 2 h |
| 写 `templates/` 14 个文件（Deployment / Service / Ingress / ConfigMap / Secret / PVC / HPA / PDB） | 4 h |
| 本地 minikube 验证 | 2 h |
| **小计** | **8 h** |

---

#### I. CI/CD（GitHub Actions）

**要做**：
| 任务 | 工作量 |
|------|--------|
| `.github/workflows/ci.yml`：lint + build + test | 1 h |
| `.github/workflows/release.yml`：build 镜像 → 推 ACR | 1 h |
| **小计** | **2 h** |

---

#### J. SSL/TLS 证书管理（Let's Encrypt）

**问题**：Ingress 暴露公网需要证书。

**要做**：
| 任务 | 工作量 |
|------|--------|
| cert-manager 安装 | 30 min |
| ClusterIssuer 配置 | 30 min |
| Ingress 集成 | 30 min |
| **小计** | **1.5 h** |

---

### 🔵 优先级 P3：长期优化

#### K. Stage 24-B：TTS 流式

`POST /api/v1/tts/stream`：流式 chunk 推送给前端（避免 base64 WAV 太大）。

**价值**：TTS 大文本场景下避免内存溢出 + 降低 TTFB。
**工作量**：4-6 h。

---

#### L. chat-svc → ai-svc 多模态串联

chat-svc 接收用户上传图片/语音 → 转给 ai-svc multimodal analyze → 返回情绪 + 文字回复 → 可选 TTS。

**价值**：完整业务链路。
**工作量**：6-8 h（chat-svc 是 go-zero 自动生成较多，需要重写 handler）。

---

#### M. Web 端 E2E 测试（Playwright）

`tests/e2e/upload.spec.ts`：浏览器上传图片 → 验证 emotion 显示 → TTS 自动播放。

**价值**：回归测试。
**工作量**：3 h。

---

#### N. 数据库迁移（gormigrate）

现在用 `db.EmotionRepo.Create()` 直接写，没有 schema 版本管理。

**价值**：生产部署必备。
**工作量**：3 h。

---

## 三、本周推荐路径

按价值 / 工作量比，**本周（1 周）**推荐做：

```
Day 1（半天）：
  C. git init + commit 所有 Stage 改动        [1 h]
  D. proto 文件规范化                         [2 h]

Day 2（半天）：
  E. shared/metrics 包抽取                    [1 h]
  E. 4 个 svc 接 /metrics                     [1 h]

Day 3（1 天）：
  A. AI profile 起 3 个模型容器               [3 h]
  A. 端到端跑通（图片 → FER）                  [1 h]

Day 4（1 天）：
  B. Nuxt 前端集成 multimodal + TTS           [6 h]

Day 5（半天）：
  F. Kafka consumer trace                     [1.5 h]
  G. 限流中间件                                [2 h]
```

**总投入**：~3.5 工作日
**总产出**：
- Git 版本管理
- proto 规范化（代码组织健康）
- 5 个 svc 都有 /metrics
- AI 模型跑通 + 前端集成
- 链路追踪 + 限流（生产化）

---

## 四、长期路线（3 个月内）

| 月份 | 主题 | 关键交付 |
|------|------|----------|
| 第 1 个月 | AI 全栈 + Git | 上述 Day 1-5 + AI 演示视频 |
| 第 2 个月 | K8s + CI/CD | H. Helm chart + I. CI/CD + 上 ACK |
| 第 3 个月 | 生产化 | J. SSL + K. TTS 流式 + L. chat-svc 串联 + M. E2E 测试 + N. DB 迁移 |

**最终目标**：可演示的完整多模态情绪 AI 产品，跑在阿里云 ACK 上，有 CI/CD + 可观测性 + 安全 + 可扩展。

---

## 五、风险与决策点

### 决策 1：要不要现在就上 K8s？

**支持**：
- 国内云原生岗位 80% 看 ACK 经验（学习价值大）
- 项目从 demo → production 的必经之路

**反对**：
- 当前在 docker compose 跑得稳，没出故障
- 学习曲线陡，K8s 配置错误代价大

**建议**：先做本周路径（C/D/E/F/G/A），**项目稳定 + 演示价值高**后再上 K8s。

### 决策 2：3 个 AI 模型要不要都启用？

**支持**：
- 启用 = 完整多模态能力
- 不启用 = 永远停在文本分析阶段

**反对**：
- XTTS 模型 2-3GB 内存，单 ECS 跑不动
- 模型加载慢（FER 30s, SenseVoice 60s, XTTS 120s）

**建议**：本地 dev 启用 FER + SenseVoice（轻量），XTTS 用云端 API（阿里云语音合成 TTS）或按需启动。

### 决策 3：Git 用 Gitee 还是 GitHub？

**Gitee 优势**：国内快、私有仓库免费（学生认证）、Gitee Pages 静态部署
**GitHub 优势**：生态好、Copilot 集成、Actions 免费额度多

**建议**：用户之前提到 Gitee，先用 Gitee，CI/CD 跑稳后再考虑同步 GitHub。

---

## 六、文档 / 代码维护清单

### 现有 docs 状态
- `docs/stage-0-learnings.md` ~ `stage-23-ai-gateway-endpoints.md`：齐全 ✅
- `docs/stage-21-k8s-strategy.md`：已写但 K8s 化未启动
- `docs/distributed-architecture.md` / `microservices-architecture.md`：架构总览，可作 onboarding 文档

### 待修正的 docs
- **stage-22.md** 中 Stage 22-A.5 状态写为 ⏳，**实际已完成**
- **stage-20.md** 中 Stage 20-P0-1 标"完成"，但实际有 bug，已被 Stage 22-B 修复
- **stage-23.md** 已写完但**没有提降级验证结果** + 没指明 ai-svc 路径
- **缺失**：stage-24 文档（本文件即补上）

### 文档 review 标准
每篇 Stage 文档应包含：
1. 日期 + 目标 + 前置依赖
2. 子任务列表 + 完成状态
3. 关键改动文件
4. 验证结果（端到端跑通截图 / log）
5. 踩坑清单
6. 后续候选（指向下一阶段文档）