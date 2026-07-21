# 11 · 从 docker-compose 到 K8s 的逐项映射手册

> 系列：[10 TDD 落地](./10-tdd-for-k8s.md) · **11 docker-compose → K8s 映射** · [12 学习路线](./12-stage-28-roadmap.md) ...

**适合谁**：已经有 docker-compose 项目想迁移 K8s 的开发者，想知道"每一行 docker-compose 对应 K8s 什么"的读者。
**读完你能**：照着表格把 `docker-compose.yml` 逐字段翻译成 Helm chart；知道哪些字段 K8s 没有等价物，要变通。

---

## 一句话总结

**docker-compose → K8s 的迁移 = "写一遍" + "删掉 `version:` / `container_name:` / `depends_on:` 等 K8s 没有的字段" + "改用 ConfigMap/Secret + Service"**。

我们 Emotion-Echo 已经做了这件事（Stage 20 → Stage 27）。这一篇把 12 类常见字段逐一对照。

---

## 一、12 类字段逐项对照

### 1. `services` → Deployment / StatefulSet

| docker-compose | K8s |
|----------------|-----|
| `services.foo.image` | `Deployment.spec.template.spec.containers[].image` |
| `services.foo.container_name` | K8s Pod 名 = `<deployment>-<rs>-<pod>`，**不能指定** |
| `services.foo.ports` | Service.spec.ports + Container.ports |
| `services.foo.environment` | ConfigMap / Secret + envFrom / env.valueFrom |
| `services.foo.volumes` | PVC / ConfigMap / Secret volume + volumeMounts |
| `services.foo.networks` | Namespace + 隐式 cluster 网络（不用写） |
| `services.foo.depends_on` | readinessProbe + initContainer（K8s 没有硬依赖） |
| `services.foo.healthcheck` | livenessProbe + readinessProbe + startupProbe |
| `services.foo.restart` | `restartPolicy: Always`（Deployment 默认） |
| `services.foo.deploy.resources` | `resources.requests / limits` |
| `services.foo.command / entrypoint` | `command / args` |
| `services.foo.profiles` | Helm values `enabled: true/false` |

### 2. `image` → Container.image

```yaml
# docker-compose
image: emotion-echo/user-svc:v0.1.0

# K8s (Helm)
image: "{{ .Values.image.repository }}:{{ .Values.image.tag }}"
# imagePullPolicy: IfNotPresent
```

### 3. `environment` → ConfigMap + Secret + envFrom

```yaml
# docker-compose
environment:
  POSTGRES_DSN: "host=postgres ..."
  API_KEY: "secret"

# K8s
# 1) ConfigMap 装普通配置
apiVersion: v1
kind: ConfigMap
metadata: { name: user-svc-config }
data:
  POSTGRES_DSN: "host=postgres.ee-data.svc.cluster.local ..."
---
# 2) Secret 装敏感配置
apiVersion: v1
kind: Secret
metadata: { name: user-svc-secret }
stringData:
  API_KEY: "secret"
---
# 3) Pod 引用
spec:
  containers:
    - name: user-svc
      envFrom:
        - configMapRef: { name: user-svc-config }
        - secretRef:    { name: user-svc-secret }
```

### 4. `ports` → Service + Container.ports

```yaml
# docker-compose
ports:
  - "8888:8888"

# K8s
# 1) Deployment 暴露端口
spec:
  containers:
    - name: user-svc
      ports:
        - { name: http, containerPort: 8888 }
---
# 2) Service 提供 ClusterIP
apiVersion: v1
kind: Service
metadata: { name: user-svc }
spec:
  selector: { app: user-svc }
  ports:
    - { name: http, port: 8888, targetPort: 8888 }
```

### 5. `volumes` → PVC + volumeMounts

```yaml
# docker-compose
volumes:
  - ./etc/user-api.yaml:/app/etc/user-api.yaml:ro
  - postgres_data:/var/lib/postgresql/data

# K8s
# 1) bind mount → ConfigMap
apiVersion: v1
kind: ConfigMap
metadata: { name: user-svc-config }
data:
  user-api.yaml: |
    {{- .Files.Get "files/user-api.yaml" | nindent 4 }}
---
spec:
  containers:
    - name: user-svc
      volumeMounts:
        - name: config
          mountPath: /app/etc
          readOnly: true
  volumes:
    - name: config
      configMap: { name: user-svc-config }
---
# 2) named volume → PVC
apiVersion: v1
kind: PersistentVolumeClaim
metadata: { name: postgres-data }
spec:
  accessModes: [ReadWriteOnce]
  resources: { requests: { storage: 10Gi } }
```

### 6. `networks` → Namespace + 隐式 cluster 网络

