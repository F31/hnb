{{- define "operation-worker.stagedProviderTokenInit" -}}
{{- $sandbox := .Values.sandbox | default dict -}}
{{- $digest := $sandbox.digest | default "" -}}
{{- if not (regexMatch "^sha256:[0-9a-f]{64}$" $digest) -}}
{{- fail "operation-worker.sandbox.digest must be a sha256 digest; a tag is not an artifact identity" -}}
{{- end }}
{{- if not .Values.serviceTokenFiles -}}
{{- fail "operation-worker.serviceTokenFiles must list the provider token keys to stage" -}}
{{- end }}
{{- $uid := .Values.runAsUser | default 65532 -}}
{{- $repository := $sandbox.repository | default "rancher/mirrored-library-busybox" -}}
{{- $registry := .Values.global.imageRegistry | default "" -}}
{{- $steps := list -}}
{{- range $key := .Values.serviceTokenFiles -}}
{{- $steps = append $steps (printf "[ -f /source/%s ] && cp /source/%s /staged/%s && chown %d:%d /staged/%s && chmod 0400 /staged/%s" $key $key $key $uid $uid $key $key) -}}
{{- end }}
- name: stage-provider-tokens
  image: {{ if $registry }}{{ $registry }}/{{ $repository }}@{{ $digest }}{{ else }}{{ $repository }}@{{ $digest }}{{ end }}
  imagePullPolicy: {{ $sandbox.pullPolicy | default .Values.global.imagePullPolicy }}
  command:
    - sh
    - -c
    - {{ join " && " $steps | quote }}
  securityContext:
    runAsUser: 0
    runAsNonRoot: false
  resources:
    requests:
      cpu: 5m
      memory: 8Mi
    limits:
      cpu: 50m
      memory: 32Mi
  volumeMounts:
    - name: provider-service-tokens
      mountPath: /source
      readOnly: true
    - name: staged-provider-tokens
      mountPath: /staged
{{- end -}}

// iam.FileTokenSource rejects any token file readable by group or other, and a
// Secret volume mounted under fsGroup resolves to 0440. Stage an owner-only copy
// into a private emptyDir so the worker can actually read its provider tokens.
// Staged files are only refreshed when the pod restarts; a token that expires
// sooner than the restart interval still needs an external minting source.
{{- define "operation-worker.stagedProviderTokenVolume" -}}
- name: staged-provider-tokens
  emptyDir:
    medium: Memory
    sizeLimit: 64Ki
{{- end -}}

{{- define "operation-worker.stagedProviderTokenMount" -}}
- name: staged-provider-tokens
  mountPath: /var/run/hnb-provider-tokens
  readOnly: true
{{- end -}}