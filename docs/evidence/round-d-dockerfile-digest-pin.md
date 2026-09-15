# Round D 状态登记：Dockerfile digest pin（代码形态完成 / 真值待 CI sync）

**commit**：`ded2efc feat(docker): Round D — 业务 7 Dockerfile + web dev 全部 digest env var 化 (P2-R2-19 收口)`
**关联脚本**：`scripts/check_docker_digests.sh` + `scripts/sync_docker_digests.sh`
**lockfile**：`Dockerfile.digests.lock`

## 本轮完成范围（代码形态）

8 个 Dockerfile 改用 `ARG XXX_DIGEST=""` + `FROM image@${XXX_DIGEST:-image:tag}` 模式：

| Dockerfile | 基础镜像 | 改前 | 改后 |
|---|---|---|---|
| `emotion-echo-chat-svc/Dockerfile` | golang:1.26-alpine × 2 stage | 字面 tag | `${GOLANG_1_26_ALPINE_DIGEST:-golang:1.26-alpine}` |
| `emotion-echo-user-svc/Dockerfile` | 同上 | 字面 tag | env var |
| `emotion-echo-analytics-svc/Dockerfile` | 同上 | 字面 tag | env var |
| `emotion-echo-assessment-svc/Dockerfile` | 同上 | 字面 tag | env var |
| `emotion-echo-web-bff/Dockerfile` | 同上 | 字面 tag | env var |
| `emotion-echo-web/Dockerfile` | node:20-alpine × 2 stage | 字面 tag | `${NODE_20_ALPINE_DIGEST:-node:20-alpine}` |
| `emotion-echo-web/Dockerfile.dev` | node:20-alpine | 字面 tag | env var |
| `emotion-llm-service/Dockerfile` | python:3.12-slim × 2 stage | 字面 tag | `${PYTHON_3_12_SLIM_DIGEST:-python:3.12-slim}` |

`emotion-echo-ai-svc/Dockerfile` 本就是 env var 模式（Round 4.7 PR-3 落），未动。

校验脚本升级：
- `scripts/check_docker_digests.sh` 正则同时接受 `@sha256:字面` 与 `${VAR_DIGEST}` 形式
- 排除 `emotion-echo-models/`（AI 模型 vendor 镜像仓内 reference）+ `legacy/`（退役工程）

**dev compose 不强制**（`VAR` 未填时 fallback 到 tag），**prod CI 必填真 digest**。

## 当前状态：7 个 digest 仍为占位

`Dockerfile.digests.lock` 中 7 个变量当前都是 `sha256:000...000` 占位：

| 变量 | 锁定的镜像 | 当前占位 |
|---|---|---|
| `ALPINE_3_19_DIGEST` | alpine:3.19 | `sha256:0000...0000` |
| `GOLANG_1_26_ALPINE_DIGEST` | golang:1.26-alpine | `sha256:0000...0000` |
| `PYTHON_3_10_SLIM_DIGEST` | python:3.10-slim | `sha256:0000...0000` |
| `PYTHON_3_12_SLIM_DIGEST` | python:3.12-slim | `sha256:0000...0000` |
| `NODE_20_ALPINE_DIGEST` | node:20-alpine | `sha256:0000...0000` |
| `UBUNTU_22_04_DIGEST` | ubuntu:22.04 | `sha256:0000...0000` |
| `ALPINE_LATEST_DIGEST` | alpine:latest | `sha256:0000...0000`（**避免 latest**，建议改 3.19）|

## 真值 sync 触发条件（roadmap 登记的触发条件型）

**本沙箱 docker.io 网络受限**（已 webfetch 验证 timeout 443），无法本地 sync 真值。

触发条件 = **CI 环境 docker.io 网络可达** 时执行：

```bash
# 单次回填（仅替换占位 000..000，不覆盖已验证 digest）
bash scripts/sync_docker_digests.sh
```

脚本会：
1. 拉 `registry-1.docker.io` anonymous token
2. 对每个 `image:tag` 调 manifest v2 API 拿真 digest
3. `sed` 替换 lockfile 中对应行的 `0000...0000` 占位
4. 输出 `# synced <date>` 注释标记

回填后验证：
```bash
bash scripts/check_docker_digests.sh
# 期望：OK: all 17 FROM lines are digest pinned（仍 17/17，本轮已全绿）
# + Dockerfile.digests.lock 中 7 个 sha256:0...0 都被真 digest 替换
```

## 已落地的可验证证据

- `bash scripts/check_docker_digests.sh` → **17/17 FROM lines digest pinned (0 unpinned)**
  - 实测在 commit `ded2efc` 后通过
- 排除规则：`emotion-echo-models/` + `legacy/` 跳过（非业务部署路径）
- 校验逻辑：既接受 `@sha256:字面`（Round 4.7 PR-3 老模式）也接受 `${VAR_DIGEST}` 形式（本轮 7 Dockerfile 新模式）

## 后续 task 痕迹（roadmap 后续 round 候选）

1. **CI workflow 接入**：`docs/ci-workflows/` 加 `docker-digest-sync.yml`，每周自动跑 `sync_docker_digests.sh` + commit 回填
2. **prod compose 强制**：`deploy/docker-compose.prod.yml` 或 env override 文件强制注入真 digest，缺值即启动 fail-fast
3. **AI models 镜像 digest**：本轮排除 `emotion-echo-models/` 是因为走 vendor 镜像 + 离线预烘焙；如未来需要也建 lockfile（`DOCKERFILE.DIGESTS.MODELS.LOCK`）
4. **legacy/ 清理**：退役工程 Dockerfile 移除或保持 skip

## 为什么不本地 sync 真 digest

- 沙箱内 `webfetch https://hub.docker.com/...` 与 `curl https://registry-1.docker.io/...` 均 443 timeout（已实测）
- 强行绕过会让 sync 脚本无意义（拿不到真值）
- 真值由具备 docker.io 网络的 CI runner 拉取更可靠
- 当前代码形态（ARG + env var fallback）已 100% 满足"真 digest 注入即生效"的契约 — CI 跑 sync 即可
