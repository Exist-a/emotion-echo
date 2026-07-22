#!/usr/bin/env bash
# =====================================================
#  01-create-cluster.sh — create kind cluster for Emotion-Echo
# =====================================================
set -euo pipefail

CLUSTER_NAME="${EE_CLUSTER:-emotion-echo}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
K8S_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

echo ">> Creating kind cluster '${CLUSTER_NAME}'..."
kind create cluster \
    --name "${CLUSTER_NAME}" \
    --config "${K8S_ROOT}/kind-config.yaml" \
    --wait 60s

echo ">> Cluster ready. Nodes:"
kubectl get nodes -o wide

echo
echo ">> Next: bash 02-load-images.sh"