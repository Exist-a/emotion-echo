# 10 · TDD 在 K8s 落地中的实操（render 断言测试 / 红绿循环）

> 系列：[09 踩坑全记录](./09-stage-27-pitfalls.md) · **10 TDD 落地** · [11 docker-compose → K8s 映射](./11-compose-to-k8s.md) ...

**适合谁**：第一次把 TDD 套到 K8s（Helm chart）上的人，困惑"测试 chart 到底测什么"的读者。
**读完你能**：解释"为什么用 helm template 不用 kubectl apply"，能自己写一个 render-assert 集成测试，能把 K8s 项目拆成多个 TDD 循环。

---

## 一句话总结

**K8s 项目的 TDD 不是"kubectl apply + 看输出"，而是"`helm template` 渲染后用 grep/断言检查输出"**。我们 Stage 27 用 `//go:build k8s` 隔离的 Go 集成测试，2 秒一轮红绿。

---

## 一、AGENTS.md 第一性原则的约束

> **从此刻起，对本项目任何一行新代码（含 AI 自动生成 / 人类提交），都必须先写测试。**

这条对 K8s / Helm chart 也一样适用。**新写一个 chart = 必须先写一个失败的测试**，然后再写 chart 让测试绿。

但 K8s 的"测试"跟 Go 单元测试不一样。

---

## 二、K8s 项目测什么？怎么测？

### 2.1 可选的 4 种测试

| 测试类型 | 速度 | 真实性 | 适合 |
|---------|------|--------|------|
| **render-assert（静态）** | < 2 秒 | ❌ 不真跑 | chart template 完整性 |
| **kubeval / kubeconform** | < 5 秒 | ✅ schema 校验 | K8s manifest schema 正确 |
| **helm-unittest** | < 5 秒 | ⚠️ 部分 | 单 template 单元 |
| **end-to-end（apply + curl）** | 30 秒 - 5 分钟 | ✅ 完全真 | 全链路集成 |

我们 Stage 27 选 **render-assert** + **end-to-end smoke** 的组合。

### 2.2 为什么首选 render-assert

| 优势 | 解释 |
|------|------|
| **极快** | 2 秒一轮红绿循环 |
| **无依赖** | 不需要 K8s 集群、Docker 镜像 |
| **CI 友好** | 在 GitHub Actions / GitLab CI 上一秒跑完 |
| **隔离明确** | `//go:build k8s` 让默认 `go test ./...` 不跑它 |
| **白盒断言** | 能检查 namespace 名、label、image tag 等细节 |

### 2.3 什么时候必须用 end-to-end

- 跨 Pod 网络（user-svc → postgres）
- 探针实际生效
- StatefulSet 持久化
- 实际流量跑通

---

## 三、render-assert 测试结构

### 3.1 完整测试代码（Stage 27-A）

```go
// D:/源码/Emotion-Echo/k8s/tests/render-assert_test.go
//go:build k8s
// +build k8s

package tests

import (
    "os/exec"
    "path/filepath"
    "strings"
    "testing"
)

// 工具函数
func render(t *testing.T, extraArgs ...string) string {
    chartPath, err := filepath.Abs("../../charts/emotion-echo")
    if err != nil {
        t.Fatal(err)
    }

    args := []string{"template", "ee", chartPath, "-f", "../../charts/emotion-echo/values-dev.yaml"}
    args = append(args, extraArgs...)

    out, err := exec.Command("helm", args...).CombinedOutput()
    if err != nil {
        t.Fatalf("helm template failed: %v\n%s", err, out)
    }
    return string(out)
}

// 测试 1：umbrella 渲染出来至少有基本资源
func TestStage27A_RendersUmbrella(t *testing.T) {
    rendered := render(t)

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

// 测试 2：17 个子 chart 都参与渲染
func TestStage27A_SubChartsPresent(t *testing.T) {
    rendered := render(t)

    expectedSubCharts := []string{
        "ee-app/postgres",          // StatefulSet namespace 路径
        "ee-data/postgres",
        "ee-app/user-svc",
        "ee-app/chat-svc",
        "ee-app/ai-svc",
        "ee-app/llm-service",
        "ee-app/web",
        "ee-system/apisix",
    }

    for _, sub := range expectedSubCharts {
        if !strings.Contains(rendered, sub) {
            t.Errorf("missing subchart namespace %q", sub)
        }
    }
}

// 测试 3：16 条 ApisixRoute + 6 个 ApisixUpstream
func TestStage27A_APISIXRoutes(t *testing.T) {
    rendered := render(t)

    expectedRoutes := []string{
        "r-user-me",
        "r-user-by-id",
        "r-user-update",
        "r-conv-create",
        "r-msg-list",
        "r-msg-send",
        "r-analytics-health",
        "r-surveys",
        "r-survey-get",
        "r-survey-submit",
        "r-survey-results-list",
        "r-survey-results-get",
        "r-emotion-by-msg",
        "r-emotion-by-conv",
        "r-ai-health",
        "r-ping",
    }

    routeCount := strings.Count(rendered, "kind: ApisixRoute")
    if routeCount < 16 {
        t.Errorf("expected 16 ApisixRoute, got %d", routeCount)
    }

    upstreamCount := strings.Count(rendered, "kind: ApisixUpstream")
    if upstreamCount < 6 {
        t.Errorf("expected 6 ApisixUpstream, got %d", upstreamCount)
    }

    for _, route := range expectedRoutes {
        if !strings.Contains(rendered, route) {
            t.Errorf("missing ApisixRoute %q", route)
        }
    }
}

// 测试 4：helm lint 全部通过
func TestStage27A_LintPasses(t *testing.T) {
    chartPath, _ := filepath.Abs("../../charts/emotion-echo")
    out, err := exec.Command("helm", "lint", chartPath).CombinedOutput()
    if err != nil {
        t.Fatalf("helm lint failed: %v\n%s", err, out)
    }
    if !strings.Contains(string(out), "no failures") {
        t.Errorf("helm lint warnings: %s", out)
    }
}
```

