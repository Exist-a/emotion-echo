#!/usr/bin/env bash
# Round 2-C: 给 still-valid 计划加 front-matter (status: planned)
set -e
FILE="$1"
SUPERSEDED="$2"
ORIGINAL="$3"
ORIGINAL_DATE="$4"
TMP="${FILE}.tmp"
cat > "$TMP" <<EOF
---
status: planned
superseded-by: $SUPERSEDED
original-path: $ORIGINAL
original-date: $ORIGINAL_DATE
migrated-at: 2026-09-03
round: 2-C
---

EOF
cat "$FILE" >> "$TMP"
mv "$TMP" "$FILE"
echo "OK: $FILE"
