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

{{- define "michelangelo-llm-gateway.validateProxyConfigSecrets" -}}
{{- $value := .value -}}
{{- $path := .path -}}
{{- $sensitive := `((^|_)(api_key|master_key|password|secret|secret_key|private_key|secret_access_key|access_key|access_key_id|credential|credentials|session_token|api_token|auth_token|authentication_token|authorization_token|access_token|bearer_token|oauth_token|id_token|refresh_token|github_token|gh_token|hf_token|vault_token|integration_token|azure_ad_token|authorization)$|^token$)` -}}
{{- if kindIs "map" $value -}}
{{- range $key, $child := $value -}}
{{- $childPath := printf "%s.%s" $path $key -}}
{{- $normalizedKey := lower (replace "-" "_" (toString $key)) -}}
{{- if and (regexMatch $sensitive $normalizedKey) (not (empty $child)) -}}
{{- if or (not (kindIs "string" $child)) (not (regexMatch `^os\.environ/[A-Za-z_][A-Za-z0-9_]*$` $child)) -}}
{{- fail (printf "%s must use os.environ/ENV_VAR; plaintext credentials are forbidden" $childPath) -}}
{{- end -}}
{{- end -}}
{{- include "michelangelo-llm-gateway.validateProxyConfigSecrets" (dict "value" $child "path" $childPath) -}}
{{- end -}}
{{- else if kindIs "slice" $value -}}
{{- range $index, $child := $value -}}
{{- include "michelangelo-llm-gateway.validateProxyConfigSecrets" (dict "value" $child "path" (printf "%s[%d]" $path $index)) -}}
{{- end -}}
{{- end -}}
{{- end -}}
