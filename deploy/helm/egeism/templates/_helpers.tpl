{{- define "egeism.name" -}}
egeism
{{- end }}

{{- define "egeism.labels" -}}
app.kubernetes.io/name: {{ include "egeism.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version }}
{{- end }}

{{- define "egeism.selectorLabels" -}}
app.kubernetes.io/name: {{ include "egeism.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "egeism.image" -}}
{{ printf "%s/egeism-%s:%s" .root.Values.global.imageRegistry .component .root.Values.global.imageTag }}
{{- end }}

