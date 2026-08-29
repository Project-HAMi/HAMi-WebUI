{{/*
Expand the name of the chart.
*/}}
{{- define "hami-webui.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Render an image reference. A verified manifest digest takes precedence over a
mutable tag; tag remains the backwards-compatible default for normal installs.
*/}}
{{- define "hami-webui.image" -}}
{{- $repository := required (printf "image.%s.repository is required" .name) .image.repository -}}
{{- $digest := default "" .image.digest -}}
{{- if $digest -}}
{{- if not (regexMatch "^sha256:[a-f0-9]{64}$" $digest) -}}
{{- fail (printf "image.%s.digest must be a sha256 digest" .name) -}}
{{- end -}}
{{- printf "%s@%s" $repository $digest -}}
{{- else -}}
{{- $appVersion := required "Chart.appVersion is required when an image digest is not set" .appVersion -}}
{{- $defaultTag := ternary $appVersion (printf "v%s" $appVersion) (hasPrefix "v" $appVersion) -}}
{{- $tag := default $defaultTag .image.tag -}}
{{- printf "%s:%s" $repository $tag -}}
{{- end -}}
{{- end -}}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "hami-webui.fullname" -}}
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
Name the internal backend Service without allowing a 63-character fullname to
drop the distinguishing suffix.
*/}}
{{- define "hami-webui.backendServiceName" -}}
{{- $baseName := include "hami-webui.fullname" . | trunc 55 | trimSuffix "-" -}}
{{- printf "%s-backend" $baseName -}}
{{- end -}}

{{/*
Allow the release namespace to be overridden for multi-namespace deployments in combined charts
*/}}
{{- define "hami-webui.namespace" -}}
  {{- if .Values.namespaceOverride -}}
    {{- .Values.namespaceOverride -}}
  {{- else -}}
    {{- .Release.Namespace -}}
  {{- end -}}
{{- end -}}

{{/*
Resolve the Prometheus address. When the embedded stack is enabled, use the
dependency chart's own helpers so its naming and override rules cannot drift.
*/}}
{{- define "hami-webui.prometheusAddress" -}}
{{- if .Values.externalPrometheus.enabled -}}
{{- .Values.externalPrometheus.address -}}
{{- else if (index .Values "kube-prometheus-stack").enabled -}}
{{- $stack := index .Subcharts "kube-prometheus-stack" -}}
{{- printf "http://%s-prometheus.%s.svc.cluster.local:%v" (include "kube-prometheus-stack.fullname" $stack) (include "kube-prometheus-stack.namespace" $stack) $stack.Values.prometheus.service.port -}}
{{- else -}}
{{- printf "http://%s-kube-prometh-prometheus.%s.svc.cluster.local:9090" (include "hami-webui.fullname" .) (include "hami-webui.namespace" .) -}}
{{- end -}}
{{- end -}}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "hami-webui.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "hami-webui.labels" -}}
helm.sh/chart: {{ include "hami-webui.chart" . }}
{{ include "hami-webui.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "hami-webui.selectorLabels" -}}
app.kubernetes.io/name: {{ include "hami-webui.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Keep the Chart 1.x backend port when the value is absent from reused values.
Do not use Helm's default function here: an explicit false is considered empty.
*/}}
{{- define "hami-webui.legacyBackendPortEnabled" -}}
{{- $serviceSettings := default (dict) .Values.service -}}
{{- $legacyBackendPort := true -}}
{{- if hasKey $serviceSettings "legacyBackendPort" -}}
{{- $configuredLegacyBackendPort := get $serviceSettings "legacyBackendPort" -}}
{{- if not (kindIs "bool" $configuredLegacyBackendPort) -}}
{{- fail "service.legacyBackendPort must be a boolean" -}}
{{- end -}}
{{- $legacyBackendPort = $configuredLegacyBackendPort -}}
{{- end -}}
{{- $legacyBackendPort -}}
{{- end -}}

{{/*
Create the name of the service account to use
*/}}
{{- define "hami-webui.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "hami-webui.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}
