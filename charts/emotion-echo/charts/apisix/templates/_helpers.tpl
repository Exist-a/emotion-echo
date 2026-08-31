{{/*
APISIX common helpers (Stage 32 PR-13)

apisix.namespaceSystem: 让 prometheus subchart 能跨 chart 引用 APISIX Service FQDN。
与 prometheus subchart 的命名模式一致（namespace fallback 逻辑）。
*/}}

{{- define "apisix.fullname" -}}
{{- default .Chart.Name .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "apisix.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
app.kubernetes.io/name: {{ include "apisix.fullname" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/component: api-gateway
app.kubernetes.io/part-of: emotion-echo
{{- end -}}

{{- define "apisix.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "apisix.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{/*
Look up namespace by logical key (system/data/app/observability).
与 prometheus subchart 模式一致，便于跨 chart 引用 FQDN。
*/}}
{{- define "apisix.namespace" -}}
{{- $key := .key -}}
{{- $fallback := .default -}}
{{- if and .Values .Values.namespaces (index .Values.namespaces $key) -}}
{{- index .Values.namespaces $key -}}
{{- else -}}
{{- $fallback -}}
{{- end -}}
{{- end -}}

{{- define "apisix.namespaceSystem" -}}
{{- include "apisix.namespace" (dict "key" "system" "default" "ee-system" "Values" .Values) -}}
{{- end -}}

{{/*
Stage 32 PR-13: Cross-chart FQDN — Etcd subchart's Service fullname.
与 charts/etcd/templates/_helpers.tpl 的 etcd.fullname 输出一致。
定义在此处让 apisix subchart 独立 lint 时也能渲染。
*/}}
{{- define "etcd.fullname" -}}
{{- default "etcd" .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}
