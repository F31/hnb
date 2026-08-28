package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/F31/hnb/cmd/platform-api/internal/store"
	"github.com/F31/hnb/pkg/iam"
)

type ClusterService interface {
	Register(ctx context.Context, input *CreateClusterInput) (*Cluster, error)
	Get(ctx context.Context, id string) (*Cluster, error)
	List(ctx context.Context) ([]*Cluster, error)
	Update(ctx context.Context, id string, input *UpdateClusterInput) (*Cluster, error)
	Delete(ctx context.Context, id string) error
	Heartbeat(ctx context.Context, id string) error
}

type CreateClusterInput struct {
	Name          string
	ClusterType   string
	APIEndpoint   string
	KubeconfigRef string
	Region        string
	Zone          string
	Labels        map[string]string
}

type UpdateClusterInput struct {
	Region *string
	Zone   *string
	Labels *map[string]string
	Status *string
}

type Cluster struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	TenantID      string            `json:"tenantID"`
	ClusterType   string            `json:"clusterType"`
	APIEndpoint   string            `json:"apiEndpoint"`
	KubeconfigRef string            `json:"kubeconfigRef,omitempty"`
	Region        string            `json:"region,omitempty"`
	Zone          string            `json:"zone,omitempty"`
	Labels        map[string]string `json:"labels,omitempty"`
	Status        string            `json:"status"`
	LastHeartbeat *string           `json:"lastHeartbeat,omitempty"`
	CreatedAt     string            `json:"createdAt"`
	UpdatedAt     string            `json:"updatedAt"`
	Version       int64             `json:"version"`
	Scope         string            `json:"scope"`
}

func NewClusterService(st store.ClusterRepository) ClusterService {
	return &clusterService{store: st}
}

type clusterService struct {
	store store.ClusterRepository
}

func (s *clusterService) Register(ctx context.Context, input *CreateClusterInput) (*Cluster, error) {
	trusted, ok := iam.TrustedContextFrom(ctx)
	if !ok {
		return nil, errors.New("no trusted context in request")
	}
	cluster, err := s.store.CreateCluster(ctx, store.CreateClusterRequest{
		Name:          input.Name,
		TenantID:      trusted.TenantID,
		ClusterType:   input.ClusterType,
		APIEndpoint:   input.APIEndpoint,
		KubeconfigRef: input.KubeconfigRef,
		Region:        input.Region,
		Zone:          input.Zone,
		Labels:        input.Labels,
	})
	if err != nil {
		return nil, fmt.Errorf("register cluster: %w", err)
	}
	return toClusterResponse(cluster), nil
}

func (s *clusterService) Get(ctx context.Context, id string) (*Cluster, error) {
	trusted, ok := iam.TrustedContextFrom(ctx)
	if !ok {
		return nil, errors.New("no trusted context in request")
	}
	c, err := s.store.GetCluster(ctx, id, trusted.TenantID)
	if err != nil {
		return nil, fmt.Errorf("get cluster: %w", err)
	}
	return toClusterResponse(c), nil
}

func (s *clusterService) List(ctx context.Context) ([]*Cluster, error) {
	trusted, ok := iam.TrustedContextFrom(ctx)
	if !ok {
		return nil, errors.New("no trusted context in request")
	}
	clusters, err := s.store.ListClusters(ctx, trusted.TenantID)
	if err != nil {
		return nil, fmt.Errorf("list clusters: %w", err)
	}
	result := make([]*Cluster, 0, len(clusters))
	for _, c := range clusters {
		result = append(result, toClusterResponse(c))
	}
	return result, nil
}

func (s *clusterService) Update(ctx context.Context, id string, input *UpdateClusterInput) (*Cluster, error) {
	trusted, ok := iam.TrustedContextFrom(ctx)
	if !ok {
		return nil, errors.New("no trusted context in request")
	}
	c, err := s.store.UpdateCluster(ctx, id, trusted.TenantID, store.UpdateClusterRequest{
		Region: input.Region,
		Zone:   input.Zone,
		Labels: input.Labels,
		Status: input.Status,
	})
	if err != nil {
		return nil, fmt.Errorf("update cluster: %w", err)
	}
	return toClusterResponse(c), nil
}

func (s *clusterService) Delete(ctx context.Context, id string) error {
	trusted, ok := iam.TrustedContextFrom(ctx)
	if !ok {
		return errors.New("no trusted context in request")
	}
	return s.store.DeleteCluster(ctx, id, trusted.TenantID)
}

func (s *clusterService) Heartbeat(ctx context.Context, id string) error {
	trusted, ok := iam.TrustedContextFrom(ctx)
	if !ok {
		return errors.New("no trusted context in request")
	}
	return s.store.HeartbeatCluster(ctx, id, trusted.TenantID)
}

func toClusterResponse(c *store.Cluster) *Cluster {
	cluster := &Cluster{
		ID:            c.ID,
		Name:          c.Name,
		TenantID:      c.TenantID,
		ClusterType:   c.ClusterType,
		APIEndpoint:   c.APIEndpoint,
		KubeconfigRef: c.KubeconfigRef,
		Region:        c.Region,
		Zone:          c.Zone,
		Labels:        c.Labels,
		Status:        c.Status,
		CreatedAt:     c.CreatedAt.Format(time.RFC3339),
		UpdatedAt:     c.UpdatedAt.Format(time.RFC3339),
		Version:       c.Version,
		Scope:         c.Scope,
	}
	if c.LastHeartbeat != nil {
		lat := c.LastHeartbeat.Format(time.RFC3339)
		cluster.LastHeartbeat = &lat
	}
	return cluster
}
