package toolkit

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestProfileLevelLadderIsOrdered(t *testing.T) {
	shapes := ProfileShapeData()
	for i := 1; i < len(ProfileLevels); i++ {
		lower := shapes[profileForLevel(t, ProfileLevels[i-1])]
		higher := shapes[profileForLevel(t, ProfileLevels[i])]
		if problems := higher.SupersetOf(lower); len(problems) != 0 {
			t.Fatalf("%s must be a superset of %s: %v", higher.Name, lower.Name, problems)
		}
	}
}

func profileForLevel(t *testing.T, level ProfileLevel) Profile {
	t.Helper()
	for p, spec := range ProfileShapeData() {
		if spec.Level == level {
			return p
		}
	}
	t.Fatalf("no profile for level %d", level)
	return ""
}

func TestHigherShapeIsNotSupersetOfLowerInReverse(t *testing.T) {
	shapes := ProfileShapeData()
	minimal, lite := shapes[ProfileMinimal], shapes[ProfileLiteHA]
	if problems := minimal.SupersetOf(lite); len(problems) == 0 {
		t.Fatal("minimal must not be a superset of lite")
	}
}

func TestMinimalShapeCarriesCoreT0T1(t *testing.T) {
	minimal := ProfileShapeData()[ProfileMinimal]
	for _, component := range []string{"apiserver", "operation-worker", "platform-api", "portal", "app-market", "alert-manager", "cluster-agent", "network-registry", "cilium-provider"} {
		if minimal.Components[component] < 1 {
			t.Fatalf("minimal missing T0/T1 component %s", component)
		}
	}
	if minimal.HA.IndependentDomains != 1 || minimal.HA.Quorum != 1 {
		t.Fatalf("minimal HA must be single-domain: %+v", minimal.HA)
	}
	if minimal.Storage["postgresql"].SizeGi != 2 || minimal.Storage["nats"].SizeGi != 1 {
		t.Fatalf("minimal storage sizes wrong: %+v", minimal.Storage)
	}
}

func TestProfileShapeDigestIsStableAndDistinct(t *testing.T) {
	shapes := ProfileShapeData()
	seen := map[string]bool{}
	for name, spec := range shapes {
		d := spec.Digest()
		if d == "" || len(d) != len("sha256:")+64 {
			t.Fatalf("bad digest for %s: %q", name, d)
		}
		if again := spec.Digest(); again != d {
			t.Fatalf("digest not stable for %s", name)
		}
		if seen[d] {
			t.Fatalf("digest collision at %s", name)
		}
		seen[d] = true
	}
}

func TestBackingRankIsMonotonicAndSymmetric(t *testing.T) {
	if backingRank(BackingBundled) >= backingRank(BackingInClusterHA) {
		t.Fatal("in-cluster-ha must outrank bundled")
	}
	if backingRank(BackingInClusterHA) != backingRank(BackingExternal) {
		t.Fatal("in-cluster-ha and external must share the same HA rank")
	}
	shapes := ProfileShapeData()
	// minimal(bundled) -> lite(in-cluster-ha) is an upgrade, not a shrink.
	if problems := shapes[ProfileLiteHA].SupersetOf(shapes[ProfileMinimal]); len(problems) != 0 {
		t.Fatalf("bundled->in-cluster-ha must be monotonic: %v", problems)
	}
	// external and in-cluster-ha are both HA: neither is a downgrade.
	external := shapes[ProfileLiteHA]
	external.Backings = BackingExternal
	external.Storage = map[string]StorageProfile{}
	if problems := external.SupersetOf(shapes[ProfileLiteHA]); len(problems) != 0 {
		t.Fatalf("in-cluster-ha->external must be monotonic: %v", problems)
	}
	if problems := shapes[ProfileLiteHA].SupersetOf(external); len(problems) != 0 {
		t.Fatalf("external->in-cluster-ha must be monotonic: %v", problems)
	}
	// A downgrade to bundled is rejected.
	bundled := shapes[ProfileLiteHA]
	bundled.Backings = BackingBundled
	if problems := bundled.SupersetOf(shapes[ProfileLiteHA]); len(problems) == 0 {
		t.Fatal("in-cluster-ha->bundled must be rejected")
	}
}

func TestSupersetRejectsStorageShrinkAndReplicaDrop(t *testing.T) {
	shapes := ProfileShapeData()
	standard := shapes[ProfileStandardHA]
	shrunk := standard
	shrunk.HA = HAProfile{IndependentDomains: 1, Quorum: 1}
	if problems := shrunk.SupersetOf(standard); len(problems) == 0 {
		t.Fatal("domain shrink must be rejected")
	}
	reduced := standard
	reduced.Components = cloneInts(standard.Components)
	reduced.Components["apiserver"] = 1
	if problems := reduced.SupersetOf(standard); len(problems) == 0 {
		t.Fatal("replica drop must be rejected")
	}
}

