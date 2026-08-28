-- Migration: 039_console_ui_registry
-- Description: Add database-backed Console UI/navigation registry metadata.
-- Tiers: T1
-- Dependencies: 038_provider_lifecycle_metadata

CREATE TABLE IF NOT EXISTS console_plugins (
    plugin_id TEXT PRIMARY KEY,
    version TEXT NOT NULL DEFAULT '1.0.0',
    display_name TEXT NOT NULL,
    tier TEXT NOT NULL CHECK (tier IN ('T0', 'T1', 'T2', 'T3')),
    mode TEXT NOT NULL DEFAULT 'local' CHECK (mode IN ('local', 'remote')),
    enabled BOOLEAN NOT NULL DEFAULT true,
    sort_order INTEGER NOT NULL DEFAULT 0,
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS console_routes (
    route_name TEXT PRIMARY KEY,
    path TEXT NOT NULL UNIQUE,
    plugin_id TEXT NOT NULL REFERENCES console_plugins(plugin_id) ON DELETE CASCADE,
    component_key TEXT,
    schema_id TEXT,
    permission TEXT NOT NULL DEFAULT '',
    capability TEXT NOT NULL DEFAULT '',
    enabled BOOLEAN NOT NULL DEFAULT true,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (component_key IS NOT NULL OR schema_id IS NOT NULL)
);

CREATE TABLE IF NOT EXISTS console_navigation_items (
    item_key TEXT NOT NULL,
    parent_key TEXT,
    title TEXT NOT NULL,
    icon TEXT NOT NULL DEFAULT '',
    route_name TEXT REFERENCES console_routes(route_name) ON DELETE SET NULL,
    permission TEXT NOT NULL DEFAULT '',
    capability TEXT NOT NULL DEFAULT '',
    locale TEXT NOT NULL DEFAULT 'zh-CN',
    level INTEGER NOT NULL CHECK (level BETWEEN 1 AND 3),
    sort_order INTEGER NOT NULL DEFAULT 0,
    enabled BOOLEAN NOT NULL DEFAULT true,
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (item_key, locale)
);

CREATE INDEX IF NOT EXISTS idx_console_navigation_parent_order
    ON console_navigation_items(locale, parent_key, sort_order, item_key);
CREATE INDEX IF NOT EXISTS idx_console_navigation_locale
    ON console_navigation_items(locale);
CREATE INDEX IF NOT EXISTS idx_console_routes_plugin
    ON console_routes(plugin_id, sort_order);

CREATE TABLE IF NOT EXISTS console_navigation_versions (
    version_key TEXT PRIMARY KEY,
    version_value TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO console_plugins (plugin_id, version, display_name, tier, mode, enabled, sort_order) VALUES
    ('dashboard', '1.0.0', '首页', 'T0', 'local', true, 10),
    ('application', '1.0.0', '应用工厂', 'T1', 'local', true, 20),
    ('container', '1.0.0', '容器', 'T1', 'local', true, 30),
    ('resource', '1.0.0', '资源', 'T1', 'local', true, 40),
    ('service', '1.0.0', '云原生服务', 'T1', 'local', true, 50),
    ('ai', '1.0.0', 'AI', 'T2', 'local', true, 60),
    ('system', '1.0.0', '系统', 'T1', 'local', true, 70)
ON CONFLICT (plugin_id) DO UPDATE SET
    version = EXCLUDED.version,
    display_name = EXCLUDED.display_name,
    tier = EXCLUDED.tier,
    mode = EXCLUDED.mode,
    enabled = EXCLUDED.enabled,
    sort_order = EXCLUDED.sort_order,
    updated_at = now();

INSERT INTO console_routes (route_name, path, plugin_id, component_key, permission, sort_order) VALUES
    ('dashboard.overview', '/dashboard', 'dashboard', 'Dashboard', '', 10),
    ('dashboard.approvals', '/dashboard/approvals', 'dashboard', 'ApprovalList', '', 20),
    ('dashboard.recent', '/dashboard/recent', 'dashboard', 'RecentOps', '', 30),
    ('application.monolith', '/application/monolith', 'application', 'ApplicationApps', '', 10),
    ('application.microservices', '/application/microservices', 'application', 'ApplicationApps', '', 20),
    ('application.environments', '/application/environments', 'application', 'EnvManager', '', 30),
    ('application.market', '/application/market', 'application', 'AppMarket', '', 40),
    ('application.templates', '/application/templates', 'application', 'AppTemplates', '', 50),
    ('application.observability.analysis', '/application/observability/analysis', 'application', 'AppAnalysis', '', 60),
    ('application.observability.topology', '/application/observability/topology', 'application', 'Topology', '', 70),
    ('application.observability.guard', '/application/observability/guard', 'application', 'SmartGuard', '', 80),
    ('application.observability.timeline', '/application/observability/timeline', 'application', 'TimeTravel', '', 90),
    ('container.instances.workloads', '/container/instances/workloads', 'container', 'Workloads', '', 10),
    ('container.instances.namespaces', '/container/instances/namespaces', 'container', 'Namespaces', '', 20),
    ('container.instances.storage', '/container/instances/storage', 'container', 'Storage', '', 30),
    ('container.instances.access', '/container/instances/access', 'container', 'Access', '', 40),
    ('container.instances.config', '/container/instances/config', 'container', 'Config', '', 50),
    ('container.instances.logs', '/container/instances/logs', 'container', 'Logs', '', 60),
    ('container.instances.events', '/container/instances/events', 'container', 'Events', '', 70),
    ('container.security.overview', '/container/security/overview', 'container', 'SecurityOverview', '', 80),
    ('container.security.protection', '/container/security/protection', 'container', 'SecurityProtection', '', 90),
    ('container.security.report', '/container/security/report', 'container', 'SecurityReport', '', 100),
    ('container.security.config', '/container/security/config', 'container', 'SecurityConfig', '', 110),
    ('resource.clusters', '/resource/clusters', 'resource', 'ClusterList', '', 10),
    ('resource.nodes', '/resource/nodes', 'resource', 'NodeList', '', 20),
    ('resource.gpu', '/resource/gpu', 'resource', 'GPUResources', '', 30),
    ('resource.network', '/resource/network', 'resource', 'Network', '', 40),
    ('resource.storage', '/resource/storage', 'resource', 'Storage', '', 50),
    ('resource.gslb', '/resource/gslb', 'resource', 'GSLB', '', 60),
    ('service.data', '/service/data', 'service', 'DataService', '', 10),
    ('service.messaging', '/service/messaging', 'service', 'MessageService', '', 20),
    ('service.governance', '/service/governance', 'service', 'Governance', '', 30),
    ('service.gateway', '/service/gateway', 'service', 'Gateway', '', 40),
    ('ai.models', '/ai/models', 'ai', 'ModelRegistry', '', 10),
    ('ai.inference', '/ai/inference', 'ai', 'Inference', '', 20),
    ('ai.gateway', '/ai/gateway', 'ai', 'AIGateway', '', 30),
    ('system.settings', '/system/settings', 'system', 'Settings', '', 10),
    ('system.users', '/system/users', 'system', 'UserList', '', 20),
    ('system.roles', '/system/roles', 'system', 'RoleList', '', 30),
    ('system.tenants', '/system/tenants', 'system', 'TenantList', '', 40),
    ('system.approvals', '/system/approvals', 'system', 'OperationApproval', '', 50),
    ('system.audit', '/system/audit', 'system', 'AuditLog', '', 60),
    ('system.extensions', '/system/extensions', 'system', 'ExtensionList', '', 70)
ON CONFLICT (route_name) DO UPDATE SET
    path = CASE
        WHEN console_routes.route_name = 'container.instances.storage'
          AND console_routes.path = '/container/storage'
        THEN console_routes.path
        ELSE EXCLUDED.path
    END,
    plugin_id = EXCLUDED.plugin_id,
    component_key = EXCLUDED.component_key,
    permission = EXCLUDED.permission,
    sort_order = EXCLUDED.sort_order,
    enabled = true,
    updated_at = now();

UPDATE console_routes SET enabled = false, updated_at = now()
WHERE route_name = 'application.apps';

WITH seed_nav(item_key, parent_key, zh_title, en_title, icon, route_name, level, sort_order) AS (VALUES
    ('nav.home', NULL, '首页', 'Home', 'dashboard', 'dashboard.overview', 1, 10),
    ('nav.application', NULL, '应用', 'Applications', 'app', NULL, 1, 20),
    ('nav.container', NULL, '容器', 'Containers', 'container', NULL, 1, 30),
    ('nav.resource', NULL, '资源', 'Resources', 'resource', NULL, 1, 40),
    ('nav.service', NULL, '服务', 'Services', 'service', NULL, 1, 50),
    ('nav.ai', NULL, 'AI', 'AI', 'ai', NULL, 1, 60),
    ('nav.system', NULL, '系统', 'System', 'system', NULL, 1, 70),
    ('nav.application.monolith', 'nav.application', '单体应用', 'Monolith Apps', 'app', 'application.monolith', 2, 10),
    ('nav.application.microservices', 'nav.application', '微服务应用', 'Microservice Apps', 'mesh', 'application.microservices', 2, 20),
    ('nav.application.environments', 'nav.application', '环境管理', 'Environments', 'env', 'application.environments', 2, 30),
    ('nav.application.market', 'nav.application', '应用市场', 'Marketplace', 'market', 'application.market', 2, 40),
    ('nav.application.templates', 'nav.application', '应用模板', 'Templates', 'template', 'application.templates', 2, 50),
    ('nav.application.observability', 'nav.application', '可观测', 'Observability', 'eye', NULL, 2, 60),
    ('nav.application.observability.analysis', 'nav.application.observability', '应用分析', 'Application Analysis', 'analysis', 'application.observability.analysis', 3, 10),
    ('nav.application.observability.topology', 'nav.application.observability', '拓扑', 'Topology', 'topology', 'application.observability.topology', 3, 20),
    ('nav.application.observability.guard', 'nav.application.observability', '智能守护', 'Smart Guard', 'guard', 'application.observability.guard', 3, 30),
    ('nav.application.observability.timeline', 'nav.application.observability', '时间线', 'Timeline', 'timeline', 'application.observability.timeline', 3, 40),
    ('nav.container.instances', 'nav.container', '集群实例', 'Cluster Instances', 'cluster', NULL, 2, 10),
    ('nav.container.instances.workloads', 'nav.container.instances', '工作负载', 'Workloads', 'workload', 'container.instances.workloads', 3, 10),
    ('nav.container.instances.namespaces', 'nav.container.instances', '命名空间', 'Namespaces', 'namespace', 'container.instances.namespaces', 3, 20),
    ('nav.container.instances.storage', 'nav.container.instances', '存储', 'Storage', 'storage', 'container.instances.storage', 3, 30),
    ('nav.container.instances.access', 'nav.container.instances', '访问', 'Access', 'access', 'container.instances.access', 3, 40),
    ('nav.container.instances.config', 'nav.container.instances', '配置', 'Configuration', 'config', 'container.instances.config', 3, 50),
    ('nav.container.instances.logs', 'nav.container.instances', '日志', 'Logs', 'logs', 'container.instances.logs', 3, 60),
    ('nav.container.instances.events', 'nav.container.instances', '事件', 'Events', 'events', 'container.instances.events', 3, 70),
    ('nav.container.security', 'nav.container', '安全治理', 'Security Governance', 'security', NULL, 2, 20),
    ('nav.container.security.overview', 'nav.container.security', '安全概览', 'Security Overview', 'overview', 'container.security.overview', 3, 10),
    ('nav.container.security.protection', 'nav.container.security', '安全防护', 'Protection', 'protection', 'container.security.protection', 3, 20),
    ('nav.container.security.report', 'nav.container.security', '安全报告', 'Security Reports', 'report', 'container.security.report', 3, 30),
    ('nav.container.security.config', 'nav.container.security', '安全配置', 'Security Settings', 'config', 'container.security.config', 3, 40),
    ('nav.resource.clusters', 'nav.resource', '集群', 'Clusters', 'cluster', 'resource.clusters', 2, 10),
    ('nav.resource.nodes', 'nav.resource', '节点', 'Nodes', 'node', 'resource.nodes', 2, 20),
    ('nav.resource.gpu', 'nav.resource', 'GPU', 'GPU', 'gpu', 'resource.gpu', 2, 30),
    ('nav.resource.network', 'nav.resource', '网络', 'Network', 'network', 'resource.network', 2, 40),
    ('nav.resource.storage', 'nav.resource', '存储', 'Storage', 'storage', 'resource.storage', 2, 50),
    ('nav.resource.gslb', 'nav.resource', 'GSLB', 'GSLB', 'globe', 'resource.gslb', 2, 60),
    ('nav.service.data', 'nav.service', '数据服务', 'Data Services', 'database', 'service.data', 2, 10),
    ('nav.service.messaging', 'nav.service', '消息服务', 'Messaging', 'message', 'service.messaging', 2, 20),
    ('nav.service.governance', 'nav.service', '服务治理', 'Service Governance', 'mesh', 'service.governance', 2, 30),
    ('nav.service.gateway', 'nav.service', '网关', 'Gateway', 'gateway', 'service.gateway', 2, 40),
    ('nav.ai.models', 'nav.ai', '模型仓库', 'Model Registry', 'model', 'ai.models', 2, 10),
    ('nav.ai.inference', 'nav.ai', '推理服务', 'Inference Services', 'inference', 'ai.inference', 2, 20),
    ('nav.ai.gateway', 'nav.ai', '智能网关', 'AI Gateway', 'ai-gateway', 'ai.gateway', 2, 30),
    ('nav.system.settings', 'nav.system', '系统设置', 'Settings', 'settings', 'system.settings', 2, 10),
    ('nav.system.users', 'nav.system', '用户', 'Users', 'user', 'system.users', 2, 20),
    ('nav.system.roles', 'nav.system', '角色', 'Roles', 'role', 'system.roles', 2, 30),
    ('nav.system.tenants', 'nav.system', '租户', 'Tenants', 'tenant', 'system.tenants', 2, 40),
    ('nav.system.approvals', 'nav.system', '审批', 'Approvals', 'approval', 'system.approvals', 2, 50),
    ('nav.system.audit', 'nav.system', '审计', 'Audit Log', 'audit', 'system.audit', 2, 60),
    ('nav.system.extensions', 'nav.system', '扩展', 'Extensions', 'extension', 'system.extensions', 2, 70)
), localized_nav AS (
    SELECT item_key, parent_key, zh_title AS title, icon, route_name, 'zh-CN' AS locale, level, sort_order FROM seed_nav
    UNION ALL
    SELECT item_key, parent_key, en_title AS title, icon, route_name, 'en-US' AS locale, level, sort_order FROM seed_nav
)
INSERT INTO console_navigation_items (item_key, parent_key, title, icon, route_name, locale, level, sort_order, enabled)
SELECT item_key, parent_key, title, icon, route_name, locale, level, sort_order, true
FROM localized_nav
ON CONFLICT (item_key, locale) DO UPDATE SET
    parent_key = EXCLUDED.parent_key,
    title = EXCLUDED.title,
    icon = EXCLUDED.icon,
    route_name = EXCLUDED.route_name,
    locale = EXCLUDED.locale,
    level = EXCLUDED.level,
    sort_order = EXCLUDED.sort_order,
    enabled = EXCLUDED.enabled,
    updated_at = now();

UPDATE console_navigation_items SET enabled = false, updated_at = now()
WHERE item_key = 'nav.application.apps';

INSERT INTO console_navigation_versions (version_key, version_value) VALUES
    ('navigation', 'db-seed-v1'),
    ('pluginCatalog', 'db-seed-v1'),
    ('license', 'mvp')
ON CONFLICT (version_key) DO UPDATE SET version_value = EXCLUDED.version_value, updated_at = now();
