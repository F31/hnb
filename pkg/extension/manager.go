package extension

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/F31/hnb/pkg/core"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	extInstallTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "hnb_extension_install_total",
		Help: "Total extension install operations",
	}, []string{"extension", "status"})

	extUpgradeTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "hnb_extension_upgrade_total",
		Help: "Total extension upgrade operations",
	}, []string{"extension", "status"})

	extUninstallTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "hnb_extension_uninstall_total",
		Help: "Total extension uninstall operations",
	}, []string{"extension", "status"})

	extHealthTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "hnb_extension_health_total",
		Help: "Total extension health checks",
	}, []string{"extension", "status"})

	extPhaseTransitions = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "hnb_extension_phase_transitions_total",
		Help: "Extension phase transitions",
	}, []string{"extension", "from_phase", "to_phase"})
)

type ExtensionStore interface {
	Get(id string) (*core.Extension, error)
	List() ([]core.Extension, error)
	Create(ext *core.Extension) error
	UpdatePhase(id string, phase core.ExtensionPhase, healthFailures int, lastError string) error
	Delete(id string) error
}

type ExtensionManager struct {
	store   ExtensionStore
	nc      *nats.Conn
	js      jetstream.JetStream
	request providerRequester
}

type providerRequester func(subject string, data []byte, timeout time.Duration) (*nats.Msg, error)

func NewExtensionManager(store ExtensionStore, nc *nats.Conn, js jetstream.JetStream) *ExtensionManager {
	manager := &ExtensionManager{
		store: store,
		nc:    nc,
		js:    js,
	}
	if nc != nil {
		manager.request = nc.Request
	}
	return manager
}

func (m *ExtensionManager) Install(ctx context.Context, ext *core.Extension) error {
	log.Printf("[extension] installing %s v%s", ext.Name, ext.Version)

	if err := m.store.UpdatePhase(ext.ID, core.ExtInstalling, 0, ""); err != nil {
		return fmt.Errorf("update phase to installing: %w", err)
	}
	extPhaseTransitions.WithLabelValues(ext.Name, string(core.ExtPending), string(core.ExtInstalling)).Inc()

	provider := extensionProvider(ext)
	payload := extensionRequestPayload(ext, provider, "install", ext.Version, "")

	subject := fmt.Sprintf("hnb.extension.provider.%s.install", provider)
	msg, err := m.requestProvider(subject, mustMarshal(payload), 5*time.Minute)
	if err != nil {
		m.fail(ctx, ext, fmt.Sprintf("provider request failed: %v", err))
		extInstallTotal.WithLabelValues(ext.Name, "failed").Inc()
		return fmt.Errorf("provider request: %w", err)
	}

	var result struct {
		Status  string          `json:"status"`
		Message string          `json:"message,omitempty"`
		Stage   string          `json:"stage,omitempty"`
		Stages  []providerStage `json:"stages,omitempty"`
	}
	if err := unmarshal(msg.Data, &result); err != nil {
		m.fail(ctx, ext, fmt.Sprintf("bad provider response: %v", err))
		extInstallTotal.WithLabelValues(ext.Name, "failed").Inc()
		return err
	}
	logProviderStages(ext.Name, result.Stages)

	if result.Status != "succeeded" {
		m.fail(ctx, ext, result.Message)
		extInstallTotal.WithLabelValues(ext.Name, "failed").Inc()
		return fmt.Errorf("provider install failed: %s", result.Message)
	}

	if err := m.store.UpdatePhase(ext.ID, core.ExtReady, 0, ""); err != nil {
		return fmt.Errorf("update phase to ready: %w", err)
	}
	extPhaseTransitions.WithLabelValues(ext.Name, string(core.ExtInstalling), string(core.ExtReady)).Inc()
	extInstallTotal.WithLabelValues(ext.Name, "succeeded").Inc()

	log.Printf("[extension] %s v%s installed successfully", ext.Name, ext.Version)
	return nil
}

