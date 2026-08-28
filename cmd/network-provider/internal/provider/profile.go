package provider

type NetworkProfile struct {
	ID                   string         `json:"id"`
	TenantID             string         `json:"tenant_id"`
	TargetID             string         `json:"target_id"`
	Provider             string         `json:"provider"`
	IPVersion            string         `json:"ip_version"`
	PodCIDR              string         `json:"pod_cidr"`
	ServiceCIDR          string         `json:"service_cidr"`
	IPv6PodCIDR          string         `json:"ipv6_pod_cidr,omitempty"`
	IPv6ServiceCIDR      string         `json:"ipv6_service_cidr,omitempty"`
	EncapMode            string         `json:"encap_mode"`
	RoutingMode          string         `json:"routing_mode"`
	MTU                  int            `json:"mtu"`
	EnablePolicy         bool           `json:"enable_policy"`
	EnableHubble         bool           `json:"enable_hubble"`
	HubbleRelay          bool           `json:"hubble_relay,omitempty"`
	HubbleUI             bool           `json:"hubble_ui,omitempty"`
	HubbleMetrics        []string       `json:"hubble_metrics,omitempty"`
	EnableOTel           bool           `json:"enable_otel,omitempty"`
	OTelTarget           string         `json:"otel_target,omitempty"`
	EnableClusterMesh    bool           `json:"enable_clustermesh,omitempty"`
	ClusterMeshID        int            `json:"clustermesh_id,omitempty"`
	ClusterMeshName      string         `json:"clustermesh_name,omitempty"`
	ClusterMeshPeers     []ClusterMeshPeer `json:"clustermesh_peers,omitempty"`
	KubeProxyReplacement string         `json:"kube_proxy_replacement"`
	IPAMMode             string         `json:"ipam_mode"`
	Version              string         `json:"version"`
	ExtraConfig          map[string]any `json:"extra_config,omitempty"`
}

type ClusterMeshPeer struct {
	ClusterID   int    `json:"cluster_id"`
	ClusterName string `json:"cluster_name"`
	Endpoint    string `json:"endpoint"`
}