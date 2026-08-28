-- Migration: 046_product_visibility.sql
-- Description: Add visibility (public/tenant) to products for cross-tenant sharing.
-- Tiers: All
-- Dependencies: 011_app_market_engine

ALTER TABLE products
    ADD COLUMN IF NOT EXISTS visibility TEXT NOT NULL DEFAULT 'tenant'
    CHECK (visibility IN ('public', 'tenant'));

ALTER TABLE products
    ADD COLUMN IF NOT EXISTS visibility_changed_at TIMESTAMP WITH TIME ZONE;

ALTER TABLE products
    ADD COLUMN IF NOT EXISTS visibility_changed_by TEXT;

CREATE INDEX IF NOT EXISTS idx_products_visibility
    ON products(visibility)
    WHERE visibility = 'public';