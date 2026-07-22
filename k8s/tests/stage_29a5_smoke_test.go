// Package tests · Stage 29-A.5: live-cluster smoke for cert-manager + APISIX TLS.
//
// Build tag: //go:build integration
//
// Reference: docs/stage-29-A.5-tls-live-smoke.md,
//            docs/stage-29-A-https-grafana.md § 五 (live smoke follow-up).
//
// This file is the GREEN-side gate for Stage 29-A.5: it asserts that,
// after `bash k8s/scripts/04-install-chart.sh` on a fresh kind cluster,
// the cert-manager subchart is structurally complete AND the
// cert-manager-issued certificate flows into APISIX's native ApisixTls
// CR and Grafana is reachable over HTTPS.
//
// The test is deliberately table-driven and runs each gate as its own
// subtest with `t.Run`, so that a single failure (e.g. webhook not
// becoming Available) reports the precise gate that broke instead of
// stopping at the first kubectl wait.
//
// Per AGENTS.md § 三.3 ("DB/Redis/Kafka 等副作用 → 必须用 mock 接口 +
// 测试替身") this is the documented exception: live cluster smoke is
// by definition an end-to-end integration test, gated behind the
// `integration` build tag so that `go test ./...` does not require a
// cluster.
//
// As of the initial RED commit:
//   - cert-manager subchart has only 1 Deployment (controller) — RED
//   - no chart-managed `cert-manager` namespace exists — RED
//   - grafana TLS uses K8s Ingress (no ingress controller in kind) — RED
//   - no Certificate CR to drive APISIX ApisixTls — RED
// All six subtests below MUST fail. They turn green when P2.1 / P2.3
// land.
package tests

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// certNamespace is the namespace the cert-manager subchart will own.
// Constant now (used by every gate); once P2.1 lands, the chart's
// templates/namespace.yaml makes this self-managed.
const certNamespace = "cert-manager"

// observabilityNamespace is where the grafana service + Certificate
// (and the resulting Secret) live.
const observabilityNamespace = "ee-observability"

// systemNamespace is where the APISIX data plane lives (per
// 03-install-ingress.sh, APISIX is installed into ee-system).
const systemNamespace = "ee-system"

