// Package tests provides integration tests for the Helm umbrella chart.
//
// Build tag: //go:build k8s
//
// These tests shell out to the `helm` binary, render the umbrella chart
// against values-dev.yaml, and assert that all expected Kubernetes resources
// are present. They intentionally do NOT spin up a cluster — this is a
// fast, hermetic "rendering smoke test" that runs in <5s and gates every PR.
//
// Reference: docs/stage-27-k8s-local-helm.md (Stage 27-A TDD loop).
package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// chartRoot is the absolute path of the umbrella chart, computed once.
// Resolved relative to the test package directory (k8s/tests/) so the
// suite works no matter where the user invokes `go test` from.
var chartRoot = func() string {
	// We are in k8s/tests; the chart lives at <repo>/charts/emotion-echo.
	abs, err := filepath.Abs("../../charts/emotion-echo")
	if err != nil {
		panic(err)
	}
	return abs
}()

// valuesDev is the dev overlay path, also relative to the test package.
var valuesDev = func() string {
	abs, err := filepath.Abs("../../charts/emotion-echo/values-dev.yaml")
	if err != nil {
		panic(err)
	}
	return abs
}()

// helm runs `helm template` against the umbrella chart and returns the
// rendered YAML as a single string. Fails the test if helm errors out.
func helm(t *testing.T, values ...string) string {
	t.Helper()
	args := []string{"template", "ee", chartRoot}
	for _, v := range values {
		args = append(args, "-f", v)
	}
	out, err := exec.Command("helm", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("helm template failed: %v\n%s", err, out)
	}
	return string(out)
}

// countKind returns the number of resources of `kind` in the rendered YAML.
// Simple grep-style counter — sufficient for an umbrella-level smoke test.
func countKind(rendered, kind string) int {
	re := regexp.MustCompile(`(?m)^kind:\s*` + kind + `\s*$`)
	return len(re.FindAllString(rendered, -1))
}

// TestStage27A_RendersUmbrella asserts the umbrella renders at all and emits
// the namespaces + at least one of each base resource type. This is the RED
// gate for Stage 27-A.
func TestStage27A_RendersUmbrella(t *testing.T) {
	rendered := helm(t, valuesDev)

	if rendered == "" {
		t.Fatal("helm template produced empty output")
	}

	// Four namespaces must be present (ee-system, ee-data, ee-app, ee-observability).
	for _, ns := range []string{"ee-system", "ee-data", "ee-app", "ee-observability"} {
		re := regexp.MustCompile(`(?m)name:\s*` + ns + `\s*$`)
		if !re.MatchString(rendered) {
			t.Errorf("expected namespace %q to be rendered", ns)
		}
	}
}

// TestStage27A_SubChartsPresent asserts every subchart whose values are not
// all disabled renders at least one Deployment (skipping optional AI svc).
//
// We assert *presence of values-driven subcharts* by checking that the
// rendered Deployment count is >= 10 (4 Go svc + ai-svc + llm-service +
// web + postgres + redis + kafka + skywalking-oap + skywalking-ui = 12
// always-on; AI svc add 3 more when enabled, default dev = on).
func TestStage27A_SubChartsPresent(t *testing.T) {
	rendered := helm(t, valuesDev)

	deps := countKind(rendered, "Deployment")
	if deps < 10 {
		t.Errorf("expected >=10 Deployments (subcharts present), got %d\n---rendered head---\n%s",
			deps, truncate(rendered, 800))
	}

	svcs := countKind(rendered, "Service")
	if svcs < 10 {
		t.Errorf("expected >=10 Services, got %d", svcs)
	}

	configs := countKind(rendered, "ConfigMap")
	if configs < 5 {
		t.Errorf("expected >=5 ConfigMaps (one per svc), got %d", configs)
	}

	secrets := countKind(rendered, "Secret")
	if secrets < 5 {
		t.Errorf("expected >=5 Secrets (DSN + keys per svc), got %d", secrets)
	}
}

// TestStage27A_APISIXRoutes asserts the apisix-routes subchart emits the
// 16 ApisixRoute CRDs + 6 ApisixUpstream CRDs that match the legacy
// docker-compose APISIX routing (see deploy/apisix/apisix.yaml).
func TestStage27A_APISIXRoutes(t *testing.T) {
	rendered := helm(t, valuesDev)

	routes := countKind(rendered, "ApisixRoute")
	if routes != 16 {
		t.Errorf("expected exactly 16 ApisixRoute CRDs, got %d", routes)
	}

	ups := countKind(rendered, "ApisixUpstream")
	if ups != 6 {
		t.Errorf("expected exactly 6 ApisixUpstream CRDs, got %d", ups)
	}
}