func TestParseQuantities(t *testing.T) {
	if parseCPU("100m") != 100 || parseCPU("1") != 1000 || parseCPU("2") != 2000 {
		t.Fatal("cpu parsing wrong")
	}
	if parseMemory("192Mi") != 192 || parseMemory("1Gi") != 1024 || parseMemory("2Gi") != 2048 {
		t.Fatal("memory parsing wrong")
	}
}

// readOverlay parses a Chart overlay relative to the toolkit package. The Chart
// remains the single source of truth; this guards the Go vector against drift.
func readOverlay(t *testing.T, name string) map[string]any {
	t.Helper()
	path := filepath.Join("..", "..", "..", "..", "deploy", "charts", "hnb", "overlays", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("overlay %s unavailable: %v", name, err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return doc
}

func overlayComponent(doc map[string]any, name string) (replicas int, enabled bool, ok bool) {
	raw, ok := doc[name]
	if !ok {
		return 0, false, false
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return 0, false, false
	}
	enabled = true
	if e, ok := m["enabled"].(bool); ok {
		enabled = e
	}
	replicas = 1
	if r, ok := m["replicaCount"].(int); ok {
		replicas = r
	}
	return replicas, enabled, true
}

func TestMinimalShapeMatchesMinimalOverlay(t *testing.T) {
	doc := readOverlay(t, "minimal.yaml")
	minimal := ProfileShapeData()[ProfileMinimal]
	for name, raw := range doc {
		if name == "global" || name == "postgresql" || name == "nats" {
			continue
		}
		if _, isMap := raw.(map[string]any); !isMap {
			continue
		}
		replicas, enabled, _ := overlayComponent(doc, name)
		got, present := minimal.Components[name]
		if !enabled {
			if present {
				t.Fatalf("minimal enables disabled overlay component %s", name)
			}
			continue
		}
		if !present || got != replicas {
			t.Fatalf("minimal component %s replicas %d != overlay %d", name, got, replicas)
		}
	}
	pg := doc["postgresql"].(map[string]any)["storage"].(map[string]any)["size"].(string)
	if parseMemory(pg) != minimal.Storage["postgresql"].SizeGi*1024 {
		t.Fatalf("minimal postgresql size %v != overlay %s", minimal.Storage["postgresql"], pg)
	}
}

func TestHAOverlayReplicasMatchShape(t *testing.T) {
	shapes := ProfileShapeData()
	cases := []struct {
		overlay string
		profile Profile
		checks  map[string]int
	}{
		{"lite-ha.yaml", ProfileLiteHA, map[string]int{"apiserver": 3, "operation-worker": 3, "platform-api": 3, "tunnel-server": 3, "cluster-router": 3, "portal": 3}},
		{"standard-ha.yaml", ProfileStandardHA, map[string]int{"apiserver": 3, "plugin-worker": 3}},
		{"enterprise.yaml", ProfileEnterprise, map[string]int{"apiserver": 5, "operation-worker": 5, "platform-api": 5, "plugin-worker": 5}},
	}
	for _, c := range cases {
		doc := readOverlay(t, c.overlay)
		spec := shapes[c.profile]
		for name, want := range c.checks {
			replicas, enabled, ok := overlayComponent(doc, name)
			if !ok || !enabled {
				t.Fatalf("%s: %s not enabled in overlay", c.overlay, name)
			}
			if spec.Components[name] != replicas || replicas != want {
				t.Fatalf("%s: shape %s replicas %d, overlay %d, want %d", c.overlay, name, spec.Components[name], replicas, want)
			}
		}
	}
}

func TestOverlayBackingsMatchShape(t *testing.T) {
	cases := []struct {
		overlay string
		profile Profile
	}{
		{"minimal.yaml", ProfileMinimal},
		{"lite-ha.yaml", ProfileLiteHA},
		{"standard-ha.yaml", ProfileStandardHA},
		{"enterprise.yaml", ProfileEnterprise},
	}
	shapes := ProfileShapeData()
	for _, c := range cases {
		doc := readOverlay(t, c.overlay)
		global, _ := doc["global"].(map[string]any)
		if global == nil {
			t.Fatalf("%s has no global block", c.overlay)
		}
		backing, _ := global["backings"].(string)
		if backing == "" {
			backing = BackingBundled
		}
		if want := shapes[c.profile].Backings; backing != want {
			t.Fatalf("%s backing %q != shape %q", c.overlay, backing, want)
		}
	}
}
