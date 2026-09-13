# Stage 88 — llm-service Python 端 Nacos 注册（v3 gRPC SDK）+ BFF 发现链第 6 处接入

> 日期：2026-09-13
> 类型：feat + fix（TDD：RED `1470c2e` → GREEN `2eab451` → BFF `e068c6c` → fix `33ed326`）
> 关联：roadmap open 第 2 项、[nacos-enablement-dev.md](../legacy-plans/landed/nacos-enablement-dev.md) residual、
> [llm-chat-real-pipeline.md](../legacy-plans/landed/llm-chat-real-pipeline.md) residual
> 背景：用户指认本项为"长任务"。调查证实：Stage 31 PR-10 写了 `nacos_client.py`
> 并随镜像发布，但用的是**旧同步 SDK 的导入路径**（`from nacos import NacosClient`），
> 而 requirements 锁 `>=3.1.0`（v3 的导入命名空间是 `v2.nacos`）——容器内 import
> 必然失败，启动按设计降级无 Nacos 运行，WARNING 从 Stage 31 起被吞到本 stage。

## 一、根因与方案

| 事实 | 证据 |
|---|---|
| `pip list` 显示 nacos-sdk-python 3.2.0 已装，`import nacos` 却 ModuleNotFoundError | 3.2.0 的 top_level 是 `v2`，无 `nacos` 模块 |
| 代码 API 是旧同步 SDK 面（add_naming_instance / add_config_watcher） | `nacos_client.py` 全文 |
| v3 SDK 是异步 gRPC 全家桶（register_instance/deregister/shutdown 均 async，redo 自动重注册） | 容器内 `inspect` 实测签名 |

**选型：移植 v3（不锁回旧同步 SDK）**——v3 与 Go SDK v2 同 gRPC 协议、连接保活 +
redo 自动重注册（从根上避开 HTTP 心跳时代"hosts: [] 被踢"问题类）；3.1+ 已修复
3.0.x 断线不重注册缺陷（requirements 锁定的本意就是 v3）；原生异步免去线程池包装。

**行为变化（有意）**：手工心跳 task 移除（liveness 委托 SDK gRPC 连接）；
`SVC_HOST=0.0.0.0`（compose 默认）时自动探测真实容器 IP——注册 0.0.0.0 会让
Discover 拿到不可拨号地址（Go 侧同坑曾在 Stage 62 PR-3.4 修过）。

## 二、落地内容

| 层 | 内容 |
|---|---|
| nacos_client.py | `_import_v3_sdk`（v2.nacos，惰性）+ `_create_naming_service` / `_create_config_service`；register_instance(RegisterInstanceParam ephemeral=True)；close = deregister → 双 shutdown；`_advertise_ip` UDP connect 探测（不发包）+ hostname 兜底 |
| main.py | `start(metadata={"grpc_port": GRPC_PORT})`——BFF WithPortHint 消费（Stage 75 模式） |
| BFF main.go | llm 拨号接 `resolveGRPCAddr(grpcResolver, c.LLM.GRPCAddr, ServiceLLM)`——Go 上游 5 处之后的**第 6 处**；TLS_SERVER_NAME 显式 CN，IP 拨号证书校验不受影响 |
| servicenames.go | ServiceLLM 过期注释更新（"Python 端 BootNacos 尚未实现"→已注册） |

## 三、e2e 揪出的两个 SDK 部署坑（均已修 + 回归锁）

1. **log_dir 不可写**：v3 默认 `./logs` → 非 root 容器 /app Permission denied，注册降级。
   → `ClientConfig(log_dir=/tmp/nacos-logs)`（env 可覆盖）。
2. **cache_dir 不可写**：默认相对 cwd → `/app/nacos` 同样 Permission denied。
   → `set_cache_dir(/tmp/nacos-cache)`。

两条都进了 `pytest 165/165` 的回归锁（`_client_config` 统一注入可写目录）。

## 四、验收

- pytest **165/165**（RED 13 红：AST 导入契约 / v3 注册流 / advertise_ip / bootstrap grpc_port）
- `go build/vet/test`（web-bff + shared）全绿
- **真实容器 e2e 全链**：
  ```
  注册：[nacos] registered emotion-llm-service at 172.18.0.14:8000
        (metadata={'grpc_port': '50051', ...})
  发现：[nacos] resolve emotion-llm-service -> 172.18.0.14:50051 (grpc)   ← 6/6 上游全走 Nacos
  回归：经 APISIX:19080 login → ai/stream SSE 3 帧 [DONE] 正常收尾（mock 降级路径）
  摘除验态：stop llm 容器 → BFF resolve failed → env 兜底 emotion-llm-service:50051
        → ai/stream SSE 14 帧降级无感（[AI 上游切换] 前缀 = handler 层 mock）
  恢复：up llm → resolve 172.18.0.23:50051 (grpc) → SSE 回归通过
  ```
  注：v1 openAPI 手动 deregister 对 gRPC 连接型实例无效（SDK redo 秒级重注册，
  恰是保活机制在工作；诚实的摘除信号 = 进程死亡断连）。与 Stage 72 记录的
  "发现列表 30s 陈旧窗口"残余一并对齐了认知。

## 五、本批未做（open）

| 项 | 说明 | 去向 |
|---|---|---|
| prod 独立 bff-client 证书 | dev BFF 对 llm mTLS 复用 ai-client.crt；prod 应单独签发——纯 prod 事项，dev 无验收手段 | prod 部署批次 |
| llm-service Nacos 注册进 k8s/Helm | 决策 23 冻结 | 多机迁移启动时 |
| Python 端 Subscribe/ListenConfig dev 真推送 | 沿用 Go 侧同一 SDK 陈旧窗口残余认知，ops 配置当前 0 bytes | Nacos 深水区 |

## 六、调研依据

- 已读：`emotion-llm-service/{nacos_client.py,main.py,tests/unit/test_nacos_client.py,
  test_nacos_bootstrap.py,Dockerfile,requirements.txt}`、
  `emotion-echo-web-bff/{main.go,internal/downstream/llm_grpc.go}`、
  `emotion-echo-shared/pkg/discovery/servicenames.go`、
  `docs/legacy-plans/landed/{nacos-enablement-dev,llm-chat-real-pipeline}.md`
- 容器实证：v3 SDK API 签名（inspect）、pip 包布局（top_level=v2）、
  §四 e2e 全链日志；BFF 启动日志 6/6 resolve（grpc）
- 已查：Stage 31 PR-10 / 39 / 62 / 72 / 75 / 76（Nacos 演进线）、决策 4/18/23

---

> 最后更新：2026-09-13 by Stage 88 实施 session
> 关联：nacos-enablement-dev.md、llm-chat-real-pipeline.md、stage-75（发现链模式）、stage-81（llm env 直连时代）
