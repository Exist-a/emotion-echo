#!/usr/bin/env bash
# E2E-23 复核轮新增：build tag 下的代码必须也能编译。
#
# 背景（真缺陷，本阶段自己造成）：E2E-23 的 commit 81da12d 改
# `Consumer.Consume` 签名时，把 ai-svc 集成测试里一行 `}()` 手误改成 `}(, nil)`，
# 又漏了新增的第 8 个参数 —— `integration_test/dlq_integration_test.go` **语法错误**。
# 但它带 `//go:build integration`，所以：
#   - `go test ./...`   跳过（默认不含 integration tag）
#   - `go vet ./...`    跳过（同上）
#   - CI 的 go-test     跳过（同上）
# ⇒ **全仓门禁全绿，而 AGENTS.md §1.1 要求的 `go test -tags integration ./...` 是坏的。**
#
# 这类缺陷"看不见"的根因不是有人疏忽，而是**没有任何门禁编译 build tag 下的代码**。
# 本守卫补上这一层。
#
# 断言强度：用 `go vet -tags integration`（它**会编译测试文件**，
# 等价于 `go build` + 完整类型检查），**不是** `go build`（后者不编译 _test.go）。

set -uo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

MODULES="emotion-echo-user-svc emotion-echo-chat-svc emotion-echo-assessment-svc emotion-echo-analytics-svc emotion-echo-ai-svc emotion-echo-web-bff emotion-echo-shared"

# 已知编译不过的模块 → 账本编号。**修好一个就从这里删一行，守卫随之自动转严。**
# 每一项都必须有账本编号可查，不允许"先挂着以后再说"。
KNOWN_BROKEN="emotion-echo-user-svc:E2E-F-167 emotion-echo-web-bff:E2E-F-167"

fail=0
pass=0
warn=0

echo "== build tag 下的代码编译守卫（go vet -tags integration）=="

if ! command -v go >/dev/null 2>&1; then
  echo "FAIL 环境无 go 可执行文件 ⇒ 本项**未验证**（不是通过）。装 Go 后重跑。"
  exit 1
fi

for m in $MODULES; do
  d="$REPO_ROOT/$m"
  if [ ! -f "$d/go.mod" ]; then
    echo "SKIP [$m] 无 go.mod，不是有 Go 模块"
    continue
  fi

  out="$(cd "$d" && go vet -tags integration ./... 2>&1)"
  rc=$?

  known=""
  for kb in $KNOWN_BROKEN; do
    [ "${kb%%:*}" = "$m" ] && known="${kb#*:}"
  done

  if [ $rc -eq 0 ]; then
    echo "PASS [$m] go vet -tags integration 干净"
    pass=$((pass + 1))
  elif [ -n "$known" ]; then
    echo "WARN [$m] 集成测试仍编译不过（账本 $known，pre-existing，本阶段未引入）"
    printf '%s\n' "$out" | head -3 | sed 's/^/       /'
    warn=$((warn + 1))
  else
    echo "FAIL [$m] go vet -tags integration 编译失败 —— 带 build tag 的代码没有任何门禁在管"
    printf '%s\n' "$out" | head -5 | sed 's/^/       /'
    fail=$((fail + 1))
  fi
done

echo
echo "PASS: $pass  WARN(已知债): $warn  FAIL(新增损坏): $fail"

# ⚠️ 退出码是 **ratchet**（棘轮），必须讲清楚，否则等于制造一个常红的门禁：
#
#   · KNOWN_BROKEN 里的模块（账本 E2E-F-167）⇒ 报醒目 WARN 但**不判红**。
#     理由：它们在本阶段之前就坏，属于别的功能块。把常红门禁接进 CI，
#     只会训练出"看到红色就跳过"——正是 anti-patterns AP-11「门禁只报不拦」
#     与"常红 = 没人看"的直接成因。
#   · **任何不在清单里的模块编译不过 ⇒ 一律判红**。这才是本守卫的价值：
#     把"没有任何门禁编译 build tag 代码"这个盲区变成"新增损坏立刻可见"。
#   · 修好一个已知项就从 KNOWN_BROKEN 删一行，守卫随之自动转严。
#     所以 WARN 数量只会单调下降；一旦变多，就是有人往清单里塞了新债。
if [ "$fail" -gt 0 ]; then
  echo "RED：出现**清单外**的 build tag 编译错误（逐条见上）"
  exit 1
fi

if [ "$warn" -gt 0 ]; then
  echo "GREEN（有条件）：新增损坏 0 项；已知债 $warn 项待修，见账本 E2E-F-167"
else
  echo "GREEN：所有模块在 -tags integration 下均能编译"
fi
