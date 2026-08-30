{{/* Rendered by NOTES.txt and by the chart contract test fixture. */}}
{{- define "hami-webui.installNotes" -}}
{{- include "hami-webui.validateChart2Values" . -}}
{{- include "hami-webui.validatePublicEntry" . -}}
{{- $namespace := include "hami-webui.namespace" . -}}
{{- $fullName := include "hami-webui.fullname" . -}}
{{- $basePath := include "hami-webui.frontendBasePath" . -}}
1. Get the application URL by running these commands:
{{- if .Values.ingress.enabled }}
{{- range $host := .Values.ingress.hosts }}
  {{- $hostName := default "" (get $host "host") -}}
  {{- $scheme := "http" -}}
  {{- range $tls := $.Values.ingress.tls }}
    {{- range $tlsHost := $tls.hosts }}
      {{- if eq $tlsHost $hostName }}
        {{- $scheme = "https" -}}
      {{- end }}
    {{- end }}
  {{- end }}
  {{- if empty $hostName }}
  export INGRESS_ADDRESS=$(kubectl get ingress --namespace {{ $namespace }} {{ $fullName }} -o jsonpath='{.status.loadBalancer.ingress[0].ip}{.status.loadBalancer.ingress[0].hostname}')
  echo "Open $INGRESS_ADDRESS at path {{ $basePath }} using the HTTP or HTTPS endpoint configured by your Ingress controller"
  {{- else if contains "*" $hostName }}
  echo 'Choose a concrete hostname matching {{ $hostName }}, then visit {{ $scheme }}://<matching-host>{{ $basePath }}'
  {{- else }}
  {{ printf "%s://%s%s" $scheme $hostName $basePath }}
  {{- end }}
{{- end }}
{{- else if eq .Values.service.type "NodePort" }}
  export NODE_PORT=$(kubectl get service --namespace {{ $namespace }} {{ $fullName }} -o jsonpath="{.spec.ports[0].nodePort}")
  export NODE_IP=$(kubectl get nodes -o jsonpath="{.items[0].status.addresses[?(@.type=='InternalIP')].address}")
  echo "http://$NODE_IP:$NODE_PORT{{ $basePath }}"
{{- else if eq .Values.service.type "LoadBalancer" }}
  NOTE: It may take a few minutes for the LoadBalancer address to be available.
        Watch it with 'kubectl get service --namespace {{ $namespace }} {{ $fullName }} --watch'.
  export SERVICE_HOST=$(kubectl get service --namespace {{ $namespace }} {{ $fullName }} -o jsonpath='{.status.loadBalancer.ingress[0].ip}{.status.loadBalancer.ingress[0].hostname}')
  echo "http://$SERVICE_HOST:{{ .Values.service.port }}{{ $basePath }}"
{{- else if eq .Values.service.type "ClusterIP" }}
  echo "Visit http://127.0.0.1:3000{{ $basePath }} to use HAMi-WebUI"
  kubectl --namespace {{ $namespace }} port-forward service/{{ $fullName }} 3000:http
{{- end }}
{{- end -}}
