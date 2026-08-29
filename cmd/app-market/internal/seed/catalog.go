package seed

import (
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
}

// Catalog is the platform-wide plugin catalog (current latest stable versions).
func Catalog() []PluginCatalogEntry {
	return []PluginCatalogEntry{
		{
			Name: "hami", DisplayName: "HAMi", Category: "GPU", Version: "v2.10.0",
			Description: "HAMi GPU 虚拟化与显存调度（多厂商 vGPU / MIG / NPU）",
			Tags:        map[string]string{"kind": "gpu", "provider": "gpu-virtualization"},
		},
		{
			Name: "gpu-operator", DisplayName: "NVIDIA GPU Operator", Category: "GPU", Version: "v26.7.0",
			Description: "NVIDIA GPU Operator（驱动托管 / Device Plugin / DCGM 监控 / MIG 与时间切片）",
			Tags:        map[string]string{"kind": "gpu", "provider": "gpu-operator"},
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
			Name: "prometheus-operator", DisplayName: "Prometheus Operator", Category: "监控", Version: "v0.93.1",
			Description: "Prometheus 监控与告警（kube-prometheus-stack：Grafana / Alertmanager / ServiceMonitor）",
			Tags:        map[string]string{"kind": "observability", "provider": "monitoring"},
		},
		{
			Name: "rook-ceph", DisplayName: "Rook Ceph", Category: "存储", Version: "v1.20.6",
			Description: "Rook Ceph 分布式存储：块 / 文件 / 对象存储与 CSI 驱动（CEPH-01）",
			Tags:        map[string]string{"kind": "storage", "provider": "storage"},
		},
		{
			Name: "longhorn", DisplayName: "Longhorn", Category: "存储", Version: "v1.12.1",
			Description: "Longhorn Kubernetes 块存储：快照、克隆、备份与 V2 NVMe 数据引擎",
			Tags:        map[string]string{"kind": "storage", "provider": "storage"},
		},
		{
			Name: "karmada", DisplayName: "Karmada", Category: "多集群", Version: "v1.18.2",
			Description: "Karmada 联邦多集群编排：统一资源分发、故障转移与差异化策略",
			Tags:        map[string]string{"kind": "multicluster", "provider": "federation"},
		},
		{
			Name: "keda", DisplayName: "KEDA", Category: "弹性伸缩", Version: "v2.20.2",
			Description: "KEDA 事件驱动弹性伸缩：Kafka Lag / NATS 消费 / Prometheus 指标 / GPU 指标（SCALE-002）",
			Tags:        map[string]string{"kind": "autoscaling", "provider": "autoscaling"},
		},
		{
			Name: "falco", DisplayName: "Falco", Category: "安全", Version: "0.44.1",
			Description: "Falco 运行时安全异常检测：容器逃逸、可疑行为与安全事件告警（SEC-ADV-05）",
			Tags:        map[string]string{"kind": "security", "provider": "runtime-security"},
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
		labels := map[string]string{"plugin": "true", "plugin.category": entry.Category, "plugin.version": entry.Version}
		for k, v := range entry.Tags {
			labels["plugin."+k] = v
		}
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

func ensureProduct(prodRepo *store.ProductRepo, publisher *appstore.Publisher, entry PluginCatalogEntry, labels map[string]string, tenantID string) (*appstore.Product, error) {
	// List is scoped to this tenant's publisher (Search with scope=public would
	// leak public products owned by other tenants into the seed check).
	existing, err := prodRepo.List(publisher.ID, tenantID)
	if err != nil {
		return nil, err
	}
	for _, p := range existing {
		if p.Name == entry.Name {
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