```yaml
# docker-compose
networks:
  app-network:
    external: true

# K8s
# 1) 把所有 svc 放同一个 namespace
metadata:
  namespace: ee-app

# 2) 同一 ns 内 svc 通过 <svc-name>.<ns>.svc.cluster.local 互通
# 3) 跨 ns 必须带 ns 后缀
```

### 7. `depends_on` → readinessProbe + initContainer

```yaml
# docker-compose
depends_on:
  postgres:
    condition: service_healthy

# K8s
# 方式 1：信任 readinessProbe（K8s 会等 Pod ready 才接流量）
spec:
  containers:
    - name: chat-svc
      readinessProbe:
        httpGet: { path: /health, port: http }
        periodSeconds: 10
        # 如果 /health 失败 → 不接流量，但不会"等待"

# 方式 2：显式 initContainer 等
initContainers:
  - name: wait-for-postgres
    image: busybox:1.36
    command:
      - sh
      - -c
      - 'until nc -z postgres.ee-data 5432; do echo waiting; sleep 2; done'
```

### 8. `healthcheck` → 3 种探针

```yaml
# docker-compose
healthcheck:
  test: ["CMD-SHELL", "wget --quiet --tries=1 --spider http://localhost:8888/health"]
  interval: 30s
  timeout: 5s
  start_period: 15s
  retries: 3

# K8s
startupProbe:        # 启动阶段宽限（start_period 类似）
  httpGet: { path: /health, port: http }
  periodSeconds: 5
  failureThreshold: 6       # 5*6=30s 宽限
readinessProbe:       # 持续探活（interval 类似）
  httpGet: { path: /health, port: http }
  periodSeconds: 10
  failureThreshold: 3
livenessProbe:        # 持续探活（retries 类似）
  httpGet: { path: /health, port: http }
  initialDelaySeconds: 60
  periodSeconds: 30
  failureThreshold: 3
```

### 9. `restart` → restartPolicy

```yaml
# docker-compose
restart: unless-stopped    # 或 always / on-failure

# K8s（Deployment 默认 Always，StatefulSet 默认 Always）
spec:
  restartPolicy: Always     # 通常不写
```

### 10. `deploy.resources` → resources.requests / limits

```yaml
# docker-compose
deploy:
  resources:
    limits: { cpus: "0.5", memory: 256M }
    reservations: { cpus: "0.1", memory: 64M }

# K8s
resources:
  requests:
    cpu: 100m            # 0.1 核
    memory: 64Mi
  limits:
    cpu: 500m            # 0.5 核
    memory: 256Mi
```

### 11. `profiles` → values.enabled

```yaml
# docker-compose
services:
  emotion-echo-xtts:
    profiles: ["ai"]

# K8s (Helm values)
xtts:
  enabled: true    # dev 默认 false，prod 设 true

# templates/deployment.yaml
{{- if .Values.xtts.enabled }}
apiVersion: apps/v1
kind: Deployment
metadata: { name: xtts }
# ...
{{- end }}
```

### 12. `depends_on` 跨主机（K8s 没有）

| docker-compose 能做的 | K8s 不能做的 |
|---------------------|--------------|
| 容器按顺序启动 | K8s Pod 都是并行启动 |
| `condition: service_healthy` 等待依赖 healthy | 用 readinessProbe + initContainer 模拟 |
| `depends_on` 强阻塞 | K8s 没有"强依赖"，要靠 readiness 控制流量 |

---

## 二、Stage 26-P → Stage 27 真实迁移案例

### 案例 1：user-svc

#### docker-compose

```yaml
emotion-echo-user-svc:
  image: emotion-echo/user-svc:v0.1.0
  container_name: emotion-echo-user-svc
  restart: unless-stopped
  environment:
    LOG_LEVEL: INFO
    GIN_MODE: release
    POSTGRES_DSN: "host=emotion-echo-postgres port=5432 user=postgres ..."
    SKYWALKING_OAP_ADDR: emotion-echo-sw-oap:11800
  ports:
    - "8888:8888"
  volumes:
    - ../emotion-echo-user-svc/etc/user-api.yaml:/app/etc/user-api.yaml:ro
  networks:
    - app-network
  deploy:
    resources:
      limits: { cpus: "0.5", memory: 256M }
      reservations: { cpus: "0.1", memory: 64M }
  depends_on:
    postgres:
      condition: service_started
  healthcheck:
    test: ["CMD-SHELL", "wget --quiet --tries=1 --spider http://localhost:8888/health"]
    interval: 30s
    start_period: 15s
```

#### 翻译成 Helm chart

