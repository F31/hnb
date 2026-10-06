package extension

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/F31/hnb/pkg/core"
	"github.com/nats-io/nats.go"
)

func TestExtensionRequestPayloadIncludesHelmConfig(t *testing.T) {
	ext := &core.Extension{
		ID:       "ext-1",
		Name:     "prometheus-operator",
		Version:  "55.5.0",
		TargetID: "target-1",
		Manifest: core.ExtensionManifest{
			Config: map[string]any{
				"helm": map[string]any{
					"chart":        "kube-prometheus-stack",
					"chartVersion": "55.5.0",
					"repoName":     "prometheus-community",
					"repoURL":      "https://prometheus-community.github.io/helm-charts",
					"namespace":    "monitoring",
					"values": map[string]any{
						"grafana": map[string]any{"enabled": false},
					},
					"valuesSchema": map[string]any{"required": []any{"grafana"}},
					"preflight":    map[string]any{"kubernetesMinVersion": "1.28.0"},
				},
			},
		},
	}

	payload := extensionRequestPayload(ext, "monitoring", "install", ext.Version, "")
	for _, key := range []string{"chart", "chartVersion", "repoName", "repoURL", "namespace", "values", "valuesSchema", "preflight"} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("payload missing %s: %+v", key, payload)
		}
	}
	if payload["provider"] != "monitoring" || payload["target_id"] != "target-1" {
		t.Fatalf("unexpected payload metadata: %+v", payload)
	}
}

func TestExtensionManagerUninstallKeepsExtensionOnProviderFailure(t *testing.T) {
	store := &fakeExtensionStore{}
	manager := NewExtensionManager(store, nil, nil)
	manager.request = func(subject string, data []byte, timeout time.Duration) (*nats.Msg, error) {
		return nil, errors.New("no responders")
	}
	ext := &core.Extension{ID: "ext-1", Name: "longhorn", Version: "1.12.1", Phase: core.ExtReady}

	err := manager.Uninstall(context.Background(), ext)
	if err == nil {
		t.Fatal("expected uninstall error")
	}
	if store.deleted {
		t.Fatal("extension should not be deleted on provider failure")
	}
	if store.phase != core.ExtDegraded || store.lastError == "" {
		t.Fatalf("phase=%s lastError=%q, want degraded with error", store.phase, store.lastError)
	}
}

func TestExtensionManagerUninstallDeletesAfterProviderSuccess(t *testing.T) {
	store := &fakeExtensionStore{}
	manager := NewExtensionManager(store, nil, nil)
	manager.request = func(subject string, data []byte, timeout time.Duration) (*nats.Msg, error) {
		return &nats.Msg{Data: []byte(`{"status":"succeeded","message":"helm-uninstall succeeded"}`)}, nil
	}
	ext := &core.Extension{ID: "ext-1", Name: "longhorn", Version: "1.12.1", Phase: core.ExtReady}

	if err := manager.Uninstall(context.Background(), ext); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if !store.deleted {
		t.Fatal("extension should be deleted after provider success")
	}
}

type fakeExtensionStore struct {
	phase     core.ExtensionPhase
	lastError string
	deleted   bool
}

func (s *fakeExtensionStore) Get(id string) (*core.Extension, error) { return nil, nil }
func (s *fakeExtensionStore) List() ([]core.Extension, error)        { return nil, nil }
func (s *fakeExtensionStore) Create(ext *core.Extension) error       { return nil }
func (s *fakeExtensionStore) UpdatePhase(id string, phase core.ExtensionPhase, healthFailures int, lastError string) error {
	s.phase = phase
	s.lastError = lastError
	return nil
}
func (s *fakeExtensionStore) Delete(id string) error {
	s.deleted = true
	return nil
}
