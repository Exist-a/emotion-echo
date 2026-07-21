{{/*
Chart name helpers.
*/}}
{{- define "loki.name" -}}
loki
{{- end -}}
{{- define "loki.promtailName" -}}
promtail
{{- end -}}

{{/*
Namespace lookup (same pattern as prometheus/grafana subcharts).
*/}}
{{- define "loki.namespace" -}}
{{- $key := .key -}}
{{- $fallback := .default -}}
{{- if and .Values .Values.namespaces (index .Values.namespaces $key) -}}
{{- index .Values.namespaces $key -}}
{{- else -}}
{{- $fallback -}}
{{- end -}}
{{- end -}}
{{- define "loki.namespaceObservability" -}}
{{- include "loki.namespace" (dict "key" "observability" "default" "ee-observability" "Values" .Values) -}}
{{- end -}}

{{/*
Common labels for loki.
*/}}
{{- define "loki.labels" -}}
app.kubernetes.io/name: {{ include "loki.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/part-of: emotion-echo
app.kubernetes.io/component: observability
{{- end -}}
{{- define "loki.selectorLabels" -}}
app.kubernetes.io/name: {{ include "loki.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/*
Common labels for promtail (separate from loki so promtail pods have
their own selector — Promtail pods are part of a DaemonSet, not the
loki Deployment).
*/}}
{{- define "loki.promtailLabels" -}}
app.kubernetes.io/name: {{ include "loki.promtailName" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/part-of: emotion-echo
app.kubernetes.io/component: observability
{{- end -}}
{{- define "loki.promtailSelectorLabels" -}}
app.kubernetes.io/name: {{ include "loki.promtailName" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}