// TestStage27A_LintPasses runs `helm lint` against the umbrella chart.
// Catches template syntax errors before any kubectl apply attempt.
func TestStage27A_LintPasses(t *testing.T) {
	out, err := exec.Command("helm", "lint", chartRoot).CombinedOutput()
	if err != nil {
		t.Fatalf("helm lint failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "no failures") &&
		!strings.Contains(string(out), "0 charts failed") {
		t.Logf("helm lint output:\n%s", out)
	}
}

// truncate is a tiny helper to keep assertion failure output readable.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "...(truncated)"
}

// ─────────────────────────────────────────────────────────────────────────────
// Stage 28 · Observability stack (Prometheus / Grafana / Loki / Alertmanager)
//
// Stage 28-A RED gate: assert that the prometheus subchart, when enabled,
// emits exactly the expected resource set into the ee-observability ns.
//
// What we expect (per docs/stage-28-observability.md plan):
//   - 1 Deployment (prometheus server)
//   - 1 Service (ClusterIP:9090)
//   - 1 ConfigMap (prometheus.yml with scrape jobs)
//   - 1 PVC (5Gi default)
//   - 1 ServiceAccount (for RBAC on kubernetes_sd_config)
//
// We assert by counting resources whose metadata.name == "prometheus".
// The umbrella-level umbrella will add its own `prometheus.enabled`
// toggle in Stage 28-F; for now we drive the toggle via --set so the
// test exercises the subchart in isolation.
// ─────────────────────────────────────────────────────────────────────────────

// prometheusOnlyValues returns a minimal values YAML that disables every
// other subchart and enables only `prometheus`. Used by the Stage 28-A tests
// so the test is hermetic — it doesn't depend on the umbrella dependency
// list being complete yet.
//
// We pass this via `helm template --values <tmpfile>` rather than --set
// because umbrella subchart toggles are nested keys that --set is awkward
// to express for. A temp file is the most reliable path.
func prometheusOnlyValues(t *testing.T) string {
	t.Helper()
	// Minimal overlay: turn every umbrella subchart off except prometheus.
	// We list all subcharts we know about so this stays isolated.
	content := `prometheus:
  enabled: true
  retention: 1d
  resources:
    requests: { cpu: 100m, memory: 128Mi }
    limits:   { cpu: 500m, memory: 512Mi }

# Disable everything else so the assertion is hermetic.
postgres:           { enabled: false }
redis:              { enabled: false }
kafka:              { enabled: false }
etcd:               { enabled: false }
skywalking:         { enabled: false }
user-svc:           { enabled: false }
chat-svc:           { enabled: false }
analytics-svc:      { enabled: false }
assessment-svc:     { enabled: false }
ai-svc:             { enabled: false }
llm-service:        { enabled: false }
fer:                { enabled: false }
sensevoice:         { enabled: false }
xtts:               { enabled: false }
web:                { enabled: false }
apisix-ingress:     { enabled: false }
apisix-routes:      { enabled: false }
`
	tmp, err := os.CreateTemp("", "stage28a-values-*.yaml")
	if err != nil {
		t.Fatalf("create temp values: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(tmp.Name()) })
	if _, err := tmp.WriteString(content); err != nil {
		t.Fatalf("write temp values: %v", err)
	}
	if err := tmp.Close(); err != nil {
		t.Fatalf("close temp values: %v", err)
	}
	return tmp.Name()
}

// countResourcesNamed returns the number of resources in the rendered YAML
// whose metadata.name equals `name`. We use a multi-line regex so we don't
// accidentally match strings inside ConfigMap data blocks.
func countResourcesNamed(rendered, name string) int {
	re := regexp.MustCompile(`(?m)- name:\s*` + regexp.QuoteMeta(name) + `\s*$`)
	return len(re.FindAllString(rendered, -1))
}

// hasMetadataName returns true if any resource in the rendered YAML has
// metadata.name == name (catches Kind / Deployment blocks where the name
// is on a separate indented line).
func hasMetadataName(rendered, name string) bool {
	// Pattern A: `  name: prometheus\n  namespace: ee-observability`
	// inside any resource block.
	pat := regexp.MustCompile(`(?m)name:\s*` + regexp.QuoteMeta(name) + `\s*$`)
	return pat.MatchString(rendered)
}

// hasAnnotation returns true if the rendered YAML contains a Pod / Deployment
// metadata.annotations block with the given key=value pair.
func hasAnnotation(rendered, key, value string) bool {
	// Match `key: value` lines that aren't `name:` / `namespace:`.
	// Conservative: we look for the literal `key: value` substring inside an
	// `annotations:` block.
	idx := strings.Index(rendered, "annotations:")
	if idx < 0 {
		return false
	}
	// Take a 2KB window after the annotations: line and look for our key.
	window := rendered[idx:]
	if len(window) > 4096 {
		window = window[:4096]
	}
	return strings.Contains(window, key+": "+value)
}

// TestStage28A_Prometheus_RendersAllResources is the Stage 28-A RED gate.
// It renders the umbrella with only prometheus enabled and asserts the
// subchart emits the expected 5 resources into ee-observability.
//
// Will FAIL until we write the prometheus subchart (Stage 28-A GREEN).
func TestStage28A_Prometheus_RendersAllResources(t *testing.T) {
	rendered := helm(t, valuesDev, prometheusOnlyValues(t))

	// The umbrella namespace template must run (it's unconditional),
	// so ee-observability must exist.
	if !hasMetadataName(rendered, "ee-observability") {
		t.Errorf("expected namespace ee-observability to be rendered")
	}

	// We expect exactly 1 Deployment named "prometheus".
	// Note: this counts ALL resources (incl. ConfigMap, Service, etc.) with
	// metadata.name == "prometheus"; we want each kind at least once.
	kinds := []struct {
		kind, name string
	}{
		{"Deployment", "prometheus"},
		{"Service", "prometheus"},
		{"ConfigMap", "prometheus-config"},
		{"PersistentVolumeClaim", "prometheus-data"},
		{"ServiceAccount", "prometheus"},
	}
	for _, k := range kinds {
		count := countKind(rendered, k.kind)
		// We look for the resource by (kind + name); simpler: use a regex
		// that requires both. To keep the helper generic we just check
		// `count >= 1` per kind and then verify the name appears via
		// a dedicated regex.
		if count < 1 {
			t.Errorf("expected at least 1 %s, got %d", k.kind, count)
		}
		if !hasMetadataName(rendered, k.name) {
			t.Errorf("expected a resource named %q (kind %s)", k.name, k.kind)
		}
	}
}

// TestStage28A_Prometheus_ScrapeConfigReferences checks that the rendered
// prometheus ConfigMap contains scrape job names we expect:
//   - kubernetes-pods (for annotation-based discovery)
//   - skywalking-oap
//   - apisix
//   - prometheus-self
//
// The check is on ConfigMap data (prometheus.yml), so we look for the
// literal `job_name: <name>` substring in the rendered output.
func TestStage28A_Prometheus_ScrapeConfigReferences(t *testing.T) {
	rendered := helm(t, valuesDev, prometheusOnlyValues(t))

	expectedJobs := []string{
		"kubernetes-pods",
		"skywalking-oap",
		"apisix",
		"prometheus-self",
	}
	for _, job := range expectedJobs {
		needle := "job_name: " + job
		if !strings.Contains(rendered, needle) {
			t.Errorf("expected rendered prometheus.yml to contain %q", needle)
		}
	}
}

// TestStage28A_Prometheus_HasAnnotationKeepRule ensures the kubernetes-pods
// scrape job uses the `prometheus.io/scrape` annotation to discover targets.
// If this string isn't in the ConfigMap, the scrape job will discover 0 pods.
func TestStage28A_Prometheus_HasAnnotationKeepRule(t *testing.T) {
	rendered := helm(t, valuesDev, prometheusOnlyValues(t))

	// The annotation key we expect in the relabel_configs block.
	needle := "__meta_kubernetes_pod_annotation_prometheus_io_scrape"
	if !strings.Contains(rendered, needle) {
		t.Errorf("expected kubernetes-pods scrape job to use annotation %q", needle)
	}
}

// TestStage28A_LintPrometheusSubchart ensures the prometheus subchart
// passes `helm lint` in isolation (catches template syntax errors fast).
func TestStage28A_LintPrometheusSubchart(t *testing.T) {
	subchartPath, err := filepath.Abs("../../charts/emotion-echo/charts/prometheus")
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("helm", "lint", subchartPath).CombinedOutput()
	if err != nil {
		t.Fatalf("helm lint failed: %v\n%s", err, out)
	}
	// helm lint exits 0 even with warnings; we only fail on errors.
	t.Logf("helm lint output:\n%s", out)
}

// ─────────────────────────────────────────────────────────────────────────────
// Stage 28-B · Grafana subchart
//
// RED gate: assert the grafana subchart, when enabled, emits:
//   - 1 Deployment (grafana server, image grafana/grafana)
//   - 1 Service (ClusterIP:3000)
//   - ≥3 ConfigMaps (datasources provisioning + grafana.ini + dashboards
//     ConfigMap carrying label grafana_dashboard: "1" for sidecar discovery)
//   - 1 Secret (admin password, learning-phase placeholder)
//
// Datasource wiring must point to prometheus.ee-observability.svc.cluster.local:9090
// and loki.ee-observability.svc.cluster.local:3100 (so the dashboard sidecar
// can ship queries against both stores we land in Stage 28-A and 28-C).
// ─────────────────────────────────────────────────────────────────────────────

// grafanaOnlyValues returns a minimal values YAML that disables every
// other subchart and enables only `grafana`. Mirrors prometheusOnlyValues.
func grafanaOnlyValues(t *testing.T) string {
	t.Helper()
	content := `grafana:
  enabled: true
  adminPassword: "dev-grafana-admin"
  resources:
    requests: { cpu: 100m, memory: 128Mi }
    limits:   { cpu: 500m, memory: 512Mi }

# Disable everything else.
postgres:           { enabled: false }
redis:              { enabled: false }
kafka:              { enabled: false }
etcd:               { enabled: false }
skywalking:         { enabled: false }
user-svc:           { enabled: false }
chat-svc:           { enabled: false }
analytics-svc:      { enabled: false }
assessment-svc:     { enabled: false }
ai-svc:             { enabled: false }
llm-service:        { enabled: false }
fer:                { enabled: false }
sensevoice:         { enabled: false }
xtts:               { enabled: false }
web:                { enabled: false }
apisix-ingress:     { enabled: false }
apisix-routes:      { enabled: false }
prometheus:         { enabled: false }
`
	tmp, err := os.CreateTemp("", "stage28b-values-*.yaml")
	if err != nil {
		t.Fatalf("create temp values: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(tmp.Name()) })
	if _, err := tmp.WriteString(content); err != nil {
		t.Fatalf("write temp values: %v", err)
	}
	if err := tmp.Close(); err != nil {
		t.Fatalf("close temp values: %v", err)
	}
	return tmp.Name()
}

// TestStage28B_Grafana_RendersAllResources is the Stage 28-B RED gate.
func TestStage28B_Grafana_RendersAllResources(t *testing.T) {
	rendered := helm(t, valuesDev, grafanaOnlyValues(t))

	// The umbrella namespace template still runs unconditionally.
	if !hasMetadataName(rendered, "ee-observability") {
		t.Errorf("expected namespace ee-observability to be rendered")
	}

	// Core resources.
	coreResources := []struct{ kind, name string }{
		{"Deployment", "grafana"},
		{"Service", "grafana"},
		{"Secret", "grafana-admin"},
	}
	for _, r := range coreResources {
		if countKind(rendered, r.kind) < 1 {
			t.Errorf("expected at least 1 %s, got 0", r.kind)
		}
		if !hasMetadataName(rendered, r.name) {
			t.Errorf("expected a resource named %q (kind %s)", r.name, r.kind)
		}
	}

	// ConfigMap count: we need at least 3 (datasources + grafana.ini + dashboards).
	configMaps := countKind(rendered, "ConfigMap")
	if configMaps < 3 {
		t.Errorf("expected at least 3 ConfigMaps (datasources/ini/dashboards), got %d", configMaps)
	}

	// Datasource configmap must declare Prometheus + Loki URLs.
	promNeedle := "http://prometheus.ee-observability.svc.cluster.local:9090"
	if !strings.Contains(rendered, promNeedle) {
		t.Errorf("expected datasource configmap to declare prometheus URL %q", promNeedle)
	}
	lokiNeedle := "http://loki.ee-observability.svc.cluster.local:3100"
	if !strings.Contains(rendered, lokiNeedle) {
		t.Errorf("expected datasource configmap to declare loki URL %q", lokiNeedle)
	}

	// At least one dashboard ConfigMap must carry the sidecar label.
	// We look for the literal `grafana_dashboard: "1"` substring anywhere
	// in the rendered output.
	if !strings.Contains(rendered, `grafana_dashboard: "1"`) {
		t.Errorf("expected at least one ConfigMap with label grafana_dashboard: \"1\" for sidecar discovery")
	}
}

// TestStage28B_LintGrafanaSubchart ensures the grafana subchart passes
// `helm lint` in isolation.
func TestStage28B_LintGrafanaSubchart(t *testing.T) {
	subchartPath, err := filepath.Abs("../../charts/emotion-echo/charts/grafana")
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("helm", "lint", subchartPath).CombinedOutput()
	if err != nil {
		t.Fatalf("helm lint failed: %v\n%s", err, out)
	}
	t.Logf("helm lint output:\n%s", out)
}