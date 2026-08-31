{{/*
Nacos common helpers (Stage 31 PR-12)
*/}}

{{- define "nacos.fullname" -}}
{{- default .Chart.Name .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "nacos.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
app.kubernetes.io/name: {{ include "nacos.fullname" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/component: registry-and-config
app.kubernetes.io/part-of: emotion-echo
{{- end -}}

{{- define "nacos.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "nacos.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}
