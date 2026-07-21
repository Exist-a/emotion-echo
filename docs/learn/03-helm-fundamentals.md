# 03 · Helm 模板语言与 chart 架构深入

> 系列：[02 本地集群](./02-local-cluster.md) · **03 Helm 入门** · [04 Umbrella chart](./04-umbrella-chart.md) ...

**适合谁**：第一次写 Helm chart、被 `{{ .Values.x }}` 和 `{{- if }}` 搞晕的读者。
**读完你能**：独立写一个最小可用的 Helm chart，能读懂 Stage 27 我们交付的 17 个 chart 的每一行 yaml，知道为什么 yaml 里写 `{{ }}` 而不是直接写值。

---

## 一句话总结

**Helm = K8s 的包管理工具 = "把一堆 yaml 打包成可复用的组件"。** 类似 `apt`（Debian 系）或 `npm`（Node.js），但 Helm 装的是 K8s 资源。

一个 **chart** = 一组 K8s 资源 yaml + 一个 **values.yaml**（配置变量）+ **模板**（用 Go template 把 values 拼进 yaml）。

---

## 一、为什么需要 Helm

没有 Helm 之前，你装一个 wordpress 大概要写 10 个 yaml（Deployment + Service + ConfigMap + Secret + Ingress + PVC + ServiceAccount + ...）。每个环境（dev/staging/prod）改镜像 tag / 副本数 / 域名都得手动改一遍。

Helm 的解法：
- **chart** = "一个应用的完整 yaml 模板包"
- **values.yaml** = "环境配置变量"
- `helm install wordpress ./chart --values prod.yaml` = 用 prod 配置装

**类比**：

| 工具 | 包格式 | 类比 |
|------|--------|------|
| Debian | `.deb` | apt |
| Node.js | `package.json` + node_modules | npm |
| Docker | `Dockerfile` + image | docker build |
| **Helm** | `Chart.yaml` + templates | helm install |

---

## 二、最简 Helm chart 结构

```bash
$ helm create my-chart
my-chart/
├── Chart.yaml          # chart 元信息（名字、版本、依赖）
├── values.yaml         # 默认配置
├── charts/             # 子 chart（依赖）
└── templates/
    ├── deployment.yaml # Deployment 模板
    ├── service.yaml    # Service 模板
    ├── _helpers.tpl    # 公共模板片段（命名、标签）
    └── NOTES.txt       # 安装完打印给用户的提示
```

运行 `helm install` 时：
1. Helm 读 `values.yaml`（可被 `-f` 覆盖）
2. 把每个 template 文件用 Go template 引擎渲染（替换 `{{ }}`）
3. 把渲染结果 apply 到 K8s

---

## 三、Chart.yaml 详解

```yaml
# Chart.yaml
apiVersion: v2              # Helm 3 用 v2（Helm 2 是 v1）
name: user-svc              # chart 名字
description: User service for Emotion-Echo
type: application           # application | library
version: 0.1.0              # chart 版本（用户改 chart 内容时 +1）
appVersion: "v0.1.0"        # 应用版本（image tag，跟 chart 版本独立）

# 依赖（子 chart）
dependencies:
  - name: postgresql
    version: "12.x.x"
    repository: "https://charts.bitnami.com/bitnami"
    condition: postgresql.enabled
```

| 字段 | 含义 | 我们项目怎么用 |
|------|------|--------------|
| `apiVersion: v2` | Helm 3 | 所有 chart 统一 |
| `name` | chart 名 | 跟 svc 一致（user-svc/chat-svc/...） |
| `version` | chart 版本 | 0.1.0（第一次） |
| `appVersion` | 应用版本 | 跟镜像 tag 一致（v0.1.0） |
| `dependencies` | 子 chart | umbrella 用 |

---

## 四、values.yaml 详解（最重要的一个文件）

```yaml
# charts/emotion-echo/charts/user-svc/values.yaml
replicaCount: 1

image:
  repository: emotion-echo/user-svc
  tag: v0.1.0
  pullPolicy: IfNotPresent

service:
  type: ClusterIP
  port: 8888

resources:
  requests:
    cpu: 100m
    memory: 64Mi
  limits:
    cpu: 500m
    memory: 256Mi

# go-zero 配置（替换 etc/user-api.yaml）
configOverrides:
  Name: user-api
  Host: 0.0.0.0
  Port: 8888

# Secret 注入
secrets:
  postgresDsn: "host=postgres.ee-data user=postgres ..."
```

**使用方式**：

```bash
# 默认 values
helm install user-svc ./chart

# 自定义 values
helm install user-svc ./chart -f values-prod.yaml

# 命令行覆盖单个值
helm install user-svc ./chart --set replicaCount=3 --set image.tag=v0.2.0
```

