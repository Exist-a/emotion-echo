#!/usr/bin/env bash
# scripts/test_compose_v55_cross_file.sh
#
# Compose v5.5.1 跨文件 depends_on 阻塞回归测试（2026-09-16）
#
# 背景：Docker Compose v5.5.1 对跨 compose 文件的 depends_on 严格校验（要求引用
#       service 在同一 project），而 infra + apps 分层文件把依赖切到了两边：
#         apps.yml 的 emotion-echo-chat-svc 引用 infra.yml 的 nacos / postgres / kafka
#       v2.20 之前允许跨文件依赖；v5.5.1 报：
#         "service ... depends on undefined service nacos: invalid compose project"
#       直接阻塞 `docker compose -f infra.yml -f apps.yml -f dev.yml up -d`。
#
# 修复策略：把跨文件 depends_on 改为"只在同一文件时保留"——infra 与 apps
#       分层架构下，业务 svc 启动时 infra 已经先 up，apps 内只保留本文件内的
#       依赖（emotion-echo-db-migrate 是 apps.yml 内的 init 容器）。
#
# 验证项：
#   1. `docker compose -f infra.yml -f apps.yml -f dev.yml config --services`
#      能列出全部 service（之前是 1 行错误后立即 bail）
#   2. 业务 svc 的 depends_on 不再引用 infra 的 service key（postgres / kafka /
#      nacos / skywalking-oap / redis），仅保留 apps.yml 内的依赖
#      （emotion-echo-db-migrate / emotion-llm-service）
#   3. dev 链路完整 docker compose config --quiet 退出码 0
#
# 关联：roadmap.md 已知 §"当前 open 清单" + §十六 第 2 轮 backlog
#       deploy/configuration.md §1 文件结构
#       QUICKSTART.md §2 步骤 4 启动命令

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
DEPLOY_DIR="$SCRIPT_DIR/../deploy"
APPS_FILE="$DEPLOY_DIR/docker-compose.apps.yml"
INFRA_FILE="$DEPLOY_DIR/docker-compose.infra.yml"
DEV_FILE="$DEPLOY_DIR/compose.dev.yml"

pass=0
fail=0

echo "=== TDD: Compose v5.5.1 跨文件 depends_on 阻塞回归 ==="
echo

# === 1) docker compose config --services 能列出全部 service ===
echo "--- 1) config --services 不应因跨文件依赖 bail ---"
cd "$DEPLOY_DIR"
if docker compose -f "$INFRA_FILE" -f "$APPS_FILE" -f "$DEV_FILE" config --services >/tmp/compose_v55_services.txt 2>&1; then
  svc_count=$(wc -l < /tmp/compose_v55_services.txt)
  echo "  ✓ config --services 成功（列出 $svc_count 个 service）"
  pass=$((pass + 1))
else
  echo "  ✗ config --services 失败（v5.5.1 跨文件依赖阻塞）："
  cat /tmp/compose_v55_services.txt
  fail=$((fail + 1))
fi
cd - >/dev/null

echo
echo "--- 2) apps.yml 业务 svc 不应再引用 infra 的 service key ---"
# infra 的 service key 集合
INFRA_KEYS="postgres|redis|kafka|nacos|skywalking-oap|skywalking-ui|etcd|apisix|emotion-echo-minio|emotion-echo-minio-init"
# apps.yml 业务 svc 列表（不含 db-migrate / apisix-seed / web / AI profile）
APPS_BUSINESS_SVCS=(
  "emotion-echo-user-svc"
  "emotion-echo-chat-svc"
  "emotion-echo-analytics-svc"
  "emotion-echo-assessment-svc"
  "emotion-llm-service"
  "emotion-echo-ai-svc"
  "emotion-echo-web-bff"
)
for svc in "${APPS_BUSINESS_SVCS[@]}"; do
  # 提取 svc block
  block=$(awk -v target="^  ${svc}:" '
    $0 ~ target { in_block = 1; next }
    in_block && /^  [a-zA-Z0-9_-]+:/ && $0 !~ target { in_block = 0 }
    in_block { print }
  ' "$APPS_FILE")
  # 检查是否引用 infra 的 service key（仅 depends_on 段）
  depends_block=$(echo "$block" | awk '/^    depends_on:/{flag=1; next} /^    [a-z]/ && !/depends_on/ {flag=0} flag')
  bad_refs=$(echo "$depends_block" | grep -E "^      ($INFRA_KEYS):" || true)
  if [ -n "$bad_refs" ]; then
    echo "  ✗ $svc 仍引用 infra service key："
    echo "$bad_refs" | sed 's/^/      /'
    fail=$((fail + 1))
  else
    echo "  ✓ $svc 不引用 infra service key"
    pass=$((pass + 1))
  fi
done

echo
echo "--- 3) dev 链路完整 config --quiet 退出码 0 ---"
cd "$DEPLOY_DIR"
if docker compose -f "$INFRA_FILE" -f "$APPS_FILE" -f "$DEV_FILE" config --quiet 2>/dev/null; then
  echo "  ✓ dev 链路 config 语法合法（退出码 0）"
  pass=$((pass + 1))
else
  echo "  ✗ dev 链路 config 语法错误（v5.5.1 仍阻塞）："
  docker compose -f "$INFRA_FILE" -f "$APPS_FILE" -f "$DEV_FILE" config 2>&1 | tail -5
  fail=$((fail + 1))
fi
cd - >/dev/null

echo
echo "=== Result: $pass passed, $fail failed ==="
if [ $fail -gt 0 ]; then
  exit 1
fi
exit 0
