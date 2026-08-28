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

	"github.com/nats-io/nats.go/jetstream"

	"github.com/F31/hnb/pkg/messaging"
)

type RouteEntry struct {
	Action    string `json:"action"`
	Provider  string `json:"provider"`
	TargetSub string `json:"target_sub"`
}

var routes = []RouteEntry{
	{Action: "install", TargetSub: "hnb.network.%s.install"},
	{Action: "uninstall", TargetSub: "hnb.network.%s.uninstall"},
	{Action: "upgrade", TargetSub: "hnb.network.%s.upgrade"},
	{Action: "health", TargetSub: "hnb.network.%s.health"},
	{Action: "apply-policy", TargetSub: "hnb.network.%s.apply-policy"},
	{Action: "delete-policy", TargetSub: "hnb.network.%s.delete-policy"},
	{Action: "apply-ccnp", TargetSub: "hnb.network.%s.apply-ccnp"},
	{Action: "delete-ccnp", TargetSub: "hnb.network.%s.delete-ccnp"},
	{Action: "apply-cec", TargetSub: "hnb.network.%s.apply-cec"},
	{Action: "delete-cec", TargetSub: "hnb.network.%s.delete-cec"},
	{Action: "policy-trace", TargetSub: "hnb.network.%s.policy-trace"},
}

func main() {
	nc, err := messaging.ConnectNATSFromEnv(getEnv("NATS_URL", "nats://localhost:4222"))
	if err != nil {
		log.Fatalf("nats: %v", err)
	}
	defer nc.Close()

	js, err := jetstream.New(nc)
	if err != nil {
		log.Fatalf("jetstream: %v", err)
	}

	_, err = js.CreateOrUpdateStream(context.Background(), jetstream.StreamConfig{
		Name:       "NETWORK",
		Subjects:   []string{"hnb.network.>"},
		Storage:    jetstream.FileStorage,
		MaxMsgs:    10000,
		Discard:    jetstream.DiscardNew,
		MaxAge:     72 * time.Hour,
		Duplicates: 5 * time.Minute,
	})
	if err != nil {
		log.Fatalf("stream: %v", err)
	}

	cons, err := js.CreateOrUpdateConsumer(context.Background(), "NETWORK", jetstream.ConsumerConfig{
		Name:          "network-registry",
		Durable:       "network-registry",
		AckPolicy:     jetstream.AckExplicitPolicy,
		MaxDeliver:    3,
		BackOff:       []time.Duration{5 * time.Second, 30 * time.Second, 5 * time.Minute},
		FilterSubject: "hnb.network.>",
	})
	if err != nil {
		log.Fatalf("consumer: %v", err)
	}

	cc, err := cons.Consume(func(msg jetstream.Msg) {
		var req struct {
			Action   string `json:"action"`
			Provider string `json:"provider"`
		}
		if err := json.Unmarshal(msg.Data(), &req); err != nil {
			log.Printf("bad message: %v", err)
			msg.Nak()
			return
		}
		if req.Action == "" || req.Provider == "" {
			msg.Ack()
			return
		}

		for _, route := range routes {
			if route.Action == req.Action {
				target := fmt.Sprintf(route.TargetSub, req.Provider)
				_, pubErr := js.Publish(context.Background(), target, msg.Data())
				if pubErr != nil {
					log.Printf("route %s→%s failed: %v", msg.Subject(), target, err)
					msg.Nak()
					return
				}
				log.Printf("routed %s → %s", msg.Subject(), target)
				msg.Ack()
				return
			}
		}
		log.Printf("no route for action=%s", req.Action)
		msg.Ack()
	})
	if err != nil {
		log.Fatalf("consume: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() { <-sigCh; cancel() }()

	log.Println("network-registry starting")
	<-ctx.Done()
	cc.Stop()
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}
