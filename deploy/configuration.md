# Emotion-Echo 部署配置参考（ADR-20 落地版）

> **关联 ADR**：[`docs/architecture/adr/adr-2026-09-env-profile-strategy.md`](../docs/architecture/adr/adr-2026-09-env-profile-strategy.md)
> **落地版本**：Stage 58 PR-ENV-1~4
> **生效日期**：2026-09-09

本文档列出 Emotion-Echo 部署时**所有可配置的环境变量**、默认值、以及 dev/prod 环境差异。配合 ADR-20 的 C 方案（compose override 文件分层），按需覆盖。

---

## 1. 部署文件结构

```
deploy/
├── docker-compose.infra.yml      # 中间件（PG/Redis/Kafka/Nacos/SkyWalking/Loki/Promtail/Prometheus/Grafana/MinIO/etcd/APISIX）— dev/prod 共用
├── docker-compose.apps.yml       # 6 业务 svc + BFF + emotion-llm-service + 可选 AI profile — **中性基线**
├── compose.dev.yml               # dev override（本地开发，dev 假设集中地）
├── compose.prod.yml              # prod override（远端部署，**空壳占位** — 待远端真部署时填具体值）
└── configuration.md              # 本文件（env 变量清单 + dev/prod 差异表）
```

---

## 2. 启动命令

| 场景 | 命令 |
|---|---|
| **dev（本地开发）** | `docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml -f compose.dev.yml up -d` |
| **dev + AI profile** | 上条 + `--profile ai`（FER/SenseVoice/XTTS 镜像已构建时） |
| **prod（远端部署）** | `docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml -f compose.prod.yml up -d`（空壳 prod.yml 待补） |
| **基线（无 override）** | `docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml up -d`（取 apps.yml 中性默认） |

---

## 3. 环境变量清单（apps.yml 中性基线）

所有变量均用 `${VAR:-default}` 形式承载，默认值即"中性推荐"（dev 推荐值，prod 通过环境变量覆盖）。

### 3.1 命名空间 / 注册中心

| 变量 | 默认值 | 说明 |
|---|---|---|
| `NACOS_NAMESPACE` | `emotion-echo-dev` | Nacos 命名空间。prod 应改为 `prod` |
| `NACOS_ENABLED` | `true` | 是否启用 Nacos 注册/配置中心 |
| `NACOS_ADDR` | `emotion-echo-nacos:8848` | Nacos 地址（容器 DNS） |
| `NACOS_HOT_RELOAD` | `false` | Nacos 配置热更新（v2.3.5 SDK ↔ v2.4.3 server 有 bug，详见 todo-pile B2） |

### 3.2 业务开关

| 变量 | 默认值 | 说明 |
|---|---|---|
| `KAFKA_ENABLED` | `true` | chat-svc 是否发 Kafka 事件。dev 改 false 走 outbox mock 路径 |
| `SKYWALKING_ENABLED` | `true` | OAP trace 采集开关。prod 保持 true |
| `LOG_FORMAT` | `json` | 日志格式（json / text） |
| `LOG_LEVEL` | `INFO` | 日志级别（DEBUG/INFO/WARN/ERROR） |

### 3.3 BFF 专属

| 变量 | 默认值 | 说明 |
|---|---|---|
| `BFF_DEV_RETURN_CODE` | `1` | dev 验证码回显（forget-pwd e2e）。**prod 必须 0** |
| `BFF_TRUST_APISIX` | `true` | dev 默认信任 APISIX 注入的 X-User-Id；prod 由 APISIX 强制注入 |
| `BFF_LLM_API_KEY` | *(空)* | 真实 LLM key。**写 deploy/env/.env.local（git ignore），不要 commit** |
| `BFF_LLM_BASE_URL` | `https://api.deepseek.com` | LLM API base（DeepSeek/OpenAI 兼容） |
| `BFF_LLM_MODEL` | `deepseek-chat` | LLM 模型名 |

### 3.4 Web 前端专属

| 变量 | 默认值 | 说明 |
|---|---|---|
| `NUXT_PUBLIC_API_BASE_URL` | `http://localhost:19080/api/v1` | SPA API base URL。prod 应改为 `https://api.emotion-echo.example.com/api/v1` |
| `NUXT_PUBLIC_DISABLE_AUTH` | `false` | dev 调试可临时禁用 auth |

### 3.5 CORS 跨域（5 业务 svc 共有）

| 变量 | 默认值 | 说明 |
|---|---|---|
| `CORS_ALLOW_ORIGINS` | `http://localhost:3000` | dev 允许 Nuxt 本地端口。prod 应改为域名白名单 |

---

