#!/usr/bin/env bash
# scripts/check_smoke_gateway.sh — E2E-30 L1 守卫：数据契约 smoke 必须经网关且带 Bearer
#
# 背景（账本 E2E-F-209 / stages/e2e-30-data-contract-closure/plan.md §0.1 F2/F3）：
#   `scripts/smoke_data_layer.py` 的基址是 `http://localhost:8894`，而 8894 宿主映射
#   已被 E2E-29 D-47 ③ 移除（那是"直连 BFF 伪造 X-User-Id 绕过网关"的入口，账本 E2E-F-202）
#   ⇒ 脚本在 §契约 1 之前就死在 `[FATAL] BFF /health 不可达`。
#   2026-10-09 实测复现：`python scripts/smoke_data_layer.py` → rc=2、宿主 `curl :8894` → 000。
#
#   正解 = 基址改走 APISIX 网关 `:19080`，业务请求带 `Authorization: Bearer <token>`
#   （决策 11/12：APISIX 是唯一业务入口）。健康前置**不能**沿用 BFF `/health`
#   —— 网关无该路由（账本 E2E-F-211），故改为"登录成功即判 BFF 可用"（零新增路由）。
#
# 本守卫（静态、无 docker 依赖）钉住该契约，防回退：
#   ① 脚本不得再出现 `:8894`（宿主直连口已收）
#   ② 必须以网关 `:19080` 为基址（且可被环境变量覆盖，便于换环境）
#   ③ 必须走 `Authorization: Bearer`（身份来自令牌，不再是裸 `X-User-Id`）
#   ④ 不得再以 BFF `/health` 作为**前置门**（网关无此路由）
# 含负向对照：把违规样例注入临时副本，必须能被检测出来（防弱断言假绿）。

set -uo pipefail
cd "$(dirname "$0")/.." || exit 2

SMOKE="scripts/smoke_data_layer.py"

fail=0
err() { echo "FAIL: $*"; fail=1; }
cnt() { grep -c -- "$1" "$2" 2>/dev/null || true; }

echo "== smoke 走网关契约守卫（E2E-30 L1 / E2E-F-209 / F-211）=="

if [ ! -f "$SMOKE" ]; then
  err "缺少 $SMOKE"
  echo "FAIL: 契约不成立"
  exit 1
fi

# ① 不得再出现宿主直连口 8894（含 127.0.0.1:8894 写法）
n8894="$(cnt '8894' "$SMOKE")"
if [ "$n8894" -ne 0 ]; then
  err "$SMOKE 仍含 $n8894 处 8894（宿主直连口已被 D-47 收掉，跑必连接失败）"
else
  echo "   8894 零命中"
fi

# ② 基址必须是网关 19080，且可由环境变量覆盖
n19080="$(cnt '19080' "$SMOKE")"
if [ "$n19080" -lt 1 ]; then
  err "$SMOKE 未见网关基址 :19080"
else
  echo "   网关基址 19080 命中 $n19080 处"
fi
if [ "$(cnt 'os.environ.get("SMOKE_BASE"' "$SMOKE")" -lt 1 ] && [ "$(cnt "os.environ.get('SMOKE_BASE'" "$SMOKE")" -lt 1 ]; then
  err "$SMOKE 基址不可由环境变量 SMOKE_BASE 覆盖（换环境要改源码）"
else
  echo "   基址可由 SMOKE_BASE 覆盖"
fi

# ③ 必须走 Bearer
if [ "$(cnt 'Authorization' "$SMOKE")" -lt 1 ] || [ "$(cnt 'Bearer' "$SMOKE")" -lt 1 ]; then
  err "$SMOKE 未走 Authorization: Bearer（经网关后身份来自令牌）"
else
  echo "   Bearer 鉴权在位"
fi

# ④ 前置不得再依赖 BFF /health（网关无该路由，F-211）
if [ "$(cnt '"/health"' "$SMOKE")" -ge 1 ] || [ "$(cnt "'/health'" "$SMOKE")" -ge 1 ]; then
  err "$SMOKE 仍以 BFF /health 为前置门（网关无此路由，见 E2E-F-211）"
else
  echo "   前置不依赖 BFF /health"
fi

# ---------- 负向对照：注入违规样例必须被抓到 ----------
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
cp "$SMOKE" "$tmp/neg.py"
printf '\nBFF_LEGACY = "http://localhost:8894"\nhealth0 = http_get("/health")\n' >> "$tmp/neg.py"
neg8894="$(cnt '8894' "$tmp/neg.py")"
neghealth="$(cnt '"/health"' "$tmp/neg.py")"
if [ "$neg8894" -ge 1 ] && [ "$neghealth" -ge 1 ]; then
  echo "   负向对照：注入 8894（$neg8894 处）+ /health 前置（$neghealth 处）→ 可检测"
else
  err "负向对照失败：注入的违规样例未被检测到（守卫是弱断言）"
fi

if [ "$fail" -ne 0 ]; then
  echo "FAIL: 契约不成立（见上）"
  exit 1
fi
echo "PASS: smoke 基址走网关、带 Bearer、前置不依赖 /health，负向对照可检测"
exit 0
