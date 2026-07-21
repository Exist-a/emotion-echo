{{- define "alertmanager.name" -}}
alertmanager
{{- end -}}

{{- define "alertmanager.namespace" -}}
{{- $key := .key -}}
{{- $fallback := .default -}}
{{- if and .Values .Values.namespaces (index .Values.namespaces $key) -}}
{{- index .Values.namespaces $key -}}
{{- else -}}
{{- $fallback -}}
{{- end -}}
{{- end -}}
{{- define "alertmanager.namespaceObservability" -}}
{{- include "alertmanager.namespace" (dict "key" "observability" "default" "ee-observability" "Values" .Values) -}}
{{- end -}}

{{- define "alertmanager.labels" -}}
app.kubernetes.io/name: {{ include "alertmanager.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/part-of: emotion-echo
app.kubernetes.io/component: observability
{{- end -}}
{{- define "alertmanager.selectorLabels" -}}
app.kubernetes.io/name: {{ include "alertmanager.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}