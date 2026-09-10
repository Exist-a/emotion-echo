# ADR-001 v2 · XTTS 推理路径最终结论（2026-09-10）

> **状态**：✅ **Accepted**（重审结论，替代 v1）
> **v1 状态**：❌ **Retired**（决策与现状长期失真 4 个月）
> **关联**：原 ADR-001 `docs/ai-models/xtts-decision.md`（2026-07-17）·
> [`stage-60-pr-tts-vendor-landing.md`](../../stages/stage-60-pr-tts-vendor-landing.md) ·
> [`stage-60-1-pr-tts-vendor-sv-landing.md`](../../stages/stage-60-1-pr-tts-vendor-sv-landing.md) ·
> [`stage-59-vendor-image-verification.md`](../../stages/stage-59-vendor-image-verification.md) ·
> [`stage-58-ai-image-build-blocked.md`](../../stages/stage-58-ai-image-build-blocked.md) ·
> 决策 18 §二 #12（失真登记）

---

## 一、v1 失真说明（决策 18 §4.4 就地更正）

**v1 决策**（2026-07-17）：阿里云智能语音 RESTful API 为 Primary，OpenAI TTS 为 Fallback，**删除本地 `emotion-echo-xtts` 容器**。

**现状实测**（2026-09-10）：
1. 阿里云/OpenAI 接入**从未落地**——`emotion-echo-ai-svc/internal/aiclient/xtts.go` 仍是本地 HTTP 客户端实现（151 行完整代码，`NewXTTSClient` 在 `BaseURL != ""` 时构造完整客户端），**从未写过云 API 适配层**。
2. 本地 `emotion-echo-xtts` 容器**未被删除**——`deploy/docker-compose.apps.yml:549` `image: ai4all/coqui:latest` 仍在 compose 定义中（含 healthcheck + deploy resources）。
3. 端到端跑通的是**第三条路径**：**vendor `ai4all/coqui:latest` Coqui TTS Docker Hub 官方镜像**（Stage 60 PR-TTS-VENDOR），端到端实测 133KB WAV RIFF 头正确（`stage-60-pr-tts-vendor-landing.md` §五）。

**v1 与现状完全矛盾**——决策写"删除本地容器走阿里云"，代码实际"保留本地容器走 vendor Coqui"。

**失真类型**（决策 18 §三）：类型 2 "陈旧结论"——决策文档 4 个月未跟随代码更新；类型 5 "自报告失真"——本会话作者在 `todo-pile-2026-09-04.md §A1` 也曾误判"XTTS 容器已被删除"。

---

## 二、v2 决策（最终）

### §A. 推理路径 = **本地容器 + vendor 双轨，dev 默认本地容器**

| 路径 | 角色 | 触发条件 | 当前实现 |
|------|------|---------|---------|
| **Primary** | 本地容器 `emotion-echo-xtts`（vendor `ai4all/coqui:latest`）| dev 默认 + 有 docker 环境的用户 | Stage 60 PR-TTS-VENDOR 落地；`deploy/docker-compose.apps.yml:549` |
| **Fallback** | 云 API（阿里云 + OpenAI TTS）| 1) 容器镜像不可拉 / build 失败；2) prod 大规模部署成本优于自建 | v1 决策已写代码骨架（`xtts-cloud-api-integration.md`），但**未实现** |
| **Future**（预留）| 自建 XTTS 镜像（`emotion-echo-models/XTTS/Dockerfile`）| 网络恢复 + 维护团队到位 | 仓库内代码保留作 learning asset |

### §B. 为什么不继续走 v1（云 API 优先）

