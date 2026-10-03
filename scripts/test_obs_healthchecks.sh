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
# APISIX：E2E-23 时只能测端口（镜像内无 HTTP 客户端）——但纯 TCP 探针是假绿：
# 2026-10-02 E2E-25 实测 etcd 停摆时数据面 404/admin 503 而容器仍 healthy。
# E2E-25 #5 改为 bash /dev/tcp 向 admin API 9180 发原始 HTTP GET（带 X-API-KEY），
# 按状态行判 200；实测 etcd 停 → unhealthy(~90s)、恢复 → healthy(~15s)。
# ⚠️ 更新此基准时必须先在跑着的容器上验证新探针真的能通（含负向：etcd 停须翻 unhealthy）。
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
  "apisix:/dev/tcp/127.0.0.1/9180"
  "apisix:X-API-KEY"
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
  # ⚠️ 截止锚必须是"2 空格缩进的服务级键"（/^  [a-z]/）——E2E-25 起探针是
  # 多行块标量，续行以小写字母开头，旧的 [[:space:]]*[a-z] 会在第一行续行
  # 处把探针截断（2026-10-02 实测：apisix 新探针被截成 "healthcheck: test:"）。
  probe="$(echo "$blk" | sed -n '/healthcheck:/,/^  [a-z]/p'            | tr -d '
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
echo "--- 宿主端口冲突检测（E2E-28 实案：--profile dev+obs 同启时撞车）---"
# 实案：E2E-26 D-38 给 sw-ui 加了 127.0.0.1:18080，而 obs-mock-receiver（profile obs）
# 本来就绑 18080:8080 —— infra.yml 注记当时写"届时须改"，E2E-28 开工启用 obs
# profile 即撞车（Bind for 127.0.0.1:18080 failed: port is already allocated）。
# 契约：infra.yml 全部显式宿主端口映射（ip:host:container 或 host:container）
# 的宿主端口全局唯一；随机宿主端口（单段写法）不参与判定。
dups="$(python - "$INFRA" <<'PY'
import re, sys, collections
text = open(sys.argv[1], encoding='utf-8').read()
hosts, in_ports = [], False
for line in text.splitlines():
    stripped = line.split('#')[0].rstrip()
    if not stripped.strip():
        continue
    if re.match(r'^\s*ports:', stripped):
        in_ports = True
        continue
    if in_ports:
        m = re.match(r'^\s+-\s+"([^"]+)"', stripped)
        if m:
            parts = m.group(1).split(':')
            if len(parts) == 3:
                hosts.append(parts[1])
            elif len(parts) == 2:
                hosts.append(parts[0])
        elif not stripped.lstrip().startswith('-'):
            in_ports = False
cnt = collections.Counter(hosts)
print(','.join(f'{p}x{n}' for p, n in sorted(cnt.items()) if n > 1))
PY
)"
if [ -n "$dups" ]; then
  echo "FAIL 宿主端口冲突: $dups —— 两服务绑同一宿主端口，profile 同启必撞车"
  fail=$((fail + 1))
else
  echo "PASS 宿主端口全局唯一（无冲突）"
  pass=$((pass + 1))
fi

echo
echo "PASS: $pass  FAIL: $fail"

if [ "$fail" -gt 0 ]; then
  echo "RED：观测栈/网关健康门禁存在缺口"
  exit 1
fi

echo "GREEN：16 个常驻服务均有 healthcheck，2 个一次性任务未被误加"
