#!/usr/bin/env bash
# =====================================================
#  04-install-chart.sh — install/upgrade the Emotion-Echo umbrella Helm chart
# =====================================================
#  Run AFTER: 01-create-cluster.sh, 02-load-images.sh, 03-install-ingress.sh
# =====================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
CHART_DIR="${REPO_ROOT}/charts/emotion-echo"
VALUES_DEV="${CHART_DIR}/values-dev.yaml"
RELEASE="${EE_RELEASE:-ee}"

echo ">> helm upgrade --install ${RELEASE} ${CHART_DIR}..."
helm upgrade --install "${RELEASE}" "${CHART_DIR}" \
    --namespace ee-app \
    --create-namespace \
    -f "${VALUES_DEV}" \
    --wait \
    --timeout 10m \
    2>&1 | tail -40

echo
echo ">> Waiting for all Deployments to be Available..."
kubectl wait --for=condition=Available \
    --timeout=300s \
    -A \
    -l app.kubernetes.io/part-of=emotion-echo \
    deployment 2>&1 | tail -20 || echo "(some deployments may still be pending)"

echo
echo ">> Cluster state:"
kubectl get pods -A -o wide | grep emotion-echo || true

echo
echo ">> Next: bash 05-port-forward.sh"