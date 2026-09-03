# Stage 18: gRPC mTLS（双向认证）

> 目标：internal svc-to-svc 通信加密 + 双向身份验证（防中间人 + 防伪客户端）。

## 1. 背景

`add_insecure_port` 只用 HTTP/2 明文，**所有消息（包括 `x-internal-api-key`）走明文**：
- 内网 ARP spoofing 可窃听
- 任何进程都能伪装成内部服务调 gRPC API

mTLS（mutual TLS）解决：
- **加密**：TLS 1.2+ 保证数据机密性
- **server 身份验证**：client 验证 server cert（CN=emotion-llm-service）
- **client 身份验证**：server 验证 client cert（CN=emotion-echo-ai-svc）

## 2. 设计

### 2.1 证书层级

```
emotion-echo-dev-ca (10 年有效期)
    │
    ├── emotion-llm-service  (1 年, SAN=localhost,127.0.0.1)
    │   用途：emotion-llm-service 用作 server cert
    │
    └── emotion-echo-ai-svc  (1 年)
        用途：ai-svc 用作 client cert
```

### 2.2 文件结构

```
deploy/tls/
├── ca.crt            # CA 根证书（10y）
├── ca.key            # CA 私钥（仅签发用，部署后可销毁）
├── llm-server.crt    # server cert (1y)
├── llm-server.key    # server key
├── ai-client.crt     # client cert (1y)
├── ai-client.key     # client key
└── generate.py       # 证书生成脚本
```

### 2.3 配置

| 环境变量 | 作用 | 默认 |
|----------|------|------|
| `TLS_ENABLED` | 启用 mTLS（1/true） | 0（明文） |
| `TLS_CA_CERT` | CA cert 路径 | deploy/tls/ca.crt |
| `TLS_SERVER_CERT` / `TLS_SERVER_KEY` | server 端 cert/key | deploy/tls/llm-server.* |
| `TLS_CLIENT_CERT` / `TLS_CLIENT_KEY` | client 端 cert/key | deploy/tls/ai-client.* |
| `TLS_REQUIRE_CLIENT_AUTH` | server 端是否强制 client cert | 1（mTLS） |
| `TLS_SERVER_NAME` | client 端校验的 server cert CN | emotion-llm-service |

## 3. 实现

### 3.1 证书生成（Python `cryptography`）

```python
# CA
ca_cert = (CertificateBuilder()
    .subject_name(ca_name)
    .issuer_name(ca_name)
    .public_key(ca_key.public_key())
    .add_extension(BasicConstraints(ca=True, path_length=None), critical=True)
    .sign(ca_key, SHA256()))

# Server cert (emotion-llm-service)
server_csr = make_csr(server_key, cn="emotion-llm-service",
                     san_dns=["localhost", "emotion-llm-service"],
                     san_ip=["127.0.0.1"])
server_cert = cert_from_csr(server_csr, ca_cert, ca_key)
```

### 3.2 Server 端（Python gRPC）

```python
creds = grpc.ssl_server_credentials(
    [(server_key, server_cert)],       # server 自己
    root_certificates=ca_cert,         # 信任的 CA
    require_client_auth=True,          # 强制 mTLS
)
server.add_secure_port(f"[::]:{port}", creds)
```

### 3.3 Client 端（Go gRPC）

```go
pool := x509.NewCertPool()
pool.AppendCertsFromPEM(caData)             // 信任的 CA
clientCert, _ := tls.LoadX509KeyPair(certPath, keyPath)
tlsConfig := &tls.Config{
    Certificates: []tls.Certificate{clientCert},
    RootCAs:      pool,
    ServerName:   "emotion-llm-service",
    MinVersion:   tls.VersionTLS12,
}
creds := credentials.NewTLS(tlsConfig)
```

## 4. E2E 验证

### 4.1 启动 server（mTLS）

```bash
$ TLS_ENABLED=1 python grpc_server.py
INFO:__main__:mTLS enabled: ca=D:\...\deploy\tls\ca.crt cert=D:\...\llm-server.crt require_client_auth=True
INFO:__main__:gRPC server started on port 50051
```

### 4.2 启动 ai-svc（mTLS client）

```bash
$ TLS_ENABLED=1 \
  TLS_CA_CERT=D:\...\ca.crt \
  TLS_CLIENT_CERT=D:\...\ai-client.crt \
  TLS_CLIENT_KEY=D:\...\ai-client.key \
  ./ai-svc.exe

[grpc-tls] mTLS client enabled: ca=... cert=... server_name=emotion-llm-service
[grpc-client] method=/grpc.health.v1.Health/Check target=localhost:50051 latency=42ms err=<nil>
[grpc-health] target=localhost:50051 service=emotion.LLM status=SERVING
[llm] using gRPC analyzer (target=localhost:50051, auth=enabled) + keyword fallback
```

✅ mTLS 双向认证成功，health check + Analyze 全部走加密通道。

## 5. K8s 部署

```yaml
# emotion-llm-service deployment
spec:
  containers:
  - name: llm
    volumeMounts:
    - name: tls-certs
      mountPath: /etc/tls
      readOnly: true
    env:
    - name: TLS_ENABLED
      value: "1"
    - name: TLS_REQUIRE_CLIENT_AUTH
      value: "1"
  volumes:
  - name: tls-certs
    secret:
      secretName: emotion-llm-tls
      items:
      - key: ca.crt
        path: ca.crt
      - key: tls.crt
        path: llm-server.crt
      - key: tls.key
        path: llm-server.key
```

## 6. 与 AuthInterceptor 协同

| 层 | 作用 | 失败行为 |
|----|------|----------|
| **mTLS** | 加密 + 身份认证（"你是谁"） | TCP 握手失败 → Unavailable |
| **AuthInterceptor** | API key 鉴权（"你能做什么"） | UNAUTHENTICATED |

两层互补：mTLS 证明"你是 emotion-echo-ai-svc"，API key 证明"你有调用权限"。
即使 mTLS 证书泄露，攻击者仍需 API key 才能调业务 RPC。

## 7. 已知限制

- **证书有效期 1 年**：需要定期轮转（建议用 cert-manager 自动续签）
- **未做 cert rotation**：旧 cert 过期时需滚动重启服务
- **未做 mTLS for client streaming**：仅 unary `Analyze` 走 mTLS
- **Python 端用 `cryptography` 生成证书**：生产环境应改用 cert-manager + Vault
- **CA key 仍保留在 `deploy/tls/`**：生产应分离，CI 不接触 CA key

## 8. 后续 TODO

- cert-manager 集成（自动续签）
- cert rotation（热加载，不重启）
- SPIFFE / SPIRE 替代自签名 CA（更强身份）
- 监控证书剩余有效期（< 30 天告警）
