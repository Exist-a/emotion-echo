#!/usr/bin/env bash
# E2E-F-198 T8（docs/plans/tts-api-cosyvoice2-f198-2026-10-06.md §D）：
# TTS 业务代码零硬编码供应商域名/模型名——ADR-2026-10 §Decision.1
# 「禁止在业务代码里直写 SiliconFlow 域名/模型名（经配置注入，便于换供应商）」的
# 机器守卫。
#
# 允许面（白名单，grep 不扫）：
#   - emotion-echo-web-bff/internal/config/   —— 配置默认值所在地（唯一注入点）
#   - 任何 *_test.go                          —— fake 构造需要字面量
#   - deploy/ etc/ docs/ scripts/             —— 配置/文档/本守卫自身
#
# 违规面（扫描目标）：
#   - emotion-echo-web-bff/internal/{handler,downstream}/*.go 中非 _test.go
#
# 负向自检：向临时副本注入违规行，守卫必须能抓到——防「守卫本身是弱断言」
# 假绿（E2E-F-107 / E2E-25 seed_test 教训）。
#
# 用法：bash scripts/test_tts_provider_isolation.sh   # FAIL=1 / GREEN=0
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PATTERN='siliconflow|FunAudioLLM'
TARGETS=()
for f in "$ROOT"/emotion-echo-web-bff/internal/handler/*.go "$ROOT"/emotion-echo-web-bff/internal/downstream/*.go; do
    case "$f" in
        *_test.go) continue ;;
    esac
    TARGETS+=("$f")
done

echo "=== TTS provider 隔离守卫 ==="
echo "扫描目标: ${#TARGETS[@]} 个业务 .go（config 默认值与 _test.go 白名单豁免）"

FAIL=0
if [ "${#TARGETS[@]}" -eq 0 ]; then
    echo "[FAIL] 扫描目标为空——守卫自身失效（目录不存在/改名未同步）"
    exit 1
fi

# 负向自检 1：守卫的模式必须真的能命中违规样例
SELFTEST_TMP="$(mktemp -d)"
trap 'rm -rf "$SELFTEST_TMP"' EXIT
printf 'package handler\n\nvar vendorLeak = "https://api.siliconflow.cn/v1"\n' > "$SELFTEST_TMP/leak.go"
if ! grep -qE "$PATTERN" "$SELFTEST_TMP/leak.go"; then
    echo "[FAIL] 负向自检失败：守卫模式抓不到注入的违规样例（弱断言假绿）"
    exit 1
fi
rm -f "$SELFTEST_TMP/leak.go"

# 主检查：业务代码零命中
VIOLATIONS=""
for f in "${TARGETS[@]}"; do
    if grep -nE "$PATTERN" "$f" >/dev/null 2>&1; then
        VIOLATIONS="${VIOLATIONS}
$(basename "$f"): $(grep -cE "$PATTERN" "$f") 处命中"
    fi
done

if [ -n "$VIOLATIONS" ]; then
    echo "[FAIL] 业务代码出现供应商域名/模型名硬编码（ADR-2026-10 §Decision.1）："
    echo "$VIOLATIONS"
    echo "修法：移入 internal/config 默认值或 env 注入（TTS_API_BASE_URL / TTS_MODEL）"
    FAIL=1
else
    echo "[OK  ] 业务代码零硬编码（域名/模型名只存在于 config 默认值与测试）"
fi

# 负向自检 2：白名单必须真的豁免 config——若有人误把 config 纳入扫描面，
# 默认值本身会触雷，守卫将恒红 → 有恒红风险的守卫会被「顺手删掉」，故钉死。
CONFIG_FILE="$ROOT/emotion-echo-web-bff/internal/config/config.go"
if [ -f "$CONFIG_FILE" ] && grep -q "siliconflow" "$CONFIG_FILE"; then
    echo "[OK  ] config 默认值在位且不在扫描面（白名单语义成立）"
else
    echo "[WARN] config.go 无 siliconflow 默认值或文件缺失——若为有意迁移请同步本守卫"
fi

if [ "$FAIL" -eq 0 ]; then
    echo ""
    echo "GREEN: TTS provider 隔离契约成立"
    exit 0
fi
exit 1
