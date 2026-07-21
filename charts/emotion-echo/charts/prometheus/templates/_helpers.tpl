{{/*
Expand the name of the chart.
*/}}
{{- define "prometheus.name" -}}
prometheus
{{- end -}}

{{/*
Look up a namespace by logical key (system/data/app/observability).
Falls back to the default convention name if .Values.namespaces is missing
or the key is missing — this lets the subchart render in isolation during
unit tests (//go:build k8s) without requiring umbrella values to set
.namespaces.<key> explicitly.

Usage: {{ include "prometheus.namespace" (dict "key" "observability" "default" "ee-observability" "Values" .Values) }}
*/}}
{{- define "prometheus.namespace" -}}
{{- $key := .key -}}
{{- $fallback := .default -}}
{{- if and .Values .Values.namespaces (index .Values.namespaces $key) -}}
{{- index .Values.namespaces $key -}}
{{- else -}}
{{- $fallback -}}
{{- end -}}
{{- end -}}

{{/*
Semantic shortcuts for the four Emotion-Echo namespaces. Use these in
templates instead of the raw dict pattern. Falls back to the umbrella
convention name so render-assert tests don't need to set .Values.namespaces.
*/}}
{{- define "prometheus.namespaceObservability" -}}
{{- include "prometheus.namespace" (dict "key" "observability" "default" "ee-observability" "Values" .Values) -}}
{{- end -}}
{{- define "prometheus.namespaceData" -}}
{{- include "prometheus.namespace" (dict "key" "data" "default" "ee-data" "Values" .Values) -}}
{{- end -}}
{{- define "prometheus.namespaceSystem" -}}
{{- include "prometheus.namespace" (dict "key" "system" "default" "ee-system" "Values" .Values) -}}
{{- end -}}
{{- define "prometheus.namespaceApp" -}}
{{- include "prometheus.namespace" (dict "key" "app" "default" "ee-app" "Values" .Values) -}}
{{- end -}}

{{/*
Common labels — applied to every resource.
*/}}
{{- define "prometheus.labels" -}}
app.kubernetes.io/name: {{ include "prometheus.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/part-of: emotion-echo
app.kubernetes.io/component: observability
{{- end -}}

{{/*
Selector labels — used in Deployment matchLabels and Service selector.
Must be a stable subset of labels (no chart-version, no release).
*/}}
{{- define "prometheus.selectorLabels" -}}
app.kubernetes.io/name: {{ include "prometheus.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/*
Full resource name: <release>-prometheus (truncated to 63 chars).
*/}}
{{- define "prometheus.fullname" -}}
{{- $name := include "prometheus.name" . -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}