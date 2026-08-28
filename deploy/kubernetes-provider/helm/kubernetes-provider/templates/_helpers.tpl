{{- define "kubernetes-provider.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "kubernetes-provider.fullname" -}}
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

{{- define "kubernetes-provider.labels" -}}
helm.sh/chart: {{ include "kubernetes-provider.name" . }}-{{ .Chart.Version | replace "+" "_" }}
{{ include "kubernetes-provider.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "kubernetes-provider.selectorLabels" -}}
app.kubernetes.io/name: {{ include "kubernetes-provider.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}