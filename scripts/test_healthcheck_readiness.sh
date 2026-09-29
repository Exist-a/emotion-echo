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

echo
echo "PASS: $pass  FAIL: $fail"

if [ "$fail" -gt 0 ]; then
  echo "RED：健康探针契约被破坏"
  exit 1
fi

echo "GREEN：6 个服务 healthcheck 均指向 readiness，且一次性任务未被误加探针"
