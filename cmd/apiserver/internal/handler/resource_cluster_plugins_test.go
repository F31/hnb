package handler

import (
	"testing"
)

func TestParsePGTextArray(t *testing.T) {
	cases := []struct {
		raw  string
		want []string
	}{
		{"{}", nil},
		{"", nil},
		{"{ovn,calico}", []string{"ovn", "calico"}},
		{`{"cephfs","hostpath"}`, []string{"cephfs", "hostpath"}},
		{" {a, , b} ", []string{"a", "b"}},
		{"{single}", []string{"single"}},
	}
	for _, tc := range cases {
		got := parsePGTextArray([]byte(tc.raw))
		if len(got) != len(tc.want) {
			t.Fatalf("parsePGTextArray(%q) = %v, want %v", tc.raw, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("parsePGTextArray(%q) = %v, want %v", tc.raw, got, tc.want)
			}
		}
	}
}

func TestNormalizePluginStatus(t *testing.T) {
	cases := map[any]string{
		nil:             "unknown",
		"":              "unknown",
		"weird-state":   "unknown",
		"running":       "running",
		"Ready":         "running",
		true:            "running",
		"installed":     "installed",
		"enabled":       "installed",
		"not-installed": "not-installed",
		"absent":        "not-installed",
		false:           "not-installed",
		"degraded":      "abnormal",
		"failed":        "abnormal",
	}
	for in, want := range cases {
		if got := normalizePluginStatus(in); got != want {
			t.Errorf("normalizePluginStatus(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestPluginStatussFromSnapshot(t *testing.T) {
	raw := []byte(`{
		"plugins": [
			{"name": "metallb", "displayName": "metallb", "status": "running"},
			{"name": "gpu-agent", "status": "not-installed"},
			{"status": "running"}
		],
		"features": {
			"ipv6DualStack": true,
			"rdma": "absent"
		}
	}`)
	got := pluginStatussFromSnapshot(raw)
	if len(got) != 4 {
		t.Fatalf("got %d entries, want 4: %+v", len(got), got)
	}
	byKey := map[string]pluginStatus{}
	for _, p := range got {
		byKey[p.Key] = p
	}
	if byKey["plugin/metallb"].Status != "running" || byKey["plugin/metallb"].DisplayName != "metallb" {
		t.Errorf("metallb = %+v", byKey["plugin/metallb"])
	}
	if byKey["plugin/gpu-agent"].Status != "not-installed" || byKey["plugin/gpu-agent"].DisplayName != "gpu-agent" {
		t.Errorf("gpu-agent = %+v", byKey["plugin/gpu-agent"])
	}
	if byKey["feature/ipv6DualStack"].Status != "running" {
		t.Errorf("ipv6DualStack = %+v", byKey["feature/ipv6DualStack"])
	}
	if byKey["feature/rdma"].Status != "not-installed" {
		t.Errorf("rdma = %+v", byKey["feature/rdma"])
	}
}

func TestPluginStatussFromSnapshotInvalidJSON(t *testing.T) {
	if got := pluginStatussFromSnapshot([]byte("not-json")); len(got) != 0 {
		t.Fatalf("expected no entries, got %+v", got)
	}
	if got := pluginStatussFromSnapshot(nil); len(got) != 0 {
		t.Fatalf("expected no entries for nil, got %+v", got)
	}
}