| v1 决策依据 | 实测结果 | v2 取舍 |
|---|---|---|
| 本地 XTTS build 4 次失败 | Stage 36 v0.1.0 build 成功过（`stage-36-landing.md:268-271`） | **build 可行性被重新评估**：失败是镜像拉取问题，不是代码本身 |
| 阿里云音色更适合情感陪伴 | 阿里云 API 从未落地，无对比数据 | vendor Coqui 音色**当前可用**（实测 133KB WAV）；云 API 对比留作未来 |
| 阿里云境内稳定 + 合规 | 同上 | 当前 dev 范围内本地容器即满足"境内"需求 |
| 月成本 20 元 | 与自建镜像维护成本（Stage 58 ~140 秒构建 + 11.3GB 磁盘）对比 | **短期 dev 用本地**，prod 规模化再评估云 |

### §C. 决策触发条件（什么时候切到 v1 云 API 路径）

满足以下**任一**才评估切云：

1. **prod 部署成本压力**：单实例 GPU/磁盘不够支撑 XTTS 推理，需要外部算力
2. **合规/审计要求**：必须用境内备案服务（如阿里云）而不能用 Docker Hub vendor
3. **vendor 镜像不可拉**：`ai4all/coqui:latest` 在阿里云 ACR 等镜像不可达，且本地 build 再次失败
4. **多语种扩展**：阿里云 60+ 中文音色 vs Coqui 默认音色的实际体验差距被用户明确反馈

### §D. 双轨架构不变项

- **接口契约**（`emotion-echo-ai-svc/internal/aiclient/xtts.go`）：
  - `Synthesize(ctx, text) → ([]byte, int, error)`
  - `Health(ctx) → error`
  - `NewXTTSClient(baseURL) → *XTTSClient | nil`（BaseURL 空时构造降级）
- **环境变量**：`XTTS_BASE_URL=http://emotion-echo-xtts:8003`（compose 注入）
- **profile 语义**：`profiles: ["ai"]` —— 用户需显式 `--profile ai up` 启用（保留显式开关，不做 dev 默认启用——决策待 owner 确认）

---

## 三、调研依据（AGENTS.md §〇.6 写文档前必做）

### 已读代码

| 文件 | 关键发现 |
|---|---|
| `emotion-echo-ai-svc/internal/aiclient/xtts.go:46-62` | `NewXTTSClient` 在 `BaseURL != ""` 构造完整客户端；空则 `nil`（构造时降级） |
| `emotion-echo-ai-svc/main.go:86-92` `applyEnvOverrides` | `XTTS_BASE_URL=http://emotion-echo-xtts:8003`（compose 注入） |
| `emotion-echo-ai-svc/etc/ai-api.yaml:69-81` | FER/SenseVoice/XTTS `BaseURL` 默认空（dev 默认仅文本情绪） |
| `deploy/docker-compose.apps.yml:549-580` | `emotion-echo-xtts` 服务定义仍在；image = `ai4all/coqui:latest`；healthcheck 改 socket probe（vendor 无 /health） |
| `deploy/docker-compose.apps.yml:387-389` `XTTS_BASE_URL` env 注入 | compose 中显式覆盖 yaml 默认空值 |

### 已查 ADR / stage

| 文档 | 关联点 |
|---|---|
| `docs/ai-models/xtts-decision.md`（v1，2026-07-17）| 决策与现状矛盾的源头；本 v2 直接 retire |
| `stage-58-ai-image-build-blocked.md §四` | "方案 A 换 pre-built 镜像"实际被 PR-TTS-VENDOR 走通 |
| `stage-59-vendor-image-verification.md` | vendor 候选实测：`ai4all/coqui` ✅ / `yiminger/sensevoice` ❌ / `serengil/deepface` ⚠ |
| `stage-60-pr-tts-vendor-landing.md` | 三 AI 模型落地总览 |
| `stage-60-1-pr-tts-vendor-sv-landing.md` | SV-fastbuild 真实三语推理证据 |
| `decisions.md` | 决策 18（文档失真治理）；本 ADR 编号沿用 ADR-001 槽位 |

### 已跑验证

