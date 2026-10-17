{{/*
Shared image reference helper.

Every component image must be pinned by content digest. A tag is not an
identity: tags are mutable, so a tag-pinned install cannot be reproduced or
verified against a release lock. This helper refuses to render without a real
sha256 digest, refuses placeholder digests, and applies the global registry
prefix so a single values overlay can retarget every image at once.

Usage: {{ include "hnb.image" . }}
where the calling chart exposes .Values.image.repository / .Values.image.digest.
*/}}
{{- define "hnb.image" -}}
{{- $image := .Values.image | default dict -}}
{{- $repository := $image.repository | default "" -}}
{{- $digest := $image.digest | default "" -}}
{{- if not $repository -}}
{{- fail (printf "%s: image.repository is required" .Chart.Name) -}}
{{- end -}}
{{- if not (regexMatch "^sha256:[0-9a-f]{64}$" $digest) -}}
{{- fail (printf "%s: image.digest must be a sha256 digest (got %q); tags are not an artifact identity" .Chart.Name $digest) -}}
{{- end -}}
{{- if eq $digest "sha256:0000000000000000000000000000000000000000000000000000000000000000" -}}
{{- fail (printf "%s: image.digest must not be a placeholder" .Chart.Name) -}}
{{- end -}}
{{- $registry := .Values.global.imageRegistry | default "" -}}
{{- if $registry -}}
{{- printf "%s/%s@%s" $registry $repository $digest -}}
{{- else -}}
{{- printf "%s@%s" $repository $digest -}}
{{- end -}}
{{- end -}}

{{/*
hnb.imagePullPolicy keeps pull behaviour consistent across components without
repeating the default lookup in every template.
*/}}
{{- define "hnb.imagePullPolicy" -}}
{{- .Values.image.pullPolicy | default .Values.global.imagePullPolicy -}}
{{- end -}}

{

{{/*
Plane account credentials for the bundled NATS server.

The server declares one username/password user per plane account, so a client
certificate alone cannot authenticate it. The component's plane Secret supplies
the account name and password, keeping plane isolation and keeping the secret
out of values and release metadata. The file mode stays 0400.

Usage:
  include "portal.natsAccountEnv" (dict "root" $ "component" .Values.nats)
*/}}
{{- define "portal.natsAccountEnv" -}}
{{- $nats := .component | default dict -}}
{{- if $nats.existingAccountSecret }}
- name: NATS_USER_FILE
  value: /var/run/nats-account/user
- name: NATS_PASSWORD_FILE
  value: /var/run/nats-account/password
{{- end }}
{{- end -}}

{{- define "portal.natsAccountMounts" -}}
{{- $nats := .component | default dict -}}
{{- if $nats.existingAccountSecret }}
- name: nats-account
  mountPath: /var/run/nats-account
  readOnly: true
{{- end }}
{{- end -}}

{{- define "portal.natsAccountVolumes" -}}
{{- $nats := .component | default dict -}}
{{- if $nats.existingAccountSecret }}
- name: nats-account
  secret:
    secretName: {{ $nats.existingAccountSecret }}
    defaultMode: 0440
    items:
      - key: user
        path: user
      - key: password
        path: password
{{- end }}
{{- end -}}
