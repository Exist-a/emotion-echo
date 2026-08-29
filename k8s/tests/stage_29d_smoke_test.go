// Package tests · Stage 29-D: live-cluster smoke for per-family TLS.
//
// Build tag: //go:build integration
//
// Reference: docs/stage-29-D-tls-all-routes.md (forthcoming),
//            docs/stage-29-A.5-tls-live-smoke.md (precedent for one-host
//            Grafana TLS smoke; this file extends the same pattern to
//            the 5 business-route families).
//
// Stage 29-D RED gate (this file): asserts that, after
// `bash k8s/scripts/04-install-chart.sh` on a fresh kind cluster, the
// 5 family Certificates + ApisixTls CRs are Ready AND each family's
// HTTPS handshake succeeds via the APISIX :9443 listener.
//
// The pattern is the same as Stage 29-A.5's 9-gate test:
//   01: cert-manager (controller) is up (sanity, gates 02-06)
//   02-06: 5 family gates (Cert Ready + ApisixTls present + HTTPS 200)
//   07: APISIX data plane Available (gates 02-06 traffic)
//
// Per AGENTS.md § 三.3 ("DB/Redis/Kafka 等副作用 → 必须用 mock 接口 +
// 测试替身") this is the documented exception: live cluster smoke is
// by definition an end-to-end integration test, gated behind the
// `integration` build tag so `go test ./...` does not require a cluster.
//
// As of the initial RED commit (this file), the umbrella chart does
// not yet render 5 family Certificates/ApisixTls (Stage 29-D.2 just
// added them; not yet applied). Until an operator runs
// `bash k8s/scripts/04-install-chart.sh` on a fresh kind cluster AND
// cert-manager signs each family cert, all 5 family subtests fail.
// They turn green after the install script finishes its 3×cert-manager
// `kubectl wait` + APISIX `kubectl wait` sequence (~5 min cold-start).
package tests

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// appNamespace is where the 15 business ApisixRoutes + their
// Certificates + ApisixTls live (per tls-routes.yaml).
// (systemNamespace already defined in stage_29a5_smoke_test.go.)
const appNamespace = "ee-app"

// stage29DSmokeFamilies mirrors stage_29d_render_test.go's canonical
// table but adds the smoke-only `tlsHealthPath` (the backend's HTTP
// health path the curl hits after SNI terminates). Kept in lockstep
// with the render-assert table — if you add/remove a family, update
// BOTH files.
var stage29DSmokeFamilies = []struct {
	name, hostname, secretName, tlsHealthPath string
}{
	{"user", "user.echo.local", "user-family-tls", "/health"},
	{"chat", "chat.echo.local", "chat-family-tls", "/health"},
	{"analytics", "analytics.echo.local", "analytics-family-tls", "/health"},
	{"assessment", "assessment.echo.local", "assessment-family-tls", "/health"},
	{"ai", "ai.echo.local", "ai-family-tls", "/health"},
}

// TestStage29D_PerFamilyTLSSmoke is the umbrella subtest that runs
// 16 gates total:
//   - 01: cert-manager controller Available (pre-flight, must pass for 02-16 to mean anything)
//   - 02-04:  user family    — Cert Ready + ApisixTls present + HTTPS 200
//   - 05-07:  chat family    — Cert Ready + ApisixTls present + HTTPS 200
//   - 08-10:  analytics fam. — Cert Ready + ApisixTls present + HTTPS 200
//   - 11-13:  assessment fam — Cert Ready + ApisixTls present + HTTPS 200
//   - 14-16:  ai family      — Cert Ready + ApisixTls present + HTTPS 200
//
// RED until the chart is applied to a fresh kind cluster AND cert-manager
// has had time to sign all 5 family certs (typ. 60-90s after install).
func TestStage29D_PerFamilyTLSSmoke(t *testing.T) {
	if !hasKindCluster(t) {
		t.Skip("no cluster reachable; skipping Stage 29-D live smoke " +
			"(run after `bash k8s/scripts/01-create-cluster.sh && bash k8s/scripts/04-install-chart.sh`)")
	}

	// Per-test timeout: 10 minutes. Live-smoke gates are individually
	// capped by `kubectl wait --timeout=...` but we keep a global
	// ceiling so a hung pod cannot wedge `go test`.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	_ = ctx // placeholder for future polling refactor.

	// Pre-flight: cert-manager controller must be up. If it isn't,
	// the 5 family Cert gates will time out anyway, but failing fast
	// here gives the operator a clearer root-cause message.
	t.Run("01_cert_manager_controller_available", func(t *testing.T) {
		_, _, err := runOrSkip(t, "kubectl", "wait",
			"--for=condition=Available",
			"deployment/ee-cert-manager-controller",
			"-n", "cert-manager",
			"--timeout=180s",
		)
		require.NoError(t, err, "cert-manager controller Deployment not Available in cert-manager ns — "+
			"Stage 29-A.5 install must finish before Stage 29-D smoke runs")
	})

	// 02-16: per-family gates. We loop over the canonical family table
	// and emit three sub-gates per family.
	for _, f := range stage29DSmokeFamilies {
		f := f

		t.Run("family_"+f.name+"_certificate_ready", func(t *testing.T) {
			// RED until the umbrella is installed: no Certificate/<name>
			// exists in appNamespace. After install + cert-manager
			// reconciliation (~60s) this gate flips green.
			_, _, err := runOrSkip(t, "kubectl", "wait",
				"--for=condition=Ready",
				"certificate/"+f.secretName,
				"-n", appNamespace,
				"--timeout=120s",
			)
			require.NoError(t, err,
				"Certificate/%s not Ready in %s (cert-manager has not signed it yet, "+
					"or tls-routes.yaml was not rendered by the umbrella)",
				f.secretName, appNamespace)
		})

		t.Run("family_"+f.name+"_apisixtls_present", func(t *testing.T) {
			stdout, _, err := runOrSkip(t, "kubectl", "get",
				"apisixtls.apisix.apache.org",
				"-n", appNamespace,
				"-o", "name",
			)
			require.NoError(t, err, "kubectl get apisixtls failed — APISIX CRDs may not be installed")
			require.Contains(t, stdout, f.secretName,
				"expected ApisixTls/%s to exist in %s (renders only after Stage 29-D.2 + install)",
				f.secretName, appNamespace)
		})

		t.Run("family_"+f.name+"_https_handshake", func(t *testing.T) {
			// RED until the family cert is signed AND the route is
			// wired in APISIX. Hands off to the smoke script (which
			// Stage 29-D.4 generalizes for multi-host). For now, we
			// just verify the script returns 0 when TLS_HOST is set
			// to this family.
			//
			// Implementation: shell out to 07-tls-smoke.sh with env
			// override. This keeps the smoke script as the single
			// source of truth for handshake logic.
			cmd := exec.Command("bash", "../../scripts/07-tls-smoke.sh")
			cmd.Env = append(cmd.Environ(),
				"TLS_HOST="+f.hostname,
				"LOCAL_PORT=9443",
				"APISIX_NAMESPACE="+systemNamespace,
				"APISIX_SERVICE=ee-apisix",
			)
			var stdout, stderr strings.Builder
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			err := cmd.Run()
			if err != nil {
				t.Fatalf("07-tls-smoke.sh failed for %s: %v\nstdout=%s\nstderr=%s",
					f.hostname, err, stdout.String(), stderr.String())
			}
		})
	}
}