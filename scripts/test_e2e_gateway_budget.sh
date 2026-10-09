#!/usr/bin/env bash
# scripts/test_e2e_gateway_budget.sh — E2E-F-214「E2E 连跑必须走请求预算闸门」守卫
#
# 背景（账本 E2E-F-214 / stages/e2e-31-internal-rpc-convergence/report.md §0 T-5）：
#   APISIX route 100（/api/v1/* catch-all）与 route 110（auth 白名单）各挂
#   `limit-count`：count=60 / time_window=60，key=remote_addr。
#   `quiz + survey-scoring + personality` 双 project 一次性连跑原本发出 route100 112 次、
#   route110 50 次（24 用例 × 2 project 各登录一次），实测某一分钟 route100 达 72 次
#   ⇒ 429 ⇒ 页面无数据 ⇒ 断言随机假红（复现：[mobile] personality #4 失败）。
#
#   修复 = spec 层请求预算（`emotion-echo-web/e2e/helpers/gateway.ts`）：
#     ① 登录令牌 worker 内复用（48 次冗余登录 → 1 次/worker）
#     ② 只读种子数据（/api/v1/surveys、/api/v1/surveys/{id}）worker 内缓存
#     ③ 所有经网关的请求先取滑动窗口令牌（≤ BUDGET/60s），浏览器自身请求经 page.route 同闸
#   修后实测：每 project 受控请求 57 → 42，闸门阻塞 0 次，每分钟峰值 54（<60），0 × 429，
#   48/48 连续 4 轮全绿。
#
# 本守卫守的是**闸门不被绕开**（静态、无需容器栈）：
#   · 三个 spec 必须从 ./helpers/gateway 导入并安装浏览器侧闸门
#   · 三个 spec 内 `page.request.` 必须为 0（裸调网关 = 绕过预算）
#   · 三个 spec 内不得再出现自己的登录实现（登录必须收敛到 loginOnce）
#   · 助手模块的 BUDGET 必须低于 APISIX 的 60，且种子缓存正则**不得**覆盖 /surveys/results
# 含负向对照：把违规样例注入临时副本，必须能被检测出来（防"弱断言假绿"）。

set -uo pipefail
cd "$(dirname "$0")/.." || exit 2

HELPER="emotion-echo-web/e2e/helpers/gateway.ts"
SPECS="emotion-echo-web/e2e/quiz.spec.ts
emotion-echo-web/e2e/survey-scoring.spec.ts
emotion-echo-web/e2e/personality.spec.ts"

fail=0
err() { echo "FAIL: $*"; fail=1; }
# grep -c 无命中时返回非零但打印 0；`|| true` 保留输出，避免 pipefail 干扰
cnt() { grep -c -- "$1" "$2" 2>/dev/null || true; }
# 固定字符串（正则字面量含 $ 锚点，须按字面比，否则 $ 会被当行尾锚）
cntF() { grep -cF -- "$1" "$2" 2>/dev/null || true; }

echo "== E2E 请求预算闸门守卫（E2E-F-214）=="

# ---------- 助手模块 ----------
if [ ! -f "$HELPER" ]; then
  err "缺少 $HELPER"
else
  # BUDGET 必须 < 60（APISIX limit-count 的 count）
  budget="$(grep -oE 'const BUDGET = [0-9]+' "$HELPER" | grep -oE '[0-9]+' | head -1)"
  if [ -z "$budget" ]; then
    err "$HELPER 未定义 BUDGET"
  elif [ "$budget" -ge 60 ]; then
    err "BUDGET=$budget 未低于 APISIX 的 60（会放过 429）"
  else
    echo "   BUDGET=$budget（< 60）"
  fi
  # 种子缓存正则必须锚定到 /surveys 与 /surveys/{id}，不得覆盖 /surveys/results*
  if [ "$(cntF '(\/\d+)?$/.test(path)' "$HELPER")" -lt 1 ]; then
    err "$HELPER 的种子缓存正则未锚定（可能误缓存 /surveys/results* —— 提交即变，禁止缓存）"
  else
    echo "   种子缓存正则已锚定（不含 /surveys/results*）"
  fi
fi

# ---------- 三个 spec ----------
for spec in $SPECS; do
  if [ ! -f "$spec" ]; then
    err "缺少 $spec"
    continue
  fi
  n_import="$(cnt 'helpers/gateway' "$spec")"
  n_gate="$(cnt 'gateBrowserRequests' "$spec")"
  n_raw="$(cnt 'page\.request\.' "$spec")"
  n_login="$(cnt 'auth/login' "$spec")"
  n_legacy="$(cnt 'loginViaAPI' "$spec")"

  [ "$n_import" -ge 1 ] || err "$spec 未从 ./helpers/gateway 导入（网关请求未走预算）"
  # 1 次 import + 1 次 beforeEach 调用
  [ "$n_gate" -ge 2 ] || err "$spec 未安装浏览器侧闸门（gateBrowserRequests 出现 $n_gate 次，应 >= 2）"
  [ "$n_raw" -eq 0 ] || err "$spec 含 $n_raw 处裸 page.request.（绕过预算闸门，须改用 gwGet/gwPost）"
  [ "$n_login" -eq 0 ] || err "$spec 含 $n_login 处 auth/login（登录须收敛到 loginOnce）"
  [ "$n_legacy" -eq 0 ] || err "$spec 仍含自有登录实现 loginViaAPI"
  echo "   $(basename "$spec"): import=$n_import gate=$n_gate raw=$n_raw login=$n_login"
done

# ---------- 负向对照（把违规样例注入临时副本，必须被抓到）----------
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
cp "emotion-echo-web/e2e/quiz.spec.ts" "$tmp/neg.spec.ts"
printf '\ntest("neg", async ({ page }) => {\n  await page.request.get("http://localhost:19080/api/v1/surveys")\n})\n' >> "$tmp/neg.spec.ts"
printf 'const x = "POST /api/v1/auth/login"\n' >> "$tmp/neg.spec.ts"
neg_raw="$(cnt 'page\.request\.' "$tmp/neg.spec.ts")"
neg_login="$(cnt 'auth/login' "$tmp/neg.spec.ts")"
if [ "$neg_raw" -ge 1 ] && [ "$neg_login" -ge 1 ]; then
  echo "   负向对照：注入裸 page.request.（$neg_raw 处）+ auth/login（$neg_login 处）→ 可检测"
else
  err "负向对照失败：注入的违规样例未被检测到（守卫是弱断言）"
fi

if [ "$fail" -ne 0 ]; then
  echo "FAIL: 闸门契约不成立（见上）"
  exit 1
fi
echo "PASS: 三个 spec 全部走预算闸门，BUDGET 低于 APISIX 阈值，负向对照可检测"
exit 0
