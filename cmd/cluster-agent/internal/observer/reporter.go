package observer

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

// Reporter periodically emits Full/Delta observations to the platform ingest
// endpoint using the signed observer token. It applies exponential backoff on
// errors and never reads or logs secret material from observation payloads.
type Reporter struct {
	ingestURL string
	tokenFile string
	producer  *Producer
	discovery *KubeDiscovery
	client    *http.Client
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

// ReportOnce performs one observation cycle: discover nodes, storage, and capability, emit
// a Full observation on the first cycle and Delta thereafter, and POST it.
func (r *Reporter) ReportOnce(ctx context.Context) error {
	token, err := r.readToken()
	if err != nil {
		return fmt.Errorf("read observer token: %w", err)
	}
	observedAt := time.Now().UTC()
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
	if err := r.post(ctx, token, payload); err != nil {
		return err
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
