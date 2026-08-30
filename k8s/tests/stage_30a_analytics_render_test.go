//go:build k8s
// +build k8s

// Package tests · Stage 30-A: analytics-svc business endpoints landing.
//
// This is the RED gate for docs/stage-30-A-analytics-business.md. Every
// assertion here MUST fail against the pre-30-A chart; Stage 30-A's
// GREEN commit flips them to passing by:
//   - extending `Deployment/analytics-svc` env block with
//     POSTGRES_ROLE / POSTGRES_SEARCH_PATH / KAFKA_BROKERS /
//       TRIGGER_QUEUE_CAP (read-only role + Kafka consumer wiring)
//   - naming the analytics-svc container port (`http`) so APISIX
//     backends can target it by name
//   - adding 9 new ApisixRoutes (reports / user-behavior / mental-health)
//     under the analytics family SNI (`analytics.echo.local`)
//   - growing ApisixRoute count from 16 -> 25
//
// Build tag: //go:build k8s (same as the rest of k8s/tests).
package tests

import (
	"regexp"
	"strings"
	"testing"
)

// analyticsDeploymentDoc returns the rendered Deployment/analytics-svc
// YAML block (everything between `^---$` markers that names analytics-svc
// as the Deployment kind/name). Returns "" if not found.
//
// helm template output prepends each resource with a `# Source: ...`
// comment header + apiVersion line, so we strip leading comment + blank
// lines and match the kind anywhere in the resulting body.
func analyticsDeploymentDoc(rendered string) string {
	docs := splitHelmResources(rendered)
	for _, doc := range docs {
		body := stripHelmSourceComment(doc)
		// Match exact `kind: Deployment` line (anchor to line start to
		// avoid matching the same string inside ConfigMap/Secret labels).
		kindRe := regexp.MustCompile(`(?m)^kind:\s*Deployment\s*$`)
		if !kindRe.MatchString(body) {
			continue
		}
		if !strings.Contains(body, "name: analytics-svc") {
			continue
		}
		return body
	}
	return ""
}

// stripHelmSourceComment removes the leading `# Source: ...` comment
// line(s) that helm template emits before each resource. Also strips
// leading blank lines so the result starts at `kind:` for the first
// resource in the document.
func stripHelmSourceComment(doc string) string {
	lines := strings.Split(doc, "\n")
	start := 0
	for start < len(lines) {
		trimmed := strings.TrimSpace(lines[start])
		if trimmed == "" || strings.HasPrefix(trimmed, "# Source:") {
			start++
			continue
		}
		break
	}
	return strings.TrimSpace(strings.Join(lines[start:], "\n"))
}

// splitHelmResources splits rendered helm template output into one
// resource per chunk. We can't just split on `\n---\n` because some
// templates (notably apisix-routes/templates/routes.yaml) emit ALL
// ApisixRoutes inside a SINGLE named template — they appear in the
// rendered output as YAML docs separated only by blank lines, with
// no `---` between them.
//
// Strategy: split on either `\n---\n` (standard helm separator),
// a newline followed by a `# Source: <path>` comment that starts a
// new resource from a different source file, OR a newline followed
// by `apiVersion:`. The `apiVersion:` split handles the apisix-routes
// case where 16+ ApisixRoutes render inside one Go template.
var (
	resourceSplitOnDashRe    = regexp.MustCompile(`\n---\n`)
	resourceSplitOnSourceRe  = regexp.MustCompile(`\n# Source:`)
	resourceSplitOnAPIVerRe  = regexp.MustCompile(`\napiVersion:\s*\S`)
)

func splitHelmResources(rendered string) []string {
	var out []string
	for _, chunk := range resourceSplitOnDashRe.Split(rendered, -1) {
		// Detect multi-resource docs by `apiVersion:` count. Most chunks
		// have exactly one `apiVersion:`; apisix-routes has 16.
		apiCount := strings.Count(chunk, "\napiVersion:")
		if apiCount <= 1 && strings.Count(chunk, "# Source:") <= 1 {
			out = append(out, chunk)
			continue
		}
		// Multi-resource doc: split on `# Source:` and `apiVersion:`.
		// The `apiVersion:` marker reattaches to the next chunk so
		// `kind:` lookup still works.
		markers := append(
			resourceSplitOnSourceRe.FindAllStringIndex(chunk, -1),
			resourceSplitOnAPIVerRe.FindAllStringIndex(chunk, -1)...,
		)
		if len(markers) == 0 {
			out = append(out, chunk)
			continue
		}
		start := 0
		for _, idx := range markers {
			out = append(out, chunk[start:idx[0]])
			// For `# Source:` lines, advance past the '\n' so the
			// marker reattaches to the next chunk. For `apiVersion:`
			// lines, also advance past '\n'.
			start = idx[0] + 1
		}
		out = append(out, chunk[start:])
	}
	return out
}

