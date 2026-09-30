#!/usr/bin/env bash
# E2E-23 B 组 · 测试点 #13/#14 的跨服务守卫。
#
# 背景（plan §0 F-d）：5 个 Go 服务的 gRPC health 都只在 New() 里置一次
# SERVING，**没有任何 NOT_SERVING 写入**。根因是 healthSrv 是局部变量、
# 没存进 Server 结构 ⇒ 停机分支无从翻转。
#
# 为什么用 shell 守卫而不是每服务一个 Go 测试：5 份几乎相同的
# 源级断言，抄成 5 个 Go 文件会漂移（改一处漏四处，正是本缺陷的形态）。
# 这里从源文本断言"接线确实存在"，一处漏改立即红。
#
# 负向对照：把任一服务的 MarkShuttingDown 调用删掉 → 本脚本非零退出。

set -uo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# 服务目录 → (目录, 真实 gRPC service 全名)
#
# ⚠️ 真实名必须**从 proto 生成的 pb.go 里查**（`ServiceName: "..."`），
#    凭直觉写会写成 proto 里不存在的名字 —— 那种名字注册进 health server 后
#    只有测试自己查得到，真实 gRPC 客户端（APISIX grpc-health-check 等）一律 NOT_FOUND。
#    这正是本守卫第 6 条要防的事。
SERVICES=(
  "emotion-echo-user-svc:emotion_user.v1.UserService"
  "emotion-echo-chat-svc:emotion_chat.v1.ChatService"
  "emotion-echo-analytics-svc:emotion_analytics.v1.AnalyticsService"
  "emotion-echo-assessment-svc:emotion_assessment.v1.AssessmentService"
  "emotion-echo-ai-svc:emotion_ai.v1.EmotionQueryService"
)

fail=0
pass=0

echo "== E2E-23 gRPC health 停机翻转守卫 =="

for entry in "${SERVICES[@]}"; do
  svc="${entry%%:*}"
  real_name="${entry#*:}"
  f="$REPO_ROOT/$svc/internal/grpcserver/server.go"

  if [ ! -f "$f" ]; then
    echo "FAIL [$svc] 找不到 $f"
    fail=$((fail + 1))
    continue
  fi

  # 1) healthSrv 必须存进 Server 结构（否则停机分支拿不到它）
  if ! grep -qE '^\s*healthSrv\s+\*health\.Server' "$f"; then
    echo "FAIL [$svc] Server 结构未持有 healthSrv 字段 —— 停机分支无从翻转状态"
    fail=$((fail + 1))
  else
    echo "PASS [$svc] Server 结构持有 healthSrv"
    pass=$((pass + 1))
  fi

  # 2) 必须有 MarkShuttingDown 方法
  if ! grep -q 'func (s \*Server) MarkShuttingDown()' "$f"; then
    echo "FAIL [$svc] 缺少 MarkShuttingDown 方法"
    fail=$((fail + 1))
  else
    echo "PASS [$svc] 存在 MarkShuttingDown"
    pass=$((pass + 1))
  fi

  # 3) 停机路径必须真的调用它（不能只定义不用）
  if ! grep -q 's\.MarkShuttingDown()' "$f"; then
    echo "FAIL [$svc] 停机路径未调用 MarkShuttingDown —— 方法定义了但没接线"
    fail=$((fail + 1))
  else
    echo "PASS [$svc] 停机路径已调用 MarkShuttingDown"
    pass=$((pass + 1))
  fi

  # 4) 顺序必须是"先翻状态、再 GracefulStop"（反了会丢停机窗口内的请求）
  mark_line="$(grep -n 's\.MarkShuttingDown()' "$f" | tail -1 | cut -d: -f1)"
  stop_line="$(grep -n 'GracefulStop()' "$f" | tail -1 | cut -d: -f1)"
  if [ -n "$mark_line" ] && [ -n "$stop_line" ] && [ "$mark_line" -lt "$stop_line" ]; then
    echo "PASS [$svc] 顺序正确（$mark_line 翻状态 → $stop_line GracefulStop）"
    pass=$((pass + 1))
  else
    echo "FAIL [$svc] 顺序错误：必须先翻 NOT_SERVING 再 GracefulStop（当前 mark=$mark_line stop=$stop_line）"
    fail=$((fail + 1))
  fi

  # 5) 必须真的写入 NOT_SERVING（不是空壳）
  if grep -q 'HealthCheckResponse_NOT_SERVING' "$f"; then
    echo "PASS [$svc] 写入 NOT_SERVING"
    pass=$((pass + 1))
  else
    echo "FAIL [$svc] 未写入 NOT_SERVING —— 翻转是空壳"
    fail=$((fail + 1))
  fi

  # 6) 停机翻转必须覆盖**真实 gRPC service 名**（不只是 "" 服务器级）
  #
  #    背景（第二方核对 2026-09-30 抓出）：此前 5 个服务注册的 per-service 名是
  #    `emotion.User` / `emotion.Chat` / `emotion.AI` ... —— 这些名字**在 proto 里不存在**，
  #    真实全名是 `emotion_user.v1.UserService` 之类（见 emotion-echo-shared 的
  #    `ServiceName: "..."`）。后果：真实客户端用真名 Check 会拿到 NOT_FOUND，
  #    per-service 健康**实际不可查询**；而因为测试用同一个字面量断言，永远绿（自证循环）。
  if grep -q ""$real_name"" "$f"; then
    echo "PASS [$svc] health 注册了真实 service 名 $real_name"
    pass=$((pass + 1))
  else
    echo "FAIL [$svc] health 未注册真实 service 名 $real_name —— per-service 健康对真实客户端不可查询"
    fail=$((fail + 1))
  fi
done

echo
echo "PASS: $pass  FAIL: $fail"

if [ "$fail" -gt 0 ]; then
  echo "RED：gRPC health 停机翻转契约存在缺口"
  exit 1
fi

echo "GREEN：5 个服务的 gRPC health 均会在停机时翻转为 NOT_SERVING"
