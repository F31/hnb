package core

import (
	"fmt"
	"sync"
)

type ProviderType string

const (
	ProviderK8sDeploy       ProviderType = "k8s_deploy"
	ProviderContainerDeploy  ProviderType = "container_deploy"
	ProviderEdgeDeploy      ProviderType = "edge_deploy"
	ProviderHelm           ProviderType = "helm"
	ProviderTerraform      ProviderType = "terraform"
	ProviderCustom         ProviderType = "custom"
)

type ProviderEntry struct {
	ProviderID     string            `json:"provider_id"`
	ProviderType   ProviderType      `json:"provider_type"`
	RuntimeTarget  *RuntimeTarget    `json:"runtime_target"`
	Name           string            `json:"name"`
	Config         map[string]any    `json:"config"`
	CapabilityPack string            `json:"capability_pack,omitempty"`
	IsDefault      bool              `json:"is_default"`
	IsActive       bool              `json:"is_active"`
	Version        int64             `json:"version"`
}

type RegistryStore interface {
	Save(entry *ProviderEntry) error
	Delete(providerID string) error
	List() ([]ProviderEntry, error)
	Get(providerID string) (*ProviderEntry, error)
}

type ProviderRegistry struct {
	mu      sync.RWMutex
	entries map[string]*ProviderEntry
	targets map[string]*RuntimeTarget
	store   RegistryStore
}

func NewProviderRegistry(store RegistryStore) *ProviderRegistry {
	r := &ProviderRegistry{
		entries: make(map[string]*ProviderEntry),
		targets: make(map[string]*RuntimeTarget),
		store:   store,
	}
	if store != nil {
		r.load()
	}
	return r
}

func (r *ProviderRegistry) load() {
	entries, err := r.store.List()
	if err != nil {
		return
	}
	for i := range entries {
		e := &entries[i]
		r.entries[e.ProviderID] = e
	}
}

func (r *ProviderRegistry) RegisterProvider(entry *ProviderEntry) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if entry.RuntimeTarget == nil {
		return fmt.Errorf("runtime target required for provider %q", entry.ProviderID)
	}

	if _, exists := r.entries[entry.ProviderID]; exists {
		return fmt.Errorf("provider %q already registered", entry.ProviderID)
	}

	entry.Version = 1
	r.entries[entry.ProviderID] = entry
	r.targets[entry.RuntimeTarget.ID] = entry.RuntimeTarget

	if r.store != nil {
		if err := r.store.Save(entry); err != nil {
			return fmt.Errorf("persist provider: %w", err)
		}
	}
	return nil
}

func (r *ProviderRegistry) UnregisterProvider(providerID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, exists := r.entries[providerID]
	if !exists {
		return fmt.Errorf("provider %q not found", providerID)
	}

	delete(r.entries, providerID)
	if entry.RuntimeTarget != nil {
		delete(r.targets, entry.RuntimeTarget.ID)
	}

	if r.store != nil {
		if err := r.store.Delete(providerID); err != nil {
			return fmt.Errorf("remove persisted provider: %w", err)
		}
	}
	return nil
}

func (r *ProviderRegistry) GetProvider(providerID string) (*ProviderEntry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, exists := r.entries[providerID]
	if !exists {
		return nil, fmt.Errorf("provider %q not found", providerID)
	}
	return entry, nil
}

func (r *ProviderRegistry) ListProviders() []*ProviderEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*ProviderEntry, 0, len(r.entries))
	for _, entry := range r.entries {
		result = append(result, entry)
	}
	return result
}

func (r *ProviderRegistry) ListProvidersByType(pType ProviderType) []*ProviderEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*ProviderEntry
	for _, entry := range r.entries {
		if entry.ProviderType == pType && entry.IsActive {
			result = append(result, entry)
		}
	}
	return result
}

func (r *ProviderRegistry) ListProvidersByTarget(targetID string) []*ProviderEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*ProviderEntry
	for _, entry := range r.entries {
		if entry.RuntimeTarget != nil && entry.RuntimeTarget.ID == targetID && entry.IsActive {
			result = append(result, entry)
		}
	}
	return result
}

func (r *ProviderRegistry) CompareAndSwapProvider(providerID string, oldVersion int64, updateFn func(*ProviderEntry) *ProviderEntry) (*ProviderEntry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, exists := r.entries[providerID]
	if !exists {
		return nil, fmt.Errorf("provider %q not found", providerID)
	}
	if entry.Version != oldVersion {
		return nil, fmt.Errorf("provider %q version mismatch: expected %d, got %d", providerID, oldVersion, entry.Version)
	}

	entry = updateFn(entry)
	entry.Version++

	result := *entry

	if r.store != nil {
		if err := r.store.Save(entry); err != nil {
			return nil, fmt.Errorf("persist provider after CAS: %w", err)
		}
	}
	return &result, nil
}

func (r *ProviderRegistry) ResolveStepProvider(providerID string) (*ProviderEntry, *RuntimeTarget, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, exists := r.entries[providerID]
	if !exists {
		return nil, nil, fmt.Errorf("provider %q not found", providerID)
	}
	if !entry.IsActive {
		return nil, nil, fmt.Errorf("provider %q is inactive", providerID)
	}
	return entry, entry.RuntimeTarget, nil
}

func (r *ProviderRegistry) GetTarget(targetID string) (*RuntimeTarget, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	target, exists := r.targets[targetID]
	if !exists {
		return nil, fmt.Errorf("target %q not found", targetID)
	}
	return target, nil
}

func (r *ProviderRegistry) RegisterTarget(target *RuntimeTarget) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.targets[target.ID] = target
	return nil
}

func (r *ProviderRegistry) UnregisterTarget(targetID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.targets, targetID)
	return nil
}