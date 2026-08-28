DO $$
DECLARE
    route_count INTEGER;
BEGIN
    SELECT count(*) INTO route_count
    FROM console_routes
    WHERE path LIKE '/resource/clusters/:clusterId%'
      AND enabled = true;

    IF route_count <> 14 THEN
        RAISE EXCEPTION 'expected 14 enabled Resource cluster detail routes, got %', route_count;
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM console_routes
        WHERE route_name = 'resource.clusters.detail'
          AND path = '/resource/clusters/:clusterId'
          AND component_key = 'ClusterDetailRedirect'
          AND permission = 'cluster:read'
    ) OR NOT EXISTS (
        SELECT 1 FROM console_routes
        WHERE route_name = 'resource.clusters.detail.overview'
          AND path = '/resource/clusters/:clusterId/overview'
          AND component_key = 'ClusterOverviewPage'
          AND permission = 'cluster:read'
    ) THEN
        RAISE EXCEPTION 'Resource cluster root or overview route is not registered correctly';
    END IF;
END
$$;
