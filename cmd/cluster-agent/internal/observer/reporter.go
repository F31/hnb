package observer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Reporter periodically emits Full/Delta observations to the platform ingest
// endpoint using the signed observer token. It applies exponential backoff on
// errors and never reads or logs secret material from observation payloads.
type Reporter struct {
	ingestURL          string
	tokenFile          string
	producer           *Producer
	discovery          *KubeDiscovery
	client             *http.Client
	statePath          string
	previousGeneration int64
	pendingPayload     []byte
}

func NewReporter(ingestURL, tokenFile string, producer *Producer, discovery *KubeDiscovery) *Reporter {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &Reporter{
		ingestURL: ingestURL,
		tokenFile: tokenFile,
		producer:  producer,
		discovery: discovery,
		client:    &http.Client{Timeout: 30 * time.Second, Transport: transport},
	}
}

// ConfigureState persists the next sequence per generation and performs the
// required source-reset when the configured generation is newer than the
// generation on disk. previousGeneration is zero for a fresh source.
func (r *Reporter) ConfigureState(path string, previousGeneration int64) {
	r.statePath = path
	r.previousGeneration = previousGeneration
}

// ReportOnce performs one observation cycle: discover nodes, storage, and capability, emit
// a Full observation on the first cycle and Delta thereafter, and POST it.
func (r *Reporter) ReportOnce(ctx context.Context) error {
	token, err := r.readToken()
	if err != nil {
		return fmt.Errorf("read observer token: %w", err)
	}
	observedAt := time.Now().UTC()
	if r.previousGeneration > 0 && r.previousGeneration < r.producer.Generation() {
		if err := r.resetSource(ctx, token, r.previousGeneration, r.producer.Generation()); err != nil {
			return fmt.Errorf("source reset: %w", err)
		}
		r.previousGeneration = 0
		r.pendingPayload = nil
	}
	if r.pendingPayload != nil {
		if err := r.post(ctx, token, r.pendingPayload); err != nil {
			return err
		}
		r.pendingPayload = nil
		return r.persistState()
	}
	nodes, err := r.discovery.DiscoverNodes(ctx, observedAt)
	if err != nil {
		return fmt.Errorf("discover nodes: %w", err)
	}
	capability, err := r.discovery.DiscoverCapability(ctx, nodes, observedAt)
	if err != nil {
		return fmt.Errorf("discover capability: %w", err)
	}
	storage, err := r.discovery.DiscoverStorageInventory(ctx, observedAt)
	if err != nil {
		return fmt.Errorf("discover storage inventory: %w", err)
	}
	target := &TargetState{
		LifecycleState:        "ACTIVE",
		HealthState:           "HEALTHY",
		ConnectivityState:     "CONNECTED",
		LastKnownStateAt:      observedAt,
		StaleThresholdSeconds: 300,
		RuntimeVersion:        capability.KubernetesVersion,
	}
	var payload []byte
	if len(r.producer.LastInventory()) == 0 {
		payload, err = r.producer.FullWithStorage(observedAt, target, capability, nodes, storage)
	} else {
		payload, err = r.producer.DeltaFromCacheWithStorage(observedAt, target, capability, nodes, storage)
	}
	if err != nil {
		return err
	}
	r.pendingPayload = payload
	if err := r.post(ctx, token, payload); err != nil {
		return err
	}
	r.pendingPayload = nil
	return r.persistState()
}

func (r *Reporter) persistState() error {
	if r.statePath == "" {
		return nil
	}
	if err := SaveState(r.statePath, State{Generation: r.producer.Generation(), NextSequence: r.producer.Sequence()}); err != nil {
		log.Printf("[observer] persist state failed: %v", err)
	}
	return nil
}

func (r *Reporter) resetSource(ctx context.Context, token string, previous, next int64) error {
	identity := r.producer.Identity()
	payload, err := json.Marshal(map[string]any{
		"schemaVersion": "1.0.0", "eventId": uuid.NewString(),
		"tenantId": identity.TenantID, "targetId": identity.TargetID,
		"targetKind": identity.TargetKind, "observerId": identity.ObserverID,
		"observerKind":               identity.ObserverKind,
		"previousObserverGeneration": previous, "newObserverGeneration": next,
		"observedAt": time.Now().UTC(), "observerLeaseId": identity.TargetID,
		"reason": "agent generation advancement",
	})
	if err != nil {
		return err
	}
	endpoint := strings.TrimRight(r.ingestURL, "/")
	endpoint = strings.TrimSuffix(endpoint, "/v1/observations") + "/v1/observations/reset"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("reset returned %d: %s", resp.StatusCode, string(body))
	}
	if r.statePath != "" {
		return SaveState(r.statePath, State{Generation: next, NextSequence: 1})
	}
	return nil
}

// Run reports on an interval with retry/backoff until ctx is cancelled.
func (r *Reporter) Run(ctx context.Context, interval time.Duration) {
	backoff := 5 * time.Second
	const maxBackoff = 2 * time.Minute
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
			if err := r.ReportOnce(ctx); err != nil {
				// Observations are retried with backoff; transient errors never
				// leak secret material into logs.
				log.Printf("[observer] report failed: %v", err)
				time.Sleep(backoff)
				if backoff < maxBackoff {
					backoff *= 2
				}
				continue
			}
			backoff = 5 * time.Second
		}
	}
}

func (r *Reporter) readToken() (string, error) {
	data, err := os.ReadFile(r.tokenFile)
	if err != nil {
		return "", err
	}
	return string(bytes.TrimSpace(data)), nil
}

func (r *Reporter) post(ctx context.Context, token string, payload []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.ingestURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := r.client.Do(req)
	if err != nil {
		return fmt.Errorf("post observation: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("ingest returned %d: %s", resp.StatusCode, string(body))
	}
	return nil
}
