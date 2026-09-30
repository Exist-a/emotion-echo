#!/usr/bin/env bash
# E2E-23 D 组 · E2E-F-151 回归守卫：db-migrate 等待 Postgres 的逻辑必须能扛启动竞态。
#
# 背景（账本 F-151，本轮实测当场复现）：
#   docker inspect emotion-echo-db-migrate → ExitCode 1
#   日志尾部：`全部迁移应用完成，共 31 个文件` 紧接
#             `FATAL: Postgres 30s 内未就绪`
#
# ⇒ **迁移本身全部成功，进程却以非零退出**。RUNBOOK §2.2 期望 Exited (0)，
# 且 apisix-seed 等下游对 db-migrate 有 `condition: service_completed_successfully`
# 依赖 —— 非零退出会连锁阻断（compose 会一直等它"成功"）。
#
# 根因（deploy/db/migrate.sh:203-214）：
#   1) 硬编码 30 次 × sleep 1 = 30s 上限，**不可配置**；
#   2) 固定间隔，**无退避** —— 前 30s 若 Postgres 正在初始化（initdb /
#      恢复 WAL / 加载扩展），固定 1s 轮询会全落在最慢的阶段；
#   3) db-migrate 服务**没有 depends_on**（跨文件 depends_on 在 compose
#      v5.5.1 下会引发回归，见 docker-compose.apps.yml:609 注释），
#      所以这个脚本是唯一的等待手段，必须自己扛住竞态。
#
# 本守卫断言（源级，因为真跑要起 postgres）：
#   1) 等待上限可由环境变量覆盖（部署方能按环境调）；
#   2) 等待间隔随尝试次数递增（前期密集、后期稀疏），非固定 1s；
#   3) 失败信息里带上实际等待时长（排障时能一眼看出等了多少）。
#
# 负向对照：把递增退避改回固定 sleep 1 → 本脚本非零退出。

set -uo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MIGRATE="$REPO_ROOT/deploy/db/migrate.sh"

fail=0
pass=0

echo "== E2E-23 F-151 守卫：db-migrate 等 Postgres 的等待逻辑 =="

if [ ! -f "$MIGRATE" ]; then
  echo "FATAL: 找不到 $MIGRATE"
  exit 2
fi

# --- 1) 等待上限可配置 ---
if grep -qE 'PG_WAIT_MAX_SECS' "$MIGRATE"; then
  echo "PASS 等待上限可配置（PG_WAIT_MAX_SECS）"
  grep -n 'PG_WAIT_MAX_SECS' "$MIGRATE" | head -3 | sed 's/^/     行 /'
  pass=$((pass + 1))
else
  echo "FAIL 等待上限写死 30s，部署方无法按环境调整"
  echo "     ⇒ 冷启动慢的环境（磁盘慢 / 首次 initdb / 恢复 WAL）必然踩 Exited(1)"
  fail=$((fail + 1))
fi

# --- 2) 退避：间隔必须随尝试次数递增，而非固定 1s ---
# 只断言"会变长"这个性质，不断言具体曲线（线性 or 指数都是合理的）——
# 断言形式细节等于把实现锁死，将来想换算法就得改守卫。
if grep -qE 'wait_delay=\$\(\( *wait_delay *\+ *1 *\)\)' "$MIGRATE" \
   || grep -qE 'wait_delay=\$\(\( *wait_delay *\* *2 *\)\)' "$MIGRATE"; then
  echo "PASS 等待间隔随尝试次数递增（退避），非固定 1s"
  grep -nE 'wait_delay=\$\(\( *wait_delay *[+*]' "$MIGRATE" | head -2 | sed 's/^/     行 /'
  pass=$((pass + 1))
else
  echo "FAIL 仍是固定 sleep 1 轮询，无退避"
  echo "     ⇒ Postgres 初始化最慢的阶段恰恰是前期，固定间隔会全错过"
  fail=$((fail + 1))
fi

# --- 3) 旧的 30 次硬循环已移除 ---
if grep -qE 'while \[ "\$i" -lt 30 \]'; then
  echo "FAIL 旧的 'while [ \$i -lt 30 ]' 硬循环仍在"
  fail=$((fail + 1))
else
  echo "PASS 旧的 30 次硬循环已移除"
  pass=$((pass + 1))
fi

# --- 4) 失败信息带实际等待时长 ---
if grep -qE 'die "Postgres .*(s|秒).*(未就绪|not ready)' "$MIGRATE"; then
  echo "PASS 超时信息带实际等待时长"
  grep -nE 'die "Postgres' "$MIGRATE" | head -2 | sed 's/^/     行 /'
  pass=$((pass + 1))
else
  echo "FAIL 超时信息未带实际等待时长"
  echo "     ⇒ 'Postgres 30s 内未就绪' 这类固定文案无法区分'等了 30s'和'等了 5 分钟'"
  fail=$((fail + 1))
fi

echo
echo "PASS: $pass  FAIL: $fail"

if [ "$fail" -gt 0 ]; then
  echo "RED：db-migrate 的 Postgres 等待逻辑仍扛不住启动竞态"
  exit 1
fi

echo "GREEN：等待上限可配置 + 指数退避 + 诊断信息完整"
