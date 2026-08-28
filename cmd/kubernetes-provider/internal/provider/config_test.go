package provider

import "testing"

func TestLoadConfig(t *testing.T) {
	t.Setenv("ALLOWED_NAMESPACES", "hnb-e2e,hnb-workloads")
	t.Setenv("MAX_REPLICAS", "5")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.MaxReplicas != 5 {
		t.Fatalf("max replicas = %d", cfg.MaxReplicas)
	}
	if _, ok := cfg.AllowedNamespaces["hnb-e2e"]; !ok {
		t.Fatal("hnb-e2e missing")
	}
}

func TestLoadConfigRequiresNamespaces(t *testing.T) {
	t.Setenv("ALLOWED_NAMESPACES", "")
	if _, err := LoadConfig(); err == nil {
		t.Fatal("expected error")
	}
}
