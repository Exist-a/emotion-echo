#!/usr/bin/env bash
# 07-tls-smoke.sh — Stage 29-A.5 / 29-D live TLS handshake verification.
#
# Stage 29-A.5: drives the same check as
# TestStage29A5_CertManagerLiveSmoke subtest 09
# (curl https://grafana.local:9443/api/health → 200) as a standalone
# script so operators can re-run it on any cluster.
#
# Stage 29-D: also drives the per-family TLS check
# (curl https://<family>.echo.local:9443/health → 200) for the 5
# business-route families (user / chat / analytics / assessment / ai).
# Use the HEALTH_PATH env var to override `/api/health` (default for
# Grafana) → `/health` (default for the 5 Go backend services).
#
# Flow:
#   1. kubectl port-forward ee-apisix :9443 → host :9443 (HTTPS)
#   2. curl --resolve <TLS_HOST>:9443:127.0.0.1 https://<TLS_HOST>:9443<HEALTH_PATH>
#   3. teardown port-forward
#
# Exit 0 on 200 OK, non-zero on any failure.
#
# This script is the canonical manual check; the Go integration tests
# in k8s/tests/stage_29a5_smoke_test.go and k8s/tests/stage_29d_smoke_test.go
# both shell out to it.

set -euo pipefail

APISIX_NAMESPACE="${APISIX_NAMESPACE:-ee-system}"
APISIX_SERVICE="${APISIX_SERVICE:-ee-apisix}"
TLS_HOST="${TLS_HOST:-grafana.local}"
LOCAL_PORT="${LOCAL_PORT:-9443}"
HEALTH_PATH="${HEALTH_PATH:-/api/health}"

echo "[07-tls-smoke] starting TLS handshake check for https://${TLS_HOST}:${LOCAL_PORT}${HEALTH_PATH}"

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
#    --resolve forces <TLS_HOST> → 127.0.0.1 so we don't need DNS.
#    HEALTH_PATH lets callers hit /health (Go svc) or /api/health (Grafana).
echo "[07-tls-smoke] curling https://${TLS_HOST}:${LOCAL_PORT}${HEALTH_PATH}"
HTTP_CODE=$(curl -k -s -o /tmp/ee-portforwards/07-tls-smoke.body \
  -w '%{http_code}' \
  --resolve "${TLS_HOST}:${LOCAL_PORT}:127.0.0.1" \
  --max-time 10 \
  "https://${TLS_HOST}:${LOCAL_PORT}${HEALTH_PATH}")

if [[ "$HTTP_CODE" != "200" ]]; then
  echo "[07-tls-smoke] FAIL: expected HTTP 200 from ${HEALTH_PATH}, got ${HTTP_CODE}" >&2
  echo "--- response body ---" >&2
  cat /tmp/ee-portforwards/07-tls-smoke.body >&2 || true
  exit 1
fi

echo "[07-tls-smoke] PASS: HTTPS 200 OK from https://${TLS_HOST}:${LOCAL_PORT}${HEALTH_PATH}"
echo "--- response body (first 200 chars) ---"
head -c 200 /tmp/ee-portforwards/07-tls-smoke.body
echo
exit 0