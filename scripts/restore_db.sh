#!/usr/bin/env bash
# scripts/restore_db.sh — 项目级数据库恢复封装（E2E-29 M4 / 账本 E2E-F-27 后续）
#
# 与 backup_db.sh 配对：`pg_restore --clean --if-exists --no-owner` 覆盖式恢复。
#
# 破坏性说明：`--clean` 会先 DROP 目标对象再重建 ⇒ 对**同名对象**是覆盖语义。
# 故本脚本：
#   - 恢复前**必须**能读出备份目录（`pg_restore --list`），否则拒绝执行
#   - 默认**不**允许警告（`errors ignored on restore` 非 0 即失败），
#     需要放行时显式设 `ALLOW_RESTORE_WARNINGS=1`（把"接受警告"变成有意选择，
#     而不是被 set -e 或 `|| true` 悄悄吞掉）
#   - 失败一律非零退出并回显 pg_restore 的错误尾部
#
# 用法：
#   bash scripts/restore_db.sh <dump 文件> [--dry-run]
# 退出码：0 成功；2 文件缺失/为空；3 备份不可读；4 pg_restore 报错（含警告未放行）

set -euo pipefail

PG_CONTAINER="${PG_CONTAINER:-emotion-echo-postgres}"
PGUSER="${PGUSER:-postgres}"
PGDATABASE="${PGDATABASE:-emotion_echo}"

FILE="${1:-}"
DRY_RUN=0
[ "${2:-}" = "--dry-run" ] && DRY_RUN=1

log() { echo "[restore] $*"; }

if [ -z "$FILE" ]; then
  echo "[restore] FAIL: 用法 bash scripts/restore_db.sh <dump 文件> [--dry-run]" >&2
  exit 2
fi
if [ ! -f "$FILE" ]; then
  echo "[restore] FAIL: 文件不存在: $FILE" >&2
  exit 2
fi
if [ ! -s "$FILE" ]; then
  echo "[restore] FAIL: 文件为空: $FILE" >&2
  exit 2
fi
if ! docker inspect "$PG_CONTAINER" --format '{{.State.Status}}' 2>/dev/null | grep -q '^running$'; then
  echo "[restore] FAIL: 容器 $PG_CONTAINER 未运行" >&2
  exit 2
fi
if ! docker exec -i "$PG_CONTAINER" pg_restore --list < "$FILE" > /dev/null; then
  echo "[restore] FAIL: 不是有效的 -Fc 备份（pg_restore --list 失败）" >&2
  exit 3
fi

if [ "$DRY_RUN" = "1" ]; then
  log "dry-run：备份可读、容器在跑；未做任何改动"
  exit 0
fi

log "pg_restore --clean --if-exists --no-owner → $PGDATABASE（源：$FILE）"
TMP_ERR="$(mktemp 2>/dev/null || echo "${TEMP:-/tmp}/restore-err-$$")"
set +e
docker exec -i "$PG_CONTAINER" pg_restore -U "$PGUSER" -d "$PGDATABASE" \
  --clean --if-exists --no-owner < "$FILE" 2> "$TMP_ERR"
RC=$?
set -e

WARN_COUNT="$(grep -oE 'errors ignored on restore: [0-9]+' "$TMP_ERR" | tail -1 | grep -oE '[0-9]+' || echo 0)"

# 已知良性报错（**窄豁免**，不做一刀切 || true）：
#   `cannot drop inherited constraint ... of relation ...` —— 分区表（如 ube_2026_01 系列）
#   的约束继承自父表，`--clean` 阶段无法单独 DROP 子表的继承约束。实测 14 条全部属此类，
#   且恢复后行数与破坏前逐项一致（见 E2E-29 #18 演练记录）。
BENIGN_PATTERN='cannot drop inherited constraint'
ERR_LINES="$(grep -c '^pg_restore: error:' "$TMP_ERR" || true)"
BENIGN_LINES="$(grep -c "^pg_restore: error:.*$BENIGN_PATTERN" "$TMP_ERR" || true)"
ALL_BENIGN=0
if [ "${ERR_LINES:-0}" != "0" ] && [ "$ERR_LINES" = "$BENIGN_LINES" ]; then ALL_BENIGN=1; fi

if [ "$RC" -ne 0 ]; then
  if [ "$ALL_BENIGN" = "1" ] || { [ "${ALLOW_RESTORE_WARNINGS:-0}" = "1" ] && [ "${WARN_COUNT:-0}" != "0" ]; }; then
    log "WARN: pg_restore 退出码 $RC（errors=$ERR_LINES，其中已知良性「分区表继承约束」=$BENIGN_LINES；errors ignored=$WARN_COUNT）——按已知良性放行"
    grep "^pg_restore: error:" "$TMP_ERR" | sort -u | head -5 | sed 's/^/[restore]   /' || true
  else
    echo "[restore] FAIL: pg_restore 退出码 $RC（errors=$ERR_LINES 良性=$BENIGN_LINES，errors ignored=$WARN_COUNT）" >&2
    echo "--- pg_restore stderr 尾部 ---" >&2
    tail -20 "$TMP_ERR" >&2 || true
    echo "[restore] 提示：只有全部错误都属已知良性类才放行；否则请先查明。" >&2
    rm -f "$TMP_ERR"
    exit 4
  fi
fi

rm -f "$TMP_ERR"
log "OK: 恢复完成（errors=$ERR_LINES 良性=$BENIGN_LINES，errors ignored=$WARN_COUNT）"
