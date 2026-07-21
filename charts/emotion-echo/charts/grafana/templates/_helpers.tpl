{{/*
Chart name.
*/}}
{{- define "grafana.name" -}}
grafana
{{- end -}}

{{/*
Generic namespace lookup by logical key with default fallback.
This is the same pattern Stage 28-A uses for prometheus. Kept here for
self-containment; Stage 28-F may factor these out into a _common.tpl
if the duplication grows.
*/}}
{{- define "grafana.namespace" -}}
{{- $key := .key -}}
{{- $fallback := .default -}}
{{- if and .Values .Values.namespaces (index .Values.namespaces $key) -}}
{{- index .Values.namespaces $key -}}
{{- else -}}
{{- $fallback -}}
{{- end -}}
{{- end -}}

{{/*
Semantic shortcut for the observability namespace (the only ns this chart
touches — Grafana reads from prometheus/loki across ns boundaries via FQDN).
*/}}
{{- define "grafana.namespaceObservability" -}}
{{- include "grafana.namespace" (dict "key" "observability" "default" "ee-observability" "Values" .Values) -}}
{{- end -}}

{{/*
Common labels.
*/}}
{{- define "grafana.labels" -}}
app.kubernetes.io/name: {{ include "grafana.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/part-of: emotion-echo
app.kubernetes.io/component: observability
{{- end -}}

{{/*
Selector labels.
*/}}
{{- define "grafana.selectorLabels" -}}
app.kubernetes.io/name: {{ include "grafana.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}