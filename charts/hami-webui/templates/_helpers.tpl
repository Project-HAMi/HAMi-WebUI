{{/*
Expand the name of the chart.
*/}}
{{- define "hami-webui.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Render an image reference. A verified manifest digest takes precedence over a
mutable tag.
*/}}
{{- define "hami-webui.image" -}}
{{- $repository := required "image.repository is required" .image.repository -}}
{{- $digest := default "" .image.digest -}}
{{- if $digest -}}
{{- if not (regexMatch "^sha256:[a-f0-9]{64}$" $digest) -}}
{{- fail "image.digest must be a sha256 digest" -}}
{{- end -}}
{{- printf "%s@%s" $repository $digest -}}
{{- else -}}
{{- $appVersion := required "Chart.appVersion is required when an image digest is not set" .appVersion -}}
{{- $fallbackTag := $appVersion -}}
{{- if regexMatch "^[0-9]+[.][0-9]+[.][0-9]+(-[0-9A-Za-z.-]+)?([+][0-9A-Za-z.-]+)?$" $appVersion -}}
{{- $fallbackTag = printf "v%s" (replace "+" "_" $appVersion) -}}
{{- end -}}
{{- $tag := default $fallbackTag .image.tag -}}
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
drop the distinguishing suffix. Hash long names so distinct releases with the
same prefix cannot collide after truncation.
*/}}
{{- define "hami-webui.backendServiceName" -}}
{{- $fullName := include "hami-webui.fullname" . -}}
{{- if le (len $fullName) 55 -}}
{{- printf "%s-backend" $fullName -}}
{{- else -}}
{{- $prefix := $fullName | trunc 46 | trimSuffix "-" -}}
{{- printf "%s-%s-backend" $prefix ($fullName | sha256sum | trunc 8) -}}
{{- end -}}
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
Normalize the public URL prefix with the same rules as the Go Web entry. The
Chart uses this value for the Deployment, Ingress validation, and install
notes, so users receive one public-path contract instead of three subtly
different spellings.
*/}}
{{- define "hami-webui.frontendBasePath" -}}
{{- $frontend := default (dict) .Values.frontend -}}
{{- $basePath := "/" -}}
{{- if hasKey $frontend "basePath" -}}
{{- $basePath = get $frontend "basePath" -}}
{{- if not (kindIs "string" $basePath) -}}
{{- fail "frontend.basePath must be a string" -}}
{{- end -}}
{{- if empty $basePath -}}
{{- $basePath = "/" -}}
{{- end -}}
{{- end -}}
{{- if ne (trim $basePath) $basePath -}}
{{- fail "frontend.basePath must not contain surrounding whitespace" -}}
{{- end -}}
{{- if not (hasPrefix "/" $basePath) -}}
{{- $basePath = printf "/%s" $basePath -}}
{{- end -}}
{{- if not (hasSuffix "/" $basePath) -}}
{{- $basePath = printf "%s/" $basePath -}}
{{- end -}}
{{- if ne $basePath "/" -}}
{{- range $segment := splitList "/" (trimAll "/" $basePath) -}}
{{- if or (empty $segment) (eq $segment ".") (eq $segment "..") (not (regexMatch "^[A-Za-z0-9._~-]+$" $segment)) -}}
{{- fail (printf "frontend.basePath %q contains an invalid path segment" $basePath) -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- $basePrefix := trimSuffix "/" $basePath -}}
{{- if or (eq $basePrefix "/health_check") (hasPrefix "/health_check/" $basePrefix) -}}
{{- fail "frontend.basePath conflicts with the public health-check endpoint" -}}
{{- end -}}
{{- $basePath -}}
{{- end -}}

