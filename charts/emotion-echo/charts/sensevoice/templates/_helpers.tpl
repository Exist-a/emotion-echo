{{- define "sensevoice.name" -}}
sensevoice
{{- end -}}
{{- define "sensevoice.labels" -}}
app.kubernetes.io/name: {{ include "sensevoice.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/part-of: emotion-echo
app.kubernetes.io/component: ai-profile
{{- end -}}
{{- define "sensevoice.selectorLabels" -}}
app.kubernetes.io/name: {{ include "sensevoice.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}