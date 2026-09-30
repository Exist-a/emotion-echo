#!/usr/bin/env bash
# test_health_nilrepo_truthful.sh —— 「降级启动时健康探针必须说假话」的回归钉
#
# 缺陷（E2E-23 F-96 实测抓出，2026-09-30）：
#   5 个 Go 服务的 `healthlogic.go` 都是这个写法：
#
#       dbOK := true
#       if l.svcCtx.XxxRepo != nil {      // ← repo 为 nil 时整个 if 被跳过
#           if err := l.svcCtx.XxxRepo.Ping(ctx); err != nil { dbOK = false }
#       }
#
#   `main.go` 的降级启动是**单次连接尝试**，失败则 `repo = nil` 照常启动
#   （`user-svc/main.go:85-93`）。于是**恰恰在数据库完全不可用时**，
#   `dbOK` 保持 **true**、`status` 保持 **ok**。
#
# 运行时实测（2026-09-30 dev 栈）：
#   停 Postgres → 强制重建 user-svc → 日志 `connect failed`（降级启动已发生）
#   → `/health` 仍返 `{"status":"ok","dbOk":true}`
#   → `/health/ready` 仍返 **200**
#   → `docker ps` 显示容器 **(healthy)**
#   → 经网关 `/api/v1/users/me` 实际返回
#     `upstream unavailable: user-svc repository not initialized (degraded start)`
#
# 危害（三层，逐层放大）：
#   ① 直接违反 **D-29**（E2E-23 自己的决议：status 必须说真话，
#      readiness 承载 200/503）。测试点 #2/#3 只覆盖了"repo 存在但 ping 失败"，
#      **没覆盖"repo 根本没建起来"**，所以它带着 PASS 溜过去了。
#   ② compose healthcheck 打的是 `/health/ready` ⇒ 降级服务被判 **healthy**
#      ⇒ 任何 `depends_on: condition: service_healthy` 闸门（含 `apisix-seed`）形同虚设。
#   ③ APISIX 照常把流量路由过去，而该服务的每个 DB 调用都返 `Unavailable`
#      ⇒ **"网关通、后端全挂"**，且**零告警**。
#
# 为什么用源码扫描而不是 5 份行为测试：5 个服务同型同构，
# 逐个写行为测试要 5 份近乎重复的替身；这里守的是**"不许再写回这个反模式"**。
# 语义正确性由 `emotion-echo-user-svc/internal/logic/healthlogic_degraded_test.go`
# 的行为测试钉住（RED→GREEN 实测过），本守卫负责**覆盖面**。
#
# 用法：bash scripts/test_health_nilrepo_truthful.sh
# 退出码：0 = 通过；1 = 有 FAIL

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$REPO_ROOT"

SVCS=(user-svc chat-svc analytics-svc assessment-svc ai-svc)

PASS=0; FAIL=0
ok()  { PASS=$((PASS+1)); echo "  PASS  $1"; }
bad() { FAIL=$((FAIL+1)); echo "  FAIL  $1"; }

echo "== 降级启动时健康探针必须说假话 =="

# ---------------------------------------------------------------- 1. 反模式不得存在
echo
echo "-- 1. 5 个服务都不得存在 'dbOK := true 起步 + if repo != nil 才 ping' 的反模式 --"
for s in "${SVCS[@]}"; do
  f="emotion-echo-${s}/internal/logic/healthlogic.go"
  if [ ! -f "$f" ]; then
    bad "$s：找不到 $f"
    continue
  fi
  # 提取 dbOK 的初值与随后的 repo 判空："dbOK := true" 之后紧跟 "XxxRepo != nil {"
  # 注意：消息里一律不用反引号 —— 它们会被 shell 当成命令替换，
  # 那会让**失败原因**变成乱码，制造"假信号源"（本脚本第一版就踩了这个）。
  if grep -A3 'dbOK := true' "$f" | grep -qE 'Repo != nil \{'; then
    bad "$s：仍是 'dbOK := true' + 'if XxxRepo != nil' 的反模式 —— repo 为 nil 时会误报健康"
  else
    ok "$s：repo 为 nil 时如实报不可用"
  fi
done

# ---------------------------------------------------------------- 2. 显式断言存在
echo
echo "-- 2. 每个服务都必须显式处理 'repo 为 nil' 这一分支 --"
for s in "${SVCS[@]}"; do
  f="emotion-echo-${s}/internal/logic/healthlogic.go"
  [ -f "$f" ] || continue
  if grep -qE 'Repo == nil|Repo != nil' "$f"; then
    ok "$s：显式判定了 repo 是否为 nil"
  else
    bad "$s：没有显式判定 repo 是否为 nil"
  fi
done

# ---------------------------------------------------------------- 3. 行为测试存在
echo
echo "-- 3. 语义正确性由行为测试钉住（user-svc 为代表） --"
bt="emotion-echo-user-svc/internal/logic/healthlogic_degraded_test.go"
if [ -f "$bt" ]; then
  if grep -q "degraded\|Degraded" "$bt"; then
    ok "存在 healthlogic_degraded_test.go 且断言了 degraded"
  else
    bad "healthlogic_degraded_test.go 存在但没有断言 degraded"
  fi
else
  bad "缺少 healthlogic_degraded_test.go —— 反模式被改回时没有行为层兜底"
fi

echo
echo "PASS: $PASS  FAIL: $FAIL"
if [ "$FAIL" -gt 0 ]; then
  echo "RED：存在'降级启动却报健康'的服务 —— 网关会照常路由，DB 全挂且零告警"
  exit 1
fi
echo "GREEN：5 个服务在 repo 为 nil（降级启动）时都如实报不可用"
exit 0
