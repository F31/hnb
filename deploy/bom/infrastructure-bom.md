# Infrastructure BOM

## NATS JetStream (async messaging backbone)

### Component Versions

| Component | Version | Digest / Ref | Support Window |
|-----------|---------|--------------|----------------|
| NATS Server | 2.11.1 | `nats:2.11.1-scratch` | 18 months from release |
| nats.go (Go Client) | v1.40.2 | `github.com/nats-io/nats.go@v1.40.2` | 12 months from release |
| NATS Helm Chart | 1.3.0 | `nats/nats:1.3.0` | 12 months from release |
| nats-server-config-loader | embedded | bundled with NATS Server | same as NATS Server |

### Image Digests

| Image | Reference | Digest |
|-------|-----------|--------|
| NATS Server | `nats:2.11.1-scratch` | `sha256:a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1` |
| NATS Config Reloader | `natsio/nats-server-config-reloader:0.16.0` | `sha256:b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2` |
| NATS Box (tooling) | `natsio/nats-box:0.27.0` | `sha256:c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3` |

> Digest values above are placeholders; lock actual digest on first certified build.

### Config Schema

NATS Server configuration follows the [NATS Server Config format](https://docs.nats.io/running-a-nats-service/configuration). HNB-specific overlay:

- `jetstream: { max_memory_store: <per-tier>, max_file_store: <per-tier> }`
- `authorization: { users: [<service-account-mapping>] }`
- `tls: { cert_file, key_file, ca_file }` — mTLS required for all production tiers
- `websocket: {}` — disabled by default; enabled only for Portal proxy if approved

### Compatibility Matrix

| NATS Server | nats.go | Helm Chart | Migration |
|-------------|---------|------------|-----------|
| 2.11.x | 1.40.x | 1.3.x | In-place rolling upgrade |
| 2.10.x | 1.39.x | 1.2.x | Requires storage format migration |
| < 2.10 | < 1.38 | < 1.1 | Not supported — full redeploy |

### Ports

| Port | Protocol | Purpose | Locked |
|------|----------|---------|--------|
| 4222 | TCP | NATS client connections | Yes |
| 7422 | TCP | NATS cluster routes | Yes |
| 8222 | TCP | NATS HTTP monitoring | Yes (internal only) |
| 6222 | TCP | NATS gateway | No (not used in MVP) |

### Resource Budget (per instance)

| Tier | CPU | Memory | Storage |
|------|-----|--------|---------|
| Development | 0.5 core | 256 MiB | 2 GiB |
| Minimal | 1 core | 512 MiB | 10 GiB |
| Lite HA | 2 core | 1 GiB | 50 GiB |
| Standard HA | 4 core | 2 GiB | 200 GiB |
| Enterprise | 8 core | 4 GiB | 500 GiB+ |

## Alert/Notification Service

### Component Versions

| Component | Version | Digest / Ref | Support Window |
|-----------|---------|--------------|----------------|
| Alert Normalizer | 1.0.0 | `hnb/alert-normalizer:1.0.0` | 12 months from release |
| Notification Dispatcher | 1.0.0 | `hnb/notification-dispatcher:1.0.0` | 12 months from release |
| Email Worker | 1.0.0 | `hnb/email-worker:1.0.0` | 12 months from release |
| Webhook Worker | 1.0.0 | `hnb/webhook-worker:1.0.0` | 12 months from release |
| SMS Provider (T2 optional) | 1.0.0 | `hnb/sms-provider:1.0.0` | 12 months from release |
| Portal Alert Center | 1.0.0 | `hnb/portal-alert-center:1.0.0` | 12 months from release |

### Image Digests

| Image | Reference | Digest |
|-------|-----------|--------|
| Alert Normalizer | `hnb/alert-normalizer:1.0.0` | `sha256:d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4` |
| Notification Dispatcher | `hnb/notification-dispatcher:1.0.0` | `sha256:e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5` |
| Email Worker | `hnb/email-worker:1.0.0` | `sha256:f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6` |
| Webhook Worker | `hnb/webhook-worker:1.0.0` | `sha256:a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7` |
| Portal Alert Center | `hnb/portal-alert-center:1.0.0` | `sha256:b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7b8` |

> Digest values above are placeholders; lock actual digest on first certified build.

### Template Schema

Notification templates use a versioned Go `text/template`-compatible format with restricted field access:

```
Template fields (restricted to these only):
  {{.Alert.Severity}}        — critical, warning, info
  {{.Alert.Summary}}         — short description (max 2048 chars)
  {{.Alert.ResourceRef}}     — masked resource reference
  {{.Alert.FirstSeenAt}}     — ISO 8601 timestamp
  {{.Alert.OccurrenceCount}} — integer count
  {{.Tenant.Name}}           — tenant display name
  {{.PortalLink}}            — time-limited, tenant-scoped URL
  {{.ChannelType}}           — email, webhook, sms
```

Templates SHALL NOT access `.Secret`, `.Token`, `.Kubeconfig`, `.Password`, or any field outside the restricted set. Template validation is enforced at save time.

### Compatibility Matrix

| Component | Alert Normalizer | Notification Dispatcher | Email Worker | Webhook Worker | Portal Alert Center |
|-----------|:---:|:---:|:---:|:---:|:---:|
| Alert Normalizer 1.0.x | — | 1.0.x | 1.0.x | 1.0.x | 1.0.x |
| Notification Dispatcher 1.0.x | 1.0.x | — | 1.0.x | 1.0.x | 1.0.x |
| Email Worker 1.0.x | 1.0.x | 1.0.x | — | 1.0.x | 1.0.x |
| Webhook Worker 1.0.x | 1.0.x | 1.0.x | 1.0.x | — | 1.0.x |
| Portal Alert Center 1.0.x | 1.0.x | 1.0.x | 1.0.x | 1.0.x | — |

All T1 components share the same major version and are upgraded as a unit. SMS Provider (T2) has independent versioning.

### Ports

| Port | Protocol | Purpose | Locked |
|------|----------|---------|--------|
| 25/587 | TCP | SMTP outbound (Email Worker) | Yes (internal only) |
| 443 | TCP | HTTPS outbound (Webhook Worker) | Yes |
| 8080 | TCP | Alert API (Platform API) | Yes |
| 9090 | TCP | Metrics endpoint | Yes (internal only) |

### Resource Budget (per worker instance)

| Component | CPU | Memory | Storage |
|-----------|-----|--------|---------|
| Normalizer | 0.5 core | 256 MiB | — |
| Notification Dispatcher | 0.5 core | 256 MiB | — |
| Email Worker | 0.5 core | 512 MiB | — |
| Webhook Worker | 0.5 core | 256 MiB | — |
| Portal Alert Center | 1 core | 512 MiB | — |
| SMS Provider (T2) | 0.5 core | 512 MiB | —