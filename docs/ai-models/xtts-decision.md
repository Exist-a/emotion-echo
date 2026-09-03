# XTTS 云端 API 选型决策记录（ADR-001）

**日期**：2026-07-17
**状态**：✅ 已确认 · 待落地
**关联**：`stage-25-final-landing.md`、`xtts-cloud-api-integration.md`、`AGENTS.md`（TDD 强制）

---

## 一、背景（Context）

本地 XTTS 容器（Coqui TTS 0.22 + torch 2.13 + torchaudio 2.11）三方依赖互锁，
反复 build 4 次失败（详细见 `stage-25-final-summary.md`）。
ai-svc 的 `POST /api/v1/tts/synthesize` 当前返回 503，**TTS 是项目最大的功能缺口**。

---

## 二、决策（Decision）

采用 **阿里云智能语音交互（短文本语音合成 RESTful API）** 作为主力 Provider，
**OpenAI TTS** 作为境外/兜底 Provider。其他厂商（火山 / ElevenLabs）暂不接入，
但保留接口位便于日后扩展。

| 角色 | Provider | 触发条件 |
|------|----------|---------|
| **Primary** | 阿里云 | 国内用户 / 默认 |
| **Fallback** | OpenAI TTS | 阿里云 5xx / 境外客户端 IP |
| **Future**（预留） | 火山引擎 / ElevenLabs | 用户显式选择 / 英文流式场景 |

---

## 三、理由（Rationale）

### 3.1 为什么主力选阿里云

| 维度 | 评估 |
|------|------|
| **场景契合** | 中文心理陪伴 + 情绪聊天，情感音色 `zhitian_emo`（智甜·情感女声）天然贴合 |
| **音色数量** | 60+ 中文音色，远多于 OpenAI 的 6 个 |
| **价格** | ~2 元/万字符，月活 1k 用户约 20 元/月 |
| **境内稳定** | 无需科学上网，备案合规 |
| **流式支持** | 支持 WebSocket 流式 PCM，便于未来语音聊扩展 |

### 3.2 为什么 fallback 选 OpenAI

| 维度 | 评估 |
|------|------|
| **鉴权最简** | 单 Bearer Token，无需 AK/SK 换 token 流程 |
| **境外可用** | 阿里云对境外用户体验差，OpenAI 可作补充 |
| **响应快** | 24kHz 直接返回二进制，零解析 |
| **已有 key** | 项目方若已持 OpenAI key，零额外成本 |

### 3.3 为什么暂不接火山 / ElevenLabs

| 厂商 | 不接的原因 |
|------|-----------|
| 火山引擎 | 中文音色略逊于阿里云；Cluster 鉴权多一层复杂度；等用完阿里云免费额度再评估 |
| ElevenLabs | 月成本是阿里云 7-10 倍；中文口音一般；英文流式场景目前不在 roadmap |

---

## 四、约束（Constraints）

### 4.1 接口契约保持兼容

```go
// 现有签名必须保持不变（analyzer 已调用）
func (c *XTTSClient) Synthesize(ctx context.Context, text string) ([]byte, int, error)
func (c *XTTSClient) Health(ctx context.Context) error
```

### 4.2 鉴权方式：阿里云启动换 token + 24h 缓存

- 使用 `sync.Once` + 过期时间检查
- Token 缓存到 `XTTSClient` 字段，避免每次合成都换 token
- token URL: `https://nls-meta-cn-shanghai.aliyuncs.com/token`
- 24h 过期前 1h 主动刷新

### 4.3 必须遵守 AGENTS.md TDD

- **先写测试** → 看红 → 写最小实现 → 看绿 → 重构
- 所有 4 个 provider 实现都要有对应测试（哪怕暂未启用，也要 RED→GREEN 一遍走）
- 4 个 provider 共 12+ 测试用例（每 provider 3 个：success / no-config / upstream-error）

---

## 五、待改造文件清单（落地清单）

按 `xtts-cloud-api-integration.md` 第十二章执行，本次只规划、不动代码。

| # | 文件 | 改造 |
|---|------|------|
| 1 | `emotion-echo-ai-svc/internal/aiclient/xtts.go` | 多 provider 路由 + 4 个 `synthesizeXxx` |
| 2 | `emotion-echo-ai-svc/internal/aiclient/xtts_cloud_test.go` | **新建**，12+ 测试 |
| 3 | `emotion-echo-ai-svc/internal/config/config.go` | 加 `TTS.Provider` + 4 个子 config |
| 4 | `emotion-echo-ai-svc/etc/ai-api.yaml` | 默认 `provider: aliyun` |
| 5 | `deploy/docker-compose.apps.yml` | 删 `emotion-echo-xtts` service + `XTTS_BASE_URL` env |
| 6 | `deploy/docker-compose.apps.yml` | 加 `ALIYUN_APPKEY` / `ALIYUN_ACCESS_KEY_ID/SECRET` env |
| 7 | `scripts/verify_stage23_endpoints.py` | 区分 cloud provider + on-prem |
| 8 | `emotion-echo-ai-svc/internal/aiclient/xtts.go` `Health()` | 探活改云端 endpoint（GET /models） |

