// Package tests · Stage 29-D: per-family TLS retrofit for the remaining
// 15 business ApisixRoutes (Grafana already on TLS in Stage 29-A.5).
//
// Build tag: //go:build k8s
//
// Reference: docs/stage-29-D-tls-all-routes.md (forthcoming),
//            docs/stage-29-A.5-tls-live-smoke.md §八 (deferral list).
//
// Why "per-family" (vs per-route or single-host):
//
//   - cert-manager best practice (https://cert-manager.io/docs/usage/certificate/):
//     one Certificate per independent identity; shared SAN certs couple
//     unrelated lifecycles and force reissuance of the whole bundle on
//     one bad host.
//   - APISIX ApisixTls.hosts is an array — one Secret can serve many SNIs
//     of the same protocol version (https://apisix.apache.org/blog/2021/10/22/cert-manager-in-ingress/).
//   - 15 certs is operator overhead you don't need yet; 1 shared cert
//     across 15 unrelated routes is one point of failure you regret on
//     the first mis-rotation.
//
// Stage 29-D assignment (5 families in ee-app ns):
//
//   user        user.echo.local         → r-user-me / -by-id / -update
//   chat        chat.echo.local         → r-conv-create / -msg-list / -msg-send
//   analytics   analytics.echo.local    → r-analytics-health
//   assessment  assessment.echo.local   → r-surveys / -get / -submit / -results-list / -results-get
//   ai          ai.echo.local           → r-emotion-by-msg / -by-conv / -ai-health
//
//   (mock-ping stays HTTP — outside the 15 retrofit)
//
// This file is the RED gate. Every assertion here MUST fail against the
// pre-29-D chart. Stage 29-D.2 (GREEN) flips them to passing by adding
// `apisix-routes/templates/tls-routes.yaml` + injecting `match.hosts`
// into each existing ApisixRoute.
package tests

import (
	"strings"
	"testing"
)

// family is the unit of Stage 29-D TLS grouping. Each family is one
// cert-manager Certificate + one APISIX ApisixTls + N existing ApisixRoute
// entries annotated with `match.hosts`.
type family struct {
	// name is the family id used in the helm template `range` and the
	// secret name suffix (final Secret: <name>-family-tls).
	name string
	// hostname is the single SNI served by the family's ApisixTls.
	hostname string
	// routes lists the existing ApisixRoute names that MUST add
	// `match.hosts: [<hostname>]` to their spec.http[*].match block.
	routes []string
}

// stage29DFamilies is the canonical assignment table for Stage 29-D.
// Any change to this table must be reflected in:
//   - charts/emotion-echo/charts/apisix-routes/values.yaml  (tls.families)
//   - charts/emotion-echo/charts/apisix-routes/templates/tls-routes.yaml
//   - charts/emotion-echo/values-dev.yaml                    (apisixRoutes.tls.enabled)
var stage29DFamilies = []family{
	{
		name:     "user",
		hostname: "user.echo.local",
		routes:   []string{"r-user-me", "r-user-by-id", "r-user-update"},
	},
	{
		name:     "chat",
		hostname: "chat.echo.local",
		routes:   []string{"r-conv-create", "r-msg-list", "r-msg-send"},
	},
	{
		name:     "analytics",
		hostname: "analytics.echo.local",
		routes:   []string{"r-analytics-health"},
	},
	{
		name:     "assessment",
		hostname: "assessment.echo.local",
		routes:   []string{
			"r-surveys",
			"r-survey-get",
			"r-survey-submit",
			"r-survey-results-list",
			"r-survey-results-get",
		},
	},
	{
		name:     "ai",
		hostname: "ai.echo.local",
		routes:   []string{"r-emotion-by-msg", "r-emotion-by-conv", "r-ai-health"},
	},
}

// certificateDocForFamily returns the rendered Certificate document
// for the given family name, or "" if not found.
func certificateDocForFamily(rendered, familyName string) string {
	const header = "kind: Certificate"
	const apiVer = "apiVersion: cert-manager.io/v1"
	docs := strings.Split(rendered, "\n---\n")
	secretName := familyName + "-family-tls"
	for _, doc := range docs {
		if !strings.Contains(doc, header) || !strings.Contains(doc, apiVer) {
			continue
		}
		if strings.Contains(doc, "secretName: "+secretName) ||
			strings.Contains(doc, `secretName: "`+secretName+`"`) {
			return doc
		}
	}
	return ""
}

// apisixTlsDocForFamily returns the rendered ApisixTls document for the
// given family name, or "" if not found.
func apisixTlsDocForFamily(rendered, familyName string) string {
	const header = "kind: ApisixTls"
	const apiVer = "apiVersion: apisix.apache.org/v2"
	docs := strings.Split(rendered, "\n---\n")
	secretName := familyName + "-family-tls"
	for _, doc := range docs {
		if !strings.Contains(doc, header) || !strings.Contains(doc, apiVer) {
			continue
		}
		if strings.Contains(doc, "name: "+secretName) ||
			strings.Contains(doc, `name: "`+secretName+`"`) {
			return doc
		}
	}
	return ""
}

// apisixRouteDocForName returns the rendered ApisixRoute document for
// the given metadata.name, or "" if not found.
func apisixRouteDocForName(rendered, routeName string) string {
	const header = "kind: ApisixRoute"
	docs := strings.Split(rendered, "\n---\n")
	for _, doc := range docs {
		if !strings.Contains(doc, header) {
			continue
		}
		if strings.Contains(doc, "name: "+routeName) {
			return doc
		}
	}
	return ""
}