// analyticsRouteCount counts ApisixRoute CRDs whose metadata.name
// matches the analytics family or any of the 9 new business route
// names introduced by Stage 30-A.
//
// Pre-30-A: 1 route in analytics family (r-analytics-health) = 1
// Post-30-A: +9 routes (r-reports-*, r-ub-*, r-mh-*) = 10
func analyticsRouteCount(rendered string) int {
	docs := splitHelmResources(rendered)
	count := 0
	kindRe := regexp.MustCompile(`(?m)^kind:\s*ApisixRoute\s*$`)
	for _, doc := range docs {
		body := stripHelmSourceComment(doc)
		if !kindRe.MatchString(body) {
			continue
		}
		name := routeMetadataName(body)
		if name == "" {
			continue
		}
		if strings.HasPrefix(name, "r-reports-") ||
			strings.HasPrefix(name, "r-ub-") ||
			strings.HasPrefix(name, "r-mh-") ||
			name == "r-analytics-health" {
			count++
		}
	}
	return count
}

// routeMetadataName returns the `metadata.name` of an ApisixRoute doc
// body, or "" if not found.
func routeMetadataName(body string) string {
	// metadata.name lives at indent 2 inside the doc.
	re := regexp.MustCompile(`(?m)^  name:\s*(\S+)\s*$`)
	m := re.FindStringSubmatch(body)
	if m == nil {
		return ""
	}
	return m[1]
}

// TestStage30A_AnalyticsDeploymentHasReadOnlyDBRole asserts that the
// analytics-svc Deployment carries the read-only DB role + multi-schema
// search_path required by stage-30-A §六.6.4. Without these env vars
// analytics-svc cannot connect to cross-schema VIEWs as analytics_reader.
//
// RED today: env block contains POSTGRES_DSN / SKYWALKING_OAP_ADDR /
// LOG_FORMAT / LOG_LEVEL / TZ — none of POSTGRES_ROLE /
// POSTGRES_SEARCH_PATH / KAFKA_BROKERS / TRIGGER_QUEUE_CAP.
func TestStage30A_AnalyticsDeploymentHasReadOnlyDBRole(t *testing.T) {
	rendered := helm(t, valuesDev)
	dep := analyticsDeploymentDoc(rendered)
	if dep == "" {
		t.Fatal("Deployment/analytics-svc not rendered (chart broken?)")
	}

	for _, key := range []string{
		"POSTGRES_ROLE=analytics_reader",
		"POSTGRES_SEARCH_PATH=emotion_echo_analytics,emotion_echo_chat,emotion_echo_ai,emotion_echo_assessment",
		"KAFKA_BROKERS=",
		"TRIGGER_QUEUE_CAP=",
	} {
		if !strings.Contains(dep, key) {
			t.Errorf("Deployment/analytics-svc env missing %q "+
				"(stage-30-A §六.6.4: read-only DB role + Kafka consumer wiring)",
				key)
		}
	}
}

// TestStage30A_AnalyticsContainerPortNamedHttp asserts the analytics-svc
// container port is named `http` so APISIX ApisixRoute backends can
// target it via servicePort name (not raw int). This is the standard
// pattern across all other ee-app Deployments (user-svc / chat-svc /
// ai-svc / assessment-svc).
//
// RED today: containerPort block has `containerPort: 8893` but no
// `name: http`.
func TestStage30A_AnalyticsContainerPortNamedHttp(t *testing.T) {
	rendered := helm(t, valuesDev)
	dep := analyticsDeploymentDoc(rendered)
	if dep == "" {
		t.Fatal("Deployment/analytics-svc not rendered")
	}

	// Look for `- name: http\n          containerPort: 8893` shape.
	portRe := regexp.MustCompile(`(?m)name:\s*http\s*\n\s*containerPort:\s*8893`)
	if !portRe.MatchString(dep) {
		t.Errorf("expected analytics-svc container port to be named 'http' " +
			"(matches sibling services' APISIX targeting pattern)")
	}
}

