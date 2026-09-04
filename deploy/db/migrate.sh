#!/usr/bin/env sh
# migrate.sh — 按依赖顺序应用全部服务迁移（幂等，可重复执行）
#
# 为什么需要它：
#   仓库里 13 个 */migrations/*.sql 此前没有任何自动执行机制，各文件注释只写了
#   "应用方式：docker exec ... psql < 本文件"，靠人工且无处记录跑没跑过。
#   实测后果是 dev 库长期缺列/缺视图/缺角色（发消息 500、smoke 掉到 1/10）。
#
# 为什么不挂 initdb.d：
#   Postgres 的 docker-entrypoint-initdb.d 只在**数据卷为空**时执行一次。
#   挂进去对已有环境无效；之后新增的迁移对所有已存在环境也无效。
#   它只能表达"初始状态"，表达不了"持续演进"。故独立成一次性 migrate 容器，
#   每次 compose up 都对齐一遍。
#
# 幂等要求：
#   本脚本每次启动都会跑，所以每个迁移都必须能重复执行。
#   多数文件已用 IF NOT EXISTS / OR REPLACE；不满足的必须补守卫
#   （如 analytics 004 的 CREATE ROLE，2026-09-04 已补 DO $$ 包装）。
#
# 执行顺序（实测验证，不能按文件名全局排序）：
#   chat → ai → analytics
#   理由：analytics 的统计视图建立在 chat / ai 的表之上；
#         ai 的 005 视图又依赖它自己 002/003 建的两张表。
#   服务内部按文件名升序（001 → 002 → ...）。
#
# 用法：
#   容器内（compose 的 emotion-echo-db-migrate 服务）：直接执行，psql 连 postgres 服务名
#   宿主机（契约测试复用）：自动降级为 docker exec 进 postgres 容器执行
#
# 退出码：0 全部成功；非 0 表示某个迁移失败（会打印是哪个文件 + psql 原始报错）

set -eu

# 服务执行顺序。新增带 migrations/ 的服务必须加到这里，
# 否则 deploy/db/test_migrations_contract.sh §契约 2 会失败。
SERVICE_ORDER="emotion-echo-chat-svc emotion-echo-ai-svc emotion-echo-analytics-svc"

PGHOST="${PGHOST:-postgres}"
PGUSER="${PGUSER:-postgres}"
PGDATABASE="${PGDATABASE:-emotion_echo}"
PGPASSWORD="${PGPASSWORD:-postgres}"
export PGPASSWORD

# 迁移根目录：容器内挂在 /migrations，宿主机跑时用仓库根
MIGRATIONS_ROOT="${MIGRATIONS_ROOT:-/migrations}"
PG_CONTAINER="${PG_CONTAINER:-emotion-echo-postgres}"

log() { echo "[migrate] $*" >&2; }
die() { echo "[migrate] FATAL: $*" >&2; exit 1; }

# 宿主机没有 /migrations，也通常没有 psql 客户端 → 降级走 docker exec
USE_DOCKER=0
if [ ! -d "$MIGRATIONS_ROOT" ]; then
  MIGRATIONS_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
  USE_DOCKER=1
  log "宿主机模式：迁移根=$MIGRATIONS_ROOT，经 docker exec $PG_CONTAINER 执行"
else
  log "容器模式：迁移根=$MIGRATIONS_ROOT，psql 连 $PGHOST"
fi

run_sql_file() {
  f="$1"
  if [ "$USE_DOCKER" = "1" ]; then
    docker exec -i "$PG_CONTAINER" psql -U "$PGUSER" -d "$PGDATABASE" \
      -v ON_ERROR_STOP=1 -q <"$f" 2>&1
  else
    psql -h "$PGHOST" -U "$PGUSER" -d "$PGDATABASE" \
      -v ON_ERROR_STOP=1 -q -f "$f" 2>&1
  fi
}

# 等 Postgres 可用（compose 的 depends_on healthy 已保证，这里兜底重试）
i=0
while [ "$i" -lt 30 ]; do
  if [ "$USE_DOCKER" = "1" ]; then
    docker exec "$PG_CONTAINER" pg_isready -U "$PGUSER" >/dev/null 2>&1 && break
  else
    pg_isready -h "$PGHOST" -U "$PGUSER" >/dev/null 2>&1 && break
  fi
  i=$((i + 1))
  [ "$i" -eq 30 ] && die "Postgres 30s 内未就绪"
  sleep 1
done

total=0
for svc in $SERVICE_ORDER; do
  dir="$MIGRATIONS_ROOT/$svc/migrations"
  if [ ! -d "$dir" ]; then
    log "跳过 $svc（无 migrations 目录）"
    continue
  fi
  # 服务内按文件名升序；用 ls 排序而非 glob 默认序，保证 001 < 002 < ...
  for f in $(ls "$dir"/*.sql 2>/dev/null | sort); do
    name="$svc/$(basename "$f")"
    if out=$(run_sql_file "$f"); then
      log "  OK  $name"
      total=$((total + 1))
    else
      log "  ERR $name"
      echo "$out" | tail -20 >&2
      die "迁移失败：$name（该文件可能非幂等，或依赖了尚未创建的对象）"
    fi
  done
done

log "全部迁移应用完成，共 $total 个文件（幂等，可重复执行）"
