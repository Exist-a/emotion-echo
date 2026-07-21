{{- define "apisix-ingress.name" -}}
apisix
{{- end -}}
{{- define "apisix-ingress.labels" -}}
app.kubernetes.io/name: {{ include "apisix-ingress.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/part-of: emotion-echo
app.kubernetes.io/component: ingress
{{- end -}}
{{- define "apisix-ingress.selectorLabels" -}}
app.kubernetes.io/name: {{ include "apisix-ingress.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}