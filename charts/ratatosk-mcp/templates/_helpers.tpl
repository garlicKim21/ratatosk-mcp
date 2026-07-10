{{- define "ratatosk-mcp.fullname" -}}
{{- if contains .Chart.Name .Release.Name }}{{ .Release.Name | trunc 63 | trimSuffix "-" }}{{ else }}{{ printf "%s-%s" .Release.Name .Chart.Name | trunc 63 | trimSuffix "-" }}{{ end }}
{{- end }}

{{- define "ratatosk-mcp.labels" -}}
app.kubernetes.io/name: {{ .Chart.Name }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "ratatosk-mcp.selectorLabels" -}}
app.kubernetes.io/name: {{ .Chart.Name }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}
