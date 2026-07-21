{{- define "fer.name" -}}
fer
{{- end -}}
{{- define "fer.labels" -}}
app.kubernetes.io/name: {{ include "fer.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/part-of: emotion-echo
app.kubernetes.io/component: ai-profile
{{- end -}}
{{- define "fer.selectorLabels" -}}
app.kubernetes.io/name: {{ include "fer.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}