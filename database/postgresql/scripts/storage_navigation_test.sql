DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM console_routes
        WHERE route_name = 'container.instances.storage'
          AND path = '/container/storage'
          AND redirect_to IS NULL
          AND capability = ''
    ) THEN
        RAISE EXCEPTION 'canonical Container storage consumption route is unavailable or gated';
    END IF;
    IF NOT EXISTS (
        SELECT 1
        FROM console_routes legacy
        JOIN console_routes canonical ON canonical.route_name = 'container.instances.storage'
        WHERE legacy.route_name = 'container.instances.storage.legacy'
          AND legacy.path = '/container/instances/storage'
          AND legacy.redirect_to = canonical.path
          AND legacy.permission = canonical.permission
          AND legacy.capability = canonical.capability
    ) THEN
        RAISE EXCEPTION 'legacy storage redirect does not inherit canonical route authorization';
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM console_routes
        WHERE route_name = 'resource.storage' AND capability = 'storage.supply'
    ) OR EXISTS (
        SELECT 1 FROM console_routes
        WHERE route_name = 'container.instances.storage' AND capability = 'storage.supply'
    ) THEN
        RAISE EXCEPTION 'storage supply capability leaked into Container consumption';
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM console_navigation_items
        WHERE item_key = 'nav.resource.storage' AND capability = 'storage.supply'
    ) THEN
        RAISE EXCEPTION 'storage supply navigation is not capability gated';
    END IF;
END
$$;
