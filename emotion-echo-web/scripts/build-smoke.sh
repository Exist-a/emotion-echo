#!/usr/bin/env bash
# build-smoke.sh — SPA 产物冒烟检查
# 用法: pnpm build && bash scripts/build-smoke.sh
set -euo pipefail

OUTPUT_DIR=".output/public"

echo "=== Build Smoke Check ==="

# 1. 目录存在
if [ ! -d "$OUTPUT_DIR" ]; then
  echo "FAIL: $OUTPUT_DIR does not exist"
  exit 1
fi
echo "PASS: $OUTPUT_DIR exists"

# 2. index.html 存在且非空
if [ ! -s "$OUTPUT_DIR/index.html" ]; then
  echo "FAIL: $OUTPUT_DIR/index.html missing or empty"
  exit 1
fi
echo "PASS: index.html exists ($(wc -c < "$OUTPUT_DIR/index.html") bytes)"

# 3. index.html 包含 <div id="__nuxt">
if ! grep -q '__nuxt' "$OUTPUT_DIR/index.html"; then
  echo "FAIL: index.html missing __nuxt div"
  exit 1
fi
echo "PASS: index.html contains __nuxt div"

# 4. _nuxt 目录有 JS 文件
JS_COUNT=$(find "$OUTPUT_DIR/_nuxt" -name "*.js" 2>/dev/null | wc -l)
if [ "$JS_COUNT" -eq 0 ]; then
  echo "FAIL: No JS files in $OUTPUT_DIR/_nuxt/"
  exit 1
fi
echo "PASS: $JS_COUNT JS files in _nuxt/"

echo "=== All checks passed ==="
