// Package tests · Stage 29-A: cert-manager + Grafana Ingress TLS RED gate.
//
// Build tag: //go:build k8s
//
// Reference: docs/stage-28-landing § 九 (HTTPS Stage 29-A),
//            docs/learn/12-stage-28-roadmap.md.
//
// Stage 29-A is the smallest first slice of the HTTPS effort:
//   1. Install cert-manager into the cluster.
//   2. Bootstrap a self-signed ClusterIssuer (dev only).
//   3. Render Grafana's existing ClusterIP service as an Ingress with
//      a TLS block + the cert-manager.io/cluster-issuer annotation
//      pointing at the self-signed issuer.
//
// The RED gate in this file fails fast if the cert-manager subchart is
// absent OR the grafana ingress TLS block is missing OR the cluster
// issuer is missing. The GREEN commit in Stage 29-A.2/29-A.4 flips
// every assertion here to passing.
package tests

import (
	"regexp"
	"strings"
	"testing"
)

// TestStage29A_CertManagerChartRenders asserts that the cert-manager
// subchart renders at all. We rely on `helm template` exiting cleanly
// and emitting both the cert-manager Deployment and a ClusterIssuer
// named `selfsigned-issuer`. This is the structural RED gate — the
// chart does not yet exist in the working tree.
func TestStage29A_CertManagerChartRenders(t *testing.T) {
	rendered := helm(t, valuesDev)

	// Must contain a Deployment whose name contains "cert-manager".
	// We use a strict, line-anchored regex (Go RE2 has no dot-matches-newline
	// flag, so we use explicit \n boundaries instead of .{0,N}).
	certMgrDepRe := regexp.MustCompile(`kind: Deployment\nmetadata:\n  name:\s*\S*cert-manager`)
	if !certMgrDepRe.MatchString(rendered) {
		t.Errorf("expected a Deployment with 'cert-manager' in its name (Stage 29-A RED gate)")
	}

	// Must contain a ClusterIssuer named `selfsigned-issuer`.
	issuerRe := regexp.MustCompile(`kind: ClusterIssuer\nmetadata:\n  name: selfsigned-issuer`)
	if !issuerRe.MatchString(rendered) {
		t.Errorf("expected ClusterIssuer 'selfsigned-issuer' to be rendered (Stage 29-A RED gate)")
	}
}

// TestStage29A_GrafanaIngressTLS asserts that when
// `grafana.ingress.tls.enabled: true` (the dev overlay default), the
// grafana subchart renders an Ingress with:
//   - kind: Ingress
//   - metadata.name containing 'grafana'
//   - spec.tls[].secretName == grafana-tls
//   - spec.tls[].hosts[] containing grafana.local
//   - spec.rules[].host containing grafana.local
//   - annotation cert-manager.io/cluster-issuer == selfsigned-issuer
func TestStage29A_GrafanaIngressTLS(t *testing.T) {
	rendered := helm(t, valuesDev)

	// Extract the grafana Ingress block by walking the YAML for "kind: Ingress"
	// followed within a reasonable window by "grafana" in the metadata.name.
	ingIdx := findGrafanaIngressIndex(rendered)
	if ingIdx < 0 {
		t.Fatalf("expected a grafana Ingress to be rendered (Stage 29-A RED gate)")
	}

	// Take a 4000-char slice starting at the Ingress kind — sufficient to
	// contain the full spec for a small Ingress resource.
	end := ingIdx + 4000
	if end > len(rendered) {
		end = len(rendered)
	}
	ingDoc := rendered[ingIdx:end]

	// TLS block must include secretName: grafana-tls.
	// Helm's `quote` template func wraps strings in double quotes, so the
	// rendered secretName appears as `"grafana-tls"`. Accept both quoted
	// and unquoted forms.
	if !strings.Contains(ingDoc, `secretName: "grafana-tls"`) &&
		!strings.Contains(ingDoc, "secretName: grafana-tls") {
		t.Errorf("expected Ingress TLS secretName 'grafana-tls', got: %s", truncate(ingDoc, 600))
	}

	// TLS hosts must include grafana.local.
	if !strings.Contains(ingDoc, `grafana.local`) {
		t.Errorf("expected Ingress TLS hosts to contain 'grafana.local'")
	}

	// Annotation cert-manager.io/cluster-issuer must point at selfsigned-issuer.
	// We match the Helm-quoted form (the `quote` template func wraps strings
	// in double quotes). Both `"selfsigned-issuer"` and `selfsigned-issuer`
	// should be considered a match; use a Contains check on both forms.
	if !strings.Contains(ingDoc, `cert-manager.io/cluster-issuer: "selfsigned-issuer"`) &&
		!strings.Contains(ingDoc, "cert-manager.io/cluster-issuer: selfsigned-issuer") {
		t.Errorf("expected annotation 'cert-manager.io/cluster-issuer: selfsigned-issuer' on the grafana Ingress, got: %s", truncate(ingDoc, 400))
	}
}

// findGrafanaIngressIndex returns the byte offset of the first `kind: Ingress`
// doc that has 'grafana' somewhere in its metadata.name within the next 1500
// bytes. Returns -1 when no such Ingress exists.
//
// Implementation note: we deliberately do not import yaml.v3 here to keep the
// test file dependency-free. A targeted sliding window is sufficient for a
// 16-route umbrella where every Ingress is < 4 KB.
func findGrafanaIngressIndex(rendered string) int {
	re := regexp.MustCompile(`(?m)^kind:\s*Ingress\s*$`)
	locs := re.FindAllStringIndex(rendered, -1)
	for _, loc := range locs {
		// Look at the next ~1500 bytes for the metadata.name.
		end := loc[1] + 1500
		if end > len(rendered) {
			end = len(rendered)
		}
		window := rendered[loc[1]:end]
		if strings.Contains(window, "name: grafana") {
			return loc[0]
		}
	}
	return -1
}