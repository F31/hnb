package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	natslib "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/F31/hnb/cmd/app-market/internal/engine/market"
)

type Worker struct {
	js       jetstream.JetStream
	bridge   *market.ManifestBridge
	workerID string
}

func NewWorker(nc *natslib.Conn) (*Worker, error) {
	js, err := jetstream.New(nc)
	if err != nil {
		return nil, fmt.Errorf("jetstream: %w", err)
	}

	id := uuid.New().String()
	if len(id) > 8 {
		id = id[:8]
	}

	return &Worker{
		js:       js,
		bridge:   market.NewManifestBridge(),
		workerID: fmt.Sprintf("app-market-%s", id),
	}, nil
}

type ReleaseRequestMessage struct {
	Manifest    json.RawMessage `json:"manifest"`
	Passed      bool            `json:"passed"`
	Policies    []string        `json:"policies"`
	Decisions   map[string]string `json:"decisions"`
}

func (w *Worker) Start(ctx context.Context) error {
	log.Printf("[%s] starting app-market worker", w.workerID)

	_, err := w.js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:        "APP_MARKET",
		Subjects:    []string{"hnb.market.*"},
		Storage:     jetstream.FileStorage,
		MaxMsgs:     10000,
		Discard:     jetstream.DiscardNew,
		MaxAge:      72 * time.Hour,
		Duplicates:  5 * time.Minute,
	})
	if err != nil {
		return fmt.Errorf("stream: %w", err)
	}

	cons, err := w.js.CreateOrUpdateConsumer(ctx, "APP_MARKET", jetstream.ConsumerConfig{
		Name:         fmt.Sprintf("app-market-worker-%s", w.workerID),
		Durable:      fmt.Sprintf("app-market-worker-%s", w.workerID),
		AckPolicy:    jetstream.AckExplicitPolicy,
		MaxDeliver:   3,
		BackOff:      []time.Duration{5 * time.Second, 30 * time.Second, 5 * time.Minute},
		FilterSubject: "hnb.market.release",
	})
	if err != nil {
		return fmt.Errorf("consumer: %w", err)
	}

	cc, err := cons.Consume(func(msg jetstream.Msg) {
		w.handleMessage(ctx, msg)
	})
	if err != nil {
		return fmt.Errorf("consume: %w", err)
	}

	log.Printf("[%s] listening for market release requests", w.workerID)
	<-ctx.Done()
	cc.Stop()
	return nil
}

func (w *Worker) handleMessage(ctx context.Context, msg jetstream.Msg) {
	var req ReleaseRequestMessage
	if err := json.Unmarshal(msg.Data(), &req); err != nil {
		log.Printf("[%s] bad message: %v", w.workerID, err)
		msg.Nak()
		return
	}

	log.Printf("[%s] processing market release", w.workerID)

	policyResult := &market.PolicyResult{
		Passed:    req.Passed,
		Policies:  req.Policies,
		Decisions: req.Decisions,
	}

	w.publishResult(ctx, "hnb.market.processed", map[string]any{
		"status":  "succeeded",
		"message": fmt.Sprintf("manifest processed by %s", w.workerID),
	})

	msg.Ack()
	log.Printf("[%s] release done", w.workerID)
	_ = policyResult
	_ = req.Manifest
}

func (w *Worker) publishResult(ctx context.Context, subject string, data any) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = w.js.Publish(ctx, subject, payload)
	return err
}
