-- Migration: 045_recycle_bin.sql
-- Description: Soft-delete for products, releases and artifacts via a unified
-- recycle bin with configurable retention per tenant.
-- Tiers: All
-- Dependencies: 044_tenant_id_foreign_keys

CREATE TABLE IF NOT EXISTS recycle_bin_settings (
    tenant_id TEXT PRIMARY KEY REFERENCES tenants(id) ON DELETE CASCADE,
    product_retention INTERVAL NOT NULL DEFAULT '7 days',
    release_retention INTERVAL NOT NULL DEFAULT '7 days',
    artifact_retention INTERVAL NOT NULL DEFAULT '24 hours',
    enabled BOOLEAN NOT NULL DEFAULT true,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS recycle_bin_entries (
    id UUID PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    resource_type TEXT NOT NULL CHECK (resource_type IN ('product','release','artifact')),
    resource_id UUID NOT NULL,
    resource_name TEXT,
    state TEXT NOT NULL DEFAULT 'retained' CHECK (state IN ('retained','deleting','deleted','cancelled')),
    delete_after TIMESTAMPTZ NOT NULL,
    operation_id UUID,
    requested_by TEXT,
    reason TEXT,
    original_data JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_recycle_bin_entries_tenant_state
    ON recycle_bin_entries(tenant_id, state);

CREATE INDEX IF NOT EXISTS idx_recycle_bin_entries_pending
    ON recycle_bin_entries(state, delete_after)
    WHERE state = 'retained';

CREATE UNIQUE INDEX IF NOT EXISTS uq_recycle_bin_entries_active
    ON recycle_bin_entries(tenant_id, resource_type, resource_id)
    WHERE state IN ('retained', 'deleting');