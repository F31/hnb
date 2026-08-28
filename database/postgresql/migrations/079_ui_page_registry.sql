-- Migration: 079_ui_page_registry
-- Description: Database-backed UI Registry for PageSchema: active revision per page + immutable revision history (UI 规范 V2.6 §2.2 / §20).
-- Tiers: T1
-- Dependencies: 039_console_ui_registry

BEGIN;

-- 每个页面当前的激活 revision（发布/回滚只切换 active_revision，不覆盖历史）
CREATE TABLE IF NOT EXISTS ui_pages (
    page_id TEXT PRIMARY KEY,
    plugin_id TEXT NOT NULL DEFAULT 'shell',
    min_shell_version TEXT NOT NULL DEFAULT '2.5.0',
    active_revision INTEGER NOT NULL DEFAULT 0 CHECK (active_revision >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 不可变 revision 历史：payload 为完整 PageSchema 信封（不含可执行代码）
CREATE TABLE IF NOT EXISTS ui_page_versions (
    page_id TEXT NOT NULL REFERENCES ui_pages(page_id) ON DELETE CASCADE,
    revision INTEGER NOT NULL CHECK (revision >= 1),
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by TEXT,
    PRIMARY KEY (page_id, revision)
);

CREATE INDEX IF NOT EXISTS idx_ui_page_versions_created
    ON ui_page_versions(page_id, created_at DESC);

-- 种子数据：承接原静态 fixture 的 canonical cluster-list 页面
INSERT INTO ui_pages (page_id, plugin_id, min_shell_version, active_revision)
VALUES ('cluster-list', 'shell', '2.5.0', 1)
ON CONFLICT (page_id) DO NOTHING;

INSERT INTO ui_page_versions (page_id, revision, payload, created_by)
SELECT 'cluster-list', 1, jsonb_build_object(
    'apiVersion', 'ui.hnb.io/v1',
    'kind', 'PageSchema',
    'metadata', jsonb_build_object(
        'id', 'cluster-list',
        'revision', 1,
        'minShellVersion', '2.5.0',
        'texts', jsonb_build_object(
            'cluster.title', 'Clusters',
            'cluster.description', 'Trusted schema runtime fixture',
            'cluster.refresh', 'Refresh'
        )
    ),
    'spec', jsonb_build_object(
        'template', 'list',
        'titleKey', 'cluster.title',
        'descriptionKey', 'cluster.description',
        'layout', jsonb_build_object('type', 'grid', 'columns', 12, 'gap', 'md'),
        'endpoints', jsonb_build_array(
            jsonb_build_object('id', 'clusters.list', 'path', '/api/v1/clusters', 'method', 'GET')
        ),
        'dataSources', jsonb_build_array(
            jsonb_build_object(
                'id', 'clusters',
                'type', 'paginatedQuery',
                'endpointId', 'clusters.list',
                'queryBindings', jsonb_build_array('status', 'provider'),
                'responseMapping', jsonb_build_object('items', 'data.items', 'total', 'data.total')
            )
        ),
        'actions', jsonb_build_array(
            jsonb_build_object(
                'id', 'refresh',
                'type', 'api',
                'labelKey', 'cluster.refresh',
                'permission', 'schema:read',
                'request', jsonb_build_object('method', 'GET', 'endpointId', 'clusters.list')
            )
        ),
        'regions', jsonb_build_array(
            jsonb_build_object(
                'id', 'cluster-table',
                'componentType', 'DataTable',
                'span', 12,
                'props', jsonb_build_object(
                    'dataSource', 'clusters',
                    'actions', jsonb_build_array('refresh'),
                    'columns', jsonb_build_array(
                        jsonb_build_object('key', 'id', 'title', 'ID'),
                        jsonb_build_object('key', 'name', 'title', 'Name')
                    )
                ),
                'condition', jsonb_build_object(
                    'all', jsonb_build_array(jsonb_build_object('permission', 'schema:read'))
                )
            )
        )
    )
),
'system'
ON CONFLICT (page_id, revision) DO NOTHING;

COMMIT;