func (m *ExtensionManager) Upgrade(ctx context.Context, ext *core.Extension, newVersion string) error {
	log.Printf("[extension] upgrading %s from %s to %s", ext.Name, ext.Version, newVersion)

	prevVersion := ext.Version
	ext.Version = newVersion

	if err := m.store.UpdatePhase(ext.ID, core.ExtInstalling, 0, ""); err != nil {
		return fmt.Errorf("update phase to installing: %w", err)
	}
	extPhaseTransitions.WithLabelValues(ext.Name, string(core.ExtReady), string(core.ExtInstalling)).Inc()

	provider := extensionProvider(ext)
	payload := extensionRequestPayload(ext, provider, "upgrade", newVersion, prevVersion)

	subject := fmt.Sprintf("hnb.extension.provider.%s.upgrade", provider)
	msg, err := m.requestProvider(subject, mustMarshal(payload), 5*time.Minute)
	if err != nil {
		m.fail(ctx, ext, fmt.Sprintf("provider upgrade request failed: %v", err))
		extUpgradeTotal.WithLabelValues(ext.Name, "failed").Inc()
		return fmt.Errorf("provider request: %w", err)
	}

	var result struct {
		Status  string          `json:"status"`
		Message string          `json:"message,omitempty"`
		Stage   string          `json:"stage,omitempty"`
		Stages  []providerStage `json:"stages,omitempty"`
	}
	if err := unmarshal(msg.Data, &result); err != nil {
		m.fail(ctx, ext, fmt.Sprintf("bad provider response: %v", err))
		extUpgradeTotal.WithLabelValues(ext.Name, "failed").Inc()
		return err
	}
	logProviderStages(ext.Name, result.Stages)

	if result.Status != "succeeded" {
		m.fail(ctx, ext, result.Message)
		extUpgradeTotal.WithLabelValues(ext.Name, "failed").Inc()
		return fmt.Errorf("provider upgrade failed: %s", result.Message)
	}

	if err := m.store.UpdatePhase(ext.ID, core.ExtReady, 0, ""); err != nil {
		return fmt.Errorf("update phase to ready: %w", err)
	}
	extPhaseTransitions.WithLabelValues(ext.Name, string(core.ExtInstalling), string(core.ExtReady)).Inc()
	extUpgradeTotal.WithLabelValues(ext.Name, "succeeded").Inc()

	log.Printf("[extension] %s upgraded to %s successfully", ext.Name, newVersion)
	return nil
}

func (m *ExtensionManager) Uninstall(ctx context.Context, ext *core.Extension) error {
	log.Printf("[extension] uninstalling %s v%s", ext.Name, ext.Version)

	if err := m.store.UpdatePhase(ext.ID, core.ExtUninstalling, 0, ""); err != nil {
		return fmt.Errorf("update phase to uninstalling: %w", err)
	}
	extPhaseTransitions.WithLabelValues(ext.Name, string(ext.Phase), string(core.ExtUninstalling)).Inc()

	provider := extensionProvider(ext)
	payload := extensionRequestPayload(ext, provider, "uninstall", ext.Version, "")

	subject := fmt.Sprintf("hnb.extension.provider.%s.uninstall", provider)
	msg, err := m.requestProvider(subject, mustMarshal(payload), 5*time.Minute)
	if err != nil {
		errMsg := fmt.Sprintf("provider uninstall request failed: %v", err)
		m.fail(ctx, ext, errMsg)
		extUninstallTotal.WithLabelValues(ext.Name, "failed").Inc()
		return fmt.Errorf("provider uninstall request: %w", err)
	}

	var result struct {
		Status  string          `json:"status"`
		Message string          `json:"message,omitempty"`
		Stage   string          `json:"stage,omitempty"`
		Stages  []providerStage `json:"stages,omitempty"`
	}
	if err := unmarshal(msg.Data, &result); err != nil {
		errMsg := fmt.Sprintf("bad provider uninstall response: %v", err)
		m.fail(ctx, ext, errMsg)
		extUninstallTotal.WithLabelValues(ext.Name, "failed").Inc()
		return fmt.Errorf("bad provider uninstall response: %w", err)
	}
	logProviderStages(ext.Name, result.Stages)
	if result.Status != "succeeded" {
		m.fail(ctx, ext, result.Message)
		extUninstallTotal.WithLabelValues(ext.Name, "failed").Inc()
		return fmt.Errorf("provider uninstall failed: %s", result.Message)
	}

	if err := m.store.Delete(ext.ID); err != nil {
		return fmt.Errorf("delete extension: %w", err)
	}
	extUninstallTotal.WithLabelValues(ext.Name, "succeeded").Inc()

	log.Printf("[extension] %s uninstalled successfully", ext.Name)
	return nil
}

