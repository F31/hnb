package worker

import (
	"database/sql"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
)

func TestValidateTargetForHelmAcceptsActiveKubernetes(t *testing.T) {
	err := validateTargetForHelm(targetCredential{targetType: "kubernetes", status: "online", lifecycleState: "ACTIVE", connectivityState: "CONNECTED"})
	if err != nil {
		t.Fatalf("active kubernetes target rejected: %v", err)
	}
}

func TestValidateTargetForHelmRejectsUnsupportedTargets(t *testing.T) {
	for name, target := range map[string]targetCredential{
		"edge target":    {targetType: "edge_runtime", status: "online"},
		"offline":        {targetType: "kubernetes", status: "offline"},
		"terminated":     {targetType: "kubernetes", status: "online", lifecycleState: "TERMINATED"},
		"disconnected":   {targetType: "kubernetes", status: "online", connectivityState: "DISCONNECTED"},
		"decommissioned": {targetType: "kubernetes", status: "decommissioned"},
	} {
		t.Run(name, func(t *testing.T) {
			err := validateTargetForHelm(target)
			if err == nil || !strings.Contains(err.Error(), "does not support helm plugins") {
				t.Fatalf("expected unsupported target error, got %v", err)
			}
		})
	}
}

func TestTargetCredentialForReadsRuntimeTargetMetadata(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	w := New(db, nil, nil, "helm", t.TempDir(), false)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT rt.tenant_id,")).
		WithArgs("515eba09-0a41-5b92-b972-69af1f0f655c").
		WillReturnRows(sqlmock.NewRows([]string{"tenant_id", "target_type", "status", "lifecycle_state", "connectivity_state", "scope", "name"}).
			AddRow("tenant-a", "kubernetes", "online", "ACTIVE", "CONNECTED", "tenant:tenant-a", "kube-demo"))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COALESCE(kube_version, '')")).
		WithArgs("515eba09-0a41-5b92-b972-69af1f0f655c").
		WillReturnRows(sqlmock.NewRows([]string{"kube_version", "cpu_cores", "memory_mb", "storage_gb", "cni_plugins", "csi_drivers", "gpu_count", "features"}).
			AddRow("v1.29.3", 8, int64(16384), int64(100), pq.StringArray{"cilium"}, pq.StringArray{"ebs.csi.aws.com"}, 1, []byte(`{"gateway-api":true}`)))

	target, err := w.targetCredentialFor("515eba09-0a41-5b92-b972-69af1f0f655c")
	if err != nil {
		t.Fatalf("targetCredentialFor: %v", err)
	}
	if target.tenantID != "tenant-a" || target.scope != "tenant:tenant-a" || target.name != "kube-demo" {
		t.Fatalf("target = %+v", target)
	}
	if target.capability.kubeVersion != "v1.29.3" || target.capability.gpuCount != 1 || !target.capability.features["gateway-api"] {
		t.Fatalf("capability = %+v", target.capability)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestTargetCredentialForAllowsMissingCapabilitySnapshot(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	w := New(db, nil, nil, "helm", t.TempDir(), false)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT rt.tenant_id,")).
		WithArgs("515eba09-0a41-5b92-b972-69af1f0f655c").
		WillReturnRows(sqlmock.NewRows([]string{"tenant_id", "target_type", "status", "lifecycle_state", "connectivity_state", "scope", "name"}).
			AddRow("tenant-a", "kubernetes", "online", "ACTIVE", "CONNECTED", "", ""))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COALESCE(kube_version, '')")).
		WithArgs("515eba09-0a41-5b92-b972-69af1f0f655c").
		WillReturnError(sql.ErrNoRows)

	if _, err := w.targetCredentialFor("515eba09-0a41-5b92-b972-69af1f0f655c"); err != nil {
		t.Fatalf("targetCredentialFor without snapshot: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateTargetPreflight(t *testing.T) {
	capability := targetCapability{
		kubeVersion: "v1.29.3",
		cpuCores:    8,
		memoryMB:    16384,
		storageGB:   100,
		cniPlugins:  []string{"cilium"},
		csiDrivers:  []string{"ebs.csi.aws.com"},
		gpuCount:    1,
		features:    map[string]bool{"gateway-api": true},
	}
	preflight := map[string]any{
		"kubernetesMinVersion": "1.28.0",
		"kubernetesMaxVersion": "1.30.0",
		"requiresGPU":          true,
		"minCPUCores":          float64(4),
		"minMemoryMB":          float64(8192),
		"minStorageGB":         float64(50),
		"requiredCNIPlugins":   []any{"cilium"},
		"requiredCSIDrivers":   []any{"ebs.csi.aws.com"},
		"requiredFeatures":     []any{"gateway-api"},
	}
	if err := validateTargetPreflight(capability, preflight); err != nil {
		t.Fatalf("valid preflight rejected: %v", err)
	}
}

func TestHelmServerDryRunPreflightFlag(t *testing.T) {
	if !helmServerDryRunEnabled(map[string]any{"helmServerDryRun": true}) {
		t.Fatal("helmServerDryRun=true should enable server dry-run")
	}
	if helmServerDryRunEnabled(map[string]any{"helmServerDryRun": false}) || helmServerDryRunEnabled(nil) {
		t.Fatal("helmServerDryRun should be opt-in")
	}
}

func TestValidateTargetPreflightRejectsIncompatibleTarget(t *testing.T) {
	tests := map[string]struct {
		capability targetCapability
		preflight  map[string]any
		want       string
	}{
		"kube min": {targetCapability{kubeVersion: "v1.27.0"}, map[string]any{"kubernetesMinVersion": "1.28.0"}, "below required"},
		"gpu":      {targetCapability{}, map[string]any{"requiresGPU": true}, "GPU"},
		"cni":      {targetCapability{cniPlugins: []string{"calico"}}, map[string]any{"requiredCNIPlugins": []any{"cilium"}}, "CNI"},
		"csi":      {targetCapability{csiDrivers: []string{"other"}}, map[string]any{"requiredCSIDrivers": []any{"ebs.csi.aws.com"}}, "CSI"},
		"feature":  {targetCapability{features: map[string]bool{"gateway-api": false}}, map[string]any{"requiredFeatures": []any{"gateway-api"}}, "feature"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			err := validateTargetPreflight(tt.capability, tt.preflight)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected %q error, got %v", tt.want, err)
			}
		})
	}
}
