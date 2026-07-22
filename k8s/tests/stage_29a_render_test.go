// Package tests · Stage 29-A: cert-manager + Grafana TLS RED gate.
//
// Build tag: //go:build k8s
//
// Reference: docs/stage-29-A.5-tls-live-smoke.md,
//            docs/stage-29-A-https-grafana.md.
//
// Stage 29-A is the smallest first slice of the HTTPS effort:
//   1. Install cert-manager into the cluster (subchart owns its own
//      `cert-manager` namespace and renders controller + cainjector +
//      webhook as separate Deployments, mirroring upstream v1.14.0).
//   2. Bootstrap a self-signed ClusterIssuer (dev only).
//   3. Emit a Certificate CR pointing at the ClusterIssuer.
//   4. Emit an ApisixTls + ApisixRoute pointing at the resulting
//      Secret — the actual ingress controller in this project is
//      APISIX (apache/apisix:3.10.0-debian), not ingress-nginx, so
//      the old `kind: Ingress` design did not match the runtime.
//
// The RED gate in this file fails fast if the cert-manager subchart is
// absent OR the grafana TLS CRs are missing OR the ClusterIssuer is
// missing. The GREEN commit in Stage 29-A.2/29-A.4 flips every
// assertion here to passing; the Stage 29-A.5 follow-up extends the
// assertions to cover the three coupled CRs (Certificate + ApisixTls +
// ApisixRoute) instead of a single Ingress.
package tests

import (
	"regexp"
	"strings"
	"testing"
)

// TestStage29A_CertManagerChartRenders asserts that the cert-manager
// subchart renders at all. We rely on `helm template` exiting cleanly
// and emitting all three component Deployments + a ClusterIssuer named
// `selfsigned-issuer`. This is the structural RED gate — the chart
// must render into a working cert-manager install shape.
func TestStage29A_CertManagerChartRenders(t *testing.T) {
	rendered := helm(t, valuesDev)

	// Must contain Deployments for all three upstream components
	// (controller, cainjector, webhook). Without all three, the
	// Certificate CR will never reach Ready (see Stage 29-A.5
	// landing doc for the failure modes).
	requiredDeployments := []string{
		"ee-cert-manager-controller",
		"ee-cert-manager-cainjector",
		"ee-cert-manager-webhook",
	}
	for _, name := range requiredDeployments {
		// Go RE2 has no dot-matches-newline flag, so we use explicit \n
		// boundaries instead of .{0,N}.
		re := regexp.MustCompile(`kind: Deployment\nmetadata:\n  name: ` + name)
		if !re.MatchString(rendered) {
			t.Errorf("expected Deployment %q to be rendered (Stage 29-A.5: split into 3 components)", name)
		}
	}

	// ClusterIssuer named 'selfsigned-issuer' must exist.
	re := regexp.MustCompile(`kind: ClusterIssuer\nmetadata:\n  name: selfsigned-issuer`)
	if !re.MatchString(rendered) {
		t.Errorf("expected ClusterIssuer 'selfsigned-issuer' to be rendered")
	}
}

