package seed

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/F31/hnb/pkg/appstore"
	"github.com/F31/hnb/pkg/appstore/store"
	"github.com/google/uuid"
)

// PluginCatalogEntry describes a platform plugin offered in the plugin market.
// It is seeded into the app-market product model (hnb-official publisher,
// public visibility) so every tenant can browse it via scope=public.
type PluginCatalogEntry struct {
	// Name is the stable plugin key used by install/uninstall (e.g. "hami").
	Name string
	// DisplayName is the human title shown in the market (e.g. "HAMi").
	DisplayName string
	// Category is the Chinese market category (GPU/网络/存储/监控/…).
	Category string
	// Version is the latest stable version offered by the catalog.
	Version string
	// Description is the one-line product description.
	Description string
	// Tags map to product labels (e.g. provider type, kind).
	Tags map[string]string
	// Helm maps to plugin.helm.* labels consumed by apiserver/plugin-worker.
	Helm      map[string]string
	Preflight map[string]any
}

// Catalog is the platform-wide plugin catalog (current latest stable versions).
func Catalog() []PluginCatalogEntry {
	return []PluginCatalogEntry{
		{
			Name: "hami", DisplayName: "HAMi", Category: "GPU", Version: "v2.10.0",
			Description: "HAMi GPU 虚拟化与显存调度（多厂商 vGPU / MIG / NPU）",
			Tags:        map[string]string{"kind": "gpu", "provider": "gpu-virtualization"},
			Helm:        map[string]string{"chart": "hami", "chartVersion": "2.10.0", "repoName": "hami", "repoURL": "https://project-hami.github.io/HAMi/", "namespace": "kube-system"},
			Preflight:   map[string]any{"requiresGPU": true},
		},
		{
			Name: "gpu-operator", DisplayName: "NVIDIA GPU Operator", Category: "GPU", Version: "v26.7.0",
			Description: "NVIDIA GPU Operator（驱动托管 / Device Plugin / DCGM 监控 / MIG 与时间切片）",
			Tags:        map[string]string{"kind": "gpu", "provider": "gpu-operator"},
			Helm:        map[string]string{"chart": "gpu-operator", "chartVersion": "v26.7.0", "repoName": "nvidia", "repoURL": "https://helm.ngc.nvidia.com/nvidia", "namespace": "gpu-operator"},
			Preflight:   map[string]any{"requiresGPU": true},
		},
		{
			Name: "calico", DisplayName: "Calico", Category: "网络", Version: "v3.32.1",
			Description: "Calico CNI：网络策略、BGP 路由、可选 eBPF 数据面（平台内置 Provider 支持）",
			Tags:        map[string]string{"kind": "cni", "provider": "cni"},
		},
		{
			Name: "cilium", DisplayName: "Cilium", Category: "网络", Version: "v1.20.1",
			Description: "Cilium eBPF CNI：L7 网络安全策略、Hubble 可观测、带宽管理与 Clustermesh",
			Tags:        map[string]string{"kind": "cni", "provider": "cni"},
		},
		{
			Name: "kube-ovn", DisplayName: "Kube-OVN", Category: "网络", Version: "v1.16.2",
			Description: "Kube-OVN：子网、固定 IP、安全组、多网卡与 QoS（OVN 虚拟网络）",
			Tags:        map[string]string{"kind": "cni", "provider": "cni"},
		},
		{
			Name: "multus-sriov", DisplayName: "Multus + SR-IOV", Category: "网络", Version: "",
			Description: "Multus 多网卡与 SR-IOV / RDMA / DPDK 高性能数据面（NET-ADV-01，规划中）",
			Tags:        map[string]string{"kind": "cni-adv", "provider": "multus"},
		},
		{
			Name: "kubeedge", DisplayName: "KubeEdge", Category: "边缘计算", Version: "v1.23.1",
			Description: "KubeEdge 云边协同：CloudCore / EdgeCore、离线自治、拓扑路由与 OTA",
			Tags:        map[string]string{"kind": "edge", "provider": "edge"},
		},
		{
			Name: "prometheus-operator", DisplayName: "Prometheus Operator", Category: "监控", Version: "v0.94.1",
			Description: "Prometheus 监控与告警（kube-prometheus-stack：Grafana / Alertmanager / ServiceMonitor）",
			Tags:        map[string]string{"kind": "observability", "provider": "monitoring"},
			Helm:        map[string]string{"chart": "kube-prometheus-stack", "chartVersion": "91.7.0", "repoName": "prometheus-community", "repoURL": "https://prometheus-community.github.io/helm-charts", "namespace": "monitoring"},
			Preflight:   map[string]any{"kubernetesMinVersion": "1.24.0"},
		},
		{
			Name: "rook-ceph", DisplayName: "Rook Ceph", Category: "存储", Version: "v1.20.6",
			Description: "Rook Ceph 分布式存储：块 / 文件 / 对象存储与 CSI 驱动（CEPH-01）",
			Tags:        map[string]string{"kind": "storage", "provider": "storage"},
			Helm:        map[string]string{"chart": "rook-ceph", "chartVersion": "v1.20.7", "repoName": "rook-release", "repoURL": "https://charts.rook.io/release", "namespace": "rook-ceph"},
		},
		{
			Name: "longhorn", DisplayName: "Longhorn", Category: "存储", Version: "v1.12.1",
			Description: "Longhorn Kubernetes 块存储：快照、克隆、备份与 V2 NVMe 数据引擎",
			Tags:        map[string]string{"kind": "storage", "provider": "storage"},
			Helm:        map[string]string{"chart": "longhorn", "chartVersion": "1.12.1", "repoName": "longhorn", "repoURL": "https://charts.longhorn.io", "namespace": "longhorn-system"},
			Preflight:   map[string]any{"kubernetesMinVersion": "1.25.0", "minCPUCores": 4, "minMemoryMB": 8192, "minStorageGB": 50},
		},
		{
			Name: "karmada", DisplayName: "Karmada", Category: "多集群", Version: "v1.18.2",
			Description: "Karmada 联邦多集群编排：统一资源分发、故障转移与差异化策略",
			Tags:        map[string]string{"kind": "multicluster", "provider": "federation"},
			Helm:        map[string]string{"chart": "karmada-operator", "chartVersion": "v1.18.1", "repoName": "karmada", "repoURL": "https://raw.githubusercontent.com/karmada-io/karmada/master/charts", "namespace": "karmada-system"},
		},
		{
			Name: "keda", DisplayName: "KEDA", Category: "弹性伸缩", Version: "v2.20.2",
			Description: "KEDA 事件驱动弹性伸缩：Kafka Lag / NATS 消费 / Prometheus 指标 / GPU 指标（SCALE-002）",
			Tags:        map[string]string{"kind": "autoscaling", "provider": "autoscaling"},
			Helm:        map[string]string{"chart": "keda", "chartVersion": "2.20.2", "repoName": "kedacore", "repoURL": "https://kedacore.github.io/charts", "namespace": "keda"},
		},
		{
			Name: "falco", DisplayName: "Falco", Category: "安全", Version: "0.44.1",
			Description: "Falco 运行时安全异常检测：容器逃逸、可疑行为与安全事件告警（SEC-ADV-05）",
			Tags:        map[string]string{"kind": "security", "provider": "runtime-security"},
			Helm:        map[string]string{"chart": "falco", "repoName": "falcosecurity", "repoURL": "https://falcosecurity.github.io/charts", "namespace": "falco"},
		},
	}
}

