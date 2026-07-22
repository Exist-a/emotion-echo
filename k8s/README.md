# Emotion-Echo · Local K8s (kind + Helm umbrella + APISIX Ingress)

**Stage 27 入口文档**。本目录把 Stage 20-26 已经容器化的全栈通过 **Helm umbrella chart** 部署到本地 **kind** 集群，并用 **APISIX Ingress Controller 3.10+**（修复 docker-compose 时代 3.9 的 nginx 301 bug）作为统一接入层。

完整策略：见 [`docs/stage-21-k8s-strategy.md`](../docs/stage-21-k8s-strategy.md)
本批次交付：见 [`docs/stage-27-k8s-local-helm.md`](../docs/stage-27-k8s-local-helm.md)

---

## 一、前置依赖（Windows / Git Bash）

| 工具 | 用途 | 安装命令 |
|------|------|----------|
| Docker Desktop | kind 容器运行时 | <https://www.docker.com/products/docker-desktop/> |
| kubectl | K8s CLI | `winget install Kubernetes.kubectl`（已自带在 Docker Desktop） |
| kind 0.27+ | 本地 K8s 集群 | 下载到 `~/bin/kind.exe` 或 `winget install Kubernetes.kind` |
| helm 3.x | Chart 渲染器 | 下载到 `~/bin/helm.exe` 或 `winget install Helm.Helm` |

> **如本机未装**：把 `kind.exe` 和 `helm.exe` 放到 `C:\Users\<you>\bin\`，然后在 Git Bash 里 `export PATH="/c/Users/<you>/bin:$PATH"`。

---

## 二、一键启动（5 步）

```bash
# 0. 跳到本目录
cd k8s/scripts

# 1. 启 kind 集群（1 control-plane + 2 worker，约 30s）
bash 01-create-cluster.sh

# 2. 把本地镜像灌进 kind（避免 docker push）
#    前置：每个 svc 的 v0.1.0 镜像已本地 build
bash 02-load-images.sh

# 3. 装 APISIX CRDs（3.10+ 修复 nginx 301 bug）
bash 03-install-ingress.sh

# 4. helm install 整套 umbrella
bash 04-install-chart.sh

# 5. 把关键端口暴露到宿主机（后台）
bash 05-port-forward.sh

# 6. 跑冒烟
bash 06-smoke.sh
```

然后浏览器打开：
- <http://localhost:3000> — Web 前端（经 APISIX 转发到 4 Go svc）
- <http://localhost:9080> — APISIX gateway（直接 curl 探活）
- <http://localhost:18080> — SkyWalking UI（tracing 观察）

---

## 三、目录结构

```
k8s/
├── README.md                       # 本文件
├── kind-config.yaml                # kind 集群定义（1 cp + 2 worker）
├── SMOKE-REPORT.md                 # 06-smoke.sh 生成的报告（运行后存在）
├── scripts/
│   ├── 01-create-cluster.sh
│   ├── 02-load-images.sh
│   ├── 03-install-ingress.sh
│   ├── 04-install-chart.sh
│   ├── 05-port-forward.sh
│   ├── 06-smoke.sh
│   └── 99-teardown.sh
└── tests/
    ├── go.mod
    ├── render_assert_test.go       //go:build k8s  — Stage 27-A 渲染断言
    └── README.md
```

---

## 四、TDD 与测试

```bash
# 默认不跑（//go:build k8s 隔离）
go test ./...

# 显式跑 K8s 集成测试（需 helm）
cd k8s/tests
go test -tags k8s -v
```

Stage 27-A 阶段测试：
- `TestStage27A_RendersUmbrella` — 渲染出 4 个 namespace
- `TestStage27A_SubChartsPresent` — ≥10 Deployments + ≥10 Services + ≥5 ConfigMaps + ≥5 Secrets
- `TestStage27A_APISIXRoutes` — 恰好 16 ApisixRoute + 6 ApisixUpstream
- `TestStage27A_LintPasses` — `helm lint` 全绿

---

## 五、收尾

```bash
bash 99-teardown.sh     # 一键：杀 port-forward + helm uninstall + kind delete
```

---

## 六、已知遗留（Stage 28+）

- APISIX Ingress Controller helm install 在 kind 第一次会拉取 `apisix-ingress-controller` 镜像（~300MB），如果 Docker Hub 不通，需配国内 mirror
- ai-svc/llm-service 的 mTLS 证书当前是占位 `REPLACE_WITH_*`，**接入真实部署前需替换**
- SkyWalking OAP 当前用 h2 内存存储，**生产前换 ES/BanyanDB**
- Postgres 单 StatefulSet 无 HA；Stage 28+ 用 Operator / RDS
- **未做**：HPA / PDB / NetworkPolicy / ServiceMonitor / cert-manager / ArgoCD — 见 Stage 28 计划