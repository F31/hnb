package core

import (
	"testing"
	"time"
)

func TestTargetTypeValues(t *testing.T) {
	if TargetKubernetes != "kubernetes" {
		t.Errorf("kubernetes = %s", TargetKubernetes)
	}
	if TargetExternalService != "external_service" {
		t.Errorf("external_service = %s", TargetExternalService)
	}
}

func TestRuntimeTarget_IsDeployable(t *testing.T) {
	tests := []struct {
		name       string
		target     RuntimeTarget
		deployable bool
	}{
		{"k8s active", RuntimeTarget{TargetType: TargetKubernetes, IsActive: true}, true},
		{"container active", RuntimeTarget{TargetType: TargetContainerEngine, IsActive: true}, true},
		{"edge active", RuntimeTarget{TargetType: TargetEdgeRuntime, IsActive: true}, true},
		{"inactive k8s", RuntimeTarget{TargetType: TargetKubernetes, IsActive: false}, false},
		{"external service", RuntimeTarget{TargetType: TargetExternalService, IsActive: true}, false},
	}
	for _, tt := range tests {
		got := tt.target.IsDeployable()
		if got != tt.deployable {
			t.Errorf("%s: IsDeployable = %v, want %v", tt.name, got, tt.deployable)
		}
	}
}

func TestRuntimeTarget_IsStale(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name   string
		target RuntimeTarget
		stale  bool
	}{
		{"no observedAt", RuntimeTarget{ObservedAt: nil, StaleThresholdSec: 300}, true},
		{"fresh", RuntimeTarget{ObservedAt: &now, StaleThresholdSec: 300}, false},
		{"stale past", RuntimeTarget{ObservedAt: &time.Time{}, StaleThresholdSec: 1}, true},
	}
	for _, tt := range tests {
		got := tt.target.IsStale()
		if got != tt.stale {
			t.Errorf("%s: IsStale = %v, want %v", tt.name, got, tt.stale)
		}
	}
}

func TestProviderRegistry_Register(t *testing.T) {
	r := NewProviderRegistry(nil)
	target := &RuntimeTarget{ID: "t1", Name: "prod-cluster", TargetType: TargetKubernetes, IsActive: true}

	entry := &ProviderEntry{
		ProviderID:    "k8s-prod",
		ProviderType:  ProviderK8sDeploy,
		RuntimeTarget: target,
		IsActive:      true,
	}
	err := r.RegisterProvider(entry)
	if err != nil {
		t.Fatal(err)
	}

	got, err := r.GetProvider("k8s-prod")
	if err != nil {
		t.Fatal("provider not found")
	}
	if got.ProviderID != "k8s-prod" {
		t.Errorf("provider_id = %s", got.ProviderID)
	}
}

func TestProviderRegistry_RegisterNilTarget(t *testing.T) {
	r := NewProviderRegistry(nil)
	err := r.RegisterProvider(&ProviderEntry{ProviderID: "bad", RuntimeTarget: nil})
	if err == nil {
		t.Error("expected error for nil target")
	}
}

func TestProviderRegistry_Unregister(t *testing.T) {
	r := NewProviderRegistry(nil)
	target := &RuntimeTarget{ID: "t1", TargetType: TargetKubernetes, IsActive: true}
	r.RegisterProvider(&ProviderEntry{ProviderID: "p1", RuntimeTarget: target, IsActive: true})

	if err := r.UnregisterProvider("p1"); err != nil {
		t.Error("unregister should succeed:", err)
	}
	_, err := r.GetProvider("p1")
	if err == nil {
		t.Error("provider should be gone")
	}
	if err := r.UnregisterProvider("nonexistent"); err == nil {
		t.Error("unregister nonexistent should return error")
	}
}

