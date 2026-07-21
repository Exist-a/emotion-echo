{{- define "skywalking.name" -}}
skywalking
{{- end -}}

{{- define "skywalking.labels" -}}
app.kubernetes.io/name: {{ include "skywalking.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/part-of: emotion-echo
app.kubernetes.io/component: observability
{{- end -}}

{{- define "skywalking.selectorLabels" -}}
app.kubernetes.io/name: {{ include "skywalking.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "skywalking.oapSelectorLabels" -}}
app.kubernetes.io/name: {{ include "skywalking.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: oap
{{- end -}}

{{- define "skywalking.uiSelectorLabels" -}}
app.kubernetes.io/name: {{ include "skywalking.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: ui
{{- end -}}