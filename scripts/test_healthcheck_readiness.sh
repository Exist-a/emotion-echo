#!/usr/bin/env bash
# E2E-23 · 测试点 #9 的回归守卫：compose healthcheck 必须探 readiness 而非 liveness。
#
# 背景（D-29）：6 个 Go 服务的 healthcheck 原先打 /health，而 /health 按决议
# **恒返 200**（保兼容存量消费方）⇒ readiness 端点即使存在，容器健康判定
# 也不会用到它，"依赖挂了就降级"依然传不到编排层。
#
# 为什么用脚本而不是 Go 测试：改的是 compose YAML，不在任何 Go 包内；
# 断言是"文本里有没有出现某个路径"，纯文本检查最直接。
#
# 负向对照：把任一 /health/ready 改回 /health，本脚本必须非零退出。
#   （见 commit 记录：正向 6/6、负向立即红）

set -uo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COMPOSE="$REPO_ROOT/deploy/docker-compose.apps.yml"

# 6 个 Go 服务的容器端口 → 服务名
PORTS=(8888 8890 8893 8889 8891 8894)

fail=0
pass=0

echo "== E2E-23 健康探针守卫：compose healthcheck 应指向 /health/ready =="

if [ ! -f "$COMPOSE" ]; then
  echo "FATAL: 找不到 $COMPOSE"
  exit 2
fi

for port in "${PORTS[@]}"; do
  # 取该端口所在服务块的 healthcheck.test 行
  line="$(grep -n "localhost:${port}/health" "$COMPOSE" | head -1)"

  if [ -z "$line" ]; then
    echo "FAIL [port $port] 找不到指向 localhost:${port}/health 的 healthcheck"
    fail=$((fail + 1))
    continue
  fi

  lineno="${line%%:*}"
  content="${line#*:}"

  case "$content" in
    *"/health/ready"*)
      echo "PASS [port $port] healthcheck 指向 /health/ready（行 $lineno）"
      pass=$((pass + 1))
      ;;
    *"/health ||"*)
      echo "FAIL [port $port] healthcheck 仍指向 /health —— 依赖挂了也不会降级（行 $lineno）"
      fail=$((fail + 1))
      ;;
    *)
      echo "WARN [port $port] 路径形态未识别，需人工确认（行 $lineno）: $content"
      fail=$((fail + 1))
      ;;
  esac
done

# 一次性任务容器（db-migrate / apisix-seed）不应有 healthcheck：
# 它们跑完即退出，healthcheck 对它们无意义。
#
# 用 scripts/_extract_compose_block.py 切块，而不是内嵌 heredoc：
#   - awk 版在"服务名后紧跟注释块"时会把下一个服务的 healthcheck 误算进来
#     （E2E-23 实施期真踩到：脚本误报 2 个 FAIL，实际两个都没有 healthcheck）；
#   - `$(...)` 里嵌 python heredoc 在 Git Bash 下会吞掉 stdin（实测退出码
#     49、零输出），脚本却继续往下跑，把"没检查到"当成"检查通过"——**假绿**。
#     这个坑本轮真的踩过一次：python3 在本机根本不存在，守卫报了假绿。
# 验证工具自身出 bug 会把结论整个搞反，比没有守卫更危险。
one_shot_result=""
one_shot_rc=0
for svc in emotion-echo-db-migrate emotion-echo-apisix-seed; do
  block="$(python "$REPO_ROOT/scripts/_extract_compose_block.py" "$COMPOSE" "$svc")"
  if [ -z "$block" ] || [ "$block" = "BLOCK_NOT_FOUND" ]; then
    one_shot_result="${one_shot_result}FAIL 提取 $svc 服务块失败（守卫自身故障，非被测对象问题）
"
    one_shot_rc=1
    continue
  fi
  if echo "$block" | grep -q 'healthcheck:'; then
    one_shot_result="${one_shot_result}FAIL $svc 一次性任务容器不应有 healthcheck（跑完即退出，探针无意义）
"
    one_shot_rc=1
  fi
done

if [ "$one_shot_rc" -ne 0 ]; then
  while IFS= read -r line; do
    [ -n "$line" ] && { echo "$line"; fail=$((fail + 1)); }
  done <<< "$one_shot_result"
