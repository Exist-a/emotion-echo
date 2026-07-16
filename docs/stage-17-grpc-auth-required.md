# Stage 17: gRPC Auth 业务闭环

> 目标：emotion-llm-service 启动时强制校验 INTERNAL_API_KEY（fail-fast），杜绝 production 误启动 dev 模式。

## 1. 背景

Stage 12 实现了 `AuthInterceptor` 校验 `x-internal-api-key` metadata，但**只在请求时校验**。问题是：
- Production 环境如果忘了设 `INTERNAL_API_KEY`，dev 模式（auth disabled）会"静默"放行
- 没启动校验 = 没强制约束 = 容易出错

Stage 17 加 fail-fast：
- `INTERNAL_API_KEY_REQUIRED=1` + key 空 → 启动失败
- 弱 key 警告（长度 < 16、含 test/dev/changeme 等）

## 2. 设计

### 2.1 启动校验

```python
required = os.environ.get("INTERNAL_API_KEY_REQUIRED", "").lower() in ("1", "true", "yes")

if required and not api_key:
    logger.error("INTERNAL_API_KEY_REQUIRED=1 but INTERNAL_API_KEY is empty.")
    sys.exit(1)
```

### 2.2 弱 key 警告

```python
if len(api_key) < 16:
    logger.warning(f"INTERNAL_API_KEY length={len(api_key)} < 16, recommend >= 32 chars")

weak = {"test", "dev", "changeme", "default", "secret", "password"}
if any(w in api_key.lower() for w in weak):
    logger.warning("INTERNAL_API_KEY contains weak pattern, use strong random key")
```

### 2.3 三种模式

| 配置 | INTERNAL_API_KEY | INTERNAL_API_KEY_REQUIRED | 行为 |
|------|------------------|--------------------------|------|
| **dev mode** | 空 | 未设 | 启动 + auth disabled（仅本地） |
| **production safe** | 32+ 强 key | 1 | 启动 + auth required |
| **production fail** | 空 | 1 | ❌ 启动失败 |
| **production warn** | 弱 key（如"secret"） | 任意 | 启动 + warn 提示 |

## 3. 实现

`grpc_server.py serve()`：

```python
def serve(port: int = 50051):
    api_key = os.environ.get("INTERNAL_API_KEY", "")
    required = os.environ.get("INTERNAL_API_KEY_REQUIRED", "").lower() in ("1", "true", "yes")

    if required and not api_key:
        logger.error("INTERNAL_API_KEY_REQUIRED=1 but INTERNAL_API_KEY is empty. Refusing to start.")
        sys.exit(1)

    if api_key:
        if len(api_key) < 16:
            logger.warning(f"INTERNAL_API_KEY length={len(api_key)} < 16")
        weak = {"test", "dev", "changeme", "default", "secret", "password"}
        if api_key.lower() in weak or any(w in api_key.lower() for w in weak):
            logger.warning("INTERNAL_API_KEY contains weak pattern")

    # ... 注册 AuthInterceptor(expected_api_key=api_key) ...
```

## 4. E2E 验证

### 4.1 场景 1：REQUIRED=1 + KEY=空 → fail

```bash
$ INTERNAL_API_KEY_REQUIRED=1 python grpc_server.py
ERROR:__main__:INTERNAL_API_KEY_REQUIRED=1 but INTERNAL_API_KEY is empty. Refusing to start.
Traceback (most recent call last):
  ...
  File "D:\...\grpc_server.py", line 276, in serve
    if required and not api_key:
SystemExit: 1
```

退出码 1，启动失败。✅

### 4.2 场景 2：弱 key → warn 启动

```bash
$ INTERNAL_API_KEY="my-super-secret-key-2026" python grpc_server.py
WARNING:__main__:INTERNAL_API_KEY contains weak pattern (test/dev/changeme/...)
INFO:__main__:gRPC server started on port 50051
INFO:__main__:interceptors: ... Auth(auth=enabled, required=False)
```

`required=False` 因为没设 REQUIRED env，但 key 非空所以 auth enabled。warn 不阻止启动。✅

### 4.3 场景 3：client 错 key → UNAUTHENTICATED

```
[wrong-key] ERR: code=UNAUTHENTICATED details=invalid api key
[correct-key] OK: emotion=neutral score=0.0
[no-key] ERR: code=UNAUTHENTICATED details=missing api key
```

✅

## 5. K8s 部署模式

```yaml
# emotion-llm-service deployment
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
      - name: llm
        env:
        - name: INTERNAL_API_KEY_REQUIRED
          value: "1"
        - name: INTERNAL_API_KEY
          valueFrom:
            secretKeyRef:
              name: emotion-llm-secrets
              key: internal-api-key
---
# ai-svc deployment
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
      - name: ai
        env:
        - name: LLM_INTERNALAPIKEY
          valueFrom:
            secretKeyRef:
              name: emotion-llm-secrets
              key: internal-api-key
```

两边用同一个 Secret → 自动同步 key，server 端 fail-fast 兜底。

## 6. 已知限制

- **未做 key 轮转**：key 长期不变，建议用 Vault / External Secrets 定期轮转
- **未做 rate limit**：被爆破时只能靠 metadata 缺失/错误识别（应加 WAF / APISIX limit-plugin）
- **key 明文比较**：当前用 `==` 比较，理论上可能受 timing attack 影响（实际上 grpc metadata 处理已经够慢，相对安全）
- **未做 key 哈希存储**：secret 直接以明文放在 env。考虑用 sealed-secrets / external-secrets

## 7. 后续 TODO

- client 端也加 REQUIRED 校验（启动时确认 key 已配）
- key 哈希存储（避免日志泄露明文）
- key 轮转（蓝绿部署时同时支持新旧 key）
