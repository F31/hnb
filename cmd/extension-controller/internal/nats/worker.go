package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	gnats "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/F31/hnb/cmd/extension-controller/internal/lifecycle"
	"github.com/F31/hnb/pkg/core"
	"github.com/F31/hnb/pkg/extension"
)

type Worker struct {
	manager   *extension.ExtensionManager
	lifecycle *lifecycle.Service
	nc        *gnats.Conn
	js        jetstream.JetStream
}

func NewWorker(manager *extension.ExtensionManager, nc *gnats.Conn, js jetstream.JetStream) *Worker {
	return &Worker{
		manager: manager,
		nc:      nc,
		js:      js,
	}
}

func NewWorkerWithLifecycle(manager *extension.ExtensionManager, lifecycleService *lifecycle.Service, nc *gnats.Conn, js jetstream.JetStream) *Worker {
	return &Worker{
		manager:   manager,
		lifecycle: lifecycleService,
		nc:        nc,
		js:        js,
	}
}

func (w *Worker) Start(ctx context.Context) error {
	log.Println("[ext-worker] starting extension worker")

	subs := []struct {
		subject string
		handler func(context.Context, []byte) ([]byte, error)
	}{
		{"hnb.extension.install", w.handleInstall},
		{"hnb.extension.upgrade", w.handleUpgrade},
		{"hnb.extension.uninstall", w.handleUninstall},
		{"hnb.extension.health", w.handleHealth},
	}
	if w.lifecycle != nil {
		subs = append(subs, struct {
			subject string
			handler func(context.Context, []byte) ([]byte, error)
		}{lifecycle.SubjectLifecycleRequested, w.handleProviderLifecycle})
	}

	for _, s := range subs {
		sub := s
		if _, err := w.nc.Subscribe(sub.subject, func(msg *gnats.Msg) {
			result, err := sub.handler(ctx, msg.Data)
			if err != nil {
				log.Printf("[ext-worker] %s error: %v", sub.subject, err)
				if err := msg.Respond(errorResponse(err.Error())); err != nil {
					log.Printf("[ext-worker] respond error: %v", err)
				}
				return
			}
			if err := msg.Respond(result); err != nil {
				log.Printf("[ext-worker] respond: %v", err)
			}
		}); err != nil {
			return fmt.Errorf("subscribe %s: %w", sub.subject, err)
		}
		log.Printf("[ext-worker] subscribed to %s", sub.subject)
	}

	<-ctx.Done()
	log.Println("[ext-worker] stopped")
	return nil
}

func (w *Worker) handleProviderLifecycle(ctx context.Context, data []byte) ([]byte, error) {
	if w.lifecycle == nil {
		return nil, fmt.Errorf("provider lifecycle service is not configured")
	}
	var cmd lifecycle.Command
	if err := json.Unmarshal(data, &cmd); err != nil {
		return nil, fmt.Errorf("invalid provider lifecycle command: %w", err)
	}
	event, err := w.lifecycle.Reconcile(ctx, cmd)
	if err != nil {
		return nil, err
	}
	return json.Marshal(event)
}

type extensionRequest struct {
	ExtensionID  string            `json:"extension_id,omitempty"`
	Name         string            `json:"name"`
	Version      string            `json:"version,omitempty"`
	PrevVersion  string            `json:"prev_version,omitempty"`
	Provider     string            `json:"provider,omitempty"`
	ProviderType string            `json:"provider_type,omitempty"`
	WorkspaceID  string            `json:"workspace_id,omitempty"`
	TargetID     string            `json:"target_id,omitempty"`
	Manifest     json.RawMessage   `json:"manifest,omitempty"`
	Labels       map[string]string `json:"labels,omitempty"`
}

func (w *Worker) handleInstall(ctx context.Context, data []byte) ([]byte, error) {
	var req extensionRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	if req.Name == "" {
		return nil, fmt.Errorf("name is required")
	}

	ext := &core.Extension{
		ID:      req.ExtensionID,
		Name:    req.Name,
		Version: req.Version,
		Phase:   core.ExtPending,
		Labels:  req.Labels,
	}

	if req.Manifest != nil {
		if err := json.Unmarshal(req.Manifest, &ext.Manifest); err != nil {
			return nil, fmt.Errorf("invalid manifest: %w", err)
		}
	}

	if ext.ID == "" {
		return nil, fmt.Errorf("extension_id is required")
	}

	if err := w.manager.Install(ctx, ext); err != nil {
		return nil, err
	}

	return successResponse("installed"), nil
}

func (w *Worker) handleUpgrade(ctx context.Context, data []byte) ([]byte, error) {
	var req extensionRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	if req.ExtensionID == "" {
		return nil, fmt.Errorf("extension_id is required")
	}
	if req.Version == "" {
		return nil, fmt.Errorf("version is required")
	}

	ext, err := w.manager.Get(ctx, req.ExtensionID)
	if err != nil {
		return nil, fmt.Errorf("extension not found: %w", err)
	}

	if err := w.manager.Upgrade(ctx, ext, req.Version); err != nil {
		return nil, err
	}

	return successResponse("upgraded"), nil
}

func (w *Worker) handleUninstall(ctx context.Context, data []byte) ([]byte, error) {
	var req extensionRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	if req.ExtensionID == "" {
		return nil, fmt.Errorf("extension_id is required")
	}

	ext, err := w.manager.Get(ctx, req.ExtensionID)
	if err != nil {
		return nil, fmt.Errorf("extension not found: %w", err)
	}

	if err := w.manager.Uninstall(ctx, ext); err != nil {
		return nil, err
	}

	return successResponse("uninstalled"), nil
}

func (w *Worker) handleHealth(ctx context.Context, data []byte) ([]byte, error) {
	var req extensionRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	ext, err := w.manager.Get(ctx, req.ExtensionID)
	if err != nil {
		return nil, fmt.Errorf("extension not found: %w", err)
	}

	healthy, err := w.manager.HealthCheck(ctx, ext)
	if err != nil {
		return nil, fmt.Errorf("health check failed: %w", err)
	}

	if healthy {
		return successResponse("healthy"), nil
	}
	return errorResponse("unhealthy"), nil
}

func successResponse(message string) []byte {
	data, _ := json.Marshal(map[string]string{
		"status":  "succeeded",
		"message": message,
	})
	return data
}

func errorResponse(message string) []byte {
	data, _ := json.Marshal(map[string]string{
		"status":  "failed",
		"message": message,
	})
	return data
}

func init() {
	// Ensure timezone loaded
	_ = time.Now()
}
