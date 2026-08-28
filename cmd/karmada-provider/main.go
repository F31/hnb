package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	natslib "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/F31/hnb/cmd/karmada-provider/internal/provider"
)

type Config struct {
	NATSURL    string
	Kubeconfig string
	WorkerID   string
}

func loadConfig() *Config {
	return &Config{
		NATSURL:    getEnv("NATS_URL", "nats://localhost:4222"),
		Kubeconfig: getEnv("KUBECONFIG", ""),
		WorkerID:   fmt.Sprintf("karmada-%s", uuid.New().String()[:8]),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

type StepRequest struct {
	OperationID    string `json:"operation_id"`
	StepID         string `json:"step_id"`
	StepType       string `json:"step_type"`
	IdempotencyKey string `json:"idempotency_key"`
	TenantID       string `json:"tenant_id"`
	Namespace      string `json:"namespace"`
	ResourceName   string `json:"resource_name"`
	ClusterLabels  map[string]string `json:"cluster_labels"`
	Placement      string `json:"placement"`
}

func main() {
	cfg := loadConfig()

	nc, err := natslib.Connect(cfg.NATSURL)
	if err != nil {
		log.Fatalf("nats: %v", err)
	}
	defer nc.Close()

	prop, err := provider.NewPropagator(cfg.Kubeconfig)
	if err != nil {
		log.Fatalf("propagator: %v", err)
	}

	js, err := jetstream.New(nc)
	if err != nil {
		log.Fatalf("jetstream: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	_, err = js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:        "KARMADA",
		Subjects:    []string{"hnb.karmada.*"},
		Storage:     jetstream.FileStorage,
		MaxMsgs:     10000,
		MaxAge:      72 * time.Hour,
		Duplicates:  5 * time.Minute,
	})
	if err != nil {
		log.Fatalf("stream: %v", err)
	}

	cons, err := js.CreateOrUpdateConsumer(ctx, "KARMADA", jetstream.ConsumerConfig{
		Name:         fmt.Sprintf("karmada-worker-%s", cfg.WorkerID),
		Durable:      fmt.Sprintf("karmada-worker-%s", cfg.WorkerID),
		AckPolicy:    jetstream.AckExplicitPolicy,
		MaxDeliver:   3,
		BackOff:      []time.Duration{5 * time.Second, 30 * time.Second, 5 * time.Minute},
		FilterSubject: "hnb.karmada.propagate",
	})
	if err != nil {
		log.Fatalf("consumer: %v", err)
	}

	cc, err := cons.Consume(func(msg jetstream.Msg) {
		var req StepRequest
		if err := json.Unmarshal(msg.Data(), &req); err != nil {
			log.Printf("bad message: %v", err)
			msg.Nak()
			return
		}

		log.Printf("propagate: %s/%s to clusters with labels %v", req.Namespace, req.ResourceName, req.ClusterLabels)

		if err := prop.ApplyPropagationPolicy(ctx, req.ResourceName, req.Namespace, req.ClusterLabels, req.Placement); err != nil {
			log.Printf("propagate error: %v", err)
			msg.Nak()
			return
		}

		payload, _ := json.Marshal(map[string]any{
			"operation_id":  req.OperationID,
			"step_id":       req.StepID,
			"status":        "succeeded",
			"resource_name": req.ResourceName,
			"namespace":     req.Namespace,
		})
		_, _ = js.Publish(ctx, "hnb.karmada.propagated", payload)
		msg.Ack()
	})
	if err != nil {
		log.Fatalf("consume: %v", err)
	}

	log.Printf("karmada-provider [%s] started", cfg.WorkerID)
	<-ctx.Done()
	cc.Stop()
}