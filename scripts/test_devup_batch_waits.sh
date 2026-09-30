#!/usr/bin/env bash
# E2E-23 D 组 · 测试点 #29 的回归守卫：dev-up.sh 每批必须等本批实际起来的对象。
#
# 背景（计划期实测发现的 copy-paste 错误）：
#   批1 起 postgres etcd redis        → wait_healthy postgres 60
#   批2 起 kafka nacos minio apisix   → wait_healthy postgres 30   ✗ 错配
#        postgres 属批1 且已等过 60s，这里又等了个无关的对象；
#        而本批真正需要等的 kafka / apisix 反而没等。
#   批3 起 7 个业务 svc               → 只 wait_healthy web-bff    ✗ 覆盖不足
#
# 为什么"只等一个"是问题：批 3.5 的 Nacos 注册校验能发现"服务没注册"，
# 但发现不了"注册了、/health/ready 却返 503"（依赖降级）——
# **两者是不同的失败面**，前者是服务发现，后者是服务健康。
#
# 解析逻辑在 scripts/_check_devup_batches.py（独立文件，避免 heredoc
# 嵌套在 $() 里被吞 stdin 造成假绿）。
#
# 负向对照：把批2 改回 `wait_healthy postgres 30` → 本脚本非零退出。

set -uo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEVUP="$REPO_ROOT/scripts/dev-up.sh"
CHECKER="$REPO_ROOT/scripts/_check_devup_batches.py"

fail=0
pass=0

echo "== E2E-23 dev-up.sh 分批等待守卫 =="

for f in "$DEVUP" "$CHECKER"; do
  if [ ! -f "$f" ]; then
    echo "FATAL: 找不到 $f"
    exit 2
  fi
done

# 1) 语法层
if sh -n "$DEVUP" 2>/dev/null; then
  echo "PASS dev-up.sh 语法合法（sh -n）"
  pass=$((pass + 1))
else
  echo "FAIL dev-up.sh 语法错误"
  fail=$((fail + 1))
fi

# 2) 语义层
out="$(python "$CHECKER" "$DEVUP" 2>&1)"
rc=$?

if [ "$rc" -ne 0 ] || [ -z "$out" ]; then
  # 检查器自身失败必须报红 —— 假绿比没有守卫更危险
  echo "FAIL 检查器执行失败（rc=$rc），本项无法判定："
  echo "$out" | sed 's/^/     /'
  fail=$((fail + 1))
else
  checked="$(echo "$out" | grep -oE 'CHECKED=[0-9]+' | cut -d= -f2)"
  batches="$(echo "$out" | grep -oE 'BATCHES=[0-9]+' | cut -d= -f2)"
  problems="$(echo "$out" | grep '^PROBLEM' | sed 's/^PROBLEM /FAIL /')"

  echo "     （解析出 $batches 批，校验 $checked 个"起服务-必须被等"的关系）"

  if [ -n "$problems" ]; then
    while IFS= read -r line; do
      [ -n "$line" ] && { echo "$line"; fail=$((fail + 1)); }
    done <<< "$problems"
  else
    echo "PASS 每批 up 的服务在本批内都被等待覆盖，且无跨批错配"
    pass=$((pass + 1))
  fi
fi

# 3) wait_healthy 必须能处理 container_name 前缀（本轮抓到的更深缺陷）
#    compose 里多数服务设了 `container_name: emotion-echo-<svc>`，而
#    `docker inspect` 只认实际对象名。原实现直接传服务名，对这些服务
#    恒返回 "no such object" ⇒ 永远超时并误报"服务起不来"。
#    实测：postgres 因未覆盖 container_name 恰好能用，掩盖了该缺陷。
if grep -qF 'docker inspect "emotion-echo-$name"' "$DEVUP"; then
  echo "PASS wait_healthy 能回退到 emotion-echo- 前缀的容器名"
  pass=$((pass + 1))
else
  echo "FAIL wait_healthy 未处理 container_name 前缀"
  echo "     ⇒ 对设了 container_name: emotion-echo-* 的服务恒返回 no such object，"
  echo "       表现为'服务起不来'，实际是探针自己坏了"
  fail=$((fail + 1))
fi

echo
echo "PASS: $pass  FAIL: $fail"

if [ "$fail" -gt 0 ]; then
  echo "RED：dev-up.sh 的分批等待存在错配或覆盖缺口"
  exit 1
fi

echo "GREEN：分批等待语义正确"