// runOrSkip shells out to a CLI. Returns (stdout, stderr, err).
// If the binary is missing or the call returns non-zero, the caller
// decides whether to t.Skip (no cluster) or t.Fatal (gate failed).
func runOrSkip(t *testing.T, name string, args ...string) (string, string, error) {
	t.Helper()
	cmd := exec.Command(name, args...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// hasKindCluster returns true if `kubectl get nodes` exits 0 with at
// least one node. Used to skip the test entirely when no cluster is
// reachable — this is the difference between "test failure" (gate
// didn't hold) and "test skipped" (no cluster to test against).
func hasKindCluster(t *testing.T) bool {
	t.Helper()
	_, _, err := runOrSkip(t, "kubectl", "get", "nodes", "--request-timeout=3s")
	return err == nil
}

// TestStage29A5_CertManagerLiveSmoke is the umbrella subtest that
// runs every Stage 29-A.5 gate in order. Subtests are independent:
// one failure does not abort the next.
func TestStage29A5_CertManagerLiveSmoke(t *testing.T) {
	if !hasKindCluster(t) {
		t.Skip("no cluster reachable; skipping Stage 29-A.5 live smoke (run after `bash k8s/scripts/01-create-cluster.sh`)")
	}

	// Per-test timeout: 10 minutes total. live-smoke gates are
	// individually capped by `kubectl wait --timeout=...` but we keep
	// a global ceiling so a hung pod cannot wedge `go test`.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	_ = ctx // currently unused; placeholder for future polling refactor.

	t.Run("01_cert_manager_namespace_exists", func(t *testing.T) {
		stdout, _, err := runOrSkip(t, "kubectl", "get", "namespace", certNamespace, "-o", "name")
		require.NoError(t, err, "kubectl get namespace %s failed — chart must own this namespace", certNamespace)
		require.Contains(t, stdout, certNamespace,
			"expected namespace %q to exist (rendered by cert-manager subchart)", certNamespace)
	})

	t.Run("02_cert_manager_controller_available", func(t *testing.T) {
		// RED until P2.1: chart only renders a single Deployment named
		// ee-cert-manager-controller (no per-component image / replicas).
		// After P2.1 this gate still passes — the controller is the
		// "head" of cert-manager — but 03/04 below turn green.
		_, _, err := runOrSkip(t, "kubectl", "wait",
			"--for=condition=Available",
			"deployment/ee-cert-manager-controller",
			"-n", certNamespace,
			"--timeout=180s",
		)
		require.NoError(t, err, "cert-manager controller Deployment not Available in %s", certNamespace)
	})

	t.Run("03_cert_manager_cainjector_available", func(t *testing.T) {
		// RED until P2.1: cainjector Deployment does not exist at all.
		_, _, err := runOrSkip(t, "kubectl", "wait",
			"--for=condition=Available",
			"deployment/ee-cert-manager-cainjector",
			"-n", certNamespace,
			"--timeout=180s",
		)
		require.NoError(t, err, "cert-manager cainjector Deployment not Available in %s", certNamespace)
	})

	t.Run("04_cert_manager_webhook_available", func(t *testing.T) {
		// RED until P2.1: webhook Deployment does not exist at all.
		_, _, err := runOrSkip(t, "kubectl", "wait",
			"--for=condition=Available",
			"deployment/ee-cert-manager-webhook",
			"-n", certNamespace,
			"--timeout=180s",
		)
		require.NoError(t, err, "cert-manager webhook Deployment not Available in %s", certNamespace)
	})

	t.Run("05_clusterissuer_ready", func(t *testing.T) {
		// RED until P2.3: even though the controller is up, no
		// Certificate CR has been wired to APISIX ApisixTls yet.
		_, _, err := runOrSkip(t, "kubectl", "wait",
			"--for=condition=Ready",
			"clusterissuer/selfsigned-issuer",
			"--timeout=60s",
		)
		require.NoError(t, err, "ClusterIssuer/selfsigned-issuer not Ready")
	})

	t.Run("06_grafana_certificate_ready", func(t *testing.T) {
		// RED until P2.3: no Certificate CR exists for grafana-tls.
		// The certificate name (per cert-manager naming convention) is
		// grafana-tls once the Certificate resource is created.
		_, _, err := runOrSkip(t, "kubectl", "wait",
			"--for=condition=Ready",
			"certificate/grafana-tls",
			"-n", observabilityNamespace,
			"--timeout=120s",
		)
		require.NoError(t, err, "Certificate/grafana-tls not Ready in %s", observabilityNamespace)
	})

	t.Run("07_apisixtls_resource_present", func(t *testing.T) {
		// RED until P2.3: chart currently emits K8s Ingress (which has
		// no controller in this kind cluster). After P2.3 it should
		// emit ApisixTls + ApisixRoute instead.
		stdout, _, err := runOrSkip(t, "kubectl", "get", "apisixtls.apisix.apache.org",
			"-n", observabilityNamespace,
			"-o", "name",
		)
		require.NoError(t, err, "kubectl get apisixtls failed — APISIX CRDs may not be installed")
		require.Contains(t, stdout, "grafana-tls",
			"expected ApisixTls/grafana-tls to exist in %s (renders only after P2.3)", observabilityNamespace)
	})

	t.Run("08_apisix_data_plane_available", func(t *testing.T) {
		// Pre-flight: APISIX data plane must be up before we can hit
		// https://grafana.local via port-forward.
		_, _, err := runOrSkip(t, "kubectl", "wait",
			"--for=condition=Available",
			"deployment/ee-apisix",
			"-n", systemNamespace,
			"--timeout=120s",
		)
		require.NoError(t, err, "APISIX data plane Deployment not Available in %s", systemNamespace)
	})

	t.Run("09_grafana_https_200", func(t *testing.T) {
		// RED until P2.3+P2.4: nothing serves TLS for grafana.local yet.
		// After GREEN: port-forward 9443 → ee-apisix :9443 and curl
		// `https://grafana.local:9443/api/health` returns 200.
		//
		// Implementation: shell out to the smoke script `07-tls-smoke.sh`
		// which encapsulates the kubectl port-forward + curl sequence.
		// The script exits 0 on success.
		_, stderr, err := runOrSkip(t, "bash", "../../scripts/07-tls-smoke.sh")
		if err != nil {
			t.Fatalf("bash 07-tls-smoke.sh failed: %v\nstderr=%s", err, stderr)
		}
	})
}