```yaml
# charts/emotion-echo/charts/user-svc/values.yaml
replicaCount: 1
image:
  repository: emotion-echo/user-svc
  tag: v0.1.0
  pullPolicy: IfNotPresent
service:
  port: 8888
resources:
  requests: { cpu: 100m, memory: 64Mi }
  limits:   { cpu: 500m, memory: 256Mi }
configOverrides:
  Name: user-api
  Host: 0.0.0.0
  Port: 8888
  Log: { Mode: json, Level: info }
secrets:
  postgresDsn: "host=postgres.ee-data.svc.cluster.local user=postgres password={{ .Values.global.secrets.postgresPassword }} dbname=emotion_echo sslmode=disable search_path=emotion_echo_user"
  skywalkingOapAddr: "skywalking-oap.ee-data.svc.cluster.local:11800"
```

```yaml
# templates/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: user-svc
  namespace: ee-app
spec:
  replicas: {{ .Values.replicaCount }}
  selector:
    matchLabels: { app: user-svc }
  template:
    metadata:
      labels: { app: user-svc }
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 65532
      containers:
        - name: user-svc
          image: "{{ .Values.image.repository }}:{{ .Values.image.tag }}"
          imagePullPolicy: {{ .Values.image.pullPolicy }}
          ports:
            - { name: http, containerPort: 8888 }
          startupProbe:
            httpGet: { path: /health, port: http }
            periodSeconds: 5
            failureThreshold: 6
          readinessProbe:
            httpGet: { path: /health, port: http }
            periodSeconds: 10
            failureThreshold: 3
          livenessProbe:
            httpGet: { path: /health, port: http }
            initialDelaySeconds: 60
            periodSeconds: 30
            failureThreshold: 3
          resources: {{- toYaml .Values.resources | nindent 12 }}
          envFrom:
            - configMapRef: { name: user-svc-config }
            - secretRef: { name: user-svc-secret }
```

```yaml
# templates/service.yaml
apiVersion: v1
kind: Service
metadata:
  name: user-svc
  namespace: ee-app
spec:
  selector: { app: user-svc }
  ports:
    - { name: http, port: 8888, targetPort: 8888 }
```

```yaml
# templates/configmap.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: user-svc-config
  namespace: ee-app
data:
  user-api.yaml: |
    {{- toYaml .Values.configOverrides | nindent 4 }}
```

### 案例 2：postgres（StatefulSet 替换）

#### docker-compose

```yaml
postgres:
  image: postgres:15-alpine
  container_name: emotion-echo-postgres
  restart: unless-stopped
  environment:
    POSTGRES_PASSWORD: postgres
    POSTGRES_USER: postgres
    POSTGRES_DB: emotion_echo
  ports:
    - "5432:5432"
  volumes:
    - postgres_data:/var/lib/postgresql/data
    - ./db/01-create-schemas.sql:/docker-entrypoint-initdb.d/01-create-schemas.sql:ro
```

#### K8s StatefulSet

```yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: postgres
  namespace: ee-data
spec:
  serviceName: postgres-headless
  replicas: 1
  selector: { matchLabels: { app: postgres } }
  template:
    spec:
      initContainers:
        - name: init-schemas
          image: postgres:15-alpine
          command: ["psql", "-h", "localhost", "-U", "postgres", "-f", "/docker-entrypoint-initdb.d/01-create-schemas.sql"]
          env:
            - name: PGPASSWORD
              valueFrom: { secretKeyRef: { name: postgres-secret, key: password } }
          volumeMounts:
            - name: init-sql
              mountPath: /docker-entrypoint-initdb.d
      containers:
        - name: postgres
          image: postgres:15-alpine
          envFrom:
            - secretRef: { name: postgres-secret }
          ports:
            - { name: pg, containerPort: 5432 }
          volumeMounts:
            - name: data
              mountPath: /var/lib/postgresql/data
      volumes:
        - name: init-sql
          configMap:
            name: postgres-init-sql
  volumeClaimTemplates:
    - metadata: { name: data }
      spec:
        accessModes: [ReadWriteOnce]
        resources: { requests: { storage: 10Gi } }
```

---

## 三、K8s 没有的 docker-compose 字段（K8s 要变通）

### 3.1 `container_name`

**K8s 没有**：Pod 名 = `<deployment>-<rs>-<pod>`，随机。

**变通**：用 Service 访问（不依赖 Pod 名）。

### 3.2 `links`

**K8s 没有**：用 Service + DNS（自动）。

### 3.3 `volumes_from`（共享另一个容器的 volume）

**K8s 没有**：用 PVC / PV 共享，或 emptyDir 在同 Pod 内多容器共享。

### 3.4 `privileged: true`

**K8s 有但要谨慎**：K8s 也支持 `privileged: true`，但生产禁止（容器逃逸）。

### 3.5 `pid: "host"` / `ipc: "host"`

**K8s 有但要谨慎**：同节点 namespace 共享，安全风险高。

---

## 四、最常见的迁移错误

### 错误 1：忘了改环境变量里的容器名

