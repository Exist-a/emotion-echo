{{- define "xtts.name" -}}
xtts
{{- end -}}
{{- define "xtts.labels" -}}
app.kubernetes.io/name: {{ include "xtts.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/part-of: emotion-echo
app.kubernetes.io/component: ai-profile
{{- end -}}
{{- define "xtts.selectorLabels" -}}
app.kubernetes.io/name: {{ include "xtts.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}