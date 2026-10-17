// The signing key Secret is mounted at 0400 and owned by root, but the scratch
// image runs as a non-root UID, so the process cannot read a key that the IAM
// hardening check also requires to be owner-only. A small init container copies
// the key into a private emptyDir and hands it to that UID with mode 0400, which
// satisfies both constraints without weakening the check.
//
// Usage: include "apiserver.stagedSigningKeyInit"
{{- define "apiserver.stagedSigningKeyInit" -}}
{{- $sandbox := .Values.sandbox | default dict -}}
{{- $digest := $sandbox.digest | default "" -}}
{{- if not (regexMatch "^sha256:[0-9a-f]{64}$" $digest) -}}
{{- fail "apiserver.sandbox.digest must be a sha256 digest; a tag is not an artifact identity" -}}
{{- end }}
{{- $repository := $sandbox.repository | default "rancher/mirrored-library-busybox" -}}
{{- $registry := .Values.global.imageRegistry | default "" -}}
- name: stage-signing-key
  image: {{ if $registry }}{{ $registry }}/{{ $repository }}@{{ $digest }}{{ else }}{{ $repository }}@{{ $digest }}{{ end }}
  imagePullPolicy: {{ $sandbox.pullPolicy | default .Values.global.imagePullPolicy }}
  command:
    - sh
    - -c
    - cp /source/{{ .Values.identity.stagedKeyName | default "active-private.pem" }} /staged/{{ .Values.identity.stagedKeyName | default "active-private.pem" }}
      && chown {{ .Values.runAsUser | default 65532 }}:{{ .Values.runAsUser | default 65532 }} /staged/{{ .Values.identity.stagedKeyName | default "active-private.pem" }}
      && chmod 0400 /staged/{{ .Values.identity.stagedKeyName | default "active-private.pem" }}
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
    - name: active-signing-key
      mountPath: /source
      readOnly: true
    - name: staged-signing-key
      mountPath: /staged
{{- end -}}

// The staged key volume and the mount the application actually reads. The
// original root-owned Secret volume is still declared for the init container.
{{- define "apiserver.stagedSigningKeyVolume" -}}
- name: staged-signing-key
  emptyDir:
    medium: Memory
    sizeLimit: 64Ki
{{- end -}}

{{- define "apiserver.stagedSigningKeyMount" -}}
- name: staged-signing-key
  mountPath: /var/run/hnb-signing
  readOnly: true
{{- end -}}