func TestProviderRegistry_ResolveStepProvider(t *testing.T) {
	r := NewProviderRegistry(nil)
	target := &RuntimeTarget{ID: "t1", TargetType: TargetKubernetes, IsActive: true}
	r.RegisterProvider(&ProviderEntry{
		ProviderID: "k8s-prod", ProviderType: ProviderK8sDeploy,
		RuntimeTarget: target, IsActive: true,
	})

	entry, resolvedTarget, err := r.ResolveStepProvider("k8s-prod")
	if err != nil {
		t.Fatal(err)
	}
	if entry.ProviderID != "k8s-prod" {
		t.Errorf("provider_id = %s", entry.ProviderID)
	}
	if resolvedTarget.ID != "t1" {
		t.Errorf("target_id = %s", resolvedTarget.ID)
	}
}

func TestProviderRegistry_ResolveStepProvider_NotFound(t *testing.T) {
	r := NewProviderRegistry(nil)
	_, _, err := r.ResolveStepProvider("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent provider")
	}
}

func TestProviderRegistry_ResolveStepProvider_Inactive(t *testing.T) {
	r := NewProviderRegistry(nil)
	target := &RuntimeTarget{ID: "t1", TargetType: TargetKubernetes, IsActive: true}
	r.RegisterProvider(&ProviderEntry{
		ProviderID: "p1", ProviderType: ProviderK8sDeploy,
		RuntimeTarget: target, IsActive: false,
	})
	_, _, err := r.ResolveStepProvider("p1")
	if err == nil {
		t.Error("expected error for inactive provider")
	}
}

func TestProviderRegistry_List(t *testing.T) {
	r := NewProviderRegistry(nil)
	target := &RuntimeTarget{ID: "t1", TargetType: TargetKubernetes, IsActive: true}
	r.RegisterProvider(&ProviderEntry{ProviderID: "p1", RuntimeTarget: target, IsActive: true})
	r.RegisterProvider(&ProviderEntry{ProviderID: "p2", RuntimeTarget: target, IsActive: true})

	providers := r.ListProviders()
	if len(providers) != 2 {
		t.Errorf("got %d providers, want 2", len(providers))
	}
}

func TestProviderRegistry_RegisterDuplicate(t *testing.T) {
	r := NewProviderRegistry(nil)
	target := &RuntimeTarget{ID: "t1", TargetType: TargetKubernetes, IsActive: true}
	err := r.RegisterProvider(&ProviderEntry{ProviderID: "p1", RuntimeTarget: target, IsActive: true})
	if err != nil {
		t.Fatal(err)
	}
	err = r.RegisterProvider(&ProviderEntry{ProviderID: "p1", RuntimeTarget: target, IsActive: true})
	if err == nil {
		t.Fatal("expected error for duplicate provider")
	}
}

func TestProviderRegistry_RegisterVersion(t *testing.T) {
	r := NewProviderRegistry(nil)
	target := &RuntimeTarget{ID: "t1", TargetType: TargetKubernetes, IsActive: true}
	r.RegisterProvider(&ProviderEntry{ProviderID: "p1", RuntimeTarget: target, IsActive: true})

	entry, err := r.GetProvider("p1")
	if err != nil {
		t.Fatal("provider not found")
	}
	if entry.Version != 1 {
		t.Errorf("version = %d, want 1", entry.Version)
	}
}