// TestStage30A_NineBusinessRoutesAdded asserts Stage 30-A introduces
// exactly 9 new ApisixRoutes (the 9 business endpoints from
// docs/stage-30-A-analytics-business.md §二) AND that they're routed
// under the analytics family host so they reach the analytics-svc
// backend.
//
// RED today: only r-analytics-health exists in analytics family; the
// other 9 route names do not exist anywhere in the chart.
func TestStage30A_NineBusinessRoutesAdded(t *testing.T) {
	rendered := helm(t, valuesDev)

	// Nine expected route names from stage-30-A §二.
	expected := []string{
		"r-reports-daily",
		"r-reports-trend",
		"r-ub-day-night",
		"r-ub-depth",
		"r-ub-frequency",
		"r-mh-assessment",
		"r-mh-history",
		"r-mh-trigger",
		"r-mh-trend",
	}

	docs := splitHelmResources(rendered)
	kindRe := regexp.MustCompile(`(?m)^kind:\s*ApisixRoute\s*$`)
	for _, want := range expected {
		want := want
		t.Run(want, func(t *testing.T) {
			found := false
			for _, doc := range docs {
				body := stripHelmSourceComment(doc)
				if !kindRe.MatchString(body) {
					continue
				}
				if routeMetadataName(body) == want {
					found = true
					// Must be reachable on analytics family host.
					if !strings.Contains(body, "analytics.echo.local") {
						t.Errorf("ApisixRoute/%s missing match.hosts: [analytics.echo.local] "+
							"(stage-30-A uses analytics family SNI)", want)
					}
					// Must backend-point to analytics-svc upstream.
					if !strings.Contains(body, "serviceName: analytics-svc") {
						t.Errorf("ApisixRoute/%s backend should point to analytics-svc",
							want)
					}
					break
				}
			}
			if !found {
				t.Errorf("ApisixRoute/%s not rendered (stage-30-A §二 introduces this endpoint)",
					want)
			}
		})
	}

	// Family-wide sanity: exactly 10 routes in analytics family
	// (1 existing + 9 new). Pre-30-A this would be 1; post-30-A it's 10.
	if got := analyticsRouteCount(rendered); got != 10 {
		t.Errorf("expected 10 ApisixRoutes in analytics family (1 health + 9 business), "+
			"got %d", got)
	}
}

// TestStage30A_AnalyticsHostGuard ensures the existing
// r-analytics-health route keeps the analytics family host set
// (Stage 29-D added it; Stage 30-A must not accidentally strip it
// while editing the routes block). Forward-guard test that should
// pass today and remain passing after GREEN.
func TestStage30A_AnalyticsHostGuard(t *testing.T) {
	rendered := helm(t, valuesDev)

	if !valuesDevHasAnalyticsTLS(t) {
		t.Skip("pre-29-D values; analytics family host block is gated on tls.enabled")
	}

	docs := splitHelmResources(rendered)
	kindRe := regexp.MustCompile(`(?m)^kind:\s*ApisixRoute\s*$`)
	for _, doc := range docs {
		body := stripHelmSourceComment(doc)
		if !kindRe.MatchString(body) {
			continue
		}
		if routeMetadataName(body) != "r-analytics-health" {
			continue
		}
		if !strings.Contains(body, "analytics.echo.local") {
			t.Errorf("ApisixRoute/r-analytics-health lost analytics family host " +
				"(Stage 29-D regression guard)")
		}
		return
	}
	t.Fatal("r-analytics-health ApisixRoute not rendered (chart broken)")
}

// valuesDevHasAnalyticsTLS inspects values-dev.yaml to decide whether
// tls.enabled=true is set. Stage 30-A works in both modes; the host
// guard only applies when TLS is enabled.
func valuesDevHasAnalyticsTLS(t *testing.T) bool {
	t.Helper()
	// We can't parse YAML here easily; just look at the rendered
	// output: if `analytics.echo.local` appears anywhere in the
	// rendered routes, TLS is on in dev values.
	rendered := helm(t, valuesDev)
	return strings.Contains(rendered, "analytics.echo.local")
}

// TestStage30A_NoRoutesAddedToOtherFamilies is the negative test:
// Stage 30-A must NOT add routes to user / chat / assessment / ai
// families. Any accidental cross-family route addition breaks
// Stage 29-D's cert-manager per-family guarantees.
func TestStage30A_NoRoutesAddedToOtherFamilies(t *testing.T) {
	rendered := helm(t, valuesDev)

	docs := splitHelmResources(rendered)
	kindRe := regexp.MustCompile(`(?m)^kind:\s*ApisixRoute\s*$`)
	for _, doc := range docs {
		body := stripHelmSourceComment(doc)
		if !kindRe.MatchString(body) {
			continue
		}
		// Only the 9 new routes + r-analytics-health are allowed
		// to backend-point at analytics-svc.
		if !strings.Contains(body, "serviceName: analytics-svc") {
			continue
		}
		name := routeMetadataName(body)
		if name == "" {
			continue
		}
		ok := name == "r-analytics-health" ||
			strings.HasPrefix(name, "r-reports-") ||
			strings.HasPrefix(name, "r-ub-") ||
			strings.HasPrefix(name, "r-mh-")
		if !ok {
			t.Errorf("ApisixRoute/%s backends to analytics-svc but is not in the "+
				"analytics family (allowed: r-analytics-health, r-reports-*, r-ub-*, r-mh-*)",
				name)
		}
	}
}