// TestStage29A_GrafanaTLSCertificates asserts that when
// `global.grafanaIngressTls.enabled: true` (the dev overlay default),
// the grafana subchart emits the three coupled CRs that wire a
// cert-manager-issued Secret through APISIX:
//   - Certificate (cert-manager.io/v1) namespaced into ee-observability
//     with secretName grafana-tls + issuerRef name=selfsigned-issuer
//   - ApisixTls (apisix.apache.org/v2) referencing the same Secret
//     under hosts=[grafana.local]
//   - ApisixRoute (apisix.apache.org/v2) routing grafana.local/* to
//     the grafana Service on port 3000
//
// Stage 29-A.5 replaces the previous design (single K8s Ingress with
// cert-manager.io/cluster-issuer annotation) because the project's
// ingress controller is APISIX data plane, not ingress-nginx.
func TestStage29A_GrafanaTLSCertificates(t *testing.T) {
	rendered := helm(t, valuesDev)

	// 1. Certificate CR — cert-manager.io/v1 in ee-observability.
	certIdx := findKindIndex(rendered, "Certificate", "cert-manager.io/v1")
	if certIdx < 0 {
		t.Fatalf("expected a cert-manager.io/v1 Certificate CR to be rendered")
	}
	certDoc := windowAround(rendered, certIdx, 3000)
	if !strings.Contains(certDoc, `secretName: "grafana-tls"`) &&
		!strings.Contains(certDoc, "secretName: grafana-tls") {
		t.Errorf("expected Certificate spec.secretName 'grafana-tls', got: %s", truncate(certDoc, 600))
	}
	if !strings.Contains(certDoc, "selfsigned-issuer") {
		t.Errorf("expected Certificate spec.issuerRef.name 'selfsigned-issuer'")
	}
	if !strings.Contains(certDoc, "grafana.local") {
		t.Errorf("expected Certificate spec.dnsNames to contain 'grafana.local'")
	}
	if !strings.Contains(certDoc, "namespace: ee-observability") {
		t.Errorf("expected Certificate metadata.namespace 'ee-observability'")
	}

	// 2. ApisixTls — apisix.apache.org/v2 referencing grafana-tls Secret.
	tlsIdx := findKindIndex(rendered, "ApisixTls", "apisix.apache.org/v2")
	if tlsIdx < 0 {
		t.Fatalf("expected an ApisixTls CR to be rendered")
	}
	tlsDoc := windowAround(rendered, tlsIdx, 2000)
	if !strings.Contains(tlsDoc, `grafana.local`) {
		t.Errorf("expected ApisixTls spec.hosts to contain 'grafana.local'")
	}
	if !strings.Contains(tlsDoc, `name: "grafana-tls"`) &&
		!strings.Contains(tlsDoc, "name: grafana-tls") {
		t.Errorf("expected ApisixTls spec.secret.name 'grafana-tls'")
	}

	// 3. ApisixRoute — must exist with name grafana-tls pointing at
	// grafana Service :3000.
	routeIdx := findApisixRouteGrafanaTLSIndex(rendered)
	if routeIdx < 0 {
		t.Fatalf("expected ApisixRoute 'grafana-tls' (TLS route) to be rendered")
	}
	routeDoc := windowAround(rendered, routeIdx, 2000)
	if !strings.Contains(routeDoc, "serviceName: grafana") {
		t.Errorf("expected ApisixRoute backend.serviceName 'grafana'")
	}
	if !strings.Contains(routeDoc, "servicePort: 3000") {
		t.Errorf("expected ApisixRoute backend.servicePort 3000")
	}
	if !strings.Contains(routeDoc, "grafana.local") {
		t.Errorf("expected ApisixRoute match.hosts to contain 'grafana.local'")
	}
}

// findKindIndex returns the byte offset of the first `kind: KIND`
// doc with the matching `apiVersion:` immediately before it (within
// 100 bytes). Returns -1 when no such resource exists.
//
// Implementation note: we deliberately do not import yaml.v3 here to
// keep the test file dependency-free. A targeted sliding window is
// sufficient for a 17-route umbrella where every resource is < 4 KB.
func findKindIndex(rendered, kind, apiVersion string) int {
	re := regexp.MustCompile(`(?m)^kind:\s*` + kind + `\s*$`)
	locs := re.FindAllStringIndex(rendered, -1)
	for _, loc := range locs {
		// Look 100 bytes BEFORE `kind:` for the apiVersion line.
		start := loc[0] - 200
		if start < 0 {
			start = 0
		}
		window := rendered[start:loc[0]]
		if strings.Contains(window, apiVersion) {
			return loc[0]
		}
	}
	return -1
}

// findApisixRouteGrafanaTLSIndex returns the byte offset of the
// ApisixRoute with metadata.name == grafana-tls. Returns -1 when not
// present.
func findApisixRouteGrafanaTLSIndex(rendered string) int {
	re := regexp.MustCompile(`(?m)^kind:\s*ApisixRoute\s*$`)
	locs := re.FindAllStringIndex(rendered, -1)
	for _, loc := range locs {
		// Look at the next ~1500 bytes for metadata.name == grafana-tls.
		end := loc[1] + 1500
		if end > len(rendered) {
			end = len(rendered)
		}
		window := rendered[loc[1]:end]
		if strings.Contains(window, "name: grafana-tls") {
			return loc[0]
		}
	}
	return -1
}

// windowAround returns a slice of length `size` bytes starting at
// `idx`, clamped to the slice bounds.
func windowAround(s string, idx, size int) string {
	end := idx + size
	if end > len(s) {
		end = len(s)
	}
	return s[idx:end]
}