#!/usr/bin/env bash
# =====================================================
#  02-load-images.sh — load pre-built Docker images into kind
# =====================================================
#  Run AFTER: bash 01-create-cluster.sh
#  Run BEFORE: bash 04-install-chart.sh
#
#  This avoids `docker push` to a registry and works offline.
#  Images MUST be built first with:
#    docker build -t emotion-echo/<svc>:v0.1.0 ...
# =====================================================
set -euo pipefail

CLUSTER_NAME="${EE_CLUSTER:-emotion-echo}"
IMAGES=(
  "emotion-echo/user-svc:v0.1.0"
  "emotion-echo/chat-svc:v0.1.0"
  "emotion-echo/analytics-svc:v0.1.0"
  "emotion-echo/assessment-svc:v0.1.0"
  "emotion-echo/ai-svc:v0.1.0"
  "emotion-echo/llm-service:v0.1.0"
  "emotion-echo/fer:v0.1.0"
  "emotion-echo/sensevoice:v0.1.0"
  "emotion-echo/xtts:v0.1.0"
  "emotion-echo/web:v0.1.0"
)

echo ">> Loading ${#IMAGES[@]} images into kind cluster '${CLUSTER_NAME}'..."
for img in "${IMAGES[@]}"; do
  if docker image inspect "${img}" >/dev/null 2>&1; then
    echo "   - ${img}"
    kind load docker-image "${img}" --name "${CLUSTER_NAME}"
  else
    echo "   ! skip (not built locally): ${img}"
  fi
done

echo
echo ">> Next: bash 03-install-ingress.sh"