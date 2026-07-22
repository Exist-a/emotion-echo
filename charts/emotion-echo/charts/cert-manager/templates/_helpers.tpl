{{/*
cert-manager label & selector helpers.
Mirrors the style of the umbrella _helpers.tpl so that
the rendered resources carry the standard emotion-echo labels.
*/}}

{{- define "cert-manager.name" -}}
{{- default "cert-manager" .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "cert-manager.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default "cert-manager" .Values.nameOverride -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{- define "cert-manager.labels" -}}
app.kubernetes.io/name: {{ include "cert-manager.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/component: cert-manager
app.kubernetes.io/part-of: emotion-echo
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" }}
{{- end -}}

{{- define "cert-manager.selectorLabels" -}}
app.kubernetes.io/name: {{ include "cert-manager.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: cert-manager
{{- end -}}

{{- /*
  cert-manager.namespace — single source of truth for the
  namespace in which every cert-manager workload runs.

  Stage 29-A.5: every template references this helper instead of
  the literal `cert-manager`. The default remains `cert-manager`;
  setting `.Values.namespace` lets operators relocate the
  installation (e.g. shared cluster where another team already
  owns the cert-manager namespace and we want to install ours
  alongside as `cert-manager-ee`).
*/ -}}
{{- define "cert-manager.namespace" -}}
{{- default "cert-manager" .Values.namespace -}}
{{- end -}}