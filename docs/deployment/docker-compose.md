# Emotion-Echo 分布式基础设施

> 本目录由 Phase 0 / Stage 0.1 落地，集中拉起所有分布式中间件。

## 1. 目录结构

```
deploy/
├── docker-compose.infra.yml         # 一键起齐所有组件（6 个容器，已删除 Nacos）
├── apisix/
│   ├── conf.yaml                    # APISIX 核心配置（standalone）
│   └── apisix.yaml                  # APISIX 路由 / upstream 配置
└── env/
    └── .env.common                  # 公共版本与端口（文档参考）
```

## 2. 包含的组件（Stage 0.1）

| 组件 | 端口（宿主机） | URL / 用途 | 内部端口 |
|------|--------------|-----------|----------|
| Postgres | 5432 | 数据库 | 5432 |
| Redis | 6379 | 缓存 | 6379 |
| etcd | 2379 | APISIX 配置存储 | 2379 + 2380 (peer) |
| SkyWalking OAP | 11800 / 12800 | APM 后端 | 同 |
| SkyWalking UI | 18080 | APM 前端 | 8080 |
| APISIX | 9080 | API 网关入口 | 9080 |
| Kafka | 9092 | 消息队列 | 9092 |

> ⚠️ 这些端口必须未被占用。如果已启动 `Emotion-Echo-Gin/docker-compose.yml` 的相同容器（5432/6379），请先停掉，避免冲突。

## 3. 启动与停止

### 3.1 一键启动

```powershell
cd d:\源码\Emotion-Echo
docker-compose -f deploy/docker-compose.infra.yml up -d
```

> 启动后等待约 30~90 秒（SkyWalking OAP 较慢），各容器 healthcheck 通过即就绪。

### 3.2 查看容器状态

```powershell
docker-compose -f deploy/docker-compose.infra.yml ps
```

### 3.3 关闭所有组件

```powershell
docker-compose -f deploy/docker-compose.infra.yml down
```

> 如需彻底清理数据卷：`docker-compose -f deploy/docker-compose.infra.yml down -v`

## 4. 验证清单（Stage 0.1 完成后请勾选）

- [ ] **Postgres**：`docker exec -it emotion-echo-postgres psql -U postgres -d emotion_echo -c "SELECT 1;"`
- [ ] **Redis**：`docker exec -it emotion-echo-redis redis-cli ping` → 返回 `PONG`
- [ ] **etcd**：`docker exec -it emotion-echo-etcd etcdctl endpoint health`
- [ ] **SkyWalking**：浏览器访问 `http://localhost:18080`，看到空仪表盘
- [ ] **APISIX**：浏览器访问 `http://localhost:9080` → 默认 404（路由 /api/v1/* 不会匹配 GET /）
- [ ] **Kafka**：`docker exec -it emotion-echo-kafka kafka-topics.sh --bootstrap-server localhost:9092 --list`

## 5. 常见问题排查

### 5.1 容器起不来 / 反复重启

```powershell
docker-compose -f deploy/docker-compose.infra.yml logs <容器名>
```

常见原因：
- 端口被占用：`netstat -ano | findstr :8848`（Windows）
- 内存不足：SkyWalking / Kafka 至少需要 1GB 空闲内存

### 5.2 APISIX 上游连不上 Gin

- 确认宿主机上 Gin 是否真的在 8080 端口（`netstat -ano | findstr :8080`）
- Docker 容器访问宿主机要使用 `host.docker.internal`（已在 `apisix.yaml` 中写好）
- Windows Docker Desktop 默认开启此特性；如未开启需升级 Docker Desktop

### 5.3 SkyWalking UI 起不来

- OAP 启动慢，可能要等 1~2 分钟
- 查看 OAP 日志：`docker-compose -f deploy/docker-compose.infra.yml logs -f skywalking-oap`
- 看到 `bind on 0.0.0.0:11800 success` 才算启动成功

### 5.4 Kafka KRaft 启动失败

- 90% 是 `KAFKA_CFG_CONTROLLER_QUORUM_VOTERS` 配置问题
- 删除数据卷重置：`docker-compose ... down -v` 再 `up -d`

## 6. 下一步

Stage 0.1 完成 → [Stage 0.2: 验证所有控制台可访问 → Stage 0.3: Gin 接入 SkyWalking trace](../../docs/distributed-roadmap.md#stage-02--一键起齐-skywalking--apisix--kafka)
