## Overview

Harden service boundaries without merging responsibilities. apiserver becomes the northbound BFF and protocol adapter; platform-api remains the platform resource/intent domain authority. Both services share IAM semantics and a common error/trace envelope.

## Boundaries

- apiserver responsibilities: authentication entry, tenant/space context resolution, coarse route authorization, Console DTO aggregation, navigation, proxy/tunnel, SSE/WebSocket and API gateway concerns.
- platform-api responsibilities: Cluster/RuntimeTarget persistence, RuntimeIntent validation, ExecutionPlan generation, Operation records, Provider catalog queries and domain authorization.
- Synchronous dependency is apiserver -> platform-api only.
- platform-api publishes state changes through Outbox/NATS; it does not call apiserver to update views.

## Implementation Approach

- Introduce apiserver application services and internal platform-api clients for platform resources.
- Remove direct SQL from apiserver handlers for resources owned by platform-api.
- Use `PLATFORM_API_URL` or service discovery in deployment config.
- Standardize `X-Trace-Id` as canonical while mirroring `X-Correlation-ID` during transition.
- Standardize error responses with `code`, `message`, `traceId`/`requestId` and optional `details`; retain compatibility wrappers where currently used.

## Authorization

- apiserver validates identity and northbound route permissions.
- platform-api independently evaluates resource instance authorization and state transitions.
- Shared `pkg/iam` constants define allowed actions and resource names to prevent drift.

## Compatibility

- Existing apiserver public paths stay stable.
- platform-api internal paths stay stable for internal clients.
- Error responses include new common fields without removing old fields in the first release.

## Non-Goals

- No full Event Sourcing migration.
- No platform-api -> apiserver synchronous callbacks.
- No browser direct platform-api access.
