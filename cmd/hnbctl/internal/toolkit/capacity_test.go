package toolkit

import (
	"strings"
	"testing"
)

const capacityFixture = `kind: Deployment
metadata: {name: apiserver}
spec:
  replicas: 2
  template:
    spec:
      containers:
      - name: main
        resources: {requests: {cpu: 500m, memory: 256Mi}}
      - name: sidecar
        resources: {requests: {cpu: 100m, memory: 64Mi}}
      initContainers:
      - name: migration
        resources: {requests: {cpu: "1", memory: 128Mi}}
---
kind: PersistentVolumeClaim
metadata: {name: data}
spec:
  resources: {requests: {storage: 10Gi}}
`

func TestCapacitySchedulingStorageAndHeadroom(t *testing.T) {
	nodes := []CapacityNode{{Name: "cp-1", FailureDomain: "zone-a", ControlPlane: true, CPUMilli: 8000, MemoryMi: 16384, DiskGi: 100}}
	r, err := BuildCapacityReport(ProfileMinimal, []byte(capacityFixture), nodes)
	if err != nil {
		t.Fatal(err)
	}
	// Init max vs sum(main + sidecar); replicas multiply the effective request.
	if r.Requests.CPUMilli != 2000 || r.Requests.MemoryMi != 640 || r.Requests.DiskGi != 10 || r.Required.CPUMilli != 3600 || r.Required.MemoryMi != 2880 || r.Required.DiskGi != 72 || !r.FitsDeclaredCapacity || r.InstallAuthorized {
		t.Fatalf("incorrect planning estimate: %+v", r)
	}
	if r.DownloadCacheGi != 20 || r.BackupGi != 10 || r.HeadroomPercent != 30 {
		t.Fatal("missing explicit disk/headroom budgets")
	}
}

func TestCapacityRefusesUnderstatedInputs(t *testing.T) {
	nodes := []CapacityNode{{Name: "cp-1", CPUMilli: 8000, MemoryMi: 16384, DiskGi: 100}}
	for _, manifest := range []string{
		strings.Replace(capacityFixture, "cpu: 500m", "cpu: unknown", 1),
		strings.Replace(capacityFixture, "memory: 256Mi", "memory: 0Mi", 1),
		strings.Replace(capacityFixture, "replicas: 2", "replicas: -1", 1),
		strings.Replace(capacityFixture, "kind: Deployment", "kind: DaemonSet", 1),
		capacityFixture + "---\n" + capacityFixture,
	} {
		if _, err := BuildCapacityReport(ProfileMinimal, []byte(manifest), nodes); err == nil {
			t.Fatal("invalid/unsupported manifest understated capacity")
		}
	}
}

func TestCapacityHAAndOptionalPacksFailClosed(t *testing.T) {
	nodes := []CapacityNode{{Name: "cp-1", FailureDomain: "same-zone", ControlPlane: true, CPUMilli: 16000, MemoryMi: 32768, DiskGi: 200}}
	for _, profile := range []Profile{ProfileLiteHA, ProfileStandardHA, ProfileEnterprise} {
		r, err := BuildCapacityReport(profile, []byte(capacityFixture), nodes)
		if err != nil {
			t.Fatal(err)
		}
		if r.FitsDeclaredCapacity || !strings.Contains(strings.Join(r.Reasons, ","), "HA_FAILURE_DOMAINS_INSUFFICIENT") || !strings.Contains(strings.Join(r.Reasons, ","), "HA_PROFILE_UNCERTIFIED") {
			t.Fatalf("single zone certified HA: %+v", r)
		}
	}
	for i, name := range []string{"cp-2", "cp-3"} {
		nodes = append(nodes, CapacityNode{Name: name, FailureDomain: name, ControlPlane: true, CPUMilli: 16000, MemoryMi: 32768, DiskGi: int64(200 + i)})
	}
	r, err := BuildCapacityReport(ProfileLiteHA, []byte(capacityFixture), nodes)
	if err != nil {
		t.Fatal(err)
	}
	if r.FitsDeclaredCapacity || strings.Contains(strings.Join(r.Reasons, ","), "HA_FAILURE_DOMAINS_INSUFFICIENT") || !strings.Contains(strings.Join(r.Reasons, ","), "HA_PROFILE_UNCERTIFIED") {
		t.Fatal("three domains substituted for actual failure drills")
	}
	r, err = BuildCapacityReport(ProfileMinimal, []byte(strings.Replace(capacityFixture, "name: apiserver", "name: edge-provider", 1)), nodes)
	if err != nil {
		t.Fatal(err)
	}
	if r.FitsDeclaredCapacity {
		t.Fatal("Minimal accepted optional Pack")
	}
}