## 4. dev vs prod 关键差异（ADR-20 §三）

| 维度 | dev（默认） | prod（应改） |
|---|---|---|
| 启动命令 | `-f compose.dev.yml` | `-f compose.prod.yml`（待建） |
| BFF 直连端口（8894） | ✅ 暴露（dev 调试） | ❌ 不暴露（仅 APISIX 可达） |
| `BFF_DEV_RETURN_CODE` | `1`（验证码回显） | `0`（生产关闭） |
| `BFF_TRUST_APISIX` | `true`（信任直连调试） | `false`（APISIX 强制注入） |
| `KAFKA_ENABLED` | `false`（dev 简化） | `true`（生产必须） |
| AI profile（FER/SenseVoice/XTTS） | `--profile ai` 按需启 | 启用 |
| 日志级别 | `DEBUG` | `INFO` |
| `NACOS_NAMESPACE` | `emotion-echo-dev` | `prod` |
| `NUXT_PUBLIC_API_BASE_URL` | `http://localhost:19080` | `https://api.example.com` |
| `CORS_ALLOW_ORIGINS` | `http://localhost:3000` | `https://emotion-echo.example.com` |
| JWT secret | dev-test 占位 | 真随机（密钥管理系统生成） |
| TLS / HTTPS | HTTP | HTTPS（APISIX ssl / nginx 终结） |
| 数据卷 | docker 默认卷 | 命名卷 + 备份策略 |
| 资源限制（CPU/内存） | 未配置 | `deploy.resources.limits` 显式 |

---

## 5. dev override 文件（compose.dev.yml）当前覆盖项

```yaml
services:
  emotion-echo-web-bff:
    environment:
      BFF_DEV_RETURN_CODE: ${BFF_DEV_RETURN_CODE:-1}
      BFF_TRUST_APISIX: ${BFF_TRUST_APISIX:-true}
    ports: ["8894:8894"]              # dev 调试直连

  emotion-echo-web:
    environment:
      NUXT_PUBLIC_API_BASE_URL: http://localhost:19080/api/v1
    ports: ["3000:3000"]              # dev 看 UI

  emotion-echo-{chat,user,ai,analytics,assessment}-svc:
    environment:
      CORS_ALLOW_ORIGINS: "${CORS_ALLOW_ORIGINS:-http://localhost:3000}"
```

---

## 6. prod override 文件（compose.prod.yml）状态

⏸ **空壳占位**（Stage 58 PR-ENV-3）。

依据：ADR-20 §六 + 决策 18 §四（"结论须带可复现证据"）。远端真部署未启动前写具体 prod 假设等于制造未来文档失真种子。

远端真部署启动后，把 [§4 差异表](#4-dev-vs-prod-关键差异adr-20-三) 中"prod（应改）"列的实际值搬到 prod.yml 即可。

---

## 7. 验证方法

### 7.1 语法合法性

```bash
# dev 链路
cd deploy
docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml -f compose.dev.yml config

# prod 链路（空壳也能解析）
docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml -f compose.prod.yml config
```

### 7.2 单元测试覆盖

| 测试 | 范围 |
|---|---|
| `scripts/test_compose_override.sh` | compose.dev.yml 覆盖项 + 语法合法性 |
| `scripts/test_apps_yml_neutral.sh` | apps.yml 中性化（无硬编码） |
| `scripts/test_prod_yml_stub.sh` | compose.prod.yml 空壳 + 语法合法性 |

跑法：`bash scripts/test_*.sh`

### 7.3 端到端 smoke

合并前跑 §2.4 数据契约 smoke（详见 AGENTS.md）：

```bash
python scripts/smoke_data_layer.py    # 数据契约（5 项）
bash scripts/healthcheck_smoke.sh    # 健康检查
bash scripts/smoke_observability.sh   # OAP trace 检查
```

---

## 8. 未来工作（不在本批）

- [ ] **PR-TTS-1~4**：dev 默认启用 AI profile（FER/SenseVoice/XTTS 镜像构建 + profiles）
- [ ] **PR-TTS-3**：`KAFKA_ENABLED` 在 dev 模式应默认 `false`（chat-svc outbox mock 路径）—— 当前 apps.yml 默认 `true` 待修
- [ ] **PR-ENV-3 后续**：远端真部署启动后填 compose.prod.yml 具体值
- [ ] **B2**：Nacos ListenConfig 热更新修复（v2.3.5 SDK ↔ v2.4.3 server 不触发）

---

> 最后更新：2026-09-09 by Stage 58 PR-ENV-4
> 调研依据：ADR-20 / 决策 18 / apps.yml 行号实测 / TDD 测试 30/30 通过