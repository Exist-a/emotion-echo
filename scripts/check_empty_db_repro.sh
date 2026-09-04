#!/usr/bin/env bash
# check_empty_db_repro.sh — db-migrate 闭环一键复现脚本
#
# 用途：从零环境验证 db-migrate 容器机制 + 14 条迁移契约 + 数据契约 smoke 全链路可用。
# 替代 todo-pile-2026-09-04.md §F.6 的人工命令序列。
#
# 流程（每阶段输出阶段标签）：
#   [A] docker compose down -v --remove-orphans  全清数据卷
#   [B] docker compose up -d                     拉起全栈
#   [C] 等 emotion-echo-db-migrate exited + exit_code=0（init 容器一次性，跑完即退）
#   [D] 等 emotion-echo-postgres healthy
#   [E] 等 5 业务 svc 全 healthy（容忍 start_period=60s）
#   [F] bash deploy/db/test_migrations_contract.sh  （14 条契约）
#   [G] python scripts/smoke_data_layer.py         （§契约 1-6 数据契约）
#   [H] 汇总 contract_rc + smoke_rc → exit 0/1/2
#
# 退出码：
#   0 = 闭环通过（契约 + smoke 全 PASS）
#   1 = 闭环失败（契约或 smoke 至少一项 FAIL）
#   2 = 启动期 FATAL（db-migrate / postgres 起不来 / 健康检查超时）
#
# 前置：docker compose v2、python3 或 Windows 真 Python 路径
#
# 用法（⚠️ 危险脚本，详见决策 18 §二 实例 #15）：
#   bash scripts/check_empty_db_repro.sh --execute      # 真跑（含 down -v 销毁数据卷）
#   bash scripts/check_empty_db_repro.sh --dry-run      # 只打印计划，不执行（默认）
#   bash scripts/check_empty_db_repro.sh --help         # 看帮助
#
# 之所以默认 --dry-run：本脚本阶段 A 执行 `docker compose down -v --remove-orphans`，
# 会**销毁所有命名数据卷**（postgres / redis / kafka / nacos 等）。dev 环境下用户的累积
# 数据全部丢失。必须显式 --execute 才允许跑。
#
# 总时间预算：2-4 分钟（db-migrate 30s + postgres 5s + 业务 svc 60-120s + 契约 20s + smoke 30s）

set -uo pipefail   # 不用 -e：分阶段捕获 rc，smoke 失败要收集证据再退出

# ---------- 参数解析：默认 --dry-run ----------
MODE="dry-run"
for arg in "$@"; do
  case "$arg" in
    --execute)        MODE="execute" ;;
    --dry-run)        MODE="dry-run" ;;
    --yes|-y)         MODE="execute" ;;  # 兼容旧习惯（仍要求 --execute 关键词）
    --help|-h)
      sed -n '2,25p' "$0"
      exit 0
      ;;
    *)
      echo "未知参数: $arg" >&2
      echo "用法: bash scripts/check_empty_db_repro.sh [--execute|--dry-run|--help]" >&2
      exit 2
      ;;
  esac
done

if [ "$MODE" = "dry-run" ]; then
  echo "[DRY-RUN] 默认模式：仅打印计划，不会真跑 down -v / up -d。"
  echo "[DRY-RUN] 真跑请加 --execute（会销毁数据卷，请先确认 dev 库无保留数据）。"
  echo
fi

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
COMPOSE_BASE="$REPO_ROOT/deploy/docker-compose.infra.yml"
COMPOSE_APPS="$REPO_ROOT/deploy/docker-compose.apps.yml"
COMPOSE_CMD="docker compose -f $COMPOSE_BASE -f $COMPOSE_APPS"

# Git Bash 上 'python3' 可能是 WindowsApps stub（exit 49），与 smoke_nacos_registry.sh 一致兜底
PYTHON_BIN="${PYTHON_BIN:-/c/Users/LENVOV/AppData/Local/Programs/Python/Python312/python.exe}"
if [ ! -x "$PYTHON_BIN" ]; then
  PYTHON_BIN="$(command -v python3 || command -v python || echo python3)"
fi

# run_cmd helper：dry-run 时打印命令并跳过；execute 时真跑
run_cmd() {
  if [ "$MODE" = "dry-run" ]; then
    echo "[DRY-RUN] $*"
    return 0
  else
    "$@"
  fi
}

# ---------- 阶段 A：down -v 全清 ----------
echo "[A] docker compose down -v --remove-orphans"
if [ "$MODE" = "dry-run" ]; then
  echo "[DRY-RUN] (上面命令会销毁所有命名数据卷)"
else
  echo "    ⚠️  真跑模式：即将销毁所有命名数据卷（含 dev 库累积数据）"
  $COMPOSE_CMD down -v --remove-orphans
fi
echo

# ---------- 阶段 B：up -d ----------
echo "[B] docker compose up -d"
run_cmd $COMPOSE_CMD up -d
echo

