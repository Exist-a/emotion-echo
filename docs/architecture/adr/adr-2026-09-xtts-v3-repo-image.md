# ADR-2026-09 XTTS 镜像源切换 v3 · 仓内 Dockerfile build 替代 vendor `ai4all/coqui`

> **状态**：✅ **Accepted**（2026-09-23，E2E-17 计划期调研触发）
> **取代关系**：v2 `adr-2026-09-xtts-v2-decision.md` 中"vendor 镜像唯一方案" 的隐含前提被实测推翻（vendor 缺端点），但不改变 v2 的核心立场（云 API 路径不复活）
> **关联**：ADR-2026-09 XTTS v2 · [plan.md](../../e2e-roadmap/stages/e2e-17-digital-human-tts/plan.md) §2.D 决策 13

---

## 一、v2 隐含前提的失真（实测推翻）

v2 决策（2026-09-15）的隐含前提：**`ai4all/coqui:latest` 镜像内部有 TTS 端点**，可直接服务 `/tts_stream`。

**实测 2026-09-23**（E2E-17 计划期调研，AGENTS.md §〇.6 文档前必做）：

```bash
# 抽出 vendor 镜像的 /app/app.py
cid=$(docker create ai4all/coqui:latest)
docker cp "$cid:/app/app.py" /tmp/coqui-app.py
docker rm "$cid"

grep -n "tts" /tmp/coqui-app.py
# 零命中

grep -nE "@app\.(post|get)" /tmp/coqui-app.py
# POST /voice/upload   ← multipart 上传
# POST /voice/generate ← 单次生成（写文件 /model/output.wav）
# GET  /voice/result   ← 取文件
# 没有任何 streaming TTS 端点
```

**v2 与现状完全矛盾**：BFF `xtts.go:75` `POST /tts_stream` → vendor 必返 404。`/api/v1/tts/stream` 实际链路已坏（无论 vendor 在不在 — vendor 无端点 → 404；vendor 不在 → 502）。历史 dev "TTS works" 应为误会。

**失真类型**（决策 18 §三）：类型 1 "接口未实现" + 类型 6 "plan 期未实测就声明能力"。

---

## 二、v3 决策

### §A. XTTS 推理路径 = **仓内镜像 `emotion-echo/xtts:v2.0.0`**（唯一方案）

| 路径 | 角色 | 触发条件 | 当前实现 |
|------|------|---------|---------|
| **唯一** | 仓内镜像 `emotion-echo/xtts:v2.0.0`（build 自 `emotion-echo-models/XTTS/Dockerfile`） | 所有环境 | 2026-09-23 build 完成；`deploy/docker-compose.apps.yml:537` `image: emotion-echo/xtts:v2.0.0` |

**build 命令**（2026-09-23 实测 ~10 分钟 cold pull + pip install + model copy）：

```bash
docker build -f emotion-echo-models/XTTS/Dockerfile \
  -t emotion-echo/xtts:v2.0.0 emotion-echo-models/
```

**compose 切换**（2026-09-23 本 ADR 落地）：

```yaml
emotion-echo-xtts:
  image: emotion-echo/xtts:v2.0.0         # 替代 ai4all/coqui:latest
  container_name: emotion-echo-xtts
  restart: on-failure                       # 替代 unless-stopped（避免 exit=0 也重启）
  memory: 6144M                             # 替代 3072M（仓 XTTS 模型 ~2GB + torch 峰值 ~3.5GB）
  profiles: ["ai"]
  # 不覆盖 command —— Dockerfile CMD 已包含 python server.py --host=0.0.0.0 --port=8003 --device cpu
```

### §B. 仓内镜像端点（实测 200 OK）

| 端点 | 方法 | 用途 | 实测 |
|------|------|------|------|
| `/health` | GET | 健康检查 | `200 {"status":"ok","model_loaded":true,"model_type":"XTTS-v2"}` |
| `/tts_stream` | POST | 流式 WAV | `200 audio/wav`（stream） |
| `/tts_with_phonemes` | POST | 带字符级时间戳的非流式 WAV | `200` + 字段 `audio(base64)/sample_rate/text/language/phonemes[{char,start_秒,duration_秒}]/duration` |
| `/tts` | POST | 非流式 WAV | `200`（未实测，按源码） |