---

## 五、Go template 语法（最容易出错的部分）

### 5.1 基础替换

```yaml
# templates/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .Release.Name }}-user-svc
spec:
  replicas: {{ .Values.replicaCount }}    # ← 替换为 values.yaml 的值
```

### 5.2 重要的内置对象

| 对象 | 含义 | 示例值 |
|------|------|--------|
| `.Release.Name` | helm install 时给的 release 名 | `ee` |
| `.Release.Namespace` | 部署的命名空间 | `ee-app` |
| `.Values` | values.yaml 合并 -f 后的值 | 字典 |
| `.Chart.Name` | Chart.yaml 的 name | `user-svc` |
| `.Chart.AppVersion` | Chart.yaml 的 appVersion | `v0.1.0` |
| `.Files` | chart 内非 template 文件（用于 ConfigMap 注入） | Get / Glob |

### 5.3 控制流：if / range / with

```yaml
# if 条件
{{- if .Values.configOverrides }}
config:
{{- toYaml .Values.configOverrides | nindent 4 }}
{{- end }}

# range 循环（遍历列表/字典）
{{- range $key, $value := .Values.env }}
- name: {{ $key }}
  value: {{ $value | quote }}
{{- end }}

# with 限定作用域（避免重复写 .Values.x.y）
{{- with .Values.service }}
service:
  type: {{ .type }}
  port: {{ .port }}
{{- end }}
```

### 5.4 函数（pipeline）

```yaml
# quote - 加引号
value: {{ .Values.something | quote }}

# default - 给默认值
value: {{ .Values.something | default "default-value" }}

# toYaml - 字典转 yaml
data:
  config.yaml: |
    {{- toYaml .Values.configOverrides | nindent 4 }}

# nindent - 缩进 N 空格后换行
containerPort: {{ .Values.service.port | default 8080 }}
```

### 5.5 我们项目踩过的坑

| 坑 | 症状 | 解决 |
|---|------|------|
| Flow style YAML `ports: [{ name: http, port: 8000 }]` | `helm template` 报 `did not find expected ',' or '}'` | 改 normal style（一行一个字段） |
| `{{ .Values.global.secrets.x }}` | `helm template` 报 `nil pointer evaluating` | 用 `{{- if and .Values.global .Values.global.secrets .Values.global.secrets.x }}` 链式判断 |
| 字符串里嵌变量 | `{{ .Values.name }}-{{ .Chart.Name }}` 不解析 | 必须用 `printf` 或 `tpl` 函数 |
| 缩进不对 | 渲染出来的 yaml 缩进错乱 | 用 `nindent 4` 而不是手动加空格 |
| 注释 `# {{ .Values.x }}` | 注释里的模板不渲染 | OK，注释里不渲染是正确的 |

---

## 六、_helpers.tpl 详解（最容易复制的一段）

```yaml
# templates/_helpers.tpl
{{/* 生成资源全名：release-name + chart-name */}}
{{- define "user-svc.fullname" -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/* 公共标签 */}}
{{- define "user-svc.labels" -}}
app.kubernetes.io/name: {{ .Chart.Name }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" }}
{{- end -}}
```

**使用**：

```yaml
metadata:
  name: {{ include "user-svc.fullname" . }}
  labels:
    {{- include "user-svc.labels" . | nindent 4 }}
```

**注意**：`include` 不是函数而是**指令**，传 `(模板名, .)`。

---

## 七、helm install 命令详解

```bash
helm install ee ./charts/emotion-echo \
  --namespace ee-app \
  --create-namespace \
  -f charts/emotion-echo/values-dev.yaml \
  --set global.secrets.postgresPassword=devpass \
  --set user-svc.replicaCount=2
```

| 参数 | 含义 |
|------|------|
| `ee` | release 名（一个 chart 可装多份，每份不同 release 名） |
| `./charts/emotion-echo` | chart 路径 |
| `--namespace ee-app` | 装到哪个 ns |
| `--create-namespace` | ns 不存在自动创建 |
| `-f values-dev.yaml` | 用 dev 配置 |
| `--set x=y` | 命令行覆盖单个值 |

---

## 八、Stage 27 chart 实际例子（user-svc 完整）

```yaml
# charts/emotion-echo/charts/user-svc/Chart.yaml
apiVersion: v2
name: user-svc
description: User service for Emotion-Echo
type: application
version: 0.1.0
appVersion: "v0.1.0"

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
  Log:
    Mode: json
    Level: info
```

