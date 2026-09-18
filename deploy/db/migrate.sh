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
#   Round 1.3 改造：原 SERVICE_ORDER 硬编码删除，改 glob 模式自动发现
#   */migrations/（legacy/ 历史归档自然排除，因其在 legacy/ 子目录下）。
#   服务发现顺序：migrations 目录的字典序（chat < ai < analytics，
#   因字母序 c < a... 实际 chat < ai 是因为 shell 字典序对 c* 与 a* 不可靠，
#   故强制加 PRIORITY_ORDER 兜底，新 svc 排最后自然正确）。
#
# 版本追踪（E2E-06）：
#   schema_migrations 表记录已应用的迁移文件 + checksum。
#   - 已应用且 checksum 一致 → 跳过（日志 SKIP）
#   - checksum 不一致 → 报错（迁移文件被修改，需手动处理）
#   - 未应用 → 执行并记录
#
# 用法：
#   容器内（compose 的 emotion-echo-db-migrate 服务）：直接执行，psql 连 postgres 服务名
#   宿主机（契约测试复用）：自动降级为 docker exec 进 postgres 容器执行
#
# 退出码：0 全部成功；非 0 表示某个迁移失败（会打印是哪个文件 + psql 原始报错）

set -eu

# Round 1.3：服务执行顺序（已删除 SERVICE_ORDER 硬编码字符串）
# 保留 PRIORITY_ORDER 作为跨 svc 顺序兜底（chat → ai → analytics 依赖关系），
# 但这是"参考"而非"必须"——后续新 svc 加在末尾即可。
# E2E-06: 新增 user-svc（密保表 + 死字段删除）
PRIORITY_ORDER="emotion-echo-chat-svc emotion-echo-ai-svc emotion-echo-analytics-svc emotion-echo-user-svc"

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

run_sql() {
  sql="$1"
  if [ "$USE_DOCKER" = "1" ]; then
    docker exec -i "$PG_CONTAINER" psql -U "$PGUSER" -d "$PGDATABASE" \
      -v ON_ERROR_STOP=1 -q -c "$sql" 2>&1
  else
    psql -h "$PGHOST" -U "$PGUSER" -d "$PGDATABASE" \
      -v ON_ERROR_STOP=1 -q -c "$sql" 2>&1
  fi
}

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

# 计算文件 SHA-256 checksum（兼容 Linux sha256sum 和 macOS shasum）
file_checksum() {
  f="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$f" | cut -d' ' -f1
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$f" | cut -d' ' -f1
  else
    # 兜底：用 cksum（非 SHA-256，但足够做变更检测）
    cksum "$f" | awk '{print $1}'
  fi
}

# 确保 schema_migrations 表存在（幂等）
ensure_migrations_table() {
  run_sql "
CREATE TABLE IF NOT EXISTS emotion_echo_user.schema_migrations (
    version VARCHAR(255) PRIMARY KEY,
    checksum VARCHAR(64) NOT NULL,
    applied_at TIMESTAMPTZ DEFAULT NOW(),
    execution_ms INTEGER
);" >/dev/null 2>&1 || true
}

# 检查迁移是否已应用
# 返回：0=已应用且 checksum 一致, 1=未应用, 2=checksum 不一致
check_migration() {
  version="$1"
  expected_checksum="$2"

  # 查询已应用的 checksum
  result=$(run_sql "SELECT checksum FROM emotion_echo_user.schema_migrations WHERE version = '$version';" 2>/dev/null || echo "")

  # 未应用
  if [ -z "$result" ] || echo "$result" | grep -q "(0 rows)"; then
    return 1
  fi

  # 提取 checksum（去掉表头和空行）
  actual_checksum=$(echo "$result" | tr -d ' ' | grep -v '^$' | grep -v 'checksum' | grep -v '^-' | head -1)

  if [ "$actual_checksum" = "$expected_checksum" ]; then
    return 0  # 已应用且一致
  else
    return 2  # checksum 不一致
  fi
}

# 记录迁移已应用
record_migration() {
  version="$1"
  checksum="$2"
  elapsed_ms="$3"
  run_sql "INSERT INTO emotion_echo_user.schema_migrations (version, checksum, execution_ms)
           VALUES ('$version', '$checksum', $elapsed_ms)
           ON CONFLICT (version) DO UPDATE SET checksum = EXCLUDED.checksum, applied_at = NOW(), execution_ms = EXCLUDED.execution_ms;" >/dev/null 2>&1
}

# 带版本追踪的迁移执行
run_tracked_sql_file() {
  f="$1"
  name="$2"
  checksum=$(file_checksum "$f")
  version=$(basename "$f")

  # 确保版本表存在
  ensure_migrations_table

  # 检查是否已应用
  check_migration "$version" "$checksum"
  rc=$?
  if [ $rc -eq 0 ]; then
    log "  SKIP $name（已应用，checksum 一致）"
    return 0
  elif [ $rc -eq 2 ]; then
    die "迁移文件被修改：$name（checksum 不一致，expected=$checksum）"
  fi

  # 执行迁移
  start_ms=$(date +%s 2>/dev/null || echo "0")
  if out=$(run_sql_file "$f"); then
    end_ms=$(date +%s 2>/dev/null || echo "0")
    elapsed_ms=$(( (end_ms - start_ms) * 1000 ))
    record_migration "$version" "$checksum" "$elapsed_ms"
    log "  OK  $name（${elapsed_ms}ms）"
    return 0
  else
    log "  ERR $name"
    echo "$out" | tail -20 >&2
    die "迁移失败：$name（该文件可能非幂等，或依赖了尚未创建的对象）"
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

# Round 1.3：glob 模式自动发现 */migrations/（不依赖 SERVICE_ORDER 硬编码）。
# 1) PRIORITY_ORDER 中的 svc 按声明顺序排前（兜底跨 svc 依赖）
# 2) 任何 MIGRATIONS_ROOT/*/migrations（不在 PRIORITY_ORDER 中的新 svc）排后
# 3) 排除 legacy/ 历史归档（其在 legacy/ 子目录下，自然被一级 glob 排除）
total=0
skipped=0
seen=""

# Phase 1: PRIORITY_ORDER 已声明的 svc
for svc in $PRIORITY_ORDER; do
  dir="$MIGRATIONS_ROOT/$svc/migrations"
  if [ ! -d "$dir" ]; then
    log "跳过 $svc（无 migrations 目录）"
    continue
  fi
  seen="$seen $svc"
  for f in $(ls "$dir"/*.sql 2>/dev/null | sort); do
    name="$svc/$(basename "$f")"
    if run_tracked_sql_file "$f" "$name"; then
      total=$((total + 1))
    fi
  done
done

# Phase 2: glob 自动发现新 svc（不需修改本脚本即生效）
# 排除 legacy/（其路径含 legacy/ 子目录）+ 已 seen 的 svc
for dir in $(ls -d "$MIGRATIONS_ROOT"/*/migrations 2>/dev/null | sort); do
  svc=$(basename "$(dirname "$dir")")
  case " $seen " in *" $svc "*) continue ;; esac
  log "新发现 svc: $svc（glob 自动模式）"
  for f in $(ls "$dir"/*.sql 2>/dev/null | sort); do
    name="$svc/$(basename "$f")"
    if run_tracked_sql_file "$f" "$name"; then
      total=$((total + 1))
    fi
  done
done

log "全部迁移应用完成，共 $total 个文件（版本追踪已启用，幂等可重复执行）"