// SeedPluginCatalog idempotently creates the hnb-official publisher and the
// plugin catalog products + published releases for the given tenant.
// Plugin products are created with public visibility so every tenant can read
// the catalog through scope=public.
func SeedPluginCatalog(pubRepo *store.PublisherRepo, prodRepo *store.ProductRepo, relRepo *store.ReleaseRepo, tenantID string) error {
	publisher, err := pubRepo.DefaultPublisher(tenantID)
	if err != nil {
		return err
	}

	for _, entry := range Catalog() {
		labels := labelsForEntry(entry)
		product, err := ensureProduct(prodRepo, publisher, entry, labels, tenantID)
		if err != nil {
			return fmt.Errorf("seed product %q: %w", entry.Name, err)
		}
		if entry.Version == "" {
			continue
		}
		if err := ensureRelease(relRepo, product, entry, tenantID); err != nil {
			return fmt.Errorf("seed release %q %s: %w", entry.Name, entry.Version, err)
		}
	}
	return nil
}

func labelsForEntry(entry PluginCatalogEntry) map[string]string {
	labels := map[string]string{"plugin": "true", "plugin.category": entry.Category, "plugin.version": entry.Version}
	for k, v := range entry.Tags {
		labels["plugin."+k] = v
	}
	for k, v := range entry.Helm {
		labels["plugin.helm."+k] = v
	}
	if len(entry.Preflight) > 0 {
		if data, err := json.Marshal(entry.Preflight); err == nil {
			labels["plugin.helm.preflight"] = string(data)
		}
	}
	return labels
}

func ensureProduct(prodRepo *store.ProductRepo, publisher *appstore.Publisher, entry PluginCatalogEntry, labels map[string]string, tenantID string) (*appstore.Product, error) {
	// List is scoped to this tenant's publisher (Search with scope=public would
	// leak public products owned by other tenants into the seed check).
	existing, err := prodRepo.List(publisher.ID, tenantID)
	if err != nil {
		return nil, err
	}
	for _, p := range existing {
		if p.Name == entry.Name {
			if p.Labels == nil {
				p.Labels = map[string]string{}
			}
			changed := false
			for key, value := range labels {
				if p.Labels[key] != value {
					p.Labels[key] = value
					changed = true
				}
			}
			if changed {
				if err := prodRepo.Update(&p, tenantID, "system:plugin-catalog"); err != nil {
					return nil, err
				}
			}
			return &p, nil
		}
	}
	product := &appstore.Product{
		ID:          uuid.NewString(),
		PublisherID: publisher.ID,
		Name:        entry.Name,
		DisplayName: entry.DisplayName,
		Description: entry.Description,
		Category:    appstore.CatTool,
		Labels:      labels,
		Status:      appstore.ProdPublished,
		Visibility:  "public",
	}
	if err := prodRepo.Create(product, tenantID); err != nil {
		return nil, err
	}
	return product, nil
}

func ensureRelease(relRepo *store.ReleaseRepo, product *appstore.Product, entry PluginCatalogEntry, tenantID string) error {
	releases, err := relRepo.ListByProduct(product.ID, tenantID)
	if err != nil {
		return err
	}
	for _, r := range releases {
		if r.Version == entry.Version {
			return nil
		}
	}
	rel := &appstore.Release{
		ID:        uuid.NewString(),
		ProductID: product.ID,
		Version:   entry.Version,
		Status:    appstore.RelPublished,
		CreatedBy: "system:plugin-catalog",
		Manifest:  map[string]any{"plugin": entry.DisplayName, "category": entry.Category, "provider": entry.Tags["provider"]},
	}
	return relRepo.Create(rel, tenantID)
}

// LogCatalogSummary emits the seeded catalog for operator visibility.
func LogCatalogSummary(count int) {
	log.Printf("[app-market] plugin catalog seeded: %d products", count)
}
