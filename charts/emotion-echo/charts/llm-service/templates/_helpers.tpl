{{- define "llm-service.name" -}}
llm-service
{{- end -}}
{{- define "llm-service.labels" -}}
app.kubernetes.io/name: {{ include "llm-service.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/part-of: emotion-echo
app.kubernetes.io/component: app
{{- end -}}
{{- define "llm-service.selectorLabels" -}}
app.kubernetes.io/name: {{ include "llm-service.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}