# ---------- 阶段 C：等 db-migrate 容器退出（service_completed_successfully）----------
echo "[C] 等待 emotion-echo-db-migrate 退出（超时 90s）"
if [ "$MODE" = "dry-run" ]; then
  echo "[DRY-RUN] 跳过等待（容器未起）"
  echo
  echo "[D] 等待 emotion-echo-postgres healthy（超时 30s）"
  echo "[DRY-RUN] 跳过等待"
  echo
  echo "[E] 等待 5 业务 svc 全 healthy（超时 120s）"
  echo "[DRY-RUN] 跳过等待"
  echo
  echo "[F] bash deploy/db/test_migrations_contract.sh"
  echo "[DRY-RUN] 跳过（契约测试需 schema 就位）"
  echo
  echo "[G] $PYTHON_BIN scripts/smoke_data_layer.py"
  echo "[DRY-RUN] 跳过（smoke 需服务就位）"
  echo
  echo "============================================="
  echo "[DRY-RUN OK] 计划如上。真跑请加 --execute。"
  exit 0
fi
migrate_ok=0
for i in $(seq 1 90); do
  state=$(docker inspect emotion-echo-db-migrate --format '{{.State.Status}}' 2>/dev/null || echo "missing")
  if [ "$state" = "exited" ]; then
    rc=$(docker inspect emotion-echo-db-migrate --format '{{.State.ExitCode}}')
    if [ "$rc" = "0" ]; then
      echo "    OK (db-migrate exited, exit_code=0, 用时 ${i}s)"
      migrate_ok=1
      break
    else
      echo "    [FATAL] db-migrate exit_code=$rc"
      echo "    --- db-migrate 日志尾部 ---"
      docker logs --tail 30 emotion-echo-db-migrate >&2 || true
      echo "    --- db-migrate 日志结束 ---"
      exit 2
    fi
  fi
  sleep 1
done
if [ "$migrate_ok" -ne 1 ]; then
  echo "    [FATAL] db-migrate 90s 内未退出（state=$state）"
  docker logs --tail 30 emotion-echo-db-migrate >&2 || true
  exit 2
fi
echo

# ---------- 阶段 D：等 postgres healthy ----------
echo "[D] 等待 emotion-echo-postgres healthy（超时 30s）"
pg_ok=0
for i in $(seq 1 30); do
  h=$(docker inspect emotion-echo-postgres --format '{{.State.Health.Status}}' 2>/dev/null || echo "missing")
  if [ "$h" = "healthy" ]; then
    echo "    OK (postgres healthy, 用时 ${i}s)"
    pg_ok=1
    break
  fi
  sleep 1
done
if [ "$pg_ok" -ne 1 ]; then
  echo "    [FATAL] postgres 30s 内未 healthy（status=$h）"
  docker logs --tail 20 emotion-echo-postgres >&2 || true
  exit 2
fi
echo

# ---------- 阶段 E：等 5 业务 svc 全 healthy ----------
echo "[E] 等待 5 业务 svc 全 healthy（超时 120s，容忍 start_period=60s）"
SVCS=(emotion-echo-user-svc emotion-echo-chat-svc emotion-echo-analytics-svc \
      emotion-echo-assessment-svc emotion-echo-ai-svc emotion-echo-web-bff)
all_ok=0
for i in $(seq 1 60); do
  all_ok=1
  for svc in "${SVCS[@]}"; do
    h=$(docker inspect "$svc" --format '{{.State.Health.Status}}' 2>/dev/null || echo "missing")
    if [ "$h" != "healthy" ]; then
      all_ok=0
      break
    fi
  done
  if [ "$all_ok" -eq 1 ]; then
    echo "    OK (6 svc 全 healthy, 用时 $((i*2))s)"
    break
  fi
  sleep 2
done
if [ "$all_ok" -ne 1 ]; then
  echo "    [FATAL] 5 业务 svc 120s 内未全 healthy"
  for svc in "${SVCS[@]}"; do
    h=$(docker inspect "$svc" --format '{{.State.Health.Status}}' 2>/dev/null || echo "missing")
    echo "      $svc: $h"
  done
  exit 2
fi
echo

# ---------- 阶段 F：14 条迁移契约 ----------
echo "[F] bash deploy/db/test_migrations_contract.sh"
bash "$REPO_ROOT/deploy/db/test_migrations_contract.sh"
contract_rc=$?
echo

# ---------- 阶段 G：数据契约 smoke ----------
echo "[G] $PYTHON_BIN scripts/smoke_data_layer.py"
"$PYTHON_BIN" "$REPO_ROOT/scripts/smoke_data_layer.py"
smoke_rc=$?
echo

# ---------- 阶段 H：汇总 ----------
echo "============================================="
echo "contract_rc=$contract_rc  smoke_rc=$smoke_rc"
if [ "$contract_rc" -eq 0 ] && [ "$smoke_rc" -eq 0 ]; then
  echo "[OK] db-migrate 闭环全 PASS"
  exit 0
else
  echo "[FAIL] db-migrate 闭环失败"
  exit 1
fi
