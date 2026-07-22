#!/usr/bin/env bash
# =====================================================
#  03-install-ingress.sh — install APISIX Ingress Controller CRDs
# =====================================================
#  APISIX 3.10+ is used to fix the nginx 301 about:blank bug (Stage 26-Q).
#  The Helm chart itself includes the gateway + dashboard (see
#  charts/emotion-echo/charts/apisix-ingress). This script just installs
#  the CRD definitions from the official chart.
# =====================================================
set -euo pipefail

APISIX_HELM_VERSION="${APISIX_HELM_VERSION:-0.11.0}"

echo ">> Adding apache-apisix-helm-chart repo..."
helm repo add apisix https://charts.apiseven.com 2>/dev/null || true
helm repo update

echo ">> Installing apisix-ingress-controller CRDs (helm chart ${APISIX_HELM_VERSION})..."
# We only need the CRDs from the upstream chart; the controller itself
# is managed by our charts/emotion-echo/charts/apisix-ingress subchart.
helm upgrade --install apisix-crds apisix/apisix \
    --version "${APISIX_HELM_VERSION}" \
    --namespace ee-system \
    --create-namespace \
    --set ingressController.enabled=false \
    --set dashboard.enabled=false \
    --set etcd.enabled=false \
    --wait 2>&1 | tail -20 || true

echo
echo ">> Verifying ApisixRoute CRD is registered..."
kubectl get crd apisixroutes.apisix.apache.org >/dev/null 2>&1 \
    && echo "   OK" \
    || echo "   ! CRD not registered — apisix-routes subchart won't apply"

echo
echo ">> Next: bash 04-install-chart.sh"