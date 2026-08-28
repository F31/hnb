-- Migration: 011_app_market_engine
-- Description: Add App Market tables: publishers, products, packages, artifacts, releases, channels, entitlements, subscriptions
-- Tiers: All
-- Dependencies: 005_identity_core (tenants)

-- 1. Publishers
CREATE TABLE IF NOT EXISTS publishers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id TEXT NOT NULL,
    name TEXT NOT NULL,
    display_name TEXT NOT NULL,
    description TEXT,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended', 'decommissioned')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_publishers_tenant_name ON publishers(tenant_id, name);

-- 2. Products
CREATE TABLE IF NOT EXISTS products (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    publisher_id UUID NOT NULL REFERENCES publishers(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    display_name TEXT NOT NULL,
    description TEXT,
    category TEXT NOT NULL CHECK (category IN (
        'application', 'database', 'middleware', 'ai', 'edge', 'tool', 'other'
    )),
    labels JSONB DEFAULT '{}',
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'archived')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_products_publisher_id ON products(publisher_id);
CREATE INDEX IF NOT EXISTS idx_products_category ON products(category);
CREATE UNIQUE INDEX IF NOT EXISTS idx_products_publisher_name ON products(publisher_id, name);

-- 3. Packages
CREATE TABLE IF NOT EXISTS packages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    package_type TEXT NOT NULL CHECK (package_type IN (
        'helm', 'container', 'oci_artifact', 'terraform', 'compose', 'custom'
    )),
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_packages_product_id ON packages(product_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_packages_product_name ON packages(product_id, name);

-- 4. Artifacts (concrete OCI/container images, Helm charts, etc.)
CREATE TABLE IF NOT EXISTS artifacts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    package_id UUID NOT NULL REFERENCES packages(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    artifact_type TEXT NOT NULL CHECK (artifact_type IN (
        'oci_image', 'helm_chart', 'container_image', 'terraform_module', 'generic'
    )),
    registry_url TEXT,
    digest TEXT NOT NULL,
    size_bytes BIGINT,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_artifacts_package_id ON artifacts(package_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_artifacts_digest ON artifacts(digest);

-- 5. Releases (immutable, versioned)
CREATE TABLE IF NOT EXISTS releases (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    version TEXT NOT NULL,
    release_notes TEXT,
    manifest JSONB NOT NULL DEFAULT '{}',
    manifest_digest TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN (
        'draft', 'published', 'superseded', 'withdrawn'
    )),
    created_by TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_releases_product_id ON releases(product_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_releases_product_version ON releases(product_id, version);
CREATE UNIQUE INDEX IF NOT EXISTS idx_releases_manifest_digest ON releases(manifest_digest);

-- 6. Channels (promotion pipeline)
CREATE TABLE IF NOT EXISTS channels (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    channel_type TEXT NOT NULL CHECK (channel_type IN (
        'dev', 'staging', 'stable', 'deprecated', 'withdrawn'
    )),
    release_id UUID REFERENCES releases(id) ON DELETE SET NULL,
    promotion_order INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_channels_product_name ON channels(product_id, name);
CREATE INDEX IF NOT EXISTS idx_channels_release_id ON channels(release_id);

-- 7. Entitlements (product access control)
CREATE TABLE IF NOT EXISTS entitlements (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    tenant_id TEXT NOT NULL,
    entitlement_type TEXT NOT NULL CHECK (entitlement_type IN (
        'evaluate', 'standard', 'premium', 'enterprise'
    )),
    max_deployments INTEGER,
    allowed_environments TEXT[] DEFAULT '{}',
    expires_at TIMESTAMPTZ,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_entitlements_product_tenant ON entitlements(product_id, tenant_id);
CREATE INDEX IF NOT EXISTS idx_entitlements_tenant_id ON entitlements(tenant_id);

-- 8. Subscriptions (tenant subscribes to product)
CREATE TABLE IF NOT EXISTS subscriptions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id TEXT NOT NULL,
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    entitlement_id UUID NOT NULL REFERENCES entitlements(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'expired', 'cancelled')),
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_subscriptions_tenant_id ON subscriptions(tenant_id);
CREATE INDEX IF NOT EXISTS idx_subscriptions_product_id ON subscriptions(product_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_subscriptions_tenant_product ON subscriptions(tenant_id, product_id);