else
  echo "PASS [一次性任务] db-migrate / apisix-seed 均无 healthcheck（符合预期）"
  pass=$((pass + 1))
fi

# ---------------------------------------------------------------------------
# Helm 侧同一契约：readinessProbe 也必须打 /health/ready。
#
# 为什么 compose 全绿还不够（D-29 落地后的实测缺口）：compose 是 dev 栈，
# 生产走 charts/emotion-echo/charts/*/templates/deployment.yaml。E2E-23 只改了
# compose 侧，Helm 的 readinessProbe 仍打 /health —— 后果是 K8s 里
# **DB 挂掉时 Pod 不会被摘出 Endpoints，继续接流量**，liveness/readiness
# 分离在生产等于没做，而且**不会报任何错**（探针返 200，判定"健康"）。
#
# startupProbe / livenessProbe 打 /health 是**正确的**（浅探针，避免依赖抖动
# 触发重启雪崩），本守卫只约束 readinessProbe 这一处。
# ---------------------------------------------------------------------------
echo
echo "-- Helm 侧 readinessProbe --"
CHARTS_DIR="$REPO_ROOT/charts/emotion-echo/charts"
if [ ! -d "$CHARTS_DIR" ]; then
  echo "FATAL: 找不到 charts 目录 $CHARTS_DIR（守卫自身故障）"
  exit 2
fi

# D-29 契约只覆盖**本仓自己实现了 /health/ready 的 6 个 Go 服务**。
# 下面这张表就是契约范围本身 —— 漏了谁、为什么别人不在范围内，都写在这里，
# 不做"扫到啥查啥"（那会把外部组件的 /-/ready 误判成缺陷，也会在范围变化时静默漏检）。
# 端口与上面 compose 段的 PORTS 一一对应。
D29_CHARTS="user-svc chat-svc assessment-svc analytics-svc ai-svc web-bff"
D29_CHARTS_EXPANDED="$D29_CHARTS"
NON_D29_REASON="外部/非 Go 组件：prometheus·alertmanager·loki·grafana 走各自官方 /-/ready 或 /api/health；apisix·web·fer·sensevoice·xtts 无 /health/ready 语义；postgres·redis 用 exec 探针（探 PG 自身而非 HTTP）"

for chart in $D29_CHARTS_EXPANDED; do
  f="$CHARTS_DIR/$chart/templates/deployment.yaml"
  if [ ! -f "$f" ]; then
    echo "FAIL [$chart] D-29 契约内的 chart 不存在（$f）—— 契约范围与实际不符"
    fail=$((fail + 1))
    continue
  fi

  ready_path="$(awk '
    /readinessProbe:/ { inblk = 1; next }
    inblk && /^[[:space:]]{0,10}[a-zA-Z]/ && !/httpGet|path|port|periodSeconds|failureThreshold|timeoutSeconds|initialDelaySeconds|scheme/ { inblk = 0 }
    inblk && /path:/ { print $2; exit }
  ' "$f")"

  case "$ready_path" in
    */health/ready)
      echo "PASS [$chart] readinessProbe → $ready_path（charts/.../$chart）"
      pass=$((pass + 1))
      ;;
    "")
      echo "FAIL [$chart] readinessProbe 块里找不到 path —— 探针路径缺失即等于永远 ready"
      fail=$((fail + 1))
      ;;
    *)
      echo "FAIL [$chart] readinessProbe → $ready_path，应为 /health/ready —— 依赖挂了 Pod 不会被摘流量"
      fail=$((fail + 1))
      ;;
  esac
done

# startupProbe / livenessProbe 必须**保持**浅探针（打 /health）。
# 反向断言：有人"顺手"把 liveness 也改成 readiness，会让 DB 抖动直接重启 Pod（雪崩）。
for chart in $D29_CHARTS_EXPANDED; do
  f="$CHARTS_DIR/$chart/templates/deployment.yaml"
  [ -f "$f" ] || continue
  live_path="$(awk '
    /livenessProbe:/ { inblk = 1; next }
    inblk && /^[[:space:]]{0,10}[a-zA-Z]/ && !/httpGet|path|port|periodSeconds|failureThreshold|timeoutSeconds|initialDelaySeconds|scheme/ { inblk = 0 }
    inblk && /path:/ { print $2; exit }
  ' "$f")"
  case "$live_path" in
    /health)
      echo "PASS [$chart] livenessProbe 保持浅探针 /health（未被误改成 readiness）"
      pass=$((pass + 1))
      ;;
    *)
      echo "FAIL [$chart] livenessProbe → ${live_path:-<无>}，应为 /health —— 依赖抖动会直接重启 Pod"
      fail=$((fail + 1))
      ;;
  esac
