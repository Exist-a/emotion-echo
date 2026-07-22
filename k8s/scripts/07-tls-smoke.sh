#!/usr/bin/env bash
# 07-tls-smoke.sh — Stage 29-A.5 live TLS handshake verification.
#
# Drives the same check as TestStage29A5_CertManagerLiveSmoke subtest
# 09 (curl https://grafana.local:9443/api/health → 200) but as a
# standalone script so operators can re-run it on any cluster
# without invoking the full Go test suite.
#
# Flow:
#   1. kubectl port-forward ee-apisix :9443 → host :9443 (HTTPS)
#   2. curl --resolve grafana.local:9443:127.0.0.1 https://.../api/health
#   3. teardown port-forward
#
# Exit 0 on 200 OK, non-zero on any failure.
#
# This script is the canonical manual check; the Go integration test
# in k8s/tests/stage_29a5_smoke_test.go just shells out to it.

set -euo pipefail

APISIX_NAMESPACE="${APISIX_NAMESPACE:-ee-system}"
APISIX_SERVICE="${APISIX_SERVICE:-ee-apisix}"
TLS_HOST="${TLS_HOST:-grafana.local}"
LOCAL_PORT="${LOCAL_PORT:-9443}"

echo "[07-tls-smoke] starting TLS handshake check for ${TLS_HOST}:${LOCAL_PORT}"

# 1. port-forward APISIX HTTPS port. APISIX 3.x data plane exposes
#    :9443 as the TLS listener by default (configured in
#    apisix-ingress subchart values.yaml).
PF_PID_FILE="/tmp/ee-portforwards/07-tls-smoke.pid"
mkdir -p /tmp/ee-portforwards

cleanup() {
  if [[ -f "$PF_PID_FILE" ]]; then
    local pid
    pid="$(cat "$PF_PID_FILE")"
    kill "$pid" 2>/dev/null || true
    rm -f "$PF_PID_FILE"
    echo "[07-tls-smoke] port-forward torn down (pid=$pid)"
  fi
}
trap cleanup EXIT

kubectl port-forward -n "$APISIX_NAMESPACE" "svc/${APISIX_SERVICE}" "${LOCAL_PORT}:9443" \
  > /tmp/ee-portforwards/07-tls-smoke.log 2>&1 &
echo $! > "$PF_PID_FILE"
echo "[07-tls-smoke] port-forward pid=$(cat "$PF_PID_FILE") → /tmp/ee-portforwards/07-tls-smoke.log"

# Wait for port-forward to come up (port-forward binds lazily).
for i in $(seq 1 30); do
  if (echo > "/dev/tcp/127.0.0.1/${LOCAL_PORT}") 2>/dev/null; then
    break
  fi
  sleep 0.2
done

if ! (echo > "/dev/tcp/127.0.0.1/${LOCAL_PORT}") 2>/dev/null; then
  echo "[07-tls-smoke] FAIL: port-forward never bound 127.0.0.1:${LOCAL_PORT}" >&2
  cat /tmp/ee-portforwards/07-tls-smoke.log >&2 || true
  exit 2
fi

# 2. curl TLS handshake + 200. -k accepts the self-signed cert;
#    --resolve forces grafana.local → 127.0.0.1 so we don't need DNS.
echo "[07-tls-smoke] curling https://${TLS_HOST}:${LOCAL_PORT}/api/health"
HTTP_CODE=$(curl -k -s -o /tmp/ee-portforwards/07-tls-smoke.body \
  -w '%{http_code}' \
  --resolve "${TLS_HOST}:${LOCAL_PORT}:127.0.0.1" \
  --max-time 10 \
  "https://${TLS_HOST}:${LOCAL_PORT}/api/health")

if [[ "$HTTP_CODE" != "200" ]]; then
  echo "[07-tls-smoke] FAIL: expected HTTP 200, got ${HTTP_CODE}" >&2
  echo "--- response body ---" >&2
  cat /tmp/ee-portforwards/07-tls-smoke.body >&2 || true
  exit 1
fi

echo "[07-tls-smoke] PASS: HTTPS 200 OK from https://${TLS_HOST}:${LOCAL_PORT}/api/health"
echo "--- response body (first 200 chars) ---"
head -c 200 /tmp/ee-portforwards/07-tls-smoke.body
echo
exit 0