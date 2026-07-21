{{- define "assessment-svc.name" -}}
assessment-svc
{{- end -}}
{{- define "assessment-svc.labels" -}}
app.kubernetes.io/name: {{ include "assessment-svc.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/part-of: emotion-echo
app.kubernetes.io/component: app
{{- end -}}
{{- define "assessment-svc.selectorLabels" -}}
app.kubernetes.io/name: {{ include "assessment-svc.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}