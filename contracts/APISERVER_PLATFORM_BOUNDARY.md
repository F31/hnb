# Apiserver And Platform API Boundary

Web Console and ordinary user CLI traffic enters through apiserver. apiserver owns authentication entry, tenant/space context, Console DTO aggregation, navigation, proxy/tunnel and realtime northbound protocols.

platform-api owns platform resource domain state: Cluster, RuntimeTarget, RuntimeIntent, ExecutionPlan, Provider catalog and Operation records. platform-api independently authorizes resource instances even when calls arrive through apiserver.

Synchronous dependency is one-way: apiserver may call platform-api. platform-api must not synchronously call apiserver to complete domain logic. State changes flow through PostgreSQL, Outbox and NATS events.

HTTP APIs accept `X-Trace-Id` as canonical and mirror `X-Correlation-ID` during transition. Error responses expose at least `code`, `message` and a trace/request identifier; compatibility wrappers may remain during migration.
