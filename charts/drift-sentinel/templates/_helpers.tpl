{{- define "drift-sentinel.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "drift-sentinel.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name (include "drift-sentinel.name" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{- define "drift-sentinel.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "drift-sentinel.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{- define "drift-sentinel.labels" -}}
app.kubernetes.io/name: {{ include "drift-sentinel.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/*
Looks up and/or generates certificates for the webhook.
*/}}
{{- define "drift-sentinel.lookupAndGenerateCerts" -}}
{{- if not (hasKey . "_driftSentinelCerts") -}}
  {{- $secretName := printf "%s-cert" (include "drift-sentinel.fullname" .) -}}
  {{- $secret := lookup "v1" "Secret" .Release.Namespace $secretName -}}
  {{- $certs := dict -}}
  {{- if and $secret $secret.data (index $secret.data "ca.crt") (index $secret.data "tls.crt") (index $secret.data "tls.key") -}}
    {{- $_ := set $certs "ca" (index $secret.data "ca.crt" | b64dec) -}}
    {{- $_ := set $certs "cert" (index $secret.data "tls.crt" | b64dec) -}}
    {{- $_ := set $certs "key" (index $secret.data "tls.key" | b64dec) -}}
  {{- else -}}
    {{- $fullname := include "drift-sentinel.fullname" . -}}
    {{- $serviceName := $fullname -}}
    {{- $ca := genCA (printf "%s-ca" $fullname) 3650 -}}
    {{- $altNames := list $serviceName (printf "%s.%s" $serviceName .Release.Namespace) (printf "%s.%s.svc" $serviceName .Release.Namespace) (printf "%s.%s.svc.cluster.local" $serviceName .Release.Namespace) -}}
    {{- $cert := genSignedCert $serviceName nil $altNames 3650 $ca -}}
    {{- $_ := set $certs "ca" $ca.Cert -}}
    {{- $_ := set $certs "cert" $cert.Cert -}}
    {{- $_ := set $certs "key" $cert.Key -}}
  {{- end -}}
  {{- $_ := set . "_driftSentinelCerts" $certs -}}
{{- end -}}
{{- get . "_driftSentinelCerts" | toYaml -}}
{{- end -}}