func TestProviderRegistry_CompareAndSwap_Success(t *testing.T) {
	r := NewProviderRegistry(nil)
	target := &RuntimeTarget{ID: "t1", TargetType: TargetKubernetes, IsActive: true}
	r.RegisterProvider(&ProviderEntry{ProviderID: "p1", RuntimeTarget: target, IsActive: true})

	updated, err := r.CompareAndSwapProvider("p1", 1, func(entry *ProviderEntry) *ProviderEntry {
		entry.IsActive = false
		return entry
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Version != 2 {
		t.Errorf("new version = %d, want 2", updated.Version)
	}

	entry, _ := r.GetProvider("p1")
	if entry.IsActive {
		t.Error("provider should be inactive after CAS")
	}
	if entry.Version != 2 {
		t.Errorf("version = %d, want 2", entry.Version)
	}
}

func TestProviderRegistry_CompareAndSwap_VersionMismatch(t *testing.T) {
	r := NewProviderRegistry(nil)
	target := &RuntimeTarget{ID: "t1", TargetType: TargetKubernetes, IsActive: true}
	r.RegisterProvider(&ProviderEntry{ProviderID: "p1", RuntimeTarget: target, IsActive: true})

	_, err := r.CompareAndSwapProvider("p1", 99, func(entry *ProviderEntry) *ProviderEntry {
		entry.IsActive = false
		return entry
	})
	if err == nil {
		t.Fatal("expected error for version mismatch")
	}

	entry, _ := r.GetProvider("p1")
	if !entry.IsActive {
		t.Error("provider should remain active after failed CAS")
	}
	if entry.Version != 1 {
		t.Errorf("version = %d, want 1 (unchanged)", entry.Version)
	}
}

func TestProviderRegistry_CompareAndSwap_NotFound(t *testing.T) {
	r := NewProviderRegistry(nil)
	_, err := r.CompareAndSwapProvider("nonexistent", 1, func(entry *ProviderEntry) *ProviderEntry {
		return entry
	})
	if err == nil {
		t.Fatal("expected error for nonexistent provider")
	}
}

func TestProviderRegistry_CompareAndSwap_BumpsVersion(t *testing.T) {
	r := NewProviderRegistry(nil)
	target := &RuntimeTarget{ID: "t1", TargetType: TargetKubernetes, IsActive: true}
	r.RegisterProvider(&ProviderEntry{ProviderID: "p1", RuntimeTarget: target, IsActive: true})

	v2, _ := r.CompareAndSwapProvider("p1", 1, func(entry *ProviderEntry) *ProviderEntry {
		entry.Name = "v2"
		return entry
	})
	v3, _ := r.CompareAndSwapProvider("p1", 2, func(entry *ProviderEntry) *ProviderEntry {
		entry.Name = "v3"
		return entry
	})

	if v2.Version != 2 || v3.Version != 3 {
		t.Errorf("versions: v2=%d (want 2), v3=%d (want 3)", v2.Version, v3.Version)
	}
	entry, _ := r.GetProvider("p1")
	if entry.Name != "v3" {
		t.Errorf("name = %s, want v3", entry.Name)
	}
	if entry.Version != 3 {
		t.Errorf("version = %d, want 3", entry.Version)
	}
}

func TestProviderRegistry_CompareAndSwap_ConcurrentCAS(t *testing.T) {
	r := NewProviderRegistry(nil)
	target := &RuntimeTarget{ID: "t1", TargetType: TargetKubernetes, IsActive: true}
	r.RegisterProvider(&ProviderEntry{ProviderID: "p1", RuntimeTarget: target, IsActive: true})

	done := make(chan bool, 3)
	for i := 0; i < 3; i++ {
		go func() {
			for attempt := 0; attempt < 5; attempt++ {
				got, err := r.GetProvider("p1")
				if err != nil {
					continue
				}
				expected := got.Version
				_, err = r.CompareAndSwapProvider("p1", expected, func(e *ProviderEntry) *ProviderEntry {
					e.Config = map[string]any{"attempt": attempt}
					return e
				})
				if err == nil {
					break
				}
			}
			done <- true
		}()
	}
	for i := 0; i < 3; i++ {
		<-done
	}

	got, err := r.GetProvider("p1")
	if err != nil {
		t.Fatal("provider not found")
	}
	if got.Version < 2 {
		t.Errorf("version = %d, expected at least 2 after concurrent CAS", got.Version)
	}
}

func TestCompatibilityChecker_Pass(t *testing.T) {
	cc := NewCompatibilityChecker()
	req := &ResourceRequirement{
		MinMemoryMB: 2048,
		MinCPUCores: 2,
	}
	cap := &CapabilitySnapshot{
		MemoryMB:   4096,
		CPUCores:   4,
		KubeVersion: "v1.28.0",
	}
	result := cc.Check(req, cap)
	if !result.Passed {
		t.Errorf("expected pass, got issues: %v", result.Issues)
	}
}

func TestCompatibilityChecker_MemoryFail(t *testing.T) {
	cc := NewCompatibilityChecker()
	req := &ResourceRequirement{MinMemoryMB: 8192}
	cap := &CapabilitySnapshot{MemoryMB: 4096}
	result := cc.Check(req, cap)
	if result.Passed {
		t.Error("expected fail due to insufficient memory")
	}
	if len(result.Issues) == 0 {
		t.Error("expected at least one issue")
	}
}

func TestCompatibilityChecker_CPUFail(t *testing.T) {
	cc := NewCompatibilityChecker()
	req := &ResourceRequirement{MinCPUCores: 8}
	cap := &CapabilitySnapshot{CPUCores: 4, MemoryMB: 16384, StorageGB: 100}
	result := cc.Check(req, cap)
	if result.Passed {
		t.Error("expected fail due to insufficient cpu")
	}
}

func TestCompatibilityChecker_StorageFail(t *testing.T) {
	cc := NewCompatibilityChecker()
	req := &ResourceRequirement{MinStorageGB: 500}
	cap := &CapabilitySnapshot{StorageGB: 100, MemoryMB: 4096, CPUCores: 4}
	result := cc.Check(req, cap)
	if result.Passed {
		t.Error("expected fail due to insufficient storage")
	}
}

func TestCompatibilityChecker_GPU(t *testing.T) {
	cc := NewCompatibilityChecker()

	result := cc.Check(&ResourceRequirement{RequiresGPU: true}, &CapabilitySnapshot{GPUCount: 0})
	if result.Passed {
		t.Error("expected fail when no GPU available")
	}

	result = cc.Check(&ResourceRequirement{RequiresGPU: true}, &CapabilitySnapshot{GPUCount: 2})
	if !result.Passed {
		t.Error("expected pass when GPU available")
	}
}

func TestCompatibilityChecker_CNI(t *testing.T) {
	cc := NewCompatibilityChecker()

	cap := &CapabilitySnapshot{CNIPlugins: []string{"calico", "flannel"}, MemoryMB: 4096, CPUCores: 2}

	result := cc.Check(&ResourceRequirement{CNIRequired: "calico"}, cap)
	if !result.Passed {
		t.Errorf("expected pass for calico, got: %v", result.Issues)
	}

	result = cc.Check(&ResourceRequirement{CNIRequired: "cilium"}, cap)
	if result.Passed {
		t.Error("expected fail when cilium not available")
	}
}

func TestCompatibilityChecker_KubeVersion(t *testing.T) {
	cc := NewCompatibilityChecker()

	result := cc.Check(
		&ResourceRequirement{KubeMinVersion: "v1.25.0"},
		&CapabilitySnapshot{KubeVersion: "v1.28.0", MemoryMB: 4096, CPUCores: 2},
	)
	if !result.Passed {
		t.Errorf("expected pass, got: %v", result.Issues)
	}

	result = cc.Check(
		&ResourceRequirement{KubeMinVersion: "v1.30.0"},
		&CapabilitySnapshot{KubeVersion: "v1.28.0", MemoryMB: 4096, CPUCores: 2},
	)
	if result.Passed {
		t.Error("expected fail when kube version too low")
	}
}

func TestCompatibilityChecker_Features(t *testing.T) {
	cc := NewCompatibilityChecker()
	req := &ResourceRequirement{Features: []string{"ingress", "monitoring"}}
	cap := &CapabilitySnapshot{
		Features:  map[string]bool{"ingress": true, "monitoring": false},
		MemoryMB:  4096,
		CPUCores:  2,
	}
	result := cc.Check(req, cap)
	if result.Passed {
		t.Error("expected fail due to missing monitoring feature")
	}
}

func TestCompatibilityChecker_MultipleIssues(t *testing.T) {
	cc := NewCompatibilityChecker()
	req := &ResourceRequirement{
		MinMemoryMB:  8192,
		MinCPUCores:  8,
		MinStorageGB: 500,
	}
	cap := &CapabilitySnapshot{
		MemoryMB:  2048,
		CPUCores:  2,
		StorageGB: 50,
	}
	result := cc.Check(req, cap)
	if result.Passed {
		t.Error("expected fail with multiple issues")
	}
	if len(result.Issues) < 3 {
		t.Errorf("expected >=3 issues, got %d", len(result.Issues))
	}
}

func TestFreshnessTracker_Default(t *testing.T) {
	ft := NewFreshnessTracker()

	target := &RuntimeTarget{TargetType: TargetKubernetes, ObservedAt: nil}
	ok, action := ft.Evaluate(target)
	if ok {
		t.Error("nil observedAt should be stale")
	}
	if action != "queue_offline" {
		t.Errorf("action = %s, want queue_offline", action)
	}
}

func TestFreshnessTracker_Fresh(t *testing.T) {
	ft := NewFreshnessTracker()
	now := time.Now()
	target := &RuntimeTarget{
		TargetType: TargetKubernetes,
		ObservedAt: &now,
	}
	ok, action := ft.Evaluate(target)
	if !ok {
		t.Error("fresh target should be ok")
	}
	if action != "" {
		t.Errorf("action = %s, want empty", action)
	}
}

func TestFreshnessTracker_EdgeOffline(t *testing.T) {
	ft := NewFreshnessTracker()
	past := time.Now().Add(-5 * time.Minute)
	target := &RuntimeTarget{
		TargetType: TargetEdgeRuntime,
		ObservedAt: &past,
	}
	ok, action := ft.Evaluate(target)
	if ok {
		t.Error("stale edge target should not be ok")
	}
	if action != "queue_offline" {
		t.Errorf("action = %s, want queue_offline", action)
	}
}

func TestFreshnessTracker_SetPolicy(t *testing.T) {
	ft := NewFreshnessTracker()
	ft.SetPolicy(TargetKubernetes, &FreshnessPolicy{
		StaleThreshold: 1 * time.Hour,
		ActionOnStale:  "reject",
	})

	p := ft.GetPolicy(TargetKubernetes)
	if p.StaleThreshold != 1*time.Hour {
		t.Errorf("threshold = %v, want 1h", p.StaleThreshold)
	}
	if p.ActionOnStale != "reject" {
		t.Errorf("action = %s, want reject", p.ActionOnStale)
	}
}

func TestFreshnessTracker_UnknownType(t *testing.T) {
	ft := NewFreshnessTracker()
	p := ft.GetPolicy("unknown")
	if p.StaleThreshold != 5*time.Minute {
		t.Errorf("unknown type threshold = %v, want 5m", p.StaleThreshold)
	}
}

func TestFreshnessTracker_ApplyEdgeDiscovery_Online(t *testing.T) {
	ft := NewFreshnessTracker()
	target := &RuntimeTarget{TargetType: TargetEdgeRuntime, Status: StatusUnknown, ObservedAt: nil}

	disc := &KubeEdgeDiscoveryResult{
		TotalNodes:   3,
		OfflineCount: 0,
		DetectedAt:   time.Now(),
		Nodes: []EdgeNodeInfo{
			{Name: "node-1", Status: EdgeNodeOnline},
			{Name: "node-2", Status: EdgeNodeOnline},
			{Name: "node-3", Status: EdgeNodeOnline},
		},
	}

	status := ft.ApplyEdgeDiscovery(target, disc)
	if status != EdgeNodeOnline {
		t.Errorf("status = %s, want online", status)
	}
	if target.Status != StatusOnline {
		t.Errorf("target status = %s, want online", target.Status)
	}
	if target.ObservedAt == nil {
		t.Error("observedAt should be set")
	}
}

func TestFreshnessTracker_ApplyEdgeDiscovery_Offline(t *testing.T) {
	ft := NewFreshnessTracker()
	target := &RuntimeTarget{TargetType: TargetEdgeRuntime, Status: StatusUnknown, ObservedAt: nil}

	disc := &KubeEdgeDiscoveryResult{
		TotalNodes:   2,
		OfflineCount: 2,
		DetectedAt:   time.Now(),
		Nodes: []EdgeNodeInfo{
			{Name: "node-1", Status: EdgeNodeOffline},
			{Name: "node-2", Status: EdgeNodeOffline},
		},
	}

	status := ft.ApplyEdgeDiscovery(target, disc)
	if status != EdgeNodeOffline {
		t.Errorf("status = %s, want offline", status)
	}
	if target.Status != StatusOffline {
		t.Errorf("target status = %s, want offline", target.Status)
	}
}

func TestFreshnessTracker_ApplyEdgeDiscovery_Degraded(t *testing.T) {
	ft := NewFreshnessTracker()
	target := &RuntimeTarget{TargetType: TargetEdgeRuntime, Status: StatusUnknown, ObservedAt: nil}

	disc := &KubeEdgeDiscoveryResult{
		TotalNodes:   3,
		OfflineCount: 1,
		DetectedAt:   time.Now(),
		Nodes: []EdgeNodeInfo{
			{Name: "node-1", Status: EdgeNodeOnline},
			{Name: "node-2", Status: EdgeNodeOnline},
			{Name: "node-3", Status: EdgeNodeOffline},
		},
	}

	status := ft.ApplyEdgeDiscovery(target, disc)
	if status != EdgeNodeOnline {
		t.Errorf("status = %s, want online", status)
	}
	if target.Status != StatusDegraded {
		t.Errorf("target status = %s, want degraded", target.Status)
	}
}

func TestFreshnessTracker_ApplyEdgeDiscovery_Nil(t *testing.T) {
	ft := NewFreshnessTracker()
	target := &RuntimeTarget{TargetType: TargetEdgeRuntime, Status: StatusOnline, ObservedAt: nil}

	status := ft.ApplyEdgeDiscovery(target, nil)
	if status != EdgeNodeUnknown {
		t.Errorf("status = %s, want unknown", status)
	}
	if target.Status != StatusUnknown {
		t.Errorf("target status = %s, want unknown", target.Status)
	}
	if target.ObservedAt == nil {
		t.Error("observedAt should be set even with nil discovery")
	}
}

func TestFreshnessTracker_EvaluateEdgeTarget_Stale(t *testing.T) {
	ft := NewFreshnessTracker()
	past := time.Now().Add(-5 * time.Minute)
	target := &RuntimeTarget{
		TargetType: TargetEdgeRuntime,
		ObservedAt: &past,
		Status:     StatusOnline,
	}

	ok, action, newStatus := ft.EvaluateEdgeTarget(target)
	if ok {
		t.Error("stale edge target should not be ok")
	}
	if action != "queue_offline" {
		t.Errorf("action = %s, want queue_offline", action)
	}
	if newStatus != StatusOffline {
		t.Errorf("status = %s, want offline", newStatus)
	}
	if target.Status != StatusOffline {
		t.Errorf("target status updated to %s, want offline", target.Status)
	}
}

func TestFreshnessTracker_EvaluateEdgeTarget_Fresh(t *testing.T) {
	ft := NewFreshnessTracker()
	now := time.Now()
	target := &RuntimeTarget{
		TargetType: TargetEdgeRuntime,
		ObservedAt: &now,
		Status:     StatusOnline,
	}

	ok, action, newStatus := ft.EvaluateEdgeTarget(target)
	if !ok {
		t.Error("fresh edge target should be ok")
	}
	if action != "" {
		t.Errorf("action = %s, want empty", action)
	}
	if newStatus != StatusOnline {
		t.Errorf("status = %s, want online", newStatus)
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"v1.28.0", "v1.25.0", 3},
		{"v1.25.0", "v1.28.0", -3},
		{"v1.25.0", "v1.25.3", -3},
		{"v1.25.0", "v1.25.0", 0},
		{"1.28", "v1.25.0", 3},
	}
	for _, tt := range tests {
		got := compareVersions(tt.a, tt.b)
		if (got > 0) != (tt.want > 0) {
			t.Errorf("compareVersions(%s, %s) = %d, want sign %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestParseDistribution(t *testing.T) {
	tests := []struct {
		input string
		want  Distribution
	}{
		{"standard", DistStandard},
		{"k3s", DistK3s},
		{"kubeedge", DistKubeEdge},
		{"other", DistOther},
		{"unknown", DistStandard},
		{"", DistStandard},
	}
	for _, tt := range tests {
		got := ParseDistribution(tt.input)
		if got != tt.want {
			t.Errorf("ParseDistribution(%q) = %s, want %s", tt.input, got, tt.want)
		}
	}
}

func TestDetectDistributionFromGitVersion(t *testing.T) {
	tests := []struct {
		version string
		want    Distribution
	}{
		{"v1.28.4+k3s1", DistK3s},
		{"v1.28.4+k3s", DistK3s},
		{"v1.28.4", DistStandard},
		{"v1.28.4-kubeedge-v1.16.0", DistKubeEdge},
		{"v1.30.0-gke.123", DistStandard},
	}
	for _, tt := range tests {
		got := DetectDistributionFromGitVersion(tt.version)
		if got != tt.want {
			t.Errorf("DetectDistributionFromGitVersion(%q) = %s, want %s", tt.version, got, tt.want)
		}
	}
}

func TestExtractCloudCoreVersion(t *testing.T) {
	tests := []struct {
		gitVersion string
		want       string
	}{
		{"v1.28.4-kubeedge-v1.16.0", "1.16.0"},
		{"v1.30.0-kubeedge-v1.17.0", "1.17.0"},
		{"v1.28.4", "v1.28.4"},
		{"v1.28.4-kubeedge", "v1.28.4-kubeedge"},
	}
	for _, tt := range tests {
		got := extractCloudCoreVersion(tt.gitVersion)
		if got != tt.want {
			t.Errorf("extractCloudCoreVersion(%q) = %q, want %q", tt.gitVersion, got, tt.want)
		}
	}
}

func TestIsEdgeNode(t *testing.T) {
	tests := []struct {
		name   string
		labels map[string]string
		want   bool
	}{
		{"edge node", map[string]string{"node-role.kubernetes.io/edge": ""}, true},
		{"control plane node", map[string]string{"node-role.kubernetes.io/control-plane": ""}, false},
		{"nil labels", nil, false},
		{"empty labels", map[string]string{}, false},
	}
	for _, tt := range tests {
		got := isEdgeNode(tt.labels)
		if got != tt.want {
			t.Errorf("%s: isEdgeNode = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestGetNodeCondition(t *testing.T) {
	type cond struct {
		Type   string `json:"type"`
		Status string `json:"status"`
	}
	tests := []struct {
		name       string
		conditions []cond
		want       EdgeNodeStatus
	}{
		{"ready true", []cond{{Type: "Ready", Status: "True"}}, EdgeNodeOnline},
		{"ready false", []cond{{Type: "Ready", Status: "False"}}, EdgeNodeOffline},
		{"ready unknown", []cond{{Type: "Ready", Status: "Unknown"}}, EdgeNodeUnknown},
		{"no ready condition", []cond{{Type: "MemoryPressure", Status: "False"}}, EdgeNodeUnknown},
	}
	for _, tt := range tests {
		typed := make([]struct {
			Type   string `json:"type"`
			Status string `json:"status"`
		}, len(tt.conditions))
		for i, c := range tt.conditions {
			typed[i] = c
		}
		got := getNodeCondition(typed)
		if got != tt.want {
			t.Errorf("%s: getNodeCondition = %s, want %s", tt.name, got, tt.want)
		}
	}
}

func TestGetDistributionNotes(t *testing.T) {
	notes := GetDistributionNotes(DistK3s)
	if notes == nil {
		t.Fatal("expected non-nil notes for K3s")
	}
	if notes.Label != "K3s - Lightweight Kubernetes" {
		t.Errorf("label = %q, want K3s", notes.Label)
	}
	if len(notes.KnownLimits) == 0 {
		t.Error("expected known limits for K3s")
	}

	notes = GetDistributionNotes(DistKubeEdge)
	if notes == nil {
		t.Fatal("expected non-nil notes for KubeEdge")
	}
	if !notes.BuiltIn["edge_autonomy"] {
		t.Error("expected edge_autonomy for KubeEdge")
	}

	notes = GetDistributionNotes(DistStandard)
	if notes.Label != "Standard Kubernetes" {
		t.Errorf("label = %q, want Standard Kubernetes", notes.Label)
	}
}