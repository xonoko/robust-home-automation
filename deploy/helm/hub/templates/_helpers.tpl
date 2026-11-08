{{- define "hub.name" -}}
{{- .Release.Name | trunc 50 | trimSuffix "-" -}}
{{- end -}}

{{- define "hub.labels" -}}
app.kubernetes.io/name: hub
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version }}
{{- end -}}

{{- define "hub.selectorLabels" -}}
app.kubernetes.io/name: hub
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "hub.image" -}}
{{ .Values.image.repository }}:{{ .Values.image.tag | default .Chart.AppVersion }}
{{- end -}}

{{/* CloudNativePG cluster name and the secret it creates for the app user. */}}
{{- define "hub.pgCluster" -}}
{{ include "hub.name" . }}-pg
{{- end -}}

{{- define "hub.pgSecret" -}}
{{ include "hub.pgCluster" . }}-app
{{- end -}}
