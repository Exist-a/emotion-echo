#!/usr/bin/env bash
# E2E-23 D 组 · 测试点 #25/#26 的回归守卫：观测栈与网关必须有 healthcheck。
#
# 背景（账本 E2E-F-151 + E2E-F-152）：
#   F-151 记录"六个观测服务全部无 healthcheck"（本轮实测扩大到 11 个常驻
#   服务，其中包括**全站入口 APISIX 自己**）。
#   F-152 记录 2026-09-29T03:12:40Z 六个容器同刻 Exited(255) 且**零告警** ——
#   发告警的东西自己死了，无人知晓。零 healthcheck 正是这条链断掉的根因之一。
#
# 本守卫断言 infra.yml 里每个常驻服务都有 healthcheck。
# 排除一次性任务容器（kafka-init / minio-init / apisix-seed / db-migrate）：
# 它们跑完即退出，探针对它们无意义（反而会让 compose 永远等它们"健康"）。
#
# 端点可信度：prometheus/alertmanager/grafana/loki/kafka-exporter/promtail
# 六者的健康端点均于 2026-09-29 计划期**逐个实测可达**后才写入
# （docker exec + wget 实际请求），非照抄文档。
# APISIX 只能测端口 —— 镜像内无 wget/curl/nc/busybox，openresty 的 resty
# 跑不通，且未启用 healthcheck/public-api 插件（/apisix/status 实测 404）。
# 该局限已在 compose 注释里如实记录。
#
# 负向对照：删掉任一 healthcheck → 本脚本非零退出。

set -uo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
INFRA="$REPO_ROOT/deploy/docker-compose.infra.yml"

fail=0
pass=0

echo "== E2E-23 观测栈 healthcheck 守卫 =="

if [ ! -f "$INFRA" ]; then
  echo "FATAL: 找不到 $INFRA"
  exit 2
fi

# 必须有 healthcheck 的常驻服务 → **探针里必须出现的关键串**。
#
# ⚠️ 为什么不能只查 `healthcheck:` 键存在（第二方核对 2026-09-30 实测的洞）：
#   把 grafana 的探针端口从 3000 改成永远不通的 9999，守卫照样 `PASS: 18 FAIL: 0` GREEN
#   —— 它只防"回归删掉 healthcheck"，不防"写错端点"。而端点写错的后果是
#   **容器永远 unhealthy，且没有任何错误信息**。
#   故这里把每个探针的关键串（端口 + 路径/子命令）固化成基准表，改错即红。
#
# 基准来源：**2026-09-30 在真实运行的 19 容器栈上逐个实测可达**后写定，
# 不是照抄官方文档；这 16 项当时全部 `Up (healthy)`。
#   ⚠️ 改任何一条探针前，先在跑着的容器上验证新探针真的能通，再同步改这里。
MUST_HAVE=(
  "postgres:pg_isready"
  "redis:redis-cli"
  "kafka:kafka-topics.sh --bootstrap-server localhost:9092"
  "nacos:http://localhost:8848/nacos/actuator/health"
  "emotion-echo-minio:http://localhost:9000/minio/health/live"
  "etcd:etcdctl endpoint health"
  "apisix:/dev/tcp/127.0.0.1/9080"
  "prometheus:http://127.0.0.1:9090/-/healthy"
  "alertmanager:http://127.0.0.1:9093/-/healthy"
  "grafana:http://127.0.0.1:3000/api/health"
  "loki:http://127.0.0.1:3100/ready"
  "promtail:http://127.0.0.1:9080/ready"
  "kafka-exporter:http://127.0.0.1:9308/metrics"
  # E2E-23 #26 补：plan D2 点名但首轮遗漏的三个（第二方核对 C-6 抓出）
  "skywalking-oap:http://127.0.0.1:1234/metrics"
  "skywalking-ui:http://127.0.0.1:8080/"
  "obs-mock-receiver:http://127.0.0.1:8080/received"
)

# 一次性任务容器：不应有 healthcheck
ONE_SHOT=(
  "kafka-init"
  "emotion-echo-minio-init"
)

block_of() {
  python "$REPO_ROOT/scripts/_extract_compose_block.py" "$INFRA" "$1"
}

echo
echo "--- 常驻服务（应各有 healthcheck）---"
for entry in "${MUST_HAVE[@]}"; do
  svc="${entry%%:*}"
  want="${entry#*:}"
  blk="$(block_of "$svc")"
  if [ -z "$blk" ] || [ "$blk" = "BLOCK_NOT_FOUND" ]; then
    echo "FAIL $svc: 提取服务块失败（守卫自身故障，非被测对象问题）"
    fail=$((fail + 1))
    continue
  fi
  if ! echo "$blk" | grep -q 'healthcheck:'; then
    echo "FAIL $svc: 缺 healthcheck —— 挂了也没人知道"
    fail=$((fail + 1))
    continue
  fi
  # 端点校验：探针命令行必须含基准关键串（端口+路径/子命令）。
  # 用包含匹配而非精确相等：compose 的 test 是数组、跨行、行内还有注释，
  # 精确相等会把格式调整也判红。
  # 归一化：去掉引号/逗号/方括号并压掉空白，让 JSON 数组式探针
  # （["CMD","etcdctl","endpoint","health"]）与 shell 式探针都能连续匹配同一关键串。
  probe="$(echo "$blk" | sed -n '/healthcheck:/,/^[[:space:]]*[a-z]/p'            | tr -d '
"' | tr -d '[],' | tr -s ' ')"
  case "$probe" in
    *"$want"*)
      echo "PASS $svc: healthcheck 存在且端点含 [$want]"
      pass=$((pass + 1))
      ;;
    *)
      echo "FAIL $svc: 探针端点与基准不符 —— 未找到 [$want]"
      echo "       实际: $probe" | head -c 300
      echo
      fail=$((fail + 1))
      ;;
  esac
done

echo
echo "--- 一次性任务（不应有 healthcheck）---"
for svc in "${ONE_SHOT[@]}"; do
  blk="$(block_of "$svc")"
  if [ -z "$blk" ] || [ "$blk" = "BLOCK_NOT_FOUND" ]; then
    echo "WARN $svc: 服务块不存在（可能已改名），跳过"
    continue
  fi
  if echo "$blk" | grep -q 'healthcheck:'; then
    echo "FAIL $svc: 一次性任务加了 healthcheck —— 跑完即退出，探针只会让 compose 永远等它健康"
    fail=$((fail + 1))
  else
    echo "PASS $svc: 无 healthcheck（符合预期）"
    pass=$((pass + 1))
  fi
done

echo
echo "PASS: $pass  FAIL: $fail"

if [ "$fail" -gt 0 ]; then
  echo "RED：观测栈/网关健康门禁存在缺口"
  exit 1
fi

echo "GREEN：16 个常驻服务均有 healthcheck，2 个一次性任务未被误加"
