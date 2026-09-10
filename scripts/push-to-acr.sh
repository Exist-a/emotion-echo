#!/usr/bin/env bash
# push-to-acr.sh — build and push SenseVoice base + app layers to Aliyun ACR.
#
# Usage (from repo root):
#   ./scripts/push-to-acr.sh                  # push both layers with default tag
#   ./scripts/push-to-acr.sh v0.2.0           # push with custom tag
#   ./scripts/push-to-acr.sh --base-only      # only rebuild + push base
#   ./scripts/push-to-acr.sh --app-only       # only rebuild + push app
#   ./scripts/push-to-acr.sh --skip-push      # build only, don't push
#
# Credential: relies on ~/.docker/config.json (set up via `docker login`
# beforehand). Passwords are NEVER passed on the command line, stored in
# this script, or echoed. If docker login has expired, the script aborts
# with a clear error and instructions.
#
# Image layout:
#   - base: python + torch + funasr + kaldi-native-fbank + transformers
#           (~500MB). Rebuilt when Python deps change.
#   - app:  business code + 936MB SenseVoiceSmall model.pt + config
#           (~1.4GB on top of base). Rebuilt when code or model changes.
#
# ACR layout (private):
#   {REGISTRY}/{NAMESPACE}/sensevoice-base:{TAG}
#   {REGISTRY}/{NAMESPACE}/sensevoice:{TAG}

set -euo pipefail

# --- Configuration (edit these if your registry/namespace differ) ---
REGISTRY="${ACR_REGISTRY:-crpi-rnawo8jx69bslvbx.cn-hongkong.personal.cr.aliyuncs.com}"
NAMESPACE="${ACR_NAMESPACE:-emotion-echo}"
TAG="${1:-v0.1.0}"

# If first arg is a flag, TAG stays default
case "${1:-}" in
  --*) TAG="v0.1.0" ;;
esac

PUSH_BASE=true
PUSH_APP=true
SKIP_PUSH=false
case "${1:-}" in
  --base-only) PUSH_APP=false ;;
  --app-only)  PUSH_BASE=false ;;
  --skip-push) SKIP_PUSH=true ;;
esac

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BUILD_DIR="${REPO_ROOT}/emotion-echo-models/SV-fastbuild"

BASE_LOCAL_TAG="sensevoice-base:${TAG}"
APP_LOCAL_TAG="emotion-echo/sensevoice-fastbuild:${TAG}"

BASE_REMOTE_TAG="${REGISTRY}/${NAMESPACE}/sensevoice-base:${TAG}"
APP_REMOTE_TAG="${REGISTRY}/${NAMESPACE}/sensevoice:${TAG}"

# --- Sanity checks ---
command -v docker >/dev/null || { echo "docker not found"; exit 1; }

# Verify docker login cache exists for our target registry
if ! python -c "
import json, sys
try:
    c = json.load(open('${HOME}/.docker/config.json'))
    if '${REGISTRY}' not in c.get('auths', {}):
        sys.exit(1)
except Exception:
    sys.exit(1)
" 2>/dev/null; then
  cat >&2 <<EOF
ACR credential not found in ~/.docker/config.json for ${REGISTRY}.

Run this in your terminal first:

    echo "\$ACR_PASSWORD" | docker login ${REGISTRY} -u YOUR_USERNAME --password-stdin

Then re-run this script.
EOF
  exit 2
fi

echo "=========================================="
echo "ACR push config:"
echo "  registry : ${REGISTRY}"
echo "  namespace: ${NAMESPACE}"
echo "  tag      : ${TAG}"
echo "  base     : ${PUSH_BASE}"
echo "  app      : ${PUSH_APP}"
echo "=========================================="

# --- Build + push base ---
if [ "${PUSH_BASE}" = true ]; then
  echo
  echo "[1/2] Building + pushing BASE layer..."
  if [ "${SKIP_PUSH}" = true ]; then
    docker buildx build \
      --provenance=false \
      --tag "${BASE_LOCAL_TAG}" \
      --output type=image,oci-mediatypes=false \
      -f "${BUILD_DIR}/Dockerfile.base" \
      "${BUILD_DIR}"
    echo "  built ${BASE_LOCAL_TAG} (push skipped)"
  else
    docker buildx build \
      --provenance=false \
      --tag "${BASE_LOCAL_TAG}" \
      --tag "${BASE_REMOTE_TAG}" \
      --output type=image,oci-mediatypes=false \
      -f "${BUILD_DIR}/Dockerfile.base" \
      "${BUILD_DIR}" \
      --push
    echo "  pushed ${BASE_REMOTE_TAG}"
  fi
fi

# --- Build + push app ---
if [ "${PUSH_APP}" = true ]; then
  echo
  echo "[2/2] Building + pushing APP layer..."
  if [ "${SKIP_PUSH}" = true ]; then
    docker buildx build \
      --provenance=false \
      --tag "${APP_LOCAL_TAG}" \
      --output type=image,oci-mediatypes=false \
      -f "${BUILD_DIR}/Dockerfile.app" \
      "${BUILD_DIR}"
    echo "  built ${APP_LOCAL_TAG} (push skipped)"
  else
    docker buildx build \
      --provenance=false \
      --tag "${APP_LOCAL_TAG}" \
      --tag "${APP_REMOTE_TAG}" \
      --output type=image,oci-mediatypes=false \
      -f "${BUILD_DIR}/Dockerfile.app" \
      "${BUILD_DIR}" \
      --push
    echo "  pushed ${APP_REMOTE_TAG}"
  fi
fi

echo
echo "Done. Verify with:"
echo "  docker pull ${APP_REMOTE_TAG}"
echo "  docker run --rm -p 8002:8002 ${APP_REMOTE_TAG}"