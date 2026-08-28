package store

import "context"

func (s *PGStore) CreateCluster(ctx context.Context, req CreateClusterRequest) (*Cluster, error) {
	return s.clusterStore.CreateCluster(ctx, req)
}

func (s *PGStore) GetCluster(ctx context.Context, id, tenantID string) (*Cluster, error) {
	return s.clusterStore.GetCluster(ctx, id, tenantID)
}

func (s *PGStore) ListClusters(ctx context.Context, tenantID string) ([]*Cluster, error) {
	return s.clusterStore.ListClusters(ctx, tenantID)
}

func (s *PGStore) DeleteCluster(ctx context.Context, id, tenantID string) error {
	return s.clusterStore.DeleteCluster(ctx, id, tenantID)
}

func (s *PGStore) HeartbeatCluster(ctx context.Context, id, tenantID string) error {
	return s.clusterStore.HeartbeatCluster(ctx, id, tenantID)
}

func (s *PGStore) UpdateCluster(ctx context.Context, id, tenantID string, req UpdateClusterRequest) (*Cluster, error) {
	return s.clusterStore.UpdateCluster(ctx, id, tenantID, req)
}
