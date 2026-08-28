-- Migration: 049_console_routes_clear_permissions
-- Description: Clear permission field for local plugin routes in console_routes.
--              Local plugins (system, dashboard, etc.) handle their own route permissions
--              via the PluginLoader; the navigation response should not gate them.
-- Tiers: All
-- Dependencies: 039_console_ui_registry

UPDATE console_routes SET permission = '', updated_at = now()
WHERE plugin_id IN ('system', 'dashboard', 'application', 'container', 'resource', 'service', 'ai')
  AND permission != '';