func (m *ExtensionManager) HealthCheck(ctx context.Context, ext *core.Extension) (bool, error) {
	provider := extensionProvider(ext)
	payload := extensionRequestPayload(ext, provider, "health", ext.Version, "")

	subject := fmt.Sprintf("hnb.extension.provider.%s.health", provider)
	msg, err := m.requestProvider(subject, mustMarshal(payload), 30*time.Second)
	if err != nil {
		extHealthTotal.WithLabelValues(ext.Name, "unreachable").Inc()
		return false, fmt.Errorf("provider health check unreachable: %w", err)
	}

	var result struct {
		Status  string          `json:"status"`
		Message string          `json:"message,omitempty"`
		Stage   string          `json:"stage,omitempty"`
		Stages  []providerStage `json:"stages,omitempty"`
	}
	if err := unmarshal(msg.Data, &result); err != nil {
		extHealthTotal.WithLabelValues(ext.Name, "bad_response").Inc()
		return false, fmt.Errorf("bad health response: %w", err)
	}
	logProviderStages(ext.Name, result.Stages)

	healthy := result.Status == "succeeded"
	if healthy {
		extHealthTotal.WithLabelValues(ext.Name, "healthy").Inc()
	} else {
		extHealthTotal.WithLabelValues(ext.Name, "unhealthy").Inc()
	}
	return healthy, nil
}

func (m *ExtensionManager) Get(ctx context.Context, id string) (*core.Extension, error) {
	return m.store.Get(id)
}

func (m *ExtensionManager) fail(ctx context.Context, ext *core.Extension, errMsg string) {
	log.Printf("[extension] %s failed: %s", ext.Name, errMsg)
	if err := m.store.UpdatePhase(ext.ID, core.ExtDegraded, ext.HealthFailures+1, errMsg); err != nil {
		log.Printf("[extension] update phase to degraded: %v", err)
	}
	extPhaseTransitions.WithLabelValues(ext.Name, string(core.ExtInstalling), string(core.ExtDegraded)).Inc()
}

func (m *ExtensionManager) requestProvider(subject string, data []byte, timeout time.Duration) (*nats.Msg, error) {
	if m.request == nil {
		return nil, fmt.Errorf("provider request transport is not configured")
	}
	return m.request(subject, data, timeout)
}

type providerStage struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

func logProviderStages(extensionName string, stages []providerStage) {
	for _, stage := range stages {
		if stage.Message != "" {
			log.Printf("[extension] %s provider stage %s=%s: %s", extensionName, stage.Name, stage.Status, stage.Message)
			continue
		}
		log.Printf("[extension] %s provider stage %s=%s", extensionName, stage.Name, stage.Status)
	}
}

func extensionProvider(ext *core.Extension) string {
	if ext.Manifest.Provider != "" {
		return ext.Manifest.Provider
	}
	return string(ext.ProviderType)
}

func extensionRequestPayload(ext *core.Extension, provider, action, version, prevVersion string) map[string]any {
	payload := map[string]any{
		"extension_id": ext.ID,
		"name":         ext.Name,
		"version":      version,
		"provider":     provider,
		"action":       action,
		"target_id":    ext.TargetID,
	}
	if prevVersion != "" {
		payload["prev_version"] = prevVersion
	}
	if helmConfig, ok := ext.Manifest.Config["helm"].(map[string]any); ok {
		for _, key := range []string{"chart", "chartVersion", "repoName", "repoURL", "namespace", "values", "valuesSchema", "preflight"} {
			if value, exists := helmConfig[key]; exists {
				payload[key] = value
			}
		}
	}
	return payload
}

func mustMarshal(v any) []byte {
	data, _ := json.Marshal(v)
	return data
}

func unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}
