#!/usr/bin/env bash
# E2E-23 C 组 · D-31 回归守卫：NACOS_REQUIRED 必须在编排层显式声明。
#
# 背景（D-31，2026-09-29 用户拍板"编排层显式声明，不改代码默认值"）：
# emotion-llm-service 的 main.py:87-94 在注册失败时默认 log+continue，
# 只有 NACOS_REQUIRED=1 才 raise（fail-fast）。若 prod 编排忘了设这个变量，
# prod 就在**静默降级**：HTTP 端口可达（看起来活着），但 Nacos 里没有实例
# ⇒ APISIX/BFF 解析不到节点 ⇒ 502。与账本 E2E-F-137 完全同型。
#
# 计划期实测：全仓 grep NACOS_REQUIRED **0 命中** —— 连变量名都没出现过，
# 说明这不是"某个环境漏配"，而是**从未被任何编排声明过**。
#
# 本守卫断言两件事：
#   1) dev 编排（docker-compose.apps.yml）显式设 NACOS_REQUIRED="0"
#      —— 让 dev 的"继续运行"成为**明示选择**而非隐式默认
#   2) prod 待办清单（compose.prod.yml）列出了 NACOS_REQUIRED
#      —— 该文件是 ADR-20 决策的"空壳占位"，故意不填具体值，
#         但"哪几项要改"必须写清楚，否则远端部署时会漏
#
# 负向对照：删掉任一处 → 本脚本非零退出。

set -uo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
APPS="$REPO_ROOT/deploy/docker-compose.apps.yml"
PROD="$REPO_ROOT/deploy/compose.prod.yml"

fail=0
pass=0

echo "== E2E-23 D-31 守卫：NACOS_REQUIRED 必须在编排层显式声明 =="

for f in "$APPS" "$PROD"; do
  if [ ! -f "$f" ]; then
    echo "FATAL: 找不到 $f"
    exit 2
  fi
done

# --- 1) dev 编排显式声明 ---
# 用独立的提取脚本而非内嵌 heredoc：`$(...)` 里嵌 python heredoc 在
# Git Bash 下会吞掉 stdin（实测退出码 49、零输出），脚本却继续往下跑，
# 把"没检查到"当成"检查通过"—— 假绿比没有守卫更危险。
llm_block="$(python "$REPO_ROOT/scripts/_extract_compose_block.py" "$APPS" emotion-llm-service)"

if [ -z "$llm_block" ] || [ "$llm_block" = "BLOCK_NOT_FOUND" ]; then
  echo "FAIL dev: 提取 emotion-llm-service 服务块失败（守卫自身故障，非被测对象问题）"
  fail=$((fail + 1))
elif echo "$llm_block" | grep -q 'NACOS_REQUIRED'; then
  echo "PASS dev: emotion-llm-service 显式声明了 NACOS_REQUIRED"
  echo "$llm_block" | grep 'NACOS_REQUIRED' | sed 's/^/     /'
  pass=$((pass + 1))
else
  echo "FAIL dev: emotion-llm-service 未声明 NACOS_REQUIRED"
  echo "     ⇒ dev 的'注册失败继续运行'是隐式默认，而非明示选择；"
  echo "       将来若改 main.py 默认值，dev 行为会静默改变"
  fail=$((fail + 1))
fi

# --- 2) prod 待办清单列出了它 ---
if grep -q 'NACOS_REQUIRED' "$PROD"; then
  echo "PASS prod: compose.prod.yml 的待办清单已列出 NACOS_REQUIRED"
  grep -n 'NACOS_REQUIRED' "$PROD" | sed 's/^/     行 /'
  pass=$((pass + 1))
else
  echo "FAIL prod: compose.prod.yml 的待办清单未列出 NACOS_REQUIRED"
  echo "     ⇒ 该文件是 ADR-20 的'空壳占位'（故意不填值），但漏了这一项"
  echo "       就等于告诉远端部署者'Nacos 注册失败策略 prod 不用管'"
  fail=$((fail + 1))
fi

# --- 3) main.py 的默认值没被改（决策：不动 Python 侧默认行为）---
MAIN="$REPO_ROOT/emotion-llm-service/main.py"
if grep -q 'os.environ.get("NACOS_REQUIRED"' "$MAIN"; then
  echo "PASS main.py: 仍以环境变量读取 NACOS_REQUIRED（未改代码默认值，符合 D-31）"
  pass=$((pass + 1))
else
  echo "FAIL main.py: 读不到 NACOS_REQUIRED 的环境变量读取点"
  echo "     ⇒ 代码可能已被改成别的形式，需回读确认 D-31 是否被绕过"
  fail=$((fail + 1))
fi

echo
echo "PASS: $pass  FAIL: $fail"

if [ "$fail" -gt 0 ]; then
  echo "RED：NACOS_REQUIRED 的编排层显式声明存在缺口"
  exit 1
fi

echo "GREEN：dev/prod/main.py 三处对 NACOS_REQUIRED 的约定一致"
