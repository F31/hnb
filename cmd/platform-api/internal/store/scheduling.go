package store

// PropagationStrategy defines how resources are deployed across multiple clusters.
type PropagationStrategy string

const (
	PropagationDuplicated PropagationStrategy = "Duplicated"
	PropagationDivide     PropagationStrategy = "Divide"
)

// ClusterSelector defines which clusters to target based on labels, region, or tenant.
type ClusterSelector struct {
	LabelSelector map[string]string `json:"labelSelector,omitempty"`
	Region        string            `json:"region,omitempty"`
	Zone          string            `json:"zone,omitempty"`
	TenantID      string            `json:"tenantId,omitempty"`
	ClusterIDs    []string          `json:"clusterIds,omitempty"`
}

func (s ClusterSelector) IsValid() bool {
	return len(s.LabelSelector) > 0 || s.Region != "" || s.Zone != "" || s.TenantID != "" || len(s.ClusterIDs) > 0
}

// SchedulingPolicy captures the full multi-cluster scheduling configuration.
type SchedulingPolicy struct {
	Strategy          PropagationStrategy `json:"strategy"`
	Selector          ClusterSelector     `json:"selector"`
	DefaultClusterIDs []string            `json:"defaultClusterIds,omitempty"`
}