{{/*
Keep the Chart's public URL discoverable and portable. Dedicated frontend
values are the single source of truth for Chart installs; the same settings
remain available as environment variables when the image is run directly.
*/}}
{{- define "hami-webui.validatePublicEntry" -}}
{{- $env := default (list) .Values.env -}}
{{- range $entry := $env -}}
{{- $name := default "" (get $entry "name") -}}
{{- if eq $name "HAMI_WEBUI_BASE_PATH" -}}
{{- fail "env must not override HAMI_WEBUI_BASE_PATH; configure frontend.basePath so the Deployment, Ingress, and install notes stay consistent" -}}
{{- end -}}
{{- if eq $name "HAMI_WEBUI_FRAME_ANCESTORS_JSON" -}}
{{- fail "env must not override HAMI_WEBUI_FRAME_ANCESTORS_JSON; configure frontend.frameAncestors instead" -}}
{{- end -}}
{{- end -}}
{{- $basePath := include "hami-webui.frontendBasePath" . -}}
{{- $basePrefix := trimSuffix "/" $basePath -}}
{{- if empty $basePrefix -}}
{{- $basePrefix = "/" -}}
{{- end -}}
{{- $ingress := default (dict) .Values.ingress -}}
{{- if (get $ingress "enabled") -}}
{{- $hosts := default (list) (get $ingress "hosts") -}}
{{- if empty $hosts -}}
{{- fail "ingress.hosts must contain at least one host when ingress.enabled=true" -}}
{{- end -}}
{{- range $hostIndex, $host := $hosts -}}
{{- $paths := default (list) (get $host "paths") -}}
{{- if empty $paths -}}
{{- fail (printf "ingress.hosts[%d].paths must contain at least one path" $hostIndex) -}}
{{- end -}}
{{- $hostCoversBasePath := false -}}
{{- range $pathIndex, $pathConfig := $paths -}}
{{- $pathType := default "" (get $pathConfig "pathType") -}}
{{- if not (has $pathType (list "Exact" "Prefix" "ImplementationSpecific")) -}}
{{- fail (printf "ingress.hosts[%d].paths[%d].pathType must be Exact, Prefix, or ImplementationSpecific" $hostIndex $pathIndex) -}}
{{- end -}}
{{- if eq $pathType "Prefix" -}}
  {{- $ingressPath := default "" (get $pathConfig "path") -}}
  {{- if and $ingressPath (hasPrefix "/" $ingressPath) -}}
    {{- $ingressPrefix := trimSuffix "/" $ingressPath -}}
    {{- if empty $ingressPrefix -}}
      {{- $ingressPrefix = "/" -}}
    {{- end -}}
    {{- $pathIsClean := and (eq (clean $ingressPath) $ingressPrefix) (not (contains "//" $ingressPath)) -}}
    {{- $coversBasePath := or (eq $ingressPrefix "/") (eq $ingressPrefix $basePrefix) (hasPrefix (printf "%s/" $ingressPrefix) $basePrefix) -}}
    {{- if and $pathIsClean $coversBasePath -}}
      {{- $hostCoversBasePath = true -}}
    {{- end -}}
  {{- end -}}
{{- end -}}
{{- end -}}
{{- if not $hostCoversBasePath -}}
{{- fail (printf "ingress.hosts[%d] needs at least one clean Prefix path that covers frontend.basePath %q without stripping it" $hostIndex $basePath) -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Require one explicit Prometheus ownership mode. A guessed in-cluster address
can let the Pod become Ready while every metrics query points at a Service that
does not exist, so an unconfigured or ambiguous mode must fail at render time.
*/}}
{{- define "hami-webui.validatePrometheusConfiguration" -}}
{{- $external := default (dict) .Values.externalPrometheus -}}
{{- $stack := default (dict) (index .Values "kube-prometheus-stack") -}}
{{- $externalEnabled := default false (get $external "enabled") -}}
{{- $stackEnabled := default false (get $stack "enabled") -}}
{{- if and $externalEnabled $stackEnabled -}}
{{- fail "externalPrometheus.enabled and kube-prometheus-stack.enabled are mutually exclusive; select exactly one Prometheus mode" -}}
{{- end -}}
{{- if not (or $externalEnabled $stackEnabled) -}}
{{- fail "Prometheus is not configured; enable externalPrometheus with an explicit address or enable kube-prometheus-stack" -}}
{{- end -}}
{{- if $externalEnabled -}}
{{- $rawAddress := default "" (get $external "address") -}}
{{- $address := trim $rawAddress -}}
{{- if empty $address -}}
{{- fail "externalPrometheus.address is required when externalPrometheus.enabled=true" -}}
{{- end -}}
{{- if ne $rawAddress $address -}}
{{- fail "externalPrometheus.address must not contain leading or trailing whitespace" -}}
{{- end -}}
{{- if not (regexMatch "^https?://[^[:space:]]+$" $address) -}}
{{- fail "externalPrometheus.address must be an absolute http:// or https:// URL without whitespace" -}}
{{- end -}}
{{- if regexMatch "^https?://[^/?#]*@" $address -}}
{{- fail "externalPrometheus.address must not include user information; configure authorization or basicAuth instead" -}}
{{- end -}}
{{- $parsedAddress := urlParse $address -}}
{{- if empty (get $parsedAddress "hostname") -}}
{{- fail "externalPrometheus.address must include a hostname" -}}
{{- end -}}
{{- end -}}
{{- if $stackEnabled -}}
{{- $prometheus := default (dict) (get $stack "prometheus") -}}
{{- if and (hasKey $prometheus "enabled") (not (get $prometheus "enabled")) -}}
{{- fail "kube-prometheus-stack.prometheus.enabled must remain true when kube-prometheus-stack.enabled=true" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Resolve the Prometheus address. When the dependency stack is enabled, use its
own helpers so release-name, namespace and port overrides cannot drift.
*/}}
{{- define "hami-webui.prometheusAddress" -}}
{{- include "hami-webui.validatePrometheusConfiguration" . -}}
{{- if .Values.externalPrometheus.enabled -}}
{{- .Values.externalPrometheus.address -}}
{{- else if (index .Values "kube-prometheus-stack").enabled -}}
{{- $stack := index .Subcharts "kube-prometheus-stack" -}}
{{- printf "http://%s-prometheus.%s.svc.cluster.local:%v" (include "kube-prometheus-stack.fullname" $stack) (include "kube-prometheus-stack.namespace" $stack) $stack.Values.prometheus.service.port -}}
{{- end -}}
{{- end -}}