### 3.2 build tag 怎么用

```go
//go:build k8s
// +build k8s
```

- **默认 `go test ./...`** 不跑（CI 里没装 helm 的人也能跑过）
- **`go test -tags k8s ./...`** 才跑（CI 里专门一个 job 跑 K8s 测试）

---

## 四、TDD 循环实操（一个 stage 一个循环）

### Stage 27-A · 集群 + umbrella 骨架

```
🔴 RED：先写 render-assert_test.go（4 个测试）
   $ go test -tags k8s ./k8s/tests/...
   FAIL: missing Deployment/Service/etc

🟢 GREEN：写最小 umbrella Chart.yaml + 17 个子 chart Chart.yaml + values.yaml
   $ go test -tags k8s ./k8s/tests/...
   PASS (4/4)

♻️ REFACTOR：抽 _helpers.tpl + 公共 labels
   $ go test -tags k8s ./k8s/tests/...
   PASS (4/4)
```

### Stage 27-B · 数据层

```
🔴 RED：在 render-assert 加 5 个数据层断言
   func TestStage27B_DataLayerResources(t *testing.T) {
       rendered := render(t)
       // 断言 postgres StatefulSet + PVC + headless Service
       // 断言 kafka StatefulSet + advertised listener
       // 断言 redis Deployment + PVC
       // 断言 etcd StatefulSet
       // 断言 skywalking StatefulSet + h2 配置
   }
   FAIL

🟢 GREEN：写 5 个数据层子 chart
   $ go test -tags k8s ./k8s/tests/...
   PASS

♻️ REFACTOR：合并通用 StatefulSet 模板（PVC、headless、probes）
   PASS
```

### Stage 27-C · 业务 svc（最复杂）

```
🔴 RED：每个 svc 一个 assert 子测试
   func TestStage27C_BusinessServices(t *testing.T) {
       rendered := render(t)
       // 断言 user-svc 有 startupProbe + readinessProbe + livenessProbe
       // 断言 chat-svc KAFKA_BROKERS 指向完整 FQDN
       // 断言 ai-svc 挂载 mTLS Secret
       // 断言所有 svc 用 imagePullPolicy: IfNotPresent
       // 断言所有 svc securityContext.runAsNonRoot: true
   }
   FAIL

🟢 GREEN：写 7 个 svc 子 chart + ConfigMap 转换 + Secret 注入
   PASS

♻️ REFACTOR：抽通用 probe / resources / securityContext helper
   PASS
```

### Stage 27-D / 27-E / 27-F / 27-G

每个 stage 独立循环，加新断言 → 写新 chart → 加 refactor。

---

## 五、End-to-End 冒烟测试（k8s/scripts/06-smoke.sh）

```bash
#!/bin/bash
# k8s/scripts/06-smoke.sh
set -e

GATEWAY="http://localhost:9080"

echo "=== 1. 检查 16 条 APISIX route 全部 200 ==="
for route in \
    "/api/v1/users/me" \
    "/api/v1/users/1" \
    "/api/v1/conversations" \
    "/api/v1/conversations/1/messages" \
    "/analytics-health" \
    "/api/v1/surveys" \
    "/api/v1/surveys/results" \
    "/api/v1/emotion/message/1" \
    "/api/v1/emotion/conversation/1" \
    "/ai-health" \
    "/ping"; do
    code=$(curl -s -o /dev/null -w "%{http_code}" "$GATEWAY$route" || echo "000")
    echo "  $code $route"
done

echo "=== 2. 直接调 4 Go svc ==="
for svc in user-svc:8888 chat-svc:8890 analytics-svc:8893 assessment-svc:8889; do
    code=$(kubectl exec -n ee-app deploy/user-svc -- wget -q -O - http://$svc/health >/dev/null 2>&1 && echo 200 || echo 000)
    echo "  $code $svc/health"
done
```

---

## 六、CI/CD 怎么配

### 6.1 GitHub Actions 示例

```yaml
# .github/workflows/k8s-test.yml
name: K8s chart tests
on: [push, pull_request]

jobs:
  render-assert:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: azure/setup-helm@v4
        with:
          version: v3.19.0
      - uses: actions/setup-go@v5
        with:
          go-version: '1.23'
      - run: go test -tags k8s ./k8s/tests/...
```

