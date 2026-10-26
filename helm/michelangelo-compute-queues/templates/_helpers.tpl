{{/*
Expand the name of the chart.
*/}}
{{- define "michelangelo-compute-queues.name" -}}
{{- .Chart.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "michelangelo-compute-queues.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Common labels applied to every resource this chart renders.
*/}}
{{- define "michelangelo-compute-queues.labels" -}}
helm.sh/chart: {{ include "michelangelo-compute-queues.chart" . }}
{{ include "michelangelo-compute-queues.selectorLabels" . }}
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
{{- define "michelangelo-compute-queues.selectorLabels" -}}
app.kubernetes.io/name: {{ include "michelangelo-compute-queues.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}
