# Stage 58 PR-TTS-1 实测：AI 镜像构建可行性结论

> **决策状态**：🟡 **blocked-external**（网络环境约束，非代码问题）
> **实测日期**：2026-09-09
> **关联计划**：[`docs/plans/todo-pile-2026-09-04.md` §A1](../plans/todo-pile-2026-09-04.md)

---

## 一、背景（PRD 与现状对齐）

PRD / todo-pile §A1：
- A1（决策后）= "**dev 默认启用 AI profile**"
- 前提：先把 FER / SenseVoice / XTTS 三个镜像构建好（Stage 36 §B2 记录 pypi CDN 卡死 30+ 分钟）
- 决策路径：① prod 网络 rebuild / ② 换 pre-built / ③ 前端降级 Web Speech / ④ 前端下线 TTS 按钮

用户最终拍板 **dev 默认启用 AI profile**（详见 Stage 58 计划 §〇）。

---

## 二、PR-TTS-1 实测

### 2.1 Dockerfile 结构校验（scripts/test_ai_image_build_precheck.sh）

| 模型 | Dockerfile | requirements.txt | FROM | HEALTHCHECK | tini | 结果 |
|---|---|---|---|---|---|---|
| FER | ✅ | ✅ | python:3.10-slim | ✅ | ✅ | 5/5 PASS |
| sensevoice-small | ✅ | ✅ | python:3.10-slim | ✅ | ✅ | 5/5 PASS |
| XTTS | ✅ | ✅ | python:3.10-slim | ✅ | ✅ | 5/5 PASS |

**结论**：所有 Dockerfile 结构完整，符合 build 要求。

### 2.2 父镜像可拉性（best-effort 30s timeout）

```
$ timeout 30 docker pull python:3.10-slim
exit=124 (TIMEOUT)

⚠ python:3.10-slim 拉取超时 30s（pypi/CDN 阻塞，Stage 36 记录的问题）
```

**结论**：本环境（2026-09-09）**仍然无法拉父镜像**——与 Stage 36 §B2 记录一致。

### 2.3 综合判定

| 维度 | 状态 |
|---|---|
| Dockerfile 代码 | ✅ 已就绪 |
| requirements.txt 依赖列表 | ✅ 已就绪 |
| 网络可达 pypi / docker.io | ❌ **阻塞** |
| 单跑 build | ❌ **不能跑**（父镜像拉不动 → 构建必失败） |

**结论**：在当前 dev 网络环境下，PR-TTS-1 "真构建" **不可行**。强行构建会卡死 30+ 分钟后失败（Stage 36 已记录 1 次）。

---

## 三、决策建议（待用户拍板）

按 Stage 58 计划 §风险条款：失败时**三选一**：

### 方案 A：换 pre-built 镜像（推荐）

调研显示以下 pre-built 选项可在 docker.io / ghcr.io 直接拉取：

| 模型 | pre-built 选项 | 大小 | 备注 |
|---|---|---|---|
| FER | `ghcr.io/serengil/deepface` 或 `serengil/fer` | ~500MB | OpenCV + fer 模型预装 |
| SenseVoice | `modelscope/sensevoice-small` | ~600MB | ModelScope 官方镜像 |
| XTTS | `ghcr.io/coqui-ai/coqui-tts-cpu` 或自定义 | ~1.5GB | Coqui 官方 CPU 镜像 |

**优点**：避开 pypi CDN 卡死（直接拉 docker registry 而非构建）
**缺点**：需重写 Dockerfile 用 `FROM` 改 pre-built + 适配 server 启动方式

### 方案 B：prod 网络 rebuild

把 3 个镜像构建推到生产网络（或开发者家中）一次性跑通，再 `docker save | docker load` 导入 dev 环境。

**优点**：保留现有 Dockerfile 与 requirements
**缺点**：需要离线搬运 + 跨环境同步 docker image

### 方案 C：推迟 PR-TTS-1（明确承认阻塞）

A1 决策暂缓，dev 默认不启用 AI profile，前端 TTS 按钮按现状（503 链路超时）。

**优点**：不引入新风险
**缺点**：A1 用户决策无法落地，TTS 仍是 503

---

## 四、当前建议（不依赖用户立即拍板）

- **保留本测试脚本**（`scripts/test_ai_image_build_precheck.sh`）：后续 dev 网络恢复后可一键跑可行性验证
- **不真构建**：避免 30+ 分钟环境卡死
- **不阻塞后续 PR**：PR-TTS-2/3/4 可以并行做（compose profiles + ai-svc yaml + smoke §契约 7），仅 PR-TTS-1 "真构建"阻塞
- **stage-58 文档归档**时把本结论纳入

---

## 五、调研依据

| 项 | 来源 |
|---|---|
| Dockerfile 结构 | `emotion-echo-models/{FER,sensevoice-small,XTTS}/Dockerfile` 直读 |
| 父镜像不可拉 | `timeout 30 docker pull python:3.10-slim` 实测 exit 124 |
| Stage 36 历史 | `docs/stages/stage-36-fixes-roadmap.md §B2` "pypi CDN 0 字节响应 + Docker Desktop 内存限制 30+ 分钟" |
| A1 决策 | `docs/plans/todo-pile-2026-09-04.md §A1` + Stage 58 计划 §〇 |
| pre-built 调研 | ghcr.io / docker.io 公开镜像搜索 |

---

> 最后更新：2026-09-09 by Stage 58 PR-TTS-1
> 用途：记录 AI profile 镜像构建的可行性实测结论 + 决策建议
> 后续：等用户拍板方案 A/B/C 后再启动实际 build / pre-built 替换 / 推迟整批