// TestStage29D_AllFamiliesCertificatesRender is the umbrella subtest that
// runs three gates per family (Certificate / ApisixTls / per-route
// match.hosts) = 15 sub-gates total.
//
// RED until Stage 29-D.2:
//   - Certificate/<family>-family-tls does not exist
//   - ApisixTls/<family>-family-tls does not exist
//   - existing ApisixRoutes have no `match.hosts` field
func TestStage29D_AllFamiliesCertificatesRender(t *testing.T) {
	rendered := helm(t, valuesDev)

	for _, f := range stage29DFamilies {
		f := f // capture
		secretName := f.name + "-family-tls"

		t.Run("01_"+f.name+"_certificate_renders", func(t *testing.T) {
			doc := certificateDocForFamily(rendered, f.name)
			if doc == "" {
				t.Fatalf("expected Certificate/%s to be rendered (cert-manager subchart must emit it when tls.enabled=true)", secretName)
			}
			if !strings.Contains(doc, f.hostname) {
				t.Errorf("expected Certificate/%s spec.dnsNames to contain %q", secretName, f.hostname)
			}
			if !strings.Contains(doc, "selfsigned-issuer") {
				t.Errorf("expected Certificate/%s spec.issuerRef.name to reference ClusterIssuer 'selfsigned-issuer'", secretName)
			}
			if !strings.Contains(doc, "namespace: ee-app") {
				t.Errorf("expected Certificate/%s metadata.namespace to be 'ee-app' (all 15 business routes live there)", secretName)
			}
			// cert-manager 2024+ best practice: rotationPolicy: Always
			// + renewBeforePercentage (not absolute renewBefore). We assert
			// rotationPolicy is set; percentage check is opt-in via values.
			if !strings.Contains(doc, "rotationPolicy: Always") {
				t.Errorf("expected Certificate/%s spec.rotationPolicy to be 'Always' (cert-manager 2024+ best practice)", secretName)
			}
		})

		t.Run("02_"+f.name+"_apisixtls_renders", func(t *testing.T) {
			doc := apisixTlsDocForFamily(rendered, f.name)
			if doc == "" {
				t.Fatalf("expected ApisixTls/%s to be rendered", secretName)
			}
			if !strings.Contains(doc, f.hostname) {
				t.Errorf("expected ApisixTls/%s spec.hosts to contain %q", secretName, f.hostname)
			}
			if !strings.Contains(doc, "namespace: ee-app") {
				t.Errorf("expected ApisixTls/%s metadata.namespace to be 'ee-app'", secretName)
			}
		})

		t.Run("03_"+f.name+"_routes_have_hosts", func(t *testing.T) {
			// Every existing ApisixRoute belonging to this family MUST
			// have `match.hosts: [<hostname>]` in spec.http[*].match.
			// This is the actual ingress rule — without it, traffic
			// would never hit the family SNI.
			for _, routeName := range f.routes {
				routeName := routeName
				t.Run(routeName, func(t *testing.T) {
					doc := apisixRouteDocForName(rendered, routeName)
					if doc == "" {
						t.Fatalf("expected ApisixRoute/%s to be rendered (it predates Stage 29-D; must remain)", routeName)
					}
					if !strings.Contains(doc, f.hostname) {
						t.Errorf("expected ApisixRoute/%s match.hosts to contain %q "+
							"(Stage 29-D injects hosts per family so APISIX routes the family SNI to the right backend)",
							routeName, f.hostname)
					}
				})
			}
		})
	}
}

// TestStage29D_AllFamiliesRouteCountUnchanged guards against accidental
// route duplication. Stage 29-D does NOT add new ApisixRoutes — it only
// injects `match.hosts` into the existing 16 ones. Grafana TLS adds 1
// ApisixRoute (`grafana-tls` in ee-observability, gated on
// global.grafanaIngressTls.enabled=true). Stage 30-A adds 9 more in the
// analytics family (r-reports-*, r-ub-*, r-mh-*) for the analytics-svc
// business endpoints.
//
// Post-30-A total = 26 (= 16 pre-29-D business + 1 grafana-tls + 9 stage-30-A).
//
// RED if any stage accidentally emits new ApisixRoutes beyond this count.
func TestStage29D_AllFamiliesRouteCountUnchanged(t *testing.T) {
	rendered := helm(t, valuesDev)

	routes := countKind(rendered, "ApisixRoute")
	const want = 26 // 16 pre-29-D + 1 grafana-tls + 9 stage-30-A
	if routes != want {
		t.Errorf("expected exactly %d ApisixRoute CRDs "+
			"(16 pre-29-D business + 1 grafana-tls + 9 stage-30-A analytics); "+
			"got %d", want, routes)
	}
}

// TestStage29D_NoNewCertBeyondFiveFamilies guards against accidental
// certificate sprawl. Exactly 5 family Certificates are expected
// (user / chat / analytics / assessment / ai) plus the Grafana one
// = 6 total Certificates in the rendered output.
func TestStage29D_NoNewCertBeyondFiveFamilies(t *testing.T) {
	rendered := helm(t, valuesDev)

	certs := countKind(rendered, "Certificate")
	// Grafana (Stage 29-A.5) + 5 family certs (Stage 29-D) = 6.
	if certs != 6 {
		t.Errorf("expected exactly 6 Certificate CRDs (1 grafana + 5 family); got %d", certs)
	}

	apisixtls := countKind(rendered, "ApisixTls")
	if apisixtls != 6 {
		t.Errorf("expected exactly 6 ApisixTls CRDs (1 grafana + 5 family); got %d", apisixtls)
	}
}