{{/*
Validate file-backed authentication for an external Prometheus endpoint. Secret
contents never enter values or rendered configuration; the Chart only accepts a
Secret name and selected data keys.
*/}}
{{- define "hami-webui.validatePrometheusAuthentication" -}}
{{- $external := default (dict) .Values.externalPrometheus -}}
{{- $rawAuthorization := get $external "authorization" -}}
{{- $rawBasicAuth := get $external "basicAuth" -}}
{{- if and $rawAuthorization (not (kindIs "map" $rawAuthorization)) -}}
{{- fail "externalPrometheus.authorization must be a map" -}}
{{- end -}}
{{- if and $rawBasicAuth (not (kindIs "map" $rawBasicAuth)) -}}
{{- fail "externalPrometheus.basicAuth must be a map" -}}
{{- end -}}
{{- $authorization := default (dict) $rawAuthorization -}}
{{- $basicAuth := default (dict) $rawBasicAuth -}}
{{- range $field := list "type" "existingSecret" "credentialsKey" -}}
{{- if and (hasKey $authorization $field) (not (kindIs "string" (get $authorization $field))) -}}
{{- fail (printf "externalPrometheus.authorization.%s must be a string" $field) -}}
{{- end -}}
{{- $value := default "" (get $authorization $field) -}}
{{- if ne $value (trim $value) -}}
{{- fail (printf "externalPrometheus.authorization.%s must not contain leading or trailing whitespace" $field) -}}
{{- end -}}
{{- end -}}
{{- range $field := list "existingSecret" "usernameKey" "passwordKey" -}}
{{- if and (hasKey $basicAuth $field) (not (kindIs "string" (get $basicAuth $field))) -}}
{{- fail (printf "externalPrometheus.basicAuth.%s must be a string" $field) -}}
{{- end -}}
{{- $value := default "" (get $basicAuth $field) -}}
{{- if ne $value (trim $value) -}}
{{- fail (printf "externalPrometheus.basicAuth.%s must not contain leading or trailing whitespace" $field) -}}
{{- end -}}
{{- end -}}
{{- $authorizationType := trim (default "Bearer" (get $authorization "type")) -}}
{{- $authorizationSecret := trim (default "" (get $authorization "existingSecret")) -}}
{{- $credentialsKey := trim (default "" (get $authorization "credentialsKey")) -}}
{{- $basicAuthSecret := trim (default "" (get $basicAuth "existingSecret")) -}}
{{- $usernameKey := trim (default "" (get $basicAuth "usernameKey")) -}}
{{- $passwordKey := trim (default "" (get $basicAuth "passwordKey")) -}}
{{- $authorizationConfigured := or (ne $authorizationSecret "") (ne $credentialsKey "") (ne $authorizationType "Bearer") -}}
{{- $basicAuthConfigured := or (ne $basicAuthSecret "") (ne $usernameKey "") (ne $passwordKey "") -}}
{{- if and $authorizationConfigured $basicAuthConfigured -}}
{{- fail "externalPrometheus.authorization and externalPrometheus.basicAuth are mutually exclusive" -}}
{{- end -}}
{{- if and (or $authorizationConfigured $basicAuthConfigured) (not (get $external "enabled")) -}}
{{- fail "external Prometheus authentication requires externalPrometheus.enabled=true" -}}
{{- end -}}
{{- if $authorizationConfigured -}}
{{- if or (empty $authorizationSecret) (empty $credentialsKey) -}}
{{- fail "externalPrometheus.authorization.existingSecret and credentialsKey must be configured together" -}}
{{- end -}}
{{- if empty $authorizationType -}}
{{- fail "externalPrometheus.authorization.type must not be empty" -}}
{{- end -}}
{{- if eq (lower $authorizationType) "basic" -}}
{{- fail "externalPrometheus.authorization.type cannot be Basic; use externalPrometheus.basicAuth" -}}
{{- end -}}
{{- if not (regexMatch "^[A-Za-z0-9._-]+$" $credentialsKey) -}}
{{- fail (printf "externalPrometheus authorization Secret key %q is invalid" $credentialsKey) -}}
{{- end -}}
{{- end -}}
{{- if $basicAuthConfigured -}}
{{- if or (empty $basicAuthSecret) (empty $usernameKey) (empty $passwordKey) -}}
{{- fail "externalPrometheus.basicAuth.existingSecret, usernameKey and passwordKey must be configured together" -}}
{{- end -}}
{{- range $key := list $usernameKey $passwordKey -}}
{{- if not (regexMatch "^[A-Za-z0-9._-]+$" $key) -}}
{{- fail (printf "externalPrometheus basicAuth Secret key %q is invalid" $key) -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Validate the HTTPS trust material used by an external Prometheus endpoint.
Secret values are mounted as files; only their key names appear in rendered
configuration.
*/}}
{{- define "hami-webui.validatePrometheusTLS" -}}
{{- $external := default (dict) .Values.externalPrometheus -}}
{{- $rawTLS := get $external "tls" -}}
{{- if and $rawTLS (not (kindIs "map" $rawTLS)) -}}
{{- fail "externalPrometheus.tls must be a map" -}}
{{- end -}}
{{- $tls := default (dict) $rawTLS -}}
{{- range $field := list "serverName" "existingSecret" "caKey" "certKey" "keyKey" -}}
{{- if and (hasKey $tls $field) (not (kindIs "string" (get $tls $field))) -}}
{{- fail (printf "externalPrometheus.tls.%s must be a string" $field) -}}
{{- end -}}
{{- end -}}
{{- if and (hasKey $tls "insecureSkipVerify") (not (kindIs "bool" (get $tls "insecureSkipVerify"))) -}}
{{- fail "externalPrometheus.tls.insecureSkipVerify must be a boolean" -}}
{{- end -}}
{{- $secretName := default "" (get $tls "existingSecret") -}}
{{- $caKey := default "" (get $tls "caKey") -}}
{{- $certKey := default "" (get $tls "certKey") -}}
{{- $keyKey := default "" (get $tls "keyKey") -}}
{{- if .Values.externalPrometheus.enabled -}}
{{- if ne (empty $certKey) (empty $keyKey) -}}
{{- fail "externalPrometheus.tls.certKey and keyKey must be configured together" -}}
{{- end -}}
{{- if and (empty $secretName) (or $caKey $certKey $keyKey) -}}
{{- fail "externalPrometheus.tls.existingSecret is required when a TLS key is configured" -}}
{{- end -}}
{{- if and $secretName (not (or $caKey $certKey $keyKey)) -}}
{{- fail "externalPrometheus.tls must reference at least one key from existingSecret" -}}
{{- end -}}
{{- range $key := list $caKey $certKey $keyKey -}}
{{- if and $key (not (regexMatch "^[A-Za-z0-9._-]+$" $key)) -}}
{{- fail (printf "externalPrometheus TLS Secret key %q is invalid" $key) -}}
{{- end -}}
{{- end -}}
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
Reject Chart 1.x split-container values as one actionable migration error.
The schema intentionally permits these shapes so Helm does not mask this
message before template evaluation.
*/}}
{{- define "hami-webui.validateChart2Values" -}}
{{- $legacy := list -}}
{{- $sections := dict
      "image" (list "frontend" "backend")
      "resources" (list "frontend" "backend")
      "env" (list "frontend" "backend")
      "frontend" (list "proxyTimeout" "livenessProbe" "readinessProbe")
      "backend" (list "grpc" "readinessProbe")
      "service" (list "legacyBackendPort") -}}
{{- range $sectionName, $keys := $sections -}}
{{- $section := get $.Values $sectionName -}}
{{- if kindIs "map" $section -}}
{{- range $key := $keys -}}
{{- if hasKey $section $key -}}
{{- $legacy = append $legacy (printf "%s.%s" $sectionName $key) -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- if $legacy -}}
{{- fail (printf "HAMi-WebUI Chart 2.0 no longer accepts Chart 1.x split-container values: %s. Migrate to the flat image, resources, env, and probes settings; when upgrading from Chart 1.x, create a fresh Chart 2 values file and pass it together with --reset-values" (join ", " (sortAlpha $legacy))) -}}
{{- end -}}
{{- if not (kindIs "slice" .Values.env) -}}
{{- fail "env must be a list of Kubernetes environment variables" -}}
{{- end -}}
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
