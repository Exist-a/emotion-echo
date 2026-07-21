{{- define "user-svc.name" -}}
user-svc
{{- end -}}

{{- define "user-svc.labels" -}}
app.kubernetes.io/name: {{ include "user-svc.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/part-of: emotion-echo
app.kubernetes.io/component: app
{{- end -}}

{{- define "user-svc.selectorLabels" -}}
app.kubernetes.io/name: {{ include "user-svc.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}