```yaml
# 错（容器名是 docker-compose 的 DNS）
POSTGRES_DSN: "host=emotion-echo-postgres ..."

# 对（K8s 用 Service FQDN）
POSTGRES_DSN: "host=postgres.ee-data.svc.cluster.local ..."
```

### 错误 2：忘了改 Kafka advertised listener

```yaml
# 错
KAFKA_ADVERTISED_LISTENERS: "PLAINTEXT://localhost:9092"

# 对
KAFKA_ADVERTISED_LISTENERS: "PLAINTEXT://kafka-0.kafka-headless.ee-data.svc.cluster.local:9092"
```

### 错误 3：忘了把 bind mount 换成 ConfigMap / Secret

```yaml
# 错（K8s 里没有宿主机 bind mount 概念）
volumes:
  - ./etc/user-api.yaml:/app/etc/user-api.yaml:ro

# 对
volumes:
  - name: config
    configMap:
      name: user-svc-config
```

### 错误 4：把 `image` 写成 `image:tag` 整体

```yaml
# docker-compose
image: emotion-echo/user-svc:v0.1.0

# K8s 拆开（便于 values 覆盖）
image:
  repository: emotion-echo/user-svc
  tag: v0.1.0
  pullPolicy: IfNotPresent
```

---

## 五、迁移实战 checklist

```
□ 改 image 拆成 repository / tag / pullPolicy
□ 改 container_name → Service
□ 改 depends_on → readinessProbe / initContainer
□ 改 environment 拆成 ConfigMap（明文）+ Secret（敏感）
□ 改 volumes 拆成 ConfigMap（配置）/ PVC（数据）/ Secret（证书）
□ 改 networks → Namespace（同一 ns 互通，跨 ns 用完整 FQDN）
□ 改 ports → Service.spec.ports + Container.ports
□ 改 healthcheck → startupProbe + readinessProbe + livenessProbe
□ 改 deploy.resources → resources.requests/limits
□ 改 profiles → values.enabled（用 if 包裹 template）
□ 改 Kafka advertised listener → StatefulSet pod DNS
□ 改 ${VAR:-default} 占位符 → 直接写值 或 values 注入
□ 改 imagePullPolicy: IfNotPresent（kind/minikube）
□ 改 securityContext.runAsNonRoot: true（如果应用支持）
□ 改 healthcheck 端口从宿主机端口 → containerPort
```

---

## 六、迁移工具（半自动）

| 工具 | 用途 | 局限 |
|------|------|------|
| **kompose** | docker-compose → K8s manifest 自动转换 | 生成的 yaml 不规范，需要手动调 |
| **helmify** | kompose 结果 → Helm chart | 同样需要手动调 |
| **kustomize** | 不直接转，但可以组合 manifest | 需要已有 manifest |

我们 Stage 27 **没用** kompose（全手写）。原因：
1. 项目需求定制多（multi-ns、APISIX CRD、mTLS Secret）
2. kompose 生成物要重写，反而费时间
3. 学习阶段手写更扎实

---

## 七、本节自检

1. **docker-compose 的 `depends_on` 在 K8s 里用什么替代？**
2. **bind mount 的配置文件怎么迁到 K8s？**
3. **`POSTGRES_DSN` 在 K8s 里 host 应该写什么？**
4. **Kafka advertised listener 写 `localhost:9092` 为什么错？**
5. **K8s 没有 `container_name`，怎么"找到"容器？**

<details>
<summary>📋 参考答案</summary>

1. readinessProbe（让 Pod 在依赖没就绪时不接流量）+ initContainer（显式 wait）。
2. 转成 ConfigMap（普通配置）/ Secret（敏感配置），用 volumeMounts 挂到容器内。
3. Service 的 FQDN，如 `postgres.ee-data.svc.cluster.local`。
4. Kafka client 拿到 advertised 后自己连；localhost 在另一个 Pod 里解析不到；要用 StatefulSet pod DNS `kafka-0.kafka-headless.ee-data.svc.cluster.local:9092`。
5. 用 Service（ClusterIP + DNS），客户端连 Service 而不是 Pod。

</details>

---

## 八、推荐阅读

| 资源 | 链接 |
|------|------|
| kompose | http://kompose.io/ |
| Docker Compose 迁移指南 | https://kubernetes.io/docs/tasks/configure-pod-container/translate-compose-kubernetes/ |
| Compose Spec on K8s | https://github.com/compose-spec/compose-spec |
| K8s for Docker Users | https://kubernetes.io/docs/reference/generated/kubectl-for-docker-users/ |

---

> **下一步**：[12 Stage 28+ 学习路线与推荐阅读](./12-stage-28-roadmap.md) —— Stage 27 之后该往哪走？HPA / cert-manager / GitOps / 多 region？