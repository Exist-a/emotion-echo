#!/usr/bin/env bash
# TDD 测试：scripts/on-device-cdn-probe/probe.sh 结构契约（Lane O · T2#2 · CDN 实测拉流）。
#
# 目的：在不依赖实际网络的前提下，验证 probe.sh 的：
#   - 5 候选 CDN host 列表（A 阿里云 / B 腾讯云 / E hf-mirror / F ModelScope + 默认对照 raw.githubusercontent.com）
#   - 4 类探测能力（HEAD probe / CORS preflight / 文件 GET / wasm magic byte）
#   - 退出码契约（0 = 全部 OK 或 SKIP，1 = 任意 FAIL —— 与 smoke_data_layer.py 一致）
#   - 输出格式（SKIP 显式，不静默 —— check_required_checks.py 纪律）
#
# 跑法（repo 根）：bash scripts/on-device-cdn-probe/test_probe.sh
# RED 阶段：本文件先于 probe.sh 存在；GREEN 阶段：probe.sh 实现后回归全绿。
#
# 调研依据：on-device-compile-cdn-2026-09-24.md §三 + smoke_data_layer.py:48-50 skip 模式 +
# check_required_checks.py:24-26 显式 SKIP 纪律。

set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROBE="$SCRIPT_DIR/probe.sh"

PASS=0
FAIL=0

assert_eq() {
  local actual="$1"
  local expected="$2"
  local label="$3"
  if [[ "$actual" == "$expected" ]]; then
    echo "  ✓ $label"
    PASS=$((PASS + 1))
  else
    echo "  ✗ $label (got: $actual, expected: $expected)"
    FAIL=$((FAIL + 1))
  fi
}

assert_contains() {
  local actual="$1"
  local needle="$2"
  local label="$3"
  if [[ "$actual" == *"$needle"* ]]; then
    echo "  ✓ $label"
    PASS=$((PASS + 1))
  else
    echo "  ✗ $label (missing: $needle)"
    FAIL=$((FAIL + 1))
  fi
}

assert_file_exists() {
  local path="$1"
  local label="$2"
  if [[ -f "$path" ]]; then
    echo "  ✓ $label"
    PASS=$((PASS + 1))
  else
    echo "  ✗ $label (missing: $path)"
    FAIL=$((FAIL + 1))
  fi
}

assert_exec_succeeds() {
  local cmd="$1"
  local label="$2"
  if eval "$cmd" >/dev/null 2>&1; then
    echo "  ✓ $label"
    PASS=$((PASS + 1))
  else
    echo "  ✗ $label (cmd failed: $cmd)"
    FAIL=$((FAIL + 1))
  fi
}

echo "=== probe.sh 结构契约 ==="

# 1. 文件存在性
assert_file_exists "$PROBE" "probe.sh 必须存在"

# 2. 必须声明 5 个候选 CDN host（含默认对照 + 4 候选）
PROBE_SRC=$(cat "$PROBE" 2>/dev/null || echo "")
assert_contains "$PROBE_SRC" "oss-cn-hangzhou.aliyuncs.com" "必须声明 A. 阿里云 OSS host"
assert_contains "$PROBE_SRC" "cos.ap-shanghai.myqcloud.com" "必须声明 B. 腾讯云 COS host"
assert_contains "$PROBE_SRC" "hf-mirror.com" "必须声明 E. HuggingFace 镜像 host"
assert_contains "$PROBE_SRC" "modelscope.cn" "必须声明 F. ModelScope host"
assert_contains "$PROBE_SRC" "raw.githubusercontent.com" "必须包含默认对照 raw.githubusercontent.com（v0.2 §4.1 wasm 默认）"

# 3. 必须有 4 类探测能力
assert_contains "$PROBE_SRC" "probe_head" "必须包含 HEAD 探测能力（函数 probe_head）"
assert_contains "$PROBE_SRC" "probe_cors" "必须包含 CORS preflight 探测能力（函数 probe_cors）"
assert_contains "$PROBE_SRC" "probe_get" "必须包含文件 GET 探测能力（函数 probe_get）"
assert_contains "$PROBE_SRC" "probe_wasm_magic" "必须包含 wasm magic byte 探测能力（函数 probe_wasm_magic）"

# 4. 输出格式契约 —— 不静默 SKIP
assert_contains "$PROBE_SRC" "SKIP" "必须输出显式 SKIP 标签（不静默 —— check_required_checks.py 纪律）"
assert_contains "$PROBE_SRC" "PASS" "必须输出显式 PASS 标签"
assert_contains "$PROBE_SRC" "FAIL" "必须输出显式 FAIL 标签"

# 5. 退出码契约（0 = 全部 OK/SKIP，1 = 任意 FAIL）
assert_exec_succeeds "bash '$PROBE' --help 2>&1 | grep -q 'Usage\|usage'" "probe.sh --help 必须输出 usage"
assert_exec_succeeds "bash '$PROBE' --dry-run 2>&1 | grep -qE 'PASS|SKIP'" "probe.sh --dry-run 必须输出 PASS/SKIP（CI 确定性兜底）"

# 6. dry-run 退出码必须 = 0（不因网络阻塞 CI）
DRY_RC=$(bash "$PROBE" --dry-run >/dev/null 2>&1; echo $?)
assert_eq "$DRY_RC" "0" "probe.sh --dry-run 退出码必须 = 0（无网络时也通过）"

echo ""
echo "=== 结果 ==="
echo "PASS: $PASS, FAIL: $FAIL"
if [[ $FAIL -gt 0 ]]; then
  exit 1
fi
exit 0