---

## 六、配置样例（落地时直接套用）

```yaml
# emotion-echo-ai-svc/etc/ai-api.yaml
TTS:
  Provider: aliyun         # aliyun / openai / volcano / elevenlabs
  Timeout: 60
  Language: zh-cn
  Speed: 0.0

  Aliyun:
    AppKey:          ${ALIYUN_APPKEY}
    AccessKeyID:     ${ALIYUN_ACCESS_KEY_ID}
    AccessKeySecret: ${ALIYUN_ACCESS_KEY_SECRET}
    Voice:           zhitian_emo   # 智甜·情感女声

  OpenAI:
    ApiKey:  ${OPENAI_API_KEY}
    BaseURL: https://api.openai.com/v1
    Voice:   alloy

  Volcano:           # 暂不启用，留位
    AppID: ""
    Token: ""
    Voice: ""

  ElevenLabs:        # 暂不启用，留位
    ApiKey: ""
    Voice: ""
```

`.env.local`（gitignored）：

```bash
ALIYUN_APPKEY=your-appkey
ALIYUN_ACCESS_KEY_ID=your-ak-id
ALIYUN_ACCESS_KEY_SECRET=your-ak-secret
OPENAI_API_KEY=sk-...    # 兜底用，可选
```

---

## 七、验收标准（Definition of Done）

- [ ] `go test ./emotion-echo-ai-svc/...` 全绿（包含 12+ 新测试）
- [ ] `go vet ./emotion-echo-ai-svc/...` 无 warning
- [ ] `python scripts/verify_stage23_endpoints.py --ai-svc http://localhost:8891`
      输出 `4/4 [AI profile LIVE]`
- [ ] `GET /api/v1/ai/health` 返回的 `xtts.enabled=true, xtts.healthy=true`
- [ ] `POST /api/v1/tts/synthesize` 返回 base64 WAV，`sampleRate=16000`
- [ ] 删 `emotion-echo-xtts` 容器后，`docker stats` 内存释放 ~2GB
- [ ] `.env.local` 不进 git（`.gitignore` 已包含）
- [ ] commit message: `feat(ai): XTTS → cloud TTS multi-provider (primary=aliyun, fallback=openai)`

---

## 八、风险与回滚

### 8.1 风险

| 风险 | 等级 | 应对 |
|------|------|------|
| 阿里云服务故障 | 中 | Fallback 自动切 OpenAI；前端 503 概率 < 0.1% |
| 阿里云 token 过期 | 低 | sync.Once + 主动刷新，无需人工 |
| 境外客户端延迟 | 中 | IP 探测切 OpenAI（前端或 ai-svc 路由层） |
| 月成本超预期 | 低 | 月活 1k 仅 20 元，月活 1 万 200 元 |

### 8.2 回滚方案

1. **快速回滚**：把 `Provider: aliyun` 改回 `BaseURL: http://emotion-echo-xtts:8003`（保留旧字段 2 周）
2. **临时降级**：前端 `TTS 按钮` 灰度 0%，让用户走文字聊天
3. **重 build XTTS 镜像**：见 `stage-25-final-handoff.md` 的重 build 命令（仅极端情况下用）

---

## 九、触发条件（什么时候真正动手）

用户说「开干」「开始实现」「按文档落地」时，按以下顺序：

1. 新建分支 `feat/ai-tts-cloud`
2. 按本文件第五节「待改造文件清单」+ 第七节「验收标准」执行
3. 严格 TDD：先 12+ 测试 → 红 → 最小实现 → 绿 → 重构
4. 完成后 `git commit` + push + 触发 verify 脚本

**当前状态**：决策已确认，代码未动，等用户指令。

---

## 十、关联文档索引

| 文档 | 用途 |
|------|------|
| `stage-25-final-landing.md` | Stage 25 完整收尾报告 |
| `stage-25-final-summary.md` | XTTS 失败原因详情 |
| `xtts-cloud-api-integration.md` | 实施指南（代码骨架） |
| `xtts-cloud-api-decision.md` | **本文档** · 决策记录 |
| `AGENTS.md` | TDD 强约束（必读） |
| `microservices-architecture.md` | ai-svc 在整体架构中的位置 |

---

> 最后更新：2026-07-17 · 决策已确认 · 代码待落地