### §C. phoneme 字段语义（**口径限制 — 必须报告**）

仓 `server.py:319-326` 的 phoneme 是 **per-char 等分近似**：

```python
phonemes = [{
    "char": c,
    "start": round(i * char_duration, 3),         # 秒
    "duration": round(char_duration, 3),          # 秒 = total_duration / len(chars)
} for i, c in enumerate(chars)]
```

**实测**（`text="你好世界"`, 4 chars, duration=1.291s）：

```
{'char':'你', 'start':0.000, 'duration':0.323}
{'char':'好', 'start':0.323, 'duration':0.323}
{'char':'世', 'start':0.645, 'duration':0.323}
{'char':'界', 'start':0.968, 'duration':0.323}
last.start + last.duration = 0.968 + 0.323 = 1.291 = duration ✓
```

⚠️ **不是真 XTTS 字符级时间戳推理**（per-char 等分 = 平均时长），仅满足 D-03 "对齐"但不等于"真字符读音时机"。**E2E-17 report.md 必须在测试点 #11/#12 明确此口径**。

### §D. 容量 / 资源约束

- 镜像：11.3 GB（ai4all/coqui 同等大小）
- 磁盘：构建期峰值 +12 GB（已扣 130 GB 压缩后 183 GB 可用，OK）
- 内存：6144M container limit（host WSL 8 GB，OK）
- 模型加载：~30 秒（CPU device）

### §E. 触发回退条件（什么时候需要换镜像）

| 触发条件 | 应对 |
|---------|------|
| 仓 Dockerfile build 失败 | 恢复 v2 vendor（404 链路，至少前端的"按钮"在；或决定彻底下线 TTS 端点） |
| 内存 6144M 不够（实测需 >6GB） | 升 host WSL 到 12GB 或切 GPU device |
| 仓 image 与 vendor 音色对比差异过大 | 留作 v4 评估（v3 仅采纳仓 image，不与 vendor 比音色） |
| 真字符级时间戳需求出现 | 升级仓 server.py 用 XTTS attention alignment（不在本决策范围） |

---

## 三、调研依据（AGENTS.md §〇.6）

### 已读代码 / 实证

| 文件 | 关键发现 |
|------|---------|
| `docker cp ai4all/coqui:latest:/app/app.py` 抽出 | vendor 端点只 `/voice/{upload,generate,result}`（multipart + 单次生成 + GET 取文件），**根本不是 streaming TTS** |
| `emotion-echo-models/XTTS/server.py:265-281` `/tts_stream` | 真流式（`StreamingResponse(stream_audio_generator)`） |
| `emotion-echo-models/XTTS/server.py:283-341` `/tts_with_phonemes` | 非流式，per-char 等分 phoneme 数组 |
| `emotion-echo-web-bff/internal/downstream/xtts.go:75` | `POST /tts_stream` —— 与 vendor 不兼容 |
| `deploy/docker-compose.apps.yml:536-575` | 镜像源 + healthcheck + memory + restart policy 本 ADR 全改 |

### 实施链

1. **build**（✅ 2026-09-23 ~10 min, 镜像 11.3 GB）
2. **compose 切换**（✅ 已落，PR 含 commit）
3. **重启 + health 验证**（✅ healthy ~40 秒，model_loaded=true）
4. **契约钉住**（✅ /health + /tts_with_phonemes 实测 200，schema 与 §B 一致）

### 关联 ADR

- [adr-2026-09-xtts-v2-decision.md](adr-2026-09-xtts-v2-decision.md) — v2 立场（云 API 路径不复活）继续生效，仅替换实现路径
- [adr-2026-09-sensevoice-runtime-constraints.md](adr-2026-09-sensevoice-runtime-constraints.md) — 同类多模态镜像构建与内存约束经验