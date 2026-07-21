{{- define "ai-svc.name" -}}
ai-svc
{{- end -}}
{{- define "ai-svc.labels" -}}
app.kubernetes.io/name: {{ include "ai-svc.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/part-of: emotion-echo
app.kubernetes.io/component: app
{{- end -}}
{{- define "ai-svc.selectorLabels" -}}
app.kubernetes.io/name: {{ include "ai-svc.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}