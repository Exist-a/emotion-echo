{{- define "apisix-routes.name" -}}
apisix-routes
{{- end -}}

{{- define "apisix-routes.labels" -}}
app.kubernetes.io/name: {{ include "apisix-routes.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/part-of: emotion-echo
app.kubernetes.io/component: ingress
{{- end -}}