```yaml
# charts/emotion-echo/charts/user-svc/templates/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ include "user-svc.fullname" . }}
  labels: {{- include "user-svc.labels" . | nindent 4 }}
spec:
  replicas: {{ .Values.replicaCount }}
  selector:
    matchLabels: {{- include "user-svc.selectorLabels" . | nindent 6 }}
  template:
    metadata:
      labels: {{- include "user-svc.selectorLabels" . | nindent 8 }}
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 65532
        fsGroup: 65532
      containers:
        - name: {{ .Chart.Name }}
          image: "{{ .Values.image.repository }}:{{ .Values.image.tag | default .Chart.AppVersion }}"
          imagePullPolicy: {{ .Values.image.pullPolicy }}
          ports:
            - name: http
              containerPort: {{ .Values.service.port }}
          livenessProbe:
            httpGet:
              path: /health
              port: http
            initialDelaySeconds: 15
            periodSeconds: 30
          readinessProbe:
            httpGet:
              path: /health
              port: http
            initialDelaySeconds: 5
            periodSeconds: 10
          resources: {{- toYaml .Values.resources | nindent 12 }}
          envFrom:
            - configMapRef:
                name: {{ include "user-svc.fullname" . }}
            - secretRef:
                name: {{ include "user-svc.fullname" . }}
```

**这段 template 在做什么**：

1. `metadata.name` 用 helper 生成 `ee-user-svc`
2. `replicas` 从 values 读
3. `image` 从 values 拼，fallback 到 appVersion
4. `imagePullPolicy` 从 values 读
5. `containerPort` 从 values 读
6. 探针 path 固定 `/health`
7. `resources` 整块用 toYaml 渲染
8. 环境变量从 ConfigMap + Secret 来（**不在 deployment 里写 env**）

---

## 九、TDD：render-assert 测试怎么写

我们 Stage 27-A 用 Go 集成测试渲染 + 断言：

```go
// k8s/tests/render-assert_test.go
//go:build k8s
package tests

import (
    "os/exec"
    "strings"
    "testing"
)

func TestStage27A_RendersUmbrella(t *testing.T) {
    out, err := exec.Command("helm", "template", "ee", "../../charts/emotion-echo",
        "-f", "../../charts/emotion-echo/values-dev.yaml").CombinedOutput()
    if err != nil {
        t.Fatalf("helm template failed: %v\n%s", err, out)
    }
    rendered := string(out)

    // 断言：至少要看到这些资源
    expectations := []string{
        "kind: Namespace",
        "kind: Deployment",
        "kind: Service",
        "kind: ConfigMap",
        "kind: Secret",
    }
    for _, exp := range expectations {
        if !strings.Contains(rendered, exp) {
            t.Errorf("rendered output missing %q", exp)
        }
    }
}
```

**为什么这样写**：
- **快**：`helm template` 在内存里渲染，不真 apply，1 秒内完成
- **纯静态**：不依赖 K8s 集群，能在 CI 里跑
- **红→绿直观**：先跑测试看缺什么，再补 chart，再跑测试看绿
- **go test 隔离**：`//go:build k8s` 让默认 `go test ./...` 不跑它（避免 CI 没装 helm 的人失败）

---

## 十、本节自检

1. **Helm chart 的 4 个核心文件是什么？**
2. **`.Values` 和 `.Release.Name` 有什么区别？**
3. **`nindent 4` 和 `indent 4` 有什么区别？**
4. **为什么我们要用 ConfigMap 注入配置，而不是 deployment 里 `env: - name: X value: Y`？**
5. **render-assert 测试相比 kubectl apply 测试有什么优势？**

<details>
<summary>📋 参考答案</summary>

1. Chart.yaml（元信息）/ values.yaml（默认值）/ templates/（K8s 资源模板）/ charts/（子 chart）。
2. `.Values` 是用户配置（可被 -f / --set 覆盖）；`.Release.Name` 是 helm install 时给的 release 名（一次 install 一个名）。
3. `nindent 4` 在缩进前先输出换行（适合块开始），`indent 4` 不换行（适合一行内续）。
4. 关注点分离：deployment 只关心镜像/端口/探针，配置放 ConfigMap。改配置不用重新 helm install（kubectl edit cm 即可）。
5. 不依赖真集群；快（1 秒）；可重复；能在 CI 跑；便于红绿循环。

</details>

---

## 十一、推荐阅读

| 资源 | 链接 | 价值 |
|------|------|------|
| Helm 官方文档 | https://helm.sh/docs/ | 权威 |
| Go template 文档 | https://pkg.go.dev/text/template | 完整语法 |
| Helm Best Practices | https://helm.sh/docs/chart_best_practices/ | chart 设计规范 |
| helm template 调试 | https://helm.sh/docs/chart_template_guide/debugging/ | 出错时看 |

---

> **下一步**：[04 Umbrella chart 设计哲学与多环境策略](./04-umbrella-chart.md) —— 我们的 umbrella chart 把 17 个子 chart 拢成一个发布单元。