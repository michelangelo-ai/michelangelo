{{/*
Expand the name of the chart.
*/}}
{{- define "michelangelo-compute.name" -}}
{{- .Chart.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this
(by the DNS naming spec). If release name contains chart name it will be used
as a full name.
*/}}
{{- define "michelangelo-compute.fullname" -}}
{{- if contains .Chart.Name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name .Chart.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "michelangelo-compute.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Common labels applied to every resource this chart renders itself. The
subcharts label their own resources.
*/}}
{{- define "michelangelo-compute.labels" -}}
helm.sh/chart: {{ include "michelangelo-compute.chart" . }}
{{ include "michelangelo-compute.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: michelangelo
{{- end -}}

{{/*
Selector labels (must be invariant across upgrades — do NOT include version
labels here, otherwise selectors break on chart bump).
*/}}
{{- define "michelangelo-compute.selectorLabels" -}}
app.kubernetes.io/name: {{ include "michelangelo-compute.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}
