#!/usr/bin/env bash
# =====================================================
#  06-smoke.sh — smoke test for the kind-deployed Emotion-Echo
# =====================================================
#  Asserts:
#    1. 4 namespaces exist
#    2. 16 ApisixRoute + 6 ApisixUpstream CRDs exist
#    3. All 12 Deployments roll out
#    4. APISIX gateway responds to /ping (the mock-ping route)
#    5. Web frontend responds to /
#    6. SkyWalking UI is up
# =====================================================
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SMOKE_REPORT="${SCRIPT_DIR}/../SMOKE-REPORT.md"
APISIX_URL="${APISIX_URL:-http://localhost:9080}"
WEB_URL="${WEB_URL:-http://localhost:3000}"
SW_URL="${SW_URL:-http://localhost:18080}"
KUBECTL="kubectl"

pass=0
fail=0
results=()

check() {
    local name="$1"; shift
    if "$@" >/dev/null 2>&1; then
        results+=("✅ PASS  ${name}")
        pass=$((pass+1))
    else
        results+=("❌ FAIL  ${name}")
        fail=$((fail+1))
    fi
}

echo ">> Running smoke checks..."

# Namespaces
check "namespace ee-system"  bash -c "${KUBECTL} get ns ee-system >/dev/null"
check "namespace ee-data"    bash -c "${KUBECTL} get ns ee-data >/dev/null"
check "namespace ee-app"     bash -c "${KUBECTL} get ns ee-app >/dev/null"
check "namespace ee-observability" bash -c "${KUBECTL} get ns ee-observability >/dev/null"

# Apisix CRDs
check "16 ApisixRoute"       bash -c "test \"\$(${KUBECTL} get apisixroute -A --no-headers | wc -l)\" = '16'"
check "6 ApisixUpstream"     bash -c "test \"\$(${KUBECTL} get apisixupstream -A --no-headers | wc -l)\" = '6'"

# All deployments
check "all deployments Available" bash -c "${KUBECTL} wait --for=condition=Available --timeout=60s -A -l app.kubernetes.io/part-of=emotion-echo deployment"

# HTTP routes via APISIX
check "APISIX /ping"          curl -sf --max-time 5 "${APISIX_URL}/ping"
check "APISIX /ai-health"     curl -sf --max-time 5 "${APISIX_URL}/ai-health"
check "APISIX /analytics-health" curl -sf --max-time 5 "${APISIX_URL}/analytics-health"

# Web frontend
check "Web frontend /"        curl -sf --max-time 5 "${WEB_URL}/"

# SkyWalking UI
check "SkyWalking UI /"       curl -sf --max-time 5 "${SW_URL}/"

# Print results
echo
for r in "${results[@]}"; do echo "$r"; done
echo
echo "TOTAL: ${pass} pass / ${fail} fail"

# Write report
{
  echo "# Stage 27-G · Smoke Test Report"
  echo
  echo "Date: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo
  echo "| # | Result | Check |"
  echo "|---|--------|-------|"
  for r in "${results[@]}"; do
      echo "| - | ${r%%  *} | ${r#*  } |"
  done
  echo
  echo "**Total**: ${pass} pass / ${fail} fail"
} > "${SMOKE_REPORT}"

exit $((fail > 0 ? 1 : 0))