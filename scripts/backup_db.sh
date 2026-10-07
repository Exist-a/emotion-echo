#!/usr/bin/env bash
# scripts/backup_db.sh — 项目级数据库备份封装（E2E-29 M4 / 账本 E2E-F-27 后续）
#
# 背景：dev 真演练 2026-09-27 已证明 `pg_dump -Fc` + `pg_restore --clean --if-exists` 可用
# （备份 245KB → 真 DROP messages CASCADE → 恢复 417→417 一致），但**全仓没有封装脚本**：
# 备份/恢复只存在于历史工作记录里，没人能一条命令重复。本脚本补上这一层。
#
# 设计要点（防"静默失败"——与 E2E-F-47/F-151 同族教训）：
#   - 任一环节失败**非零退出**并打印可操作信息（不用 `|| true` 吞错）
#   - 备份后做**完整性校验**：文件非空 + `pg_restore --list` 能读出目录
#     （只"生成了文件"不算成功——空文件/截断文件都曾把演练变成假通过）
#   - 宿主机通常无 pg_dump/psql ⇒ 一律走 `docker exec` 进 PG 容器
#   - 凭据沿用 migrate.sh 口径（PGUSER/PGDATABASE/PG_CONTAINER），不读 .env.local 内容
#
# 用法：
#   bash scripts/backup_db.sh [输出目录]      # 默认 deploy/backups/
# 退出码：0 成功；2 容器未运行；3 pg_dump 失败；4 备份为空；5 备份不可读

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PG_CONTAINER="${PG_CONTAINER:-emotion-echo-postgres}"
PGUSER="${PGUSER:-postgres}"
PGDATABASE="${PGDATABASE:-emotion_echo}"
OUT_DIR="${1:-$REPO_ROOT/deploy/backups}"

log() { echo "[backup] $*"; }

if ! docker inspect "$PG_CONTAINER" --format '{{.State.Status}}' 2>/dev/null | grep -q '^running$'; then
  echo "[backup] FAIL: 容器 $PG_CONTAINER 未运行（先 docker compose up -d）" >&2
  exit 2
fi

mkdir -p "$OUT_DIR"
TS="$(date +%Y%m%dT%H%M%S)"
FILE="$OUT_DIR/${PGDATABASE}-${TS}.dump"

log "pg_dump -Fc → $FILE"
if ! docker exec "$PG_CONTAINER" pg_dump -U "$PGUSER" -Fc -d "$PGDATABASE" > "$FILE"; then
  rm -f "$FILE"
  echo "[backup] FAIL: pg_dump 失败（详见上方 stderr）" >&2
  exit 3
fi

if [ ! -s "$FILE" ]; then
  echo "[backup] FAIL: 备份文件为空（0 字节）——不算成功" >&2
  exit 4
fi

if ! docker exec -i "$PG_CONTAINER" pg_restore --list < "$FILE" > /dev/null; then
  echo "[backup] FAIL: 备份不可读（pg_restore --list 失败）——文件可能被截断/编码损坏" >&2
  exit 5
fi

SIZE="$(wc -c < "$FILE" | tr -d ' ')"
TABLES="$(docker exec -i "$PG_CONTAINER" pg_restore --list < "$FILE" | grep -c 'TABLE DATA' || true)"
log "OK: $FILE（$SIZE 字节，含 $TABLES 个 TABLE DATA 条目）"
log "恢复：bash scripts/restore_db.sh \"$FILE\""
