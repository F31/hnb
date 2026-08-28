## Overview

Create a clear Provider lifecycle controller boundary. platform-api exposes catalog metadata and compatibility queries; extension-controller owns Bundle install, deployment, enablement, upgrade, rollback, health reconciliation, capability registration and lifecycle events.

## Components

- `extension-controller`: reconciles Provider lifecycle requests and installed Bundle state.
- Provider registry: stores manifest, version, digest, phase, health and conformance evidence.
- Capability registry: records capabilities exposed by active Providers.
- Navigation metadata registry: records raw menu/route/plugin metadata consumed by apiserver Navigation Service.
- Operation worker: executes approved high-risk lifecycle steps and reports status.

## Lifecycle Flow

1. User/API requests Provider lifecycle action through apiserver or internal API.
2. Request is validated against manifest, compatibility matrix, signatures, permissions and policy.
3. An Operation is created or correlated for install/upgrade/rollback/uninstall.
4. extension-controller reconciles desired state and emits lifecycle events.
5. On success, capability and navigation metadata snapshots are updated atomically.
6. apiserver consumes events or version changes to invalidate navigation/capability caches.

## Safety

- Bundle artifacts are referenced by immutable digest.
- Secret values are never embedded in lifecycle events; only SecretReferences are allowed.
- Upgrade creates a new inactive candidate, runs health/conformance checks, then promotes.
- Rollback restores previous active version and capability snapshot.
- Uninstall is refused while active Operations, capabilities, runtime references or protected resources still depend on the Provider.

## Compatibility

- Existing platform-api manifest endpoints remain catalog/query APIs.
- Lifecycle commands use versioned contracts and idempotency keys.
- Conformance evidence remains bound to Provider ID, version and runtime target compatibility.

## Non-Goals

- platform-api does not execute Bundle deployment.
- apiserver does not manage Provider lifecycle internals.
- Controller does not proxy artifact bytes.