### 6.2 kubeval 静态校验（额外一层）

```yaml
- name: Install kubeval
  run: |
    curl -LO https://github.com/instrumenta/kubeval/releases/latest/download/kubeval-linux-amd64.tar.gz
    tar xf kubeval-linux-amd64.tar.gz
    sudo mv kubeval /usr/local/bin/

- name: Static validation
  run: |
    helm template ee ./charts/emotion-echo -f ./charts/emotion-echo/values-dev.yaml > /tmp/rendered.yaml
    kubeval /tmp/rendered.yaml --strict
```

---

## 七、TDD 节奏实战建议

### 7.1 一个 PR 一个 TDD 循环

| Stage | 内容 | PR 数量 |
|-------|------|--------|
| 27-A | umbrella + kind | 1 PR |
| 27-B | 数据层 5 组件 | 1 PR |
| 27-C | 业务 svc 7 个 | 1 PR |
| 27-D | AI 模型 3 个 | 1 PR |
| 27-E | Web | 0.5 PR（合到 27-C） |
| 27-F | APISIX | 1 PR |
| 27-G | smoke + docs | 1 PR |

### 7.2 commit 格式

```
test(stage-27-A): add render-assert for umbrella chart
feat(stage-27-A): umbrella chart with 17 subchart stubs
refactor(stage-27-A): extract common labels helper
```

### 7.3 关键纪律

1. **测试先写，chart 后写** —— 永远不要先写 chart 再补测试
2. **测试一次只加 1-2 个断言** —— 增量推进，避免一次写 10 个断言失败时无法定位
3. **每个 commit 必须绿** —— 不要让 main 分支红
4. **重构前先绿** —— 任何 refactor commit 都必须保持测试全绿

---

## 八、常见反模式（我们避开过）

### 反模式 1：测试只覆盖 happy path

```go
// 错
func TestPod(t *testing.T) {
    pod := render()
    if !strings.Contains(pod, "kind: Pod") {
        t.Fail()
    }
}

// 对
func TestPod(t *testing.T) {
    pod := render()
    // happy: Pod 存在
    // 边界: Pod 有正确的 label
    // 边界: Pod 有正确的 image
    // 边界: Pod 有正确的 resources
    // 边界: Pod 有正确的 probes
    // 边界: Pod 有正确的 securityContext
}
```

### 反模式 2：测试是真集群 apply

```go
// 错（30-300 秒 + 需要 Docker + 需要 K8s）
func TestDeployment(t *testing.T) {
    kubectl("apply", "-f", "deployment.yaml")
    time.Sleep(30 * time.Second)
    pods := kubectl("get", "pods")
    if !strings.Contains(pods, "Running") { t.Fail() }
}

// 对（2 秒 + 零依赖）
func TestDeployment(t *testing.T) {
    rendered := helmTemplate("ee", "./chart")
    if !strings.Contains(rendered, "kind: Deployment") { t.Fail() }
}
```

### 反模式 3：测试互依赖

```go
// 错（共享全局变量）
var sharedRendered string

func TestA(t *testing.T) {
    sharedRendered = render()
}

func TestB(t *testing.T) {
    // 假设 TestA 先跑
    if strings.Contains(sharedRendered, "x") { t.Fail() }
}

// 对（每个 test 独立渲染）
func TestB(t *testing.T) {
    rendered := render()
    if strings.Contains(rendered, "x") { t.Fail() }
}
```

---

## 九、本节自检

1. **为什么 K8s 测试首选 render-assert 而不是 kubectl apply？**
2. **`//go:build k8s` 这个 build tag 有什么用？**
3. **一个 TDD 循环的 3 个阶段分别是什么？**
4. **什么时候必须用 end-to-end 测试？**
5. **commit 格式 `test:` / `feat:` / `refactor:` 的顺序含义是什么？**

<details>
<summary>📋 参考答案</summary>

1. 速度快（2 秒 vs 30 秒+）、无依赖（不需要 K8s 集群/镜像）、CI 友好、白盒断言。
2. 默认 go test 不跑（避免 CI 没装 helm 失败）；专门 go test -tags k8s 才跑。
3. RED 写失败测试 → GREEN 写最小实现让测试绿 → REFACTOR 改进实现保持绿。
4. 跨 Pod 网络、探针实际生效、StatefulSet 持久化、真实流量验证。
5. test 先写（红）→ feat 写最小实现（绿）→ refactor 重构（保持绿）。

</details>

---

## 十、推荐阅读

| 资源 | 链接 |
|------|------|
| helm-unittest | https://github.com/helm-unittest/helm-unittest |
| kubeval | https://www.kubeval.com/ |
| kubeconform | https://github.com/yannh/kubeconform |
| Test Pyramid（Martin Fowler）| https://martinfowler.com/articles/practical-test-pyramid.html |
| K8s Testing Guide | https://kubernetes.io/docs/tasks/access-application-cluster/ |

---

> **下一步**：[11 从 docker-compose 到 K8s 的逐项映射手册](./11-compose-to-k8s.md) —— 你已经有 docker-compose 文件时，怎么"翻译"成 Helm chart？