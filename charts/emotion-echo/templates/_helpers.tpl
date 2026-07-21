{{/*
Expand the name of the chart.
*/}}
{{- define "emotion-echo.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some K8s name fields are limited to this (RFC 1123).
*/}}
{{- define "emotion-echo.fullname" -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Common labels — applied to every resource.
*/}}
{{- define "emotion-echo.labels" -}}
app.kubernetes.io/name: {{ include "emotion-echo.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: emotion-echo
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end -}}

{{/*
Layered namespace labels.
Usage:  {{ include "emotion-echo.layerLabel" "app" | nindent 4 }}

Returns multiple lines (each indented by caller). The first line intentionally
ends with a newline so callers can use nindent cleanly.
*/}}
{{- define "emotion-echo.layerLabel" -}}
{{- if eq . "system" -}}
layer: ingress
{{- else if eq . "data" -}}
layer: data
{{- else if eq . "app" -}}
layer: app
{{- else if eq . "observability" -}}
layer: observability
{{- end -}}
{{- end -}}

{{/*
Per-subchart image — used by subchart deployment templates.
Usage: {{ include "emotion-echo.image" (dict "image" .Values.user-svc.image "global" .Values.image) }}
*/}}
{{- define "emotion-echo.image" -}}
{{- $img := .image -}}
{{- $global := .global -}}
{{- if $img.registry -}}
{{ $img.registry }}/{{ $img.repository }}:{{ $img.tag }}
{{- else -}}
{{ $img.repository }}:{{ $img.tag }}
{{- end -}}
{{- end -}}