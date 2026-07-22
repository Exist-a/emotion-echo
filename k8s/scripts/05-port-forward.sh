#!/usr/bin/env bash
# =====================================================
#  05-port-forward.sh — expose key services via kubectl port-forward
# =====================================================
#  Useful when kind extraPortMappings aren't sufficient. Runs in background.
#  Stop with: bash 99-teardown.sh OR kill the PIDs.
# =====================================================
set -euo pipefail

mkdir -p /tmp/ee-portforwards

echo ">> Starting port-forwards (background)..."
nohup kubectl port-forward -n ee-system svc/apisix-gateway 9080:9080 \
    > /tmp/ee-portforwards/apisix.log 2>&1 &
echo $! > /tmp/ee-portforwards/apisix.pid

nohup kubectl port-forward -n ee-app svc/web 3000:3000 \
    > /tmp/ee-portforwards/web.log 2>&1 &
echo $! > /tmp/ee-portforwards/web.pid

nohup kubectl port-forward -n ee-data svc/skywalking-ui 18080:8080 \
    > /tmp/ee-portforwards/swui.log 2>&1 &
echo $! > /tmp/ee-portforwards/swui.pid

sleep 2
echo ">> Port-forward PIDs:"
ls /tmp/ee-portforwards/
echo
echo ">> Try: curl http://localhost:9080/ping  (APISIX mock-ping route)"
echo "       curl http://localhost:3000/        (Web frontend)"
echo "       open http://localhost:18080        (SkyWalking UI)"
echo
echo ">> Next: bash 06-smoke.sh"