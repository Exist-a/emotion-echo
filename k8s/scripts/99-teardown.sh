#!/usr/bin/env bash
# =====================================================
#  99-teardown.sh — nuke everything to a clean slate
# =====================================================
set -euo pipefail

CLUSTER_NAME="${EE_CLUSTER:-emotion-echo}"

echo ">> Killing port-forwards..."
for f in /tmp/ee-portforwards/*.pid; do
    [ -f "$f" ] && kill "$(cat "$f")" 2>/dev/null && rm "$f"
done

echo ">> Deleting Helm release..."
helm uninstall ee -n ee-app 2>/dev/null || true
helm uninstall ee -n ee-data 2>/dev/null || true
helm uninstall ee -n ee-system 2>/dev/null || true
helm uninstall apisix-crds -n ee-system 2>/dev/null || true

echo ">> Deleting kind cluster '${CLUSTER_NAME}'..."
kind delete cluster --name "${CLUSTER_NAME}"

echo ">> Done."