func TestCapacityAggregateCannotConcealUnschedulablePod(t *testing.T) {
	var nodes []CapacityNode
	for _, name := range []string{"a", "b", "c"} {
		nodes = append(nodes, CapacityNode{Name: name, CPUMilli: 2000, MemoryMi: 8192, DiskGi: 100})
	}
	manifest := strings.Replace(capacityFixture, "replicas: 2", "replicas: 1", 1)
	manifest = strings.Replace(manifest, "cpu: 500m", "cpu: 1500m", 1)
	r, err := BuildCapacityReport(ProfileMinimal, []byte(manifest), nodes)
	if err != nil {
		t.Fatal(err)
	}
	if r.FitsDeclaredCapacity || !strings.Contains(strings.Join(r.Reasons, ","), "POD_EXCEEDS_SINGLE_NODE_CAPACITY") {
		t.Fatalf("aggregate hid scheduling failure: %+v", r)
	}
}

func TestCapacitySchedulingUsesSelectedReserve(t *testing.T) {
	manifest := []byte(`kind: Deployment
metadata: {name: core}
spec:
  replicas: 1
  template:
    spec:
      containers:
      - name: core
        resources: {requests: {cpu: 1200m, memory: 2200Mi}}
`)
	node := CapacityNode{Name: "cp-1", CPUMilli: 2000, MemoryMi: 4096, DiskGi: 100}
	for _, scope := range []string{"delivery", "startup"} {
		r, err := BuildCapacityReportWithOptions(ProfileMinimal, manifest, []CapacityNode{node}, CapacityPlanningOptions{Scope: scope})
		if err != nil {
			t.Fatal(err)
		}
		unschedulable := strings.Contains(strings.Join(r.Reasons, ","), "POD_EXCEEDS_SINGLE_NODE_CAPACITY")
		if unschedulable != (scope == "delivery") {
			t.Fatalf("scheduling ignored %s reserve: %+v", scope, r)
		}
	}
	node.ReserveCPUMilli, node.ReserveMemoryMi = 900, 2000
	r, err := BuildCapacityReportWithOptions(ProfileMinimal, manifest, []CapacityNode{node}, CapacityPlanningOptions{Scope: "startup"})
	if err != nil || !strings.Contains(strings.Join(r.Reasons, ","), "POD_EXCEEDS_SINGLE_NODE_CAPACITY") {
		t.Fatalf("explicit reserve ignored: %+v, %v", r, err)
	}
}

func TestCapacityRejectsInvalidNodeReserves(t *testing.T) {
	for _, node := range []CapacityNode{
		{Name: "cp-1", CPUMilli: 2000, MemoryMi: 4096, DiskGi: 100, ReserveCPUMilli: -1},
		{Name: "cp-1", CPUMilli: 2000, MemoryMi: 4096, DiskGi: 100, ReserveMemoryMi: -1},
		{Name: "cp-1", CPUMilli: 2000, MemoryMi: 4096, DiskGi: 100, ReserveDiskGi: -1},
		{Name: "cp-1", CPUMilli: 2000, MemoryMi: 4096, DiskGi: 100, ReserveCPUMilli: 2000},
	} {
		if _, err := BuildCapacityReport(ProfileMinimal, []byte(capacityFixture), []CapacityNode{node}); err == nil {
			t.Fatal("invalid reserve understated node consumption")
		}
	}
}

