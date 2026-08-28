package core

import (
	"strings"
	"testing"
	"time"
)

func TestProviderManifestValidate(t *testing.T) {
	tests := []struct {
		name     string
		manifest *ProviderManifest
		wantErr  string
	}{
		{
			name:     "empty provider ID",
			manifest: &ProviderManifest{Name: "test", Version: "1.0.0", ProtocolVersion: "v1", Actions: []string{"validate"}},
			wantErr:  "provider_id",
		},
		{
			name:     "empty name",
			manifest: &ProviderManifest{ProviderID: "p1", Version: "1.0.0", ProtocolVersion: "v1", Actions: []string{"validate"}},
			wantErr:  "name",
		},
		{
			name:     "invalid semver",
			manifest: &ProviderManifest{ProviderID: "p1", Name: "test", Version: "latest", ProtocolVersion: "v1", Actions: []string{"validate"}},
			wantErr:  "semver",
		},
		{
			name:     "no actions",
			manifest: &ProviderManifest{ProviderID: "p1", Name: "test", Version: "1.0.0", ProtocolVersion: "v1"},
			wantErr:  "action",
		},
		{
			name:     "invalid action",
			manifest: &ProviderManifest{ProviderID: "p1", Name: "test", Version: "1.0.0", ProtocolVersion: "v1", Actions: []string{"invalid_action"}},
			wantErr:  "action",
		},
		{
			name:     "valid minimal manifest",
			manifest: &ProviderManifest{ProviderID: "p1", Name: "test", Version: "1.0.0", ProtocolVersion: "v1", Actions: []string{"validate"}},
			wantErr:  "",
		},
		{
			name:     "valid with all actions",
			manifest: &ProviderManifest{ProviderID: "p1", Name: "test", Version: "2.0.0", ProtocolVersion: "v2", Actions: []string{"validate", "plan", "provision", "observe", "update", "scale", "backup", "restore", "delete"}},
			wantErr:  "",
		},
		{
			name:     "invalid conformance level",
			manifest: &ProviderManifest{ProviderID: "p1", Name: "test", Version: "1.0.0", ProtocolVersion: "v1", Actions: []string{"validate"}, ConformanceLevel: "invalid"},
			wantErr:  "conformance_level",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.manifest.Validate()
			if tt.wantErr == "" && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantErr != "" && err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if tt.wantErr != "" && err != nil && !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestProviderManifestSupportsAction(t *testing.T) {
	m := &ProviderManifest{Actions: []string{"validate", "provision", "delete"}}
	if !m.SupportsAction("validate") {
		t.Fatal("expected validate to be supported")
	}
	if !m.SupportsAction("provision") {
		t.Fatal("expected provision to be supported")
	}
	if m.SupportsAction("restore") {
		t.Fatal("expected restore to NOT be supported")
	}
}

func TestProviderManifestConformance(t *testing.T) {
	now := time.Now()

	t.Run("none level always valid", func(t *testing.T) {
		m := &ProviderManifest{ConformanceLevel: ConformanceNone}
		if !m.IsConformanceValid() {
			t.Fatal("none conformance should always be valid")
		}
		if m.ConformanceSummary() != "not certified" {
			t.Fatalf("summary = %q", m.ConformanceSummary())
		}
	})

	t.Run("production_ready without expiry", func(t *testing.T) {
		m := &ProviderManifest{ConformanceLevel: ConformanceProductionReady}
		if !m.IsConformanceValid() {
			t.Fatal("production_ready without expiry should be valid")
		}
	})

	t.Run("expired conformance", func(t *testing.T) {
		expired := now.Add(-time.Hour)
		m := &ProviderManifest{ConformanceLevel: ConformanceProductionReady, ConformanceExpiresAt: &expired}
		if m.IsConformanceValid() {
			t.Fatal("expired conformance should be invalid")
		}
		if m.ConformanceSummary() != "production_ready (expired)" {
			t.Fatalf("summary = %q", m.ConformanceSummary())
		}
	})

	t.Run("valid conformance with future expiry", func(t *testing.T) {
		future := now.Add(24 * time.Hour)
		m := &ProviderManifest{ConformanceLevel: ConformanceBasic, ConformanceExpiresAt: &future}
		if !m.IsConformanceValid() {
			t.Fatal("conformance with future expiry should be valid")
		}
	})
}

func TestStorageDriverPackageValidation(t *testing.T) {
	now := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		mutate  func(*StorageDriverPackage)
		wantErr string
	}{
		{name: "valid"},
		{name: "package version mismatch", mutate: func(p *StorageDriverPackage) { p.PackageVersion = "1.1.0" }, wantErr: "match manifest version"},
		{name: "signature digest mismatch", mutate: func(p *StorageDriverPackage) { p.Signature.SignedDigest = "sha256:" + strings.Repeat("c", 64) }, wantErr: "bind packageDigest"},
		{name: "expired evidence", mutate: func(p *StorageDriverPackage) { p.ConformanceEvidence[0].ExpiresAt = now.Add(-time.Hour) }, wantErr: "expired"},
		{name: "evidence package mismatch", mutate: func(p *StorageDriverPackage) { p.ConformanceEvidence[0].PackageVersion = "1.1.0" }, wantErr: "does not match"},
		{name: "evidence outside Kubernetes range", mutate: func(p *StorageDriverPackage) { p.ConformanceEvidence[0].KubernetesVersion = "1.34.0" }, wantErr: "supported Kubernetes"},
		{name: "capacity none combined with source", mutate: func(p *StorageDriverPackage) { p.Capabilities.CapacityTracking = []string{"None", "Provider"} }, wantErr: "cannot be combined"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkg := validStorageDriverPackage(now)
			if tt.mutate != nil {
				tt.mutate(pkg)
			}
			err := pkg.Validate("1.0.0", now)
			if tt.wantErr == "" && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestProviderAndExtensionManifestsValidateStorageDriverPackage(t *testing.T) {
	now := time.Now()
	provider := &ProviderManifest{
		ProviderID: "storage.example", Name: "storage", Version: "1.0.0", ProtocolVersion: "v1",
		Actions: []string{"validate"}, StorageDriverPackage: validStorageDriverPackage(now),
	}
	if err := provider.Validate(); err != nil {
		t.Fatalf("provider manifest: %v", err)
	}
	extension := &ExtensionManifest{Name: "storage", Version: "1.0.0", Provider: "storage.example", StorageDriverPackage: validStorageDriverPackage(now)}
	if err := extension.Validate(); err != nil {
		t.Fatalf("extension manifest: %v", err)
	}
}

func validStorageDriverPackage(now time.Time) *StorageDriverPackage {
	digest := "sha256:" + strings.Repeat("a", 64)
	return &StorageDriverPackage{
		SchemaVersion: "1.0.0", PackageID: "example.csi.io/driver", PackageVersion: "1.0.0",
		Provisioners: []string{"example.csi.io"},
		Compatibility: StorageDriverCompatibility{
			KubernetesVersions:  []VersionRange{{MinInclusive: "1.30.0", MaxExclusive: "1.34.0"}},
			UpgradeFromVersions: []string{"0.9.0"}, RollbackToVersions: []string{"0.9.0"},
		},
		Capabilities: StorageDriverCapabilities{
			VolumeModes: []string{"Block", "File"}, AccessModes: []string{"ReadWriteOnce", "ReadWriteMany"},
			Topology: "Supported", CapacityTracking: []string{"CSIStorageCapacity"}, Expansion: "Supported",
			Clone: "Supported", Snapshot: "Supported", Ephemeral: "Unsupported", Health: "Supported",
		},
		RequiredComponents: StorageRequiredComponents{
			CRDs:        []ComponentRequirement{{Name: "volumesnapshots.snapshot.storage.k8s.io", Versions: []string{"v1"}}},
			Controllers: []ComponentRequirement{{Name: "example-csi-controller", Versions: []string{"1.0.0"}}},
		},
		PackageDigest: digest,
		Signature:     PackageSignature{Format: "Cosign", KeyID: "release-key", SignedDigest: digest, EvidenceRef: "oci://signature"},
		ConformanceEvidence: []StorageConformanceEvidence{{
			PackageVersion: "1.0.0", KubernetesVersion: "1.32.0", SuiteVersion: "1.0.0",
			PassedAt: now.Add(-time.Hour), ExpiresAt: now.Add(24 * time.Hour), EvidenceRef: "oci://evidence",
			EvidenceDigest: "sha256:" + strings.Repeat("b", 64),
		}},
	}
}
