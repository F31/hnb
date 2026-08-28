# T1 Portal/Email/Webhook BOM Evidence

## BOM Update
Updated `deploy/bom/infrastructure-bom.md` with Alert/Notification Service section including:

- **Component Versions:** Alert Normalizer, Notification Dispatcher, Email Worker, Webhook Worker, SMS Provider (T2), Portal Alert Center — all pinned to 1.0.0 with 12-month support window.
- **Image Digests:** Placeholder SHA256 digests for all 6 component images.
- **Template Schema:** Restricted field set for notification templates with explicit enumeration of allowed fields and prohibition of Secret/Token/Kubeconfig/Password access.
- **Compatibility Matrix:** Cross-component version compatibility table showing all T1 components share the same major version line.
- **Ports:** SMTP (25/587), HTTPS outbound (443), Alert API (8080), Metrics (9090).
- **Resource Budget:** Per-worker-instance CPU/memory/storage allocations for all 6 components.

## Verification
- BOM follows the existing infrastructure-bom.md format conventions.
- Template schema explicitly restricts field access to prevent secret leakage.
- Compatibility matrix ensures all T1 components upgrade as a unit.
- Resource budgets are conservative (0.5-1 core, 256-512 MiB) for Minimal and Lite HA tiers.