// External download cache and backup are reported as budgets but no longer
// consume node disk, so a lean node can plan a core-only runtime.
func TestCapacityExternalCacheAndBackupDoNotConsumeNodeDisk(t *testing.T) {
	nodes := []CapacityNode{{Name: "cp-1", CPUMilli: 16000, MemoryMi: 32768, DiskGi: 100}}
	r, err := BuildCapacityReportWithOptions(ProfileMinimal, []byte(capacityFixture), nodes,
		CapacityPlanningOptions{CacheSource: "external", BackupSource: "external"})
	if err != nil {
		t.Fatal(err)
	}
	if r.CacheSource != "external" || r.BackupSource != "external" {
		t.Fatalf("report lost placement: %+v", r)
	}
	// Requests 10Gi, no cache/backup on node; required disk = 10*1.3 + 20 reserve.
	if r.DownloadCacheGi != 20 || r.BackupGi != 10 {
		t.Fatalf("external budgets must still be reported: %+v", r)
	}
	if r.Required.DiskGi != (10*130+99)/100+20 {
		t.Fatalf("external placement must not add cache/backup to node disk: %+v", r)
	}
	if !r.FitsDeclaredCapacity {
		t.Fatalf("external placement should fit the declared node: %+v", r)
	}
}

// Local placement is the default and keeps the conservative full-delivery budget.
func TestCapacityLocalPlacementIsDefault(t *testing.T) {
	nodes := []CapacityNode{{Name: "cp-1", CPUMilli: 16000, MemoryMi: 32768, DiskGi: 200}}
	r, err := BuildCapacityReport(ProfileMinimal, []byte(capacityFixture), nodes)
	if err != nil {
		t.Fatal(err)
	}
	if r.CacheSource != "local" || r.BackupSource != "local" {
		t.Fatalf("default placement must be local: %+v", r)
	}
}

func TestCapacityRejectsUnknownPlacement(t *testing.T) {
	nodes := []CapacityNode{{Name: "cp-1", CPUMilli: 16000, MemoryMi: 32768, DiskGi: 200}}
	for _, opts := range []CapacityPlanningOptions{
		{CacheSource: "remote", BackupSource: "local"},
		{CacheSource: "local", BackupSource: "s3"},
	} {
		if _, err := BuildCapacityReportWithOptions(ProfileMinimal, []byte(capacityFixture), nodes, opts); err == nil {
			t.Fatal("unknown placement accepted")
		}
	}
}

