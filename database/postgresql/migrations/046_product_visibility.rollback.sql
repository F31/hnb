-- Rollback: 046_product_visibility
DROP INDEX IF EXISTS idx_products_visibility;
ALTER TABLE products DROP COLUMN IF EXISTS visibility_changed_by;
ALTER TABLE products DROP COLUMN IF EXISTS visibility_changed_at;
ALTER TABLE products DROP COLUMN IF EXISTS visibility;