done

echo "N/A [Helm 其余 chart] $NON_D29_REASON"

# ---------------------------------------------------------------------------
# 渲染验证：改了 chart 模板必须证明它还能渲染。
#
# 为什么静态 grep 不够：grep 只证明"文本里有 /health/ready"，
# 证明不了 YAML 结构没被改坏。本轮 6 份 deployment.yaml 改完后，
# 第一次实测就是 `helm template` 全 6 个 RENDER OK —— 但那只是**手工跑过一次**，
# 没有留痕就等于下轮会烂掉。守卫要能长期守住，就得每次都渲染。
#
# ⚠️ helm 缺失时的处置：如实报"未验证"并判红，**不静默跳过**。
#    本阶段吃过教训——`python3` 在本机不存在，守卫那段检查从未执行却报 PASS（假绿）。
#    "没验证"和"验证通过"必须能被区分。
# ---------------------------------------------------------------------------
echo
echo "-- Helm 渲染验证 --"
if ! command -v helm >/dev/null 2>&1; then
  echo "FAIL [Helm 渲染] 环境无 helm 可执行文件 ⇒ 本项**未验证**（不是通过）。"
  echo "      装 helm 后重跑本守卫；如需在无 helm 环境放行，请显式设置 SKIP_HELM_RENDER=1 并知悉该项失效。"
  fail=$((fail + 1))
elif [ "${SKIP_HELM_RENDER:-0}" = "1" ]; then
  echo "SKIP [Helm 渲染] 由 SKIP_HELM_RENDER=1 显式跳过 ⇒ 本项**未验证**"
else
  for chart in $D29_CHARTS_EXPANDED; do
    rendered="$(helm template "$chart" "$CHARTS_DIR/$chart" 2>&1)"
    if [ $? -ne 0 ]; then
      echo "FAIL [$chart] helm template 渲染失败（模板被改坏？）"
      printf '%s
' "$rendered" | head -3
      fail=$((fail + 1))
      continue
    fi
    r_path="$(printf '%s\n' "$rendered" | grep -A3 'readinessProbe:' | grep -m1 'path:' | sed 's|.*path: *||' | cut -d'#' -f1 | tr -d ' \r')"
    l_path="$(printf '%s\n' "$rendered" | grep -A3 'livenessProbe:' | grep -m1 'path:' | sed 's|.*path: *||' | cut -d'#' -f1 | tr -d ' \r')"
    # ⚠️ 必须是**精确相等**，不能用 /health/ready* 这种前缀匹配 ——
    #    前缀写法下 `/health/ready-but-wrong` 也会通过（弱断言，AP-01）。
    #    行内注释已在上面 cut -d'#' -f1 去掉。
    if [ "$r_path" != "/health/ready" ]; then
      echo "FAIL [$chart] 渲染产物里 readinessProbe 路径 = '$r_path'（期望精确等于 /health/ready）"
      fail=$((fail + 1)); continue
    fi
    if [ "$l_path" != "/health" ]; then
      echo "FAIL [$chart] 渲染产物里 livenessProbe 路径 = '$l_path'（期望精确等于 /health）"
      fail=$((fail + 1)); continue
    fi
    echo "PASS [$chart] helm template 渲染 OK（ready=$r_path live=$l_path）"
    pass=$((pass + 1))
  done
fi

echo
echo "PASS: $pass  FAIL: $fail"

if [ "$fail" -gt 0 ]; then
  echo "RED：健康探针契约被破坏"
  exit 1
fi

echo "GREEN：compose 6 服务 + Helm 6 服务的 readinessProbe 指向 /health/ready，liveness 保持 /health"
