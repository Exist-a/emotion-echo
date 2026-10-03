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
| 9000 | S3 API（**127.0.0.1 限定映射**——仅宿主本机；prod 通过容器名:9000 内网访问） |
| 9001 | Web 控制台 <http://localhost:9001>（**127.0.0.1 限定**；dev 默认账号 `minioadmin` / `minioadmin`） |

> **E2E-27 M2 / D-41（2026-10-03）**：两端口原为 `0.0.0.0` 绑定。配合桶级匿名
> download 策略，局域网任意机器可**绕过网关 JWT** 直读全部对象（含 voice 音频）。
> 现改为 `127.0.0.1` 限定（与 E2E-26 D-38 对 sw-ui 的处置同型）；宿主浏览器、
> 宿主 curl/脚本不受影响，容器网走 `emotion-echo-minio:9000`（expose，与本映射无关）。
> 结构守卫：`scripts/test_check_minio_health.sh` §5。

## 匿名读边界（M2 裁定留档）

- `avatars` 桶策略 = `download`（init `mc anonymous set download`，桶级）——
  `avatars/` `uploads/` `voice/` 三前缀**全部**无鉴权可 GET/HEAD（`mc anonymous get` 实测）
- **应用侧唯一入口是网关**：`/api/v1/*` 反代端点走 APISIX jwt-auth；直连 9000 仅本机可达（上节绑定）
- voice 音频**不在**独立桶（分桶方案为 M2 备选②，未采纳）——安全依赖 = 127.0.0.1 绑定 + key 含 uuid 不可枚举

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
| `scripts/test_check_minio_health.sh` | 上者的结构守卫 + M2 端口 127.0.0.1 限定断言（E2E-27 接入 CI） |

## 调研依据

- 决策 18 §四.6 破坏性脚本护栏：本 README 内的 `down -v` 等命令禁止直接列出
- Sprint 1 PR-4 plan §PR-4a
- MinIO S3 API: <https://min.io/docs/minio/linux/developers/go/API.html>

## 已知限制

- `emotion-echo-minio-init` 是一次性 init（跑完 Exited 0）—— bucket 已建就不会再跑
- dev 用单实例 single-drive；prod 用 distributed mode（4 节点 erasure coding）
- 数据卷 `minio-data` 在 `down -v` 时会被销毁（决策 18 §四.6：注意恢复）