| 验证 | 结果 |
|---|---|
| 端到端 TTS（upload voice + generate + result）| ✅ 133KB WAV RIFF 头正确（stage-60 §五）|
| FER `/analyze` 200 | ✅ 49/49 单测（emotion-echo-models/FER-tflite/tests/unit/）|
| SenseVoice 三语推理 | ✅ zh/en/ja 200（stage-60-1 §五）|
| `docker compose --profile ai up` 三容器 healthy | ✅ XTTS socket probe；FER healthcheck；SenseVoice start_period=90s |

---

## 四、落地实施（本 ADR 已事实落地）

### 4.1 Stage 60 PR-TTS-VENDOR 实际代码改动（v2 决策的具体实现）

| Commit | 内容 |
|---|---|
| `817ceee` | `emotion-echo-xtts` 改用 `ai4all/coqui:latest`；覆盖 command 8003；healthcheck socket probe |
| `011c0b0` | FER-tflite 538MB 替换原 tensorflow 572MB 后端 |
| `183cf66` | `emotion-echo-fer` + `emotion-echo-xtts` 切阿里云 ACR 镜像 |
| `9ff3335` | revert：XTTS 仍拉 docker.io（vendor 不可达时兜底） |
| `8e418a6` | SV-fastbuild 拆分 base + app Dockerfile |
| `7979f17` | `stage-60-pr-tts-vendor-landing.md` 收口 |
| `90c0820` | `stage-60-1-pr-tts-vendor-sv-landing.md` 收口 |

### 4.2 决策 9 vs 决策 11/12 字面冲突的同步修复

本 ADR 触及决策栈"产品路径"层面，与 `decisions.md` 决策 9（web-bff 入口）/决策 11/12（APISIX 网关层）**无字面冲突**——v2 只限定 XTTS 推理路径。**但同时**本会话修复了"前端 fallback 字面值 bug"（决策 18 #24），需要把决策 9 vs 11/12 字面冲突正式收口——见 PR-C。

---

## 五、未做项（不在 v2 范围）

- **本地 vs vendor 音色对比评测**：vendor `ai4all/coqui` 默认音色与阿里云 `zhitian_emo` 无用户主观评测数据
- **云 API 多 provider 实现层**：v1 写过的代码骨架（`xtts-cloud-api-integration.md`）保留作参考，未来 §C 触发条件成熟时按 TDD 重启
- **dev 默认启用 AI profile**：Stage 60 §五.1 建议保留显式 `--profile ai`，决策待 owner 确认（与 PR-TTS-VENDOR 同步）
- **XTTS 模型挂载路径跨平台**：当前 `${XTTS_MODEL_PATH:-/c/Users/LENVOV/AppData/Local/Temp/coqui_model}` 是 Git Bash 风格；Mac/Linux 用户需 export（cf. QUICKSTART.md 跨平台说明）

---

## 六、ADR-001 v1 → v2 变更记录

| 日期 | 决策 | 旧 → 新 | 原因 |
|---|---|---|---|
| 2026-07-17 | 推理路径 | （未定）→ **阿里云 API + OpenAI fallback，删除本地容器**（v1）| Stage 25 反复 build 失败的临时判断 |
| 2026-09-10 | 推理路径 | v1 → **本地 vendor 容器为主，云 API 作 fallback**（v2）| Stage 60 PR-TTS-VENDOR 验证 vendor Coqui 端到端跑通 + 4 个月决策与代码长期失真 |

**v1 retire 但不删除**：`docs/ai-models/xtts-decision.md` 保留作为历史决策记录，未来若 §C 触发条件成熟可参考 v1 的代码骨架与配置样例。

---

> 最后更新：2026-09-10 by PR-B（本会话）
> 用途：正式 retire ADR-001 v1，记录 v2 最终结论（本地 vendor + 云 API 双轨）；登记决策 18 #12 失真类型 2+5 自报告复合成因
> 后续：等待 PR-C（决策 9 vs 11/12 字面冲突正式收口）落地后一并 commit