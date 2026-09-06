# MinIO 对象存储 (Sprint 1 PR-4a)

## 用途

PR-4 落地的对象存储后端。`emotion-echo-web-bff` 的 `avatar_handler`（PR-4c）
写用户头像到这里的 `avatars` bucket；后续文件上传也走这里。

S3 兼容 API（minio-go / aws-sdk-go-v2 S3 client 都能用）。

## dev 启动

```bash
# 1. 启动 minio + init（自动建 avatars bucket）
cd deploy
docker compose -f docker-compose.infra.yml up -d emotion-echo-minio emotion-echo-minio-init

# 2. 验证（应 4/4 PASS）
cd ..
bash scripts/check_minio_health.sh
```

期望输出：
```
[OK] MinIO 闭环全 PASS
```

## 端口

| 端口 | 用途 |
|---|---|
| 9000 | S3 API（dev 暴露给宿主机；prod 通过容器名:9000 内网访问） |
| 9001 | Web 控制台 <http://localhost:9001>（dev 默认账号 `minioadmin` / `minioadmin`） |

## 镜像源选择

`quay.io/minio/minio:latest` + `quay.io/minio/mc:latest`——**不用 docker.io**：

- docker.io 偶发 401 / pull denied（参考 PR-0 重建时的限制速现象）
- quay.io 是 MinIO 官方第二镜像源，限速宽
- 与 emotion-echo-minio 容器大小一致

## dev 凭证

| 项 | 值 |
|---|---|
| `MINIO_ROOT_USER` | `minioadmin` |
| `MINIO_ROOT_PASSWORD` | `minioadmin` |

**prod 必须覆盖**——用 K8s Secret 注入 + Helm values 引用。
PR-4 只落地 dev 默认；prod K8s Secret 模板留作后续 Sprint。

## 文件清单

| 文件 | 说明 |
|---|---|
| `deploy/docker-compose.infra.yml` | `emotion-echo-minio` + `emotion-echo-minio-init` 服务定义 |
| `deploy/minio/README.md` | 本文档 |
| `scripts/check_minio_health.sh` | 4 契约健康检查（容器 running + liveness + console + avatars bucket） |

## 调研依据

- 决策 18 §四.6 破坏性脚本护栏：本 README 内的 `down -v` 等命令禁止直接列出
- Sprint 1 PR-4 plan §PR-4a
- MinIO S3 API: <https://min.io/docs/minio/linux/developers/go/API.html>

## 已知限制

- `emotion-echo-minio-init` 是一次性 init（跑完 Exited 0）—— bucket 已建就不会再跑
- dev 用单实例 single-drive；prod 用 distributed mode（4 节点 erasure coding）
- 数据卷 `minio-data` 在 `down -v` 时会被销毁（决策 18 §四.6：注意恢复）