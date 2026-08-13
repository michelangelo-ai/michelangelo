{{- define "michelangelo-llm-gateway.name" -}}
{{- .Chart.Name | trunc 63 | trimSuffix "-" -}}
{{- end }}

{{- define "michelangelo-llm-gateway.fullname" -}}
{{- if contains .Chart.Name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name .Chart.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end }}

{{- define "michelangelo-llm-gateway.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end }}

{{- define "michelangelo-llm-gateway.labels" -}}
helm.sh/chart: {{ include "michelangelo-llm-gateway.chart" . }}
{{ include "michelangelo-llm-gateway.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "michelangelo-llm-gateway.selectorLabels" -}}
app.kubernetes.io/name: {{ include "michelangelo-llm-gateway.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "michelangelo-llm-gateway.litellmSelectorLabels" -}}
app.kubernetes.io/name: litellm
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "michelangelo-llm-gateway.litellmImage" -}}
{{- printf "%s:%s" .Values.litellm.image.repository .Values.litellm.image.tag -}}
{{- end }}

{{- define "michelangelo-llm-gateway.testImage" -}}
{{- printf "%s:%s" .Values.tests.image.repository .Values.tests.image.tag -}}
{{- end }}
