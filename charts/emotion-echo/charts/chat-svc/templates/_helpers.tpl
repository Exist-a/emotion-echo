{{- define "chat-svc.name" -}}
chat-svc
{{- end -}}

{{- define "chat-svc.labels" -}}
app.kubernetes.io/name: {{ include "chat-svc.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/part-of: emotion-echo
app.kubernetes.io/component: app
{{- end -}}

{{- define "chat-svc.selectorLabels" -}}
app.kubernetes.io/name: {{ include "chat-svc.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}