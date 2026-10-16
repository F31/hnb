package toolkit

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func renderFixture(t *testing.T) (*DeploymentConfig, *ReleaseLock, string, []byte) {
	t.Helper()
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	files := map[string]string{
		"hnb/Chart.yaml":            "apiVersion: v2\nname: hnb\nversion: 1.0.0\n",
		"hnb/overlays/startup.yaml": "global: {deploymentStage: startup}\n",
		"hnb/overlays/minimal.yaml": "global: {deploymentStage: full}\n",
		"hnb/templates/deployment.yaml": `apiVersion: apps/v1
kind: Deployment
metadata: {name: apiserver}
spec:
  selector: {matchLabels: {app: api}}
  template:
    metadata: {labels: {app: api}}
    spec:
      containers:
        - name: api
          image: {{ .Values.apiserver.image.repository }}@{{ .Values.apiserver.image.digest }}
          env:
            - name: DB_DSN
              valueFrom:
                secretKeyRef:
                  name: {{ .Values.apiserver.db.existingSecret }}
                  key: {{ .Values.apiserver.db.secretKey }}
`,
	}
	for name, contents := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: int64(len(contents)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(contents)); err != nil {
			t.Fatal(err)
		}
	}
	tw.Close()
	gz.Close()
	path := filepath.Join(t.TempDir(), "hnb.tgz")
	if err := os.WriteFile(path, archive.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := &DeploymentConfig{Metadata: Metadata{Name: "hnb"}, Spec: ConfigSpec{Mode: ModeExisting,
		Profile: ProfileMinimal, Platform: PlatformConfig{Namespace: "hnb-system",
			Infrastructure: InfraConfig{Planes: []PlaneConfig{{Name: "platform", PostgreSQLSecretRef: "hnb-system/platform-db", NATSSecretRef: "platform-nats"}}}}}}
	lock := &ReleaseLock{Charts: []PinnedChart{{Name: "hnb", Version: "1.0.0", Digest: "sha256:" + sha256Hex(archive.Bytes())}},
		Images: []PinnedImage{{Name: "apiserver", Ref: "registry.test:5000/hnb/api:1.0", ManifestDigest: "sha256:" + strings.Repeat("a", 64), Platform: "linux/amd64"}}}
	return cfg, lock, path, []byte("apiserver:\n  db: {existingSecret: stale, secretKey: stale}\n")
}

func TestRenderUsesLockedPackageImagesPlaneSecretsAndNamespace(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("Helm unavailable")
	}
	cfg, lock, chart, bindings := renderFixture(t)
	first, err := RenderLockedChart(context.Background(), cfg, lock, chart, bindings, "startup")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(first, &doc); err != nil {
		t.Fatal(err)
	}
	if doc["metadata"].(map[string]any)["namespace"] != "hnb-system" {
		t.Fatal("namespace not bound")
	}
	if !bytes.Contains(first, []byte("registry.test:5000/hnb/api@")) || !bytes.Contains(first, []byte("name: platform-db")) {
		t.Fatalf("locked bindings missing: %s", first)
	}
	second, err := RenderLockedChart(context.Background(), cfg, lock, chart, bindings, "startup")
	if err != nil || !bytes.Equal(first, second) {
		t.Fatalf("not deterministic: %v", err)
	}
	output := filepath.Join(t.TempDir(), "render.yaml")
	if err := WriteRenderedManifest(output, first); err != nil {
		t.Fatal(err)
	}
	if err := WriteRenderedManifest(output, second); err != nil {
		t.Fatal(err)
	}
	if err := WriteRenderedManifest(output, []byte("changed")); err == nil {
		t.Fatal("different output replaced")
	}
}

func TestRenderRefusesWrongPackageInlineCredentialsAndCrossNamespace(t *testing.T) {
	cfg, lock, chart, bindings := renderFixture(t)
	lock.Charts[0].Digest = "sha256:" + strings.Repeat("b", 64)
	if _, err := RenderLockedChart(context.Background(), cfg, lock, chart, bindings, "startup"); err == nil {
		t.Fatal("unlocked chart accepted")
	}
	cfg, lock, chart, _ = renderFixture(t)
	if _, err := RenderLockedChart(context.Background(), cfg, lock, chart, []byte("apiserver: {db: {dsn: secret}}"), "startup"); err == nil {
		t.Fatal("inline DSN accepted")
	}
	cfg.Spec.Platform.Infrastructure.Planes[0].PostgreSQLSecretRef = "other/db"
	if _, err := RenderLockedChart(context.Background(), cfg, lock, chart, bindings, "startup"); err == nil {
		t.Fatal("cross namespace secret accepted")
	}
}

func TestExistingPlanStartsInfrastructureBeforeMigration(t *testing.T) {
	cfg, lock, _, _ := renderFixture(t)
	cfg.Spec.Cluster.Strategy = StrategyExisting
	cfg.Spec.Platform.Storage.ClassName = "local-path"
	cfg.Spec.Platform.Infrastructure.PostgreSQL.Mode = "bundled"
	cfg.Spec.Platform.Infrastructure.NATS.Mode = "bundled"
	plan, problems := cfg.PlanExistingDependencies(lock)
	if len(problems) != 0 {
		t.Fatal(problems)
	}
	positions := map[PhaseName]int{}
	for i, stage := range plan.Stages {
		positions[stage.Phase] = i
	}
	if positions[PhaseInfrastructure] >= positions[PhaseDatabaseMigrate] || positions[PhaseDatabaseMigrate] >= positions[PhasePlatformRelease] {
		t.Fatal("dependency stages are out of order")
	}
}
