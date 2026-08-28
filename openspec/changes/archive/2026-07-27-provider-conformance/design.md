## Context

HNB 平台已有 10+ Provider，但 Provider 注册仅验证基本字段，缺乏 Manifest 完整性校验、标准化生命周期契约测试、版本兼容性管理和认证状态追踪。Provider 升级可能引入不兼容行为，影响生产环境稳定性。

现有 Provider Registry 位于 `pkg/core/registry`，支持 Provider 的 CRUD 和健康检查。Conformance 框架将在此基础上扩展 Manifest 校验和认证状态管理。

## Goals / Non-Goals

**Goals:**
- Provider Manifest 注册时强制校验完整性（名称、版本、协议、Capability、动作、权限、资源需求、依赖、兼容范围）
- Conformance 测试 CLI 工具，支持契约测试、功能测试、故障测试、安全测试和性能基线
- 版本兼容矩阵：HNB Core ↔ Provider ↔ RuntimeTarget 版本关系，阻止不兼容组合
- 认证状态生命周期：注册时指定 `conformance_level`，支持升级认证和过期降级

**Non-Goals:**
- 自动运行 Conformance 测试（CI 集成由外部触发）
- Provider 运行时隔离（PROV-002 由部署架构保证，非此 change 范围）
- 修改 Portal 显示认证状态（由 portal-experience 负责）

## Decisions

### Decision 1: Conformance 测试为独立 CLI，非 Platform API 内嵌
Conformance 测试需要模拟完整的 Provider 调用链，包括 NATS 消息和数据库操作，不适合内嵌在 API 服务中。独立 CLI 工具 `cmd/provider-conformance` 可离线运行，输出 JSON 结果报告。

### Decision 2: Manifest 校验在 Platform API 注册流程中执行
Provider 注册时直接校验 Manifest 完整性，拒绝不完整 Manifest。校验逻辑在 `pkg/core/registry` 中实现，可被 API 和 CLI 复用。

### Decision 3: 兼容矩阵使用 PostgreSQL 表存储，不硬编码
兼容矩阵关系存储在 `provider_compatibility_matrix` 表中，支持动态查询和更新。矩阵不应写死在业务代码中（PROV-005）。

## Data Model

### provider_manifests 表
```sql
CREATE TABLE provider_manifests (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id     TEXT NOT NULL REFERENCES runtime_providers(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    version         TEXT NOT NULL,
    protocol_version TEXT NOT NULL,
    capabilities    TEXT[] NOT NULL,
    actions         TEXT[] NOT NULL,
    permissions     JSONB DEFAULT '{}',
    resource_requirements JSONB DEFAULT '{}',
    dependencies    JSONB DEFAULT '[]',
    compatibility   JSONB DEFAULT '{}',
    conformance_level TEXT NOT NULL DEFAULT 'none',
    conformance_expires_at TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### provider_compatibility_matrix 表
```sql
CREATE TABLE provider_compatibility_matrix (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    core_version        TEXT NOT NULL,
    provider_id         TEXT NOT NULL,
    provider_version    TEXT NOT NULL,
    runtime_target_type TEXT NOT NULL,
    compatible          BOOLEAN NOT NULL DEFAULT true,
    constraint_reason   TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

## Risks / Trade-offs

| 风险 | 缓解措施 |
|------|---------|
| Conformance 测试覆盖不全 | 提供可扩展的测试框架，允许 Provider 开发者添加自定义测试 |
| 兼容矩阵维护成本 | 矩阵数据可通过 CI 自动更新，平台提供查询 API |
| 认证过期导致 Provider 不可用 | 提供过期前告警通知，给予宽限期 |