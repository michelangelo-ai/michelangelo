{{/*
Expand the name of the chart.
*/}}
{{- define "michelangelo.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this
(by the DNS naming spec). If release name contains chart name it will be used
as a full name.
*/}}
{{- define "michelangelo.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "michelangelo.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Common labels applied to every resource.
*/}}
{{- define "michelangelo.labels" -}}
helm.sh/chart: {{ include "michelangelo.chart" . }}
{{ include "michelangelo.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: michelangelo
{{- end -}}

{{/*
Selector labels (must be invariant across upgrades — do NOT include version
labels here, otherwise Deployment selectors break on chart bump).
*/}}
{{- define "michelangelo.selectorLabels" -}}
app.kubernetes.io/name: {{ include "michelangelo.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/*
Per-component selector labels (used by per-Deployment selectors).
Usage: {{ include "michelangelo.componentSelectorLabels" (dict "context" . "component" "apiserver") }}
*/}}
{{- define "michelangelo.componentSelectorLabels" -}}
{{ include "michelangelo.selectorLabels" .context }}
app.kubernetes.io/component: {{ .component }}
{{- end -}}

{{/*
Per-component full labels (used on metadata of per-Deployment objects).
Usage: {{ include "michelangelo.componentLabels" (dict "context" . "component" "apiserver") }}
*/}}
{{- define "michelangelo.componentLabels" -}}
{{ include "michelangelo.labels" .context }}
app.kubernetes.io/component: {{ .component }}
{{- end -}}

{{/*
Per-component fullname (release-scoped, prevents cross-namespace collisions).
Usage: {{ include "michelangelo.componentFullname" (dict "context" . "component" "apiserver") }}
Output: <fullname>-<component>, e.g. "michelangelo-apiserver"
*/}}
{{- define "michelangelo.componentFullname" -}}
{{- printf "%s-%s" (include "michelangelo.fullname" .context) .component | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
ServiceAccount name to use.
*/}}
{{- define "michelangelo.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "michelangelo.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{/*
Object storage credentials Secret name (release-scoped so multiple installs
in different namespaces don't collide; matches design §"Credentials Idempotency").
*/}}
{{- define "michelangelo.objectStorageSecretName" -}}
{{- printf "%s-object-storage-credentials" (include "michelangelo.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Schema-init volume name (used by apiserver Deployment + ConfigMap).
*/}}
{{- define "michelangelo.schemaInitConfigMapName" -}}
{{- printf "%s-schema-init" (include "michelangelo.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}
