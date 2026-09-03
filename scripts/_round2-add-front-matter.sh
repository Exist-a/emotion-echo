#!/usr/bin/env bash
# Round 2-A: 给每份已落地的 .trae 文档加 YAML front-matter 状态头
# 用法: bash scripts/_round2-add-front-matter.sh <file> <status> <superseded-by> <original-path> <original-date>

set -e
FILE="$1"
STATUS="$2"
SUPERSEDED="$3"
ORIGINAL="$4"
ORIGINAL_DATE="$5"

TMP="${FILE}.tmp"
cat > "$TMP" <<EOF
---
status: $STATUS
superseded-by: $SUPERSEDED
original-path: $ORIGINAL
original-date: $ORIGINAL_DATE
migrated-at: 2026-09-03
round: 2-A
---

EOF
cat "$FILE" >> "$TMP"
mv "$TMP" "$FILE"
echo "OK: $FILE"
