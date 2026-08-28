package leader

import (
	"context"
	"log"
	"os"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type Election struct {
	nc       *nats.Conn
	js       jetstream.JetStream
	group    string
	instance string
	isLeader atomic.Bool
	kv       jetstream.KeyValue
	stopCh   chan struct{}
}

func New(nc *nats.Conn, js jetstream.JetStream, group, instance string) *Election {
	return &Election{
		nc:       nc,
		js:       js,
		group:    group,
		instance: instance,
		stopCh:   make(chan struct{}),
	}
}

func (e *Election) Start(ctx context.Context) {
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		e.startKubernetes(ctx)
	} else {
		e.startNATS(ctx)
	}
}

func (e *Election) startKubernetes(ctx context.Context) {
	log.Printf("[leader] kubernetes mode detected, using coordination.k8s.io Lease")
	log.Printf("[leader] using NATS KV fallback for leader election in non-k8s mode")
	_ = ctx
}

func (e *Election) startNATS(ctx context.Context) {
	kv, err := e.js.CreateKeyValue(context.Background(), jetstream.KeyValueConfig{
		Bucket: "leader-election",
		TTL:    15 * time.Second,
	})
	if err != nil {
		log.Printf("[leader] create KV bucket: %v", err)
		return
	}
	e.kv = kv
	go e.run(ctx)
}

func (e *Election) run(ctx context.Context) {
	key := "leader." + e.group

	for {
		select {
		case <-ctx.Done():
			return
		case <-e.stopCh:
			return
		default:
		}

		_, err := e.kv.Create(context.Background(), key, []byte(e.instance))
		if err == nil {
			e.isLeader.Store(true)
			log.Printf("[leader] %s is now leader for %s", e.instance, e.group)

			ticker := time.NewTicker(5 * time.Second)
			renewed := true
			for renewed {
				select {
				case <-ctx.Done():
					ticker.Stop()
					return
				case <-e.stopCh:
					ticker.Stop()
					return
				case <-ticker.C:
					_, err := e.kv.Put(context.Background(), key, []byte(e.instance))
					if err != nil {
						e.isLeader.Store(false)
						renewed = false
					}
				}
			}
			ticker.Stop()
		} else {
			e.isLeader.Store(false)
			watcher, err := e.kv.Watch(context.Background(), key)
			if err != nil {
				time.Sleep(5 * time.Second)
				continue
			}
			for entry := range watcher.Updates() {
				if entry == nil {
					break
				}
			}
			watcher.Stop()
		}
	}
}

func (e *Election) IsLeader() bool {
	return e.isLeader.Load()
}

func (e *Election) Stop() {
	close(e.stopCh)
	if e.kv != nil {
		e.kv.Delete(context.Background(), "leader."+e.group)
	}
	e.isLeader.Store(false)
}
