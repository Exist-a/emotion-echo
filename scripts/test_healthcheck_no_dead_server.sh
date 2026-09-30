#!/usr/bin/env bash
# test_healthcheck_no_dead_server.sh —— 防"生产死代码回流"的回归钉
#
# 背景（E2E-23 F-163，用户 2026-09-30 裁定"选 A：删掉 Resume()"）：
#   `emotion-echo-shared/pkg/healthcheck` 里曾有一个 `Server` 包装类型
#   （NewServer / RegisterWith / SetServingStatus / GetServingStatus / Shutdown / Resume），
#   承载 plan B1 的"停机翻 NOT_SERVING"接线需求。
#
#   核实结论：**它在整个生产路径上一次都没有被实例化**。
#   · 5 个 Go 服务的 `internal/grpcserver/server.go` import 的是上游
#     `google.golang.org/grpc/health`，用的是上游自带的 `health.NewServer()`；
#   · 承载 Shutdown()/Resume() 的那个类型，全仓 grep 只在它自己包的测试里被 new；
#   · 该包在生产里被用到的只有 **client 侧**（ai-svc 的 `healthcheck.NewClient`）。
#
#   即：plan B1 想要的语义**早已达成**（经由 `MarkShuttingDown()` 调
#   `SetServingStatus(..., NOT_SERVING)`），而这个包装类型是纯粹的孤儿 ——
#   正是 anti-patterns **AP-10「孤儿产出物」**的形态。
#
# 为什么需要守卫（守卫只报不拦是本项目反复吃过的亏，这里要有钉）：
#   删除死代码之后，若日后有人"顺手"再写一个 server 侧包装，
#   会同时踩两个坑：① 没人调用（AP-10）；② 更坏的是**它与 5 个服务实际使用的
#   上游 health server 形成两套并存的健康语义**，运维会去查错地方。
#   本守卫把"生产只用上游 health server、client 侧唯一活着的 API"钉死。
#
# 用法：bash scripts/test_healthcheck_no_dead_server.sh
# 退出码：0 = 通过；1 = 有 FAIL；2 = 缺解释器 / 缺 go

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$REPO_ROOT"

PKG_DIR="emotion-echo-shared/pkg/healthcheck"

PY=""
for cand in python3 python py; do
  command -v "$cand" >/dev/null 2>&1 && v="$("$cand" --version 2>&1)" && [ -n "$v" ] && { PY="$cand"; break; }
done
if [ -z "$PY" ]; then
  echo "FATAL: 找不到可用 python" >&2; exit 2
fi
if ! command -v go >/dev/null 2>&1; then
  echo "FATAL: 找不到 go" >&2; exit 2
fi

PASS=0; FAIL=0
ok()  { PASS=$((PASS+1)); echo "  PASS  $1"; }
bad() { FAIL=$((FAIL+1)); echo "  FAIL  $1"; }

echo "== healthcheck 生产死代码守卫 =="

# ---------------------------------------------------------------- 1. server 侧包装已删除
echo
echo "-- 1. healthcheck 包不得再有 server 侧包装 --"
dead=""
for sym in "type Server struct" "func NewServer()" "func (s \*Server) RegisterWith" \
           "func (s \*Server) Shutdown()" "func (s \*Server) Resume()"; do
  if grep -qR "$sym" "$PKG_DIR" 2>/dev/null; then
    dead="$dead $sym"
  fi
done
if [ -z "$dead" ]; then
  ok "Server / NewServer / RegisterWith / Shutdown / Resume 均已不存在"
else
  bad "以下 server 侧符号仍存在（生产无调用方 = AP-10 孤儿）：$dead"
fi

# ---------------------------------------------------------------- 2. 生产代码不引用它
echo
echo "-- 2. 全仓**非测试**代码不得引用 healthcheck 的 server 侧 --"
hits="$(grep -rn --include=*.go "healthcheck\.\(NewServer\|Server\)\|\.RegisterWith(" . 2>/dev/null \
        | grep -v "_test\.go" || true)"
if [ -z "$hits" ]; then
  ok "生产代码零引用"
else
  bad "生产代码仍引用：$(printf '%s' "$hits" | tr '\n' ' ' | cut -c1-160)"
fi

# ---------------------------------------------------------------- 3. 5 个服务用上游 health server
echo
echo "-- 3. 5 个服务的 gRPC 端必须用上游 google.golang.org/grpc/health --"
missing=""
for svc in user chat ai analytics assessment; do
  f="emotion-echo-${svc}-svc/internal/grpcserver/server.go"
  [ -f "$f" ] || { missing="$missing $f(缺文件)"; continue; }
  grep -q '"google.golang.org/grpc/health"' "$f" || missing="$missing ${svc}(未import上游health)"
  grep -q 'MarkShuttingDown' "$f" || missing="$missing ${svc}(无MarkShuttingDown)"
done
if [ -z "$missing" ]; then
  ok "user/chat/ai/analytics/assessment 五个服务均用上游 health server 且有 MarkShuttingDown"
else
  bad "以下服务不符合：$missing"
fi

# ---------------------------------------------------------------- 4. 停机翻转语义仍被钉住
echo
echo "-- 4. 停机翻 NOT_SERVING 的**真实行为**必须仍被测试钉住 --"
# 删除死代码不得连带删掉有效覆盖：MarkShuttingDown 的行为测试必须还在
bev="$(grep -rl "MarkShuttingDown" --include=*_test.go . 2>/dev/null | tr '\n' ' ')"
if [ -n "$bev" ]; then
  ok "MarkShuttingDown 的行为测试存在：$(printf '%s' "$bev" | cut -c1-120)"
else
  bad "找不到 MarkShuttingDown 的行为测试 —— 删死代码时把有效覆盖一起删了"
fi

# ---------------------------------------------------------------- 5. client 测试跑在真上游 server 上
echo
echo "-- 5. client 测试必须跑在**上游** health server 上（保真度，不是包装替身）--"
if grep -q '"google.golang.org/grpc/health"' "$PKG_DIR/health_test.go" 2>/dev/null; then
  ok "health_test.go 使用上游 health server —— client 与生产同一实现对话"
else
  bad "health_test.go 未使用上游 health server —— client 测试在对着被删/替身实现测，无生产保真度"
fi

# ---------------------------------------------------------------- 6. 编译与测试
echo
echo "-- 6. 包能编译且测试全绿 --"
if (cd emotion-echo-shared && go vet ./pkg/healthcheck/ 2>&1 | tail -5 | grep -q .); then
  bad "go vet 有输出：$(cd emotion-echo-shared && go vet ./pkg/healthcheck/ 2>&1 | tail -3 | tr '\n' ' ')"
elif (cd emotion-echo-shared && go test ./pkg/healthcheck/ -count=1 2>&1 | tail -3 | grep -q "^ok"); then
  ok "go vet 干净 + go test 通过"
else
  bad "go test 未通过：$(cd emotion-echo-shared && go test ./pkg/healthcheck/ -count=1 2>&1 | tail -3 | tr '\n' ' ')"
fi

echo
echo "PASS: $PASS  FAIL: $FAIL"
if [ "$FAIL" -gt 0 ]; then
  echo "RED：healthcheck 存在生产死代码，或删死代码时连带删掉了有效覆盖"
  exit 1
fi
echo "GREEN：生产只用上游 health server；client 侧 API 唯一存活；停机翻转语义仍被行为测试钉住"
exit 0
