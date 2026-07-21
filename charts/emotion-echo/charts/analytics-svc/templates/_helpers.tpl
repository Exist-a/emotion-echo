{{- define "analytics-svc.name" -}}
analytics-svc
{{- end -}}
{{- define "analytics-svc.labels" -}}
app.kubernetes.io/name: {{ include "analytics-svc.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/part-of: emotion-echo
app.kubernetes.io/component: app
{{- end -}}
{{- define "analytics-svc.selectorLabels" -}}
app.kubernetes.io/name: {{ include "analytics-svc.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}