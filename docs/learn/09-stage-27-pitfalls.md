# 09 · Stage 27 踩坑全记录（helm template 语法 / Windows 文件锁 / YAML 解析 / CRD 版本）

> 系列：[08 探针/安全/资源](./08-probes-and-security.md) · **09 踩坑全记录** · [10 TDD 在 K8s 落地](./10-tdd-for-k8s.md) ...

**适合谁**：要写 Helm chart 或部署 K8s 的人，特别是第一次在 Windows + Git Bash + Helm + kind 链路下搞全套的人。
**读完你能**：提前避坑，知道每个错误信息背后是什么；如果你也踩到同样的坑，能 30 秒定位原因。

---

## 一句话总结

**Stage 27 写了 17 个 chart + 79 个资源，遇到 12+ 个真实坑**。这一篇把每个坑的**症状 → 排查路径 → 根因 → 修复**列成清单，下次再遇到能秒查。

---

## 坑 1 · Windows 文件锁住 kind.exe

### 症状

```bash
$ mv kind.exe /usr/local/bin/
mv: cannot move 'kind.exe' to '/usr/local/bin/kind.exe': Device or resource busy
$ rm kind.exe
rm: cannot remove 'kind.exe': Device or resource busy
```

### 排查

```bash
$ tasklist | grep -i kind
kind.exe     12345    Console    1    150,000 K
```

### 根因

Windows 不允许删除/移动正在被进程使用的文件。kind.exe 之前被某个 shell 进程加载了，文件句柄还在。

### 修复

1. TaskStop 所有 kind 相关进程
2. 如果还不行，**重启 shell / IDE**
3. 或者**先下载到非 bin 目录**（如 `C:/Users/<you>/bin/kind.exe`），让 PATH 指向它

### 预防

下载二进制后立刻让 PATH 找到，避免后续 mv。

---

## 坑 2 · Helm Chart.yaml 描述符字符串解析失败

### 症状

```bash
$ helm lint ./charts/emotion-echo
Error: failed to parse Chart.yaml: error converting YAML to JSON: ...
```

### 排查

看 Chart.yaml 的 description 字段：

```yaml
# 错
description: Emotion-Echo microservices - {user,chat,ai,assessment,analytics}
```

### 根因

YAML 解析器把 `{user,chat,...}` 当成 flow-style 字典（set/map），解析时报错。

### 修复

```yaml
# 对
description: Five schemas - one per service.
```

### 教训

**任何包含 `{`、`}`、`[`、`]`、`&`、`*` 等 YAML 特殊字符的字符串，要么加引号，要么改写法**。

---

## 坑 3 · Flow style YAML 在 Helm template 里报错

### 症状

```bash
$ helm template ee ./chart
Error: failed to parse template: ... yaml: line 13: did not find expected ',' or '}'
```

### 排查

看 templates 里这种写法：

```yaml
# 错（flow style）
ports:
  - { name: http, port: 8000 }
env:
  - { name: FOO, value: bar }
```

### 根因

Helm template 渲染后 Go template 引擎把 `{...}` 当成自己的结构体，导致 YAML 解析器收到的不是合法 YAML。

### 修复

```yaml
# 对（normal style）
ports:
  - name: http
    port: 8000
env:
  - name: FOO
    value: bar
```

### 教训

**所有 K8s manifest 写 normal-style YAML**（每个字段一行），永远不要用 flow style。

---

## 坑 4 · `nil pointer evaluating` 访问不存在的 values

### 症状

```bash
$ helm template ee ./chart
Error: nil pointer evaluating interface{}.postgresPassword
```

### 排查

template 里这么写：

```yaml
# 错
data:
  POSTGRES_DSN: "host=postgres user={{ .Values.global.secrets.postgresPassword }}"
```

如果用户没设 `global.secrets.postgresPassword`，`.Values.global.secrets` 是 nil。

### 根因

Go template 对 nil 字典取值会 panic。

### 修复

链式判断：

```yaml
# 对
data:
  {{- if and .Values.global .Values.global.secrets .Values.global.secrets.postgresPassword }}
  POSTGRES_DSN: "host=postgres user={{ .Values.global.secrets.postgresPassword }}"
  {{- end }}
```

或者用 `default` 函数：

```yaml
# 也对
POSTGRES_DSN: "host=postgres user={{ .Values.global.secrets.postgresPassword | default "postgres" }}"
```

### 教训

**任何嵌套 .Values.x.y.z 都要么用 `default`，要么用 `if and` 链式保护**。

---

## 坑 5 · helper 模板第一行少了换行

### 症状

渲染出来的 yaml 里两行粘在一起：

```yaml
metadata:
  name: ee-user-svc
  labels:
    app.kubernetes.io/part-of: emotion-echolayer: app
    # ↑ 这里被合并成一行
```

### 排查

```yaml
# 错
{{- define "user-svc.labels" -}}
app.kubernetes.io/part-of: emotion-echo
layer: app
{{- end -}}
```

### 根因

`{{- define "user-svc.labels" -}}` 用 `-}}` 删了前面的换行，导致 `app.kubernetes.io/part-of: emotion-echo` 这一行直接被替换到调用处，没有独立换行。

### 修复

```yaml
# 对
{{- define "user-svc.labels" }}
{{/* 注意第一行有换行 */}}
app.kubernetes.io/part-of: emotion-echo
layer: app
{{- end }}
```

或者用 `nindent` 调用：

```yaml
metadata:
  labels:
    {{- include "user-svc.labels" . | nindent 4 }}
```

### 教训

**helper 模板第一行不要 trim**，让调用者自己用 `nindent` 处理缩进。

---

## 坑 6 · umbrella chart 缺少 dependencies 声明

### 症状

```bash
$ helm lint ./charts/emotion-echo
Error: chart "emotion-echo" has no dependencies
```

### 排查

umbrella Chart.yaml 里没声明子 chart：

```yaml
# 错
apiVersion: v2
name: emotion-echo
```

### 根因

Helm lint 期望 umbrella 在 `dependencies:` 里列出子 chart（即使内联）。

### 修复

```yaml
# 对
apiVersion: v2
name: emotion-echo
dependencies:
  - name: postgres
    version: 0.1.0
    condition: postgres.enabled
  - name: user-svc
    version: 0.1.0
    condition: user-svc.enabled
  # ... 共 17 个
```

### 教训

**umbrella 必须显式列依赖**，即使子 chart 在 `charts/` 目录里。

---

## 坑 7 · `helm dependency update` 卡死（外网不可达）

### 症状

```bash
$ helm dependency update ./charts/emotion-echo
Hang... (waiting 30s for repo download)
```

### 根因

Helm 默认从子 chart 仓库（charts.bitnami.com 等）下载。我们内联了子 chart，但 helm 还是想去下元数据。

### 修复

**用内联模式 + 完整的 Chart.yaml 声明**。如果还有问题，可以加 `--skip-refresh`：

```bash
helm dependency update ./charts/emotion-echo --skip-refresh
```

或者干脆不用 `helm dependency update`，直接 `helm install`（Helm 3 会自动识别内联子 chart）。

### 教训

**离线/学习环境尽量用内联子 chart**，省去依赖管理。

---

## 坑 8 · Kafka advertised listener 在 K8s 内解析不到

### 症状

ai-svc Pod 启动后 Kafka client 报错：

```
kafka: client has run out of available brokers to talk to: dial tcp: lookup kafka on 10.96.0.10: no such host
```

### 排查

K8s 里 Kafka advertised 写的是 `kafka:9092` 或 `localhost:9092`。

### 根因

Kafka client 拿到 broker advertised 地址后**自己去连**。ai-svc Pod 在 ee-app ns，DNS 解析 `kafka` 找不到（broker 在 ee-data ns）。

### 修复

```yaml
# 用 StatefulSet pod DNS
KAFKA_ADVERTISED_LISTENERS: "PLAINTEXT://kafka-0.kafka-headless.ee-data.svc.cluster.local:9092"
```

### 教训

**跨 ns + 跨 Pod 直连必须用完整 FQDN**（详见 [07 网络](./07-networking-and-dns.md)）。

---

## 坑 9 · APISIX 3.9 触发 nginx `301 about:blank`

### 症状

```bash
$ curl http://localhost:9080/api/v1/users/me
<title>about:blank</title>
... (返回空白页)
```

### 根因

apache/apisix:3.9.0-debian 镜像的 openresty 内部 nginx 在某些情况下触发 `return 301 about:blank`，把 API 路径 redirect 到空白页。这是 3.9 已知 bug。

### 修复

升级到 **apache/apisix:3.10+**（已修复）。

### 教训

**遇到 301 重定向到 about:blank 立刻查网关版本**。

---

## 坑 10 · test 找不到 chart 路径

### 症状

```go
out, err := exec.Command("helm", "template", "ee", "../charts/emotion-echo").CombinedOutput()
// exec: "../charts/emotion-echo": no such file or directory
```

### 根因

测试在 `k8s/tests/` 目录跑，相对路径要算到顶层 `charts/`。

### 修复

```go
import "path/filepath"

chartPath, _ := filepath.Abs("../../charts/emotion-echo")
out, err := exec.Command("helm", "template", "ee", chartPath, ...).CombinedOutput()
```

### 教训

**集成测试用绝对路径或 `filepath.Abs`**，不要依赖 cwd。

---

## 坑 11 · Secret 注入的密码含特殊字符

### 症状

ai-svc 启动后日志：

```
parse "host=postgres user=postgres password=my!pass#word ...": invalid URL escape
```

### 根因

Secret 里的密码含 `!`、`#`、`$` 等 shell / Go 特殊字符，没转义。

### 修复

```yaml
# Secret 用 stringData（自动处理）
apiVersion: v1
kind: Secret
stringData:
  POSTGRES_PASSWORD: "my!pass#word"    # stringData 会自动 base64
```

或者让 DSN 字符串用单引号包裹、URL-encode 特殊字符。

### 教训

**Secret 字符串默认用 stringData**，避免手写 base64 出错。

---

## 坑 12 · 镜像拉取策略在 kind 里不匹配

### 症状

```bash
$ kubectl get pods
NAME                       READY   STATUS             RESTARTS   AGE
user-svc-xxx               0/1     ImagePullBackOff   0          1m
```

### 根因

kind 里镜像用 `kind load docker-image` 加载，**镜像 tag 是 `:v0.1.0`，但 K8s 默认会去 Docker Hub 拉**。

### 修复

```yaml
imagePullPolicy: IfNotPresent    # 如果本地有镜像就用本地的
```

### 教训

**kind / minikube / Docker Desktop 本地集群必须设 `imagePullPolicy: IfNotPresent`**。

---

## 坑 13 · StatefulSet volumeClaimTemplates 删不掉

### 症状

```bash
$ kubectl delete pvc data-postgres-0
Error: persistentvolumeclaims "data-postgres-0" not found
# 但 kubectl get pvc 也没了
$ helm uninstall ee
# 删了 StatefulSet 但 PVC 还留着
```

### 根因

StatefulSet 的 `volumeClaimTemplates` 生成的 PVC **不归 StatefulSet 管**（K8s 设计：用户应该手动管理 PVC）。

### 修复

```bash
# 手动删
kubectl delete pvc -l app=postgres
```

或者用 `policy: Delete` 让 PVC 自动删（K8s 1.27+）：

```yaml
persistentVolumeClaimRetentionPolicy:
  whenDeleted: Delete
  whenScaled: Retain
```

### 教训

**StatefulSet + PVC 的清理是手动操作**，不要以为 helm uninstall 会一并清理。

---

## 坑 14 · namespace 已有资源时 helm install 失败

### 症状

```bash
$ helm install ee ./chart
Error: namespaces "ee-app" already exists
```

### 根因

template 里用 `apiVersion: v1, kind: Namespace` 创建，但 ns 已存在。

### 修复

用 `lookup` 函数做幂等：

```yaml
{{- if not (lookup "v1" "Namespace" "" "ee-app") }}
apiVersion: v1
kind: Namespace
metadata:
  name: ee-app
{{- end }}
```

### 教训

**所有"创建型"资源用 lookup 包裹**，保证 helm install / upgrade 幂等。

---

## 坑 15 · gRPC 探针路径错误

### 症状

```bash
$ kubectl describe pod chat-svc
Liveness probe failed: Get "http://10.244.1.5:8890/health": dial tcp 10.244.1.5:8890: connect: connection refused
```

### 根因

chat-svc 的 gRPC 端口（8890）**没有 HTTP /health 端点**，HTTP 探针当然失败。

### 修复

chat-svc 暴露 HTTP 端口（8891）专门做 healthz；gRPC 端口（8890）只跑 gRPC。或者用 gRPC 探针：

```yaml
livenessProbe:
  grpc:
    port: 8890
```

### 教训

**gRPC 服务也要有 HTTP /health 端点**，K8s 探针默认是 HTTP。

---

## 坑 16 · ConfigMap hot reload 不生效

### 症状

```bash
$ kubectl edit configmap user-svc-config
# 改了配置
$ curl http://user-svc:8888/config
# 还是旧值
```

### 根因

很多应用**启动时**读 ConfigMap，运行时**不重读**。

### 修复

- **挂载成 volume**：用 `subPath: user-api.yaml` 让文件单独更新触发 inotify
- 或者用 **Reloader** operator 监听 ConfigMap 变化自动重启 Pod

### 教训

**改 ConfigMap 不一定生效**，要看应用有没有 hot reload 能力。我们 go-zero 是启动时读，所以改 ConfigMap 要 `kubectl rollout restart deployment/user-svc`。

---

## 坑 17 · Helm template 渲染慢（17 个 chart）

### 症状

```bash
$ time helm template ee ./charts/emotion-echo
real    0m2.140s     # 单次渲染 2 秒
```

### 根因

17 个子 chart 一起渲染，加上 Secret 加密、JSON 序列化等，2 秒属正常。

### 修复

1. 用 `helm template --debug` 看哪个 chart 最慢
2. 拆分 release：数据层一个 release，业务层一个 release（但失去 umbrella 优势）
3. 接受这个延迟（CI 跑一次 2 秒完全可以）

### 教训

**umbrella chart 渲染慢是正常的**，不要为了"快"拆 release。

---

## 坑 18 · kind 节点 OOM 导致 control-plane 重启

### 症状

```bash
$ kubectl get nodes
NAME                       STATUS   ROLES           AGE
ee-cluster-control-plane   Ready    control-plane   5m    # 实际重启了 3 次
```

### 排查

```bash
$ docker logs ee-cluster-control-plane
... Out of memory: Killed process ...
```

### 根因

节点容器设的内存小（kind 默认），开 17 个 svc 后 OOM。

### 修复

```yaml
# kind-config.yaml
nodes:
  - role: control-plane
    extraPortMappings: [...]
    # 不加这行，kind 默认就够了；如要加大：
  - role: worker
```

如果是宿主机内存不够：
1. 关掉 xtts.enabled / sensevoice.enabled
2. 升级宿主内存到 16GB+

### 教训

**本地 kind 资源有限，先关掉重的 svc 再跑全套**。

---

## 总结：12 个坑的根因分布

| 根因 | 出现次数 | 防范 |
|------|---------|------|
| YAML 解析 / 语法 | 4 | 永远用 normal style |
| values / template 路径 | 3 | 用 default / if and / lookup 保护 |
| Windows 平台特性 | 2 | 注意文件锁、路径分隔 |
| 网络 / DNS | 2 | 用完整 FQDN、imagePullPolicy IfNotPresent |
| 网关 / 镜像 bug | 2 | APISIX 3.10+、本地镜像 IfNotPresent |
| StatefulSet 副作用 | 1 | PVC 手动清理 |

---

## 本节自检

1. **YAML 字符串含 `{` 或 `[` 怎么办？**
2. **`.Values.global.secrets.x` 怎么写才不 panic？**
3. **Kafka advertised 在 K8s 里必须怎么写？为什么？**
4. **kind 集群里 imagePullPolicy 应该是什么？**
5. **改 ConfigMap 后应用没更新，可能的原因？**

<details>
<summary>📋 参考答案</summary>

1. 用 normal style YAML；描述符字符串加引号或避免特殊字符。
2. 用 `if and a b c d` 链式判断或 `| default "x"` 函数。
3. 完整 FQDN `kafka-0.kafka-headless.ee-data.svc.cluster.local:9092`。因为 Kafka client 拿到 advertised 后自己连，短名在跨 ns 解析不到。
4. IfNotPresent。kind 集群通过 `kind load docker-image` 把镜像灌进节点，本地已存在；Always 会去 Docker Hub 拉不到。
5. 应用没实现 hot reload；需要 `kubectl rollout restart deployment/<svc>` 触发 Pod 重建；或挂载成 volume + subPath 触发文件 inotify。

</details>

---

## 推荐阅读

| 资源 | 链接 |
|------|------|
| Helm 调试技巧 | https://helm.sh/docs/chart_template_guide/debugging/ |
| K8s 常见错误 | https://kubernetes.io/docs/tasks/debug/debug-application/ |
| kind 故障排查 | https://kind.sigs.k8s.io/docs/user/known-issues/ |
| APISIX 3.10 release notes | https://github.com/apache/apisix/blob/release/3.10/CHANGELOG.md |

---

> **下一步**：[10 TDD 在 K8s 落地中的实操](./10-tdd-for-k8s.md) —— 为什么我们用 `helm template | grep` 做 K8s 测试，而不是 kubectl apply？