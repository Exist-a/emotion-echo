#!/usr/bin/env bash
# smoke_nacos_registry.sh — PR-5 §契约 7：Nacos 注册表 hosts 非空
#
# 验证：dev 启动后，6 个业务 svc 在 Nacos 注册表里 hosts.length >= 1 且 healthy。
# 防止"dev 模式走通但 prod 走通是巧合"——本契约锁定注册表可信度。
#
# 用法：bash scripts/smoke_nacos_registry.sh
# 前置：docker compose -f deploy/docker-compose.infra.yml -f deploy/docker-compose.apps.yml up -d
# 退出码：0 = 全绿；1 = 至少 1 个 svc hosts=[]

set -euo pipefail

NACOS_ADDR="${NACOS_ADDR:-http://localhost:8848}"
NAMESPACE="${NAMESPACE:-emotion-echo-dev}"
GROUP="${GROUP:-DEFAULT_GROUP}"

# Git Bash 上 'python3' 命令可能是 WindowsApps stub（exit 49），用真 Python 全路径兜底。
PYTHON_BIN="${PYTHON_BIN:-/c/Users/LENVOV/AppData/Local/Programs/Python/Python312/python.exe}"
if [ ! -x "$PYTHON_BIN" ]; then
  PYTHON_BIN="$(command -v python3 || command -v python || echo python3)"
fi

fail=0
for svc in emotion-echo-user-svc emotion-echo-chat-svc emotion-echo-analytics-svc \
           emotion-echo-assessment-svc emotion-echo-ai-svc emotion-echo-web-bff; do
  body=$(curl -sf "${NACOS_ADDR}/nacos/v1/ns/instance/list?serviceName=${svc}&namespaceId=${NAMESPACE}&groupName=${GROUP}" 2>/dev/null) || {
    echo "[FAIL] ${svc}: Nacos 不可达 (${NACOS_ADDR})"
    fail=1; continue
  }
  hosts=$(printf '%s' "$body" | "$PYTHON_BIN" -c "import sys,json; d=json.load(sys.stdin); print(len(d.get('hosts',[])))" 2>/dev/null || echo 0)
  if [ "$hosts" -ge 1 ]; then
    echo "[PASS] ${svc}: ${hosts} instance(s) registered"
  else
    echo "[FAIL] ${svc}: hosts=[] (注册表无实例)"
    fail=1
  fi
done

if [ $fail -ne 0 ]; then
  echo "smoke §契约 7 FAIL"
  exit 1
fi
echo "smoke §契约 7 PASS — 6/6 svc registered"