// Startup scope plans a lean dev/test host: lower headroom and lower default
// reserve. It is not a production certification.
func TestCapacityStartupScopePlansLeanHost(t *testing.T) {
	// 2 vCPU / 4 GiB / 40 GiB declared node with external cache/backup.
	nodes := []CapacityNode{{Name: "cp-1", CPUMilli: 2000, MemoryMi: 4096, DiskGi: 40}}
	r, err := BuildCapacityReportWithOptions(ProfileMinimal, []byte(capacityFixture), nodes,
		CapacityPlanningOptions{CacheSource: "external", BackupSource: "external", Scope: "startup"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Scope != "startup" || r.HeadroomPercent != 10 {
		t.Fatalf("startup scope not applied: %+v", r)
	}
	// Default startup reserve: 500m / 1024Mi / 10Gi.
	if r.SystemReserve.CPUMilli != 500 || r.SystemReserve.MemoryMi != 1024 || r.SystemReserve.DiskGi != 10 {
		t.Fatalf("startup default reserve wrong: %+v", r.SystemReserve)
	}
	// Required CPU = 2000m requests *1.1 + 500 = 2700m > 2000m declared, so even
	// startup scope must not hide an over-committed node.
	if r.FitsDeclaredCapacity {
		t.Fatalf("startup scope must not conceal insufficient declared capacity: %+v", r)
	}
	if !strings.Contains(strings.Join(r.Reasons, ","), "DECLARED_CAPACITY_INSUFFICIENT") {
		t.Fatalf("expected explicit insufficiency reason: %+v", r)
	}
}

// Startup scope with a 4 vCPU / 8 GiB / 50 GiB node and external placement plans
// the calibrated Minimal footprint honestly.
func TestCapacityStartupScopeFitsCalibratedMinimal(t *testing.T) {
	nodes := []CapacityNode{{Name: "cp-1", CPUMilli: 4000, MemoryMi: 8192, DiskGi: 50,
		ReserveCPUMilli: 500, ReserveMemoryMi: 1024, ReserveDiskGi: 10}}
	// Simulate the calibrated Minimal: 1600m CPU / 2240Mi / 30Gi requests.
	manifest := strings.Replace(capacityFixture, "cpu: 500m", "cpu: 160m", 1)
	manifest = strings.Replace(manifest, "memory: 256Mi", "memory: 224Mi", 1)
	manifest = strings.Replace(manifest, "replicas: 2", "replicas: 1", 1)
	manifest = strings.Replace(manifest, "storage: 10Gi", "storage: 30Gi", 1)
	r, err := BuildCapacityReportWithOptions(ProfileMinimal, []byte(manifest), nodes,
		CapacityPlanningOptions{CacheSource: "external", BackupSource: "external", Scope: "startup"})
	if err != nil {
		t.Fatal(err)
	}
	if !r.FitsDeclaredCapacity {
		t.Fatalf("calibrated Minimal should fit a lean 4c/8Gi/50Gi node: %+v", r)
	}
}

func TestCapacityRejectsUnknownScope(t *testing.T) {
	nodes := []CapacityNode{{Name: "cp-1", CPUMilli: 16000, MemoryMi: 32768, DiskGi: 200}}
	if _, err := BuildCapacityReportWithOptions(ProfileMinimal, []byte(capacityFixture), nodes,
		CapacityPlanningOptions{Scope: "production"}); err == nil {
		t.Fatal("unknown scope accepted")
	}
}

// 安装授权是独立于容量估算的证据决策: 无授权输入时恒为 false，不新增原因。
func TestCapacityInstallAuthorizationDefaultsDenied(t *testing.T) {
	nodes := []CapacityNode{{Name: "cp-1", CPUMilli: 16000, MemoryMi: 32768, DiskGi: 200}}
	r, err := BuildCapacityReport(ProfileMinimal, []byte(capacityFixture), nodes)
	if err != nil {
		t.Fatal(err)
	}
	if r.InstallAuthorized {
		t.Fatalf("planning-only report must not authorize without evidence: %+v", r)
	}
	if !r.FitsDeclaredCapacity {
		t.Fatalf("fit is a planning fact independent of authorization: %+v", r)
	}
	for _, reason := range r.Reasons {
		if strings.HasPrefix(reason, "INSTALL_AUTHORIZATION_DENIED") || strings.HasPrefix(reason, "INSTALL_CAPACITY_BLOCKED") {
			t.Fatalf("no authorization input must not add denial reason: %v", r.Reasons)
		}
	}
}

// 只有 certified + material-verified + 容量足够时才能授权；任一缺失必须失败关闭。
func TestCapacityInstallAuthorizationRequiresAllEvidence(t *testing.T) {
	nodes := []CapacityNode{{Name: "cp-1", CPUMilli: 16000, MemoryMi: 32768, DiskGi: 200}}
	blocked := []string{"profile Minimal has no certified evidence", "route existing has no full platform delivery evidence"}
	denied, err := BuildCapacityReportWithOptions(ProfileMinimal, []byte(capacityFixture), nodes,
		CapacityPlanningOptions{Authorization: &InstallAuthorization{Certified: false, MaterialVerified: true, Rejections: blocked}})
	if err != nil {
		t.Fatal(err)
	}
	if denied.InstallAuthorized || !strings.Contains(strings.Join(denied.Reasons, ","), "INSTALL_AUTHORIZATION_DENIED") {
		t.Fatalf("denied evidence must stay unauthorized: %+v", denied)
	}

	unverified, err := BuildCapacityReportWithOptions(ProfileMinimal, []byte(capacityFixture), nodes,
		CapacityPlanningOptions{Authorization: &InstallAuthorization{Certified: true, MaterialVerified: false}})
	if err != nil {
		t.Fatal(err)
	}
	if unverified.InstallAuthorized || !strings.Contains(strings.Join(unverified.Reasons, ","), "OFFLINE_MATERIAL_UNVERIFIED") {
		t.Fatalf("unverified material must stay unauthorized: %+v", unverified)
	}

	// Capacity insufficient defeats otherwise-complete evidence.
	small, err := BuildCapacityReportWithOptions(ProfileMinimal, []byte(capacityFixture), []CapacityNode{{Name: "cp-1", CPUMilli: 2000, MemoryMi: 4096, DiskGi: 40}},
		CapacityPlanningOptions{Authorization: &InstallAuthorization{Certified: true, MaterialVerified: true}})
	if err != nil {
		t.Fatal(err)
	}
	if small.InstallAuthorized || !strings.Contains(strings.Join(small.Reasons, ","), "INSTALL_CAPACITY_BLOCKED") {
		t.Fatalf("insufficient capacity must defeat authorization: %+v", small)
	}

	authorized, err := BuildCapacityReportWithOptions(ProfileMinimal, []byte(capacityFixture), nodes,
		CapacityPlanningOptions{Authorization: &InstallAuthorization{Certified: true, MaterialVerified: true}})
	if err != nil {
		t.Fatal(err)
	}
	if !authorized.InstallAuthorized || !authorized.FitsDeclaredCapacity {
		t.Fatalf("complete evidence + fit must authorize: %+v", authorized)
	}
}

// A per-node reserve override lets a lean single-purpose host plan core runtime
// instead of the conservative 1-core/2GiB/20GiB default per node.
func TestCapacityNodeReserveOverride(t *testing.T) {
	nodes := []CapacityNode{{Name: "cp-1", CPUMilli: 8000, MemoryMi: 16384, DiskGi: 100,
		ReserveCPUMilli: 500, ReserveMemoryMi: 1024, ReserveDiskGi: 10}}
	r, err := BuildCapacityReport(ProfileMinimal, []byte(capacityFixture), nodes)
	if err != nil {
		t.Fatal(err)
	}
	if r.SystemReserve.CPUMilli != 500 || r.SystemReserve.MemoryMi != 1024 || r.SystemReserve.DiskGi != 10 {
		t.Fatalf("reserve override ignored: %+v", r.SystemReserve)
	}
}

// The disk floor accepts nodes down to 10Gi so real small cloud hosts can be
// planned; below that the node is still rejected as undersized.
func TestCapacityDiskFloorAllowsSmallHosts(t *testing.T) {
	for _, disk := range []int64{10, 11, 12, 20} {
		nodes := []CapacityNode{{Name: "cp-1", CPUMilli: 16000, MemoryMi: 32768, DiskGi: disk}}
		if _, err := BuildCapacityReport(ProfileMinimal, []byte(capacityFixture), nodes); err != nil {
			t.Fatalf("disk %dGi should be accepted: %v", disk, err)
		}
	}
	nodes := []CapacityNode{{Name: "cp-1", CPUMilli: 16000, MemoryMi: 32768, DiskGi: 9}}
	if _, err := BuildCapacityReport(ProfileMinimal, []byte(capacityFixture), nodes); err == nil {
		t.Fatal("disk below the floor must still be rejected as undersized")
	}
}
