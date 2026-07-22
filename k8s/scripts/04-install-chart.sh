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

# The cert-manager subchart manages its own `cert-manager` namespace
# (Stage 29-A.5), but the umbrella's release namespace is `ee-app`.
# Pre-creating ee-app ensures `helm install --create-namespace`
# doesn't race with the chart's own namespace creation if the
# release is reinstalled after a manual namespace deletion.
echo ">> Ensuring release namespace exists..."
kubectl create namespace ee-app --dry-run=client -o yaml | kubectl apply -f - >/dev/null

echo ">> helm upgrade --install ${RELEASE} ${CHART_DIR}..."
helm upgrade --install "${RELEASE}" "${CHART_DIR}" \
    --namespace ee-app \
    -f "${VALUES_DEV}" \
    --wait \
    --wait-for-jobs \
    --timeout 15m \
    2>&1 | tail -60

# Bumped timeout 10m → 15m and added --wait-for-jobs: cert-manager
# needs to roll out three Deployments (controller + cainjector +
# webhook) plus the webhook serving cert dance. On cold-start
# clusters the cert-manager image pull alone can take 2-3 minutes
# behind a slow registry mirror.

echo
echo ">> Waiting for cert-manager components (controller + cainjector + webhook)..."
for component in controller cainjector webhook; do
  kubectl wait --for=condition=Available \
      --timeout=240s \
      -n cert-manager \
      "deployment/ee-cert-manager-${component}" \
      2>&1 | tail -5 || echo "(cert-manager-${component} may still be rolling out)"
done

echo
echo ">> Waiting for all emotion-echo Deployments to be Available..."
kubectl wait --for=condition=Available \
    --timeout=300s \
    -A \
    -l app.kubernetes.io/part-of=emotion-echo \
    deployment 2>&1 | tail -20 || echo "(some deployments may still be pending)"

# Stage 29-A.5: APISIX must be Ready before the TLS smoke (07) can
# succeed. Wait on it explicitly so callers don't have to repeat.
echo
echo ">> Waiting for APISIX data plane..."
kubectl wait --for=condition=Available \
    --timeout=180s \
    -n ee-system \
    deployment/ee-apisix 2>&1 | tail -5 || echo "(APISIX may still be rolling out)"

echo
echo ">> Cluster state:"
kubectl get pods -A -o wide | grep emotion-echo || true

echo
echo ">> Next: bash 05-port-forward.sh  (then optional: bash 07-tls-smoke.sh)"