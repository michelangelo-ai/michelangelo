{{/*
Expand the name of the chart.
*/}}
{{- define "michelangelo.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "michelangelo.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "michelangelo.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "michelangelo.labels" -}}
helm.sh/chart: {{ include "michelangelo.chart" . }}
{{ include "michelangelo.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "michelangelo.selectorLabels" -}}
app.kubernetes.io/name: {{ include "michelangelo.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "michelangelo.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "michelangelo.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Return the proper image name for a component
*/}}
{{- define "michelangelo.image" -}}
{{- $registry := .registry }}
{{- $repository := .repository }}
{{- $tag := .tag | default $.Chart.AppVersion }}
{{- printf "%s/%s:%s" $registry $repository $tag }}
{{- end }}

{{/*
Return S3/Storage endpoint based on cloud provider
*/}}
{{- define "michelangelo.storageEndpoint" -}}
{{- if eq .Values.cloud "gcp" -}}
{{- .Values.storage.s3.endpoint | default "storage.googleapis.com" -}}
{{- else if eq .Values.cloud "aws" -}}
{{- .Values.storage.s3.endpoint | default (printf "s3.%s.amazonaws.com" .Values.storage.s3.region) -}}
{{- else if eq .Values.cloud "azure" -}}
{{- .Values.storage.s3.endpoint | default "blob.core.windows.net" -}}
{{- else -}}
{{- .Values.storage.s3.endpoint | default "storage.googleapis.com" -}}
{{- end -}}
{{- end }}

{{/*
Return storage type
*/}}
{{- define "michelangelo.storageType" -}}
{{- if eq .Values.cloud "gcp" -}}
gcs
{{- else if eq .Values.cloud "aws" -}}
s3
{{- else if eq .Values.cloud "azure" -}}
azure
{{- else -}}
{{- .Values.storage.s3.type -}}
{{- end -}}
{{- end }}

{{/*
Return Gateway API endpoint for inference routing
*/}}
{{- define "michelangelo.gatewayEndpoint" -}}
{{- if .Values.gateway.enabled -}}
http://ma-gateway-istio.{{ .Values.namespace.name }}.svc.cluster.local:80
{{- else -}}
""
{{- end -}}
{{- end }}
