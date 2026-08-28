package network

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	natslib "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	opDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "hnb_cni_operation_duration_seconds",
		Help:    "Duration of CNI provider operations",
		Buckets: prometheus.DefBuckets,
	}, []string{"provider", "operation", "status"})
)

type Worker struct {
	js        jetstream.JetStream
	providers map[string]NetworkProvider
	workerID  string
	subject   string
}

func NewWorker(nc *natslib.Conn, providers map[string]NetworkProvider, subject string) (*Worker, error) {
	js, err := jetstream.New(nc)
	if err != nil {
		return nil, fmt.Errorf("jetstream: %w", err)
	}

	id := uuid.New().String()
	if len(id) > 8 {
		id = id[:8]
	}

	return &Worker{
		js:        js,
		providers: providers,
		workerID:  fmt.Sprintf("net-provider-%s", id),
		subject:   subject,
	}, nil
}

func (w *Worker) Start(ctx context.Context) error {
	log.Printf("[%s] starting network provider on %s", w.workerID, w.subject)

	_, err := w.js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:        "NETWORK",
		Subjects:    []string{"hnb.network.>"},
		Storage:     jetstream.FileStorage,
		MaxMsgs:     10000,
		Discard:     jetstream.DiscardNew,
		MaxAge:      72 * time.Hour,
		Duplicates:  5 * time.Minute,
	})
	if err != nil {
		return fmt.Errorf("stream: %w", err)
	}

	cons, err := w.js.CreateOrUpdateConsumer(ctx, "NETWORK", jetstream.ConsumerConfig{
		Name:         fmt.Sprintf("network-worker-%s", w.workerID),
		Durable:      fmt.Sprintf("network-worker-%s", w.workerID),
		AckPolicy:    jetstream.AckExplicitPolicy,
		MaxDeliver:   3,
		BackOff:      []time.Duration{5 * time.Second, 30 * time.Second, 5 * time.Minute},
		FilterSubject: w.subject,
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

	log.Printf("[%s] listening on %s", w.workerID, w.subject)
	<-ctx.Done()
	cc.Stop()
	return nil
}

func (w *Worker) handleMessage(ctx context.Context, msg jetstream.Msg) {
	var req NetworkRequestMessage
	if err := json.Unmarshal(msg.Data(), &req); err != nil {
		log.Printf("[%s] bad message: %v", w.workerID, err)
		msg.Nak()
		return
	}

	log.Printf("[%s] request: %s/%s (action=%s)", w.workerID, req.OperationID, req.Action, req.Action)

	prov, ok := w.providers[req.Provider]
	if !ok {
		log.Printf("[%s] unknown provider: %s", w.workerID, req.Provider)
		w.publishResult(ctx, msg.Subject()+".result", NetworkResultMessage{
			OperationID: req.OperationID, Status: "failed", Provider: req.Provider,
			Error: fmt.Sprintf("unknown provider: %s", req.Provider),
		})
		msg.Ack()
		return
	}

	target := &RuntimeTarget{ID: req.TargetID}
	operationStart := time.Now()
	var err error

	switch req.Action {
	case "install":
		var profile NetworkProfile
		if req.ProfileJSON != "" {
			if uerr := json.Unmarshal([]byte(req.ProfileJSON), &profile); uerr != nil {
				w.fail(ctx, operationStart, msg, req, "invalid profile: %v", uerr)
				return
			}
		}
		err = prov.Install(ctx, &profile, target)

	case "uninstall":
		var profile NetworkProfile
		if req.ProfileJSON != "" {
			if uerr := json.Unmarshal([]byte(req.ProfileJSON), &profile); uerr != nil {
				w.fail(ctx, operationStart, msg, req, "invalid profile: %v", uerr)
				return
			}
		}
		err = prov.Uninstall(ctx, &profile, target)

	case "upgrade":
		var profile NetworkProfile
		if req.ProfileJSON != "" {
			if uerr := json.Unmarshal([]byte(req.ProfileJSON), &profile); uerr != nil {
				w.fail(ctx, operationStart, msg, req, "invalid profile: %v", uerr)
				return
			}
		}
		err = prov.Upgrade(ctx, &profile, target, req.Version)

	case "health":
		err = prov.Health(ctx, target)

	case "apply-policy":
		if pm, ok := prov.(NetworkPolicyManager); ok {
			var policy NetworkPolicy
			if req.PolicyJSON != "" {
				if uerr := json.Unmarshal([]byte(req.PolicyJSON), &policy); uerr != nil {
					w.fail(ctx, operationStart, msg, req, "invalid policy: %v", uerr)
					return
				}
			}
			err = pm.ApplyNetworkPolicy(ctx, &policy, target)
		} else {
			w.fail(ctx, operationStart, msg, req, "provider does not support policy management")
			return
		}

	case "delete-policy":
		if pm, ok := prov.(NetworkPolicyManager); ok {
			err = pm.DeleteNetworkPolicy(ctx, req.PolicyName, req.PolicyNS, target)
		} else {
			w.fail(ctx, operationStart, msg, req, "provider does not support policy management")
			return
		}

	case "apply-ccnp":
		if ccm, ok := prov.(ClusterwidePolicyManager); ok {
			var policy NetworkPolicy
			if req.PolicyJSON != "" {
				if uerr := json.Unmarshal([]byte(req.PolicyJSON), &policy); uerr != nil {
					w.fail(ctx, operationStart, msg, req, "invalid policy: %v", uerr)
					return
				}
			}
			err = ccm.ApplyClusterwidePolicy(ctx, &policy, target)
		} else {
			w.fail(ctx, operationStart, msg, req, "provider does not support clusterwide policy")
			return
		}

	case "delete-ccnp":
		if ccm, ok := prov.(ClusterwidePolicyManager); ok {
			err = ccm.DeleteClusterwidePolicy(ctx, req.PolicyName, target)
		} else {
			w.fail(ctx, operationStart, msg, req, "provider does not support clusterwide policy")
			return
		}

	case "apply-cec":
		if ecm, ok := prov.(EnvoyConfigManager); ok {
			var cfg EnvoyConfig
			if req.PolicyJSON != "" {
				if uerr := json.Unmarshal([]byte(req.PolicyJSON), &cfg); uerr != nil {
					w.fail(ctx, operationStart, msg, req, "invalid envoy config: %v", uerr)
					return
				}
			}
			err = ecm.ApplyEnvoyConfig(ctx, &cfg, target)
		} else {
			w.fail(ctx, operationStart, msg, req, "provider does not support envoy config")
			return
		}

	case "delete-cec":
		if ecm, ok := prov.(EnvoyConfigManager); ok {
			err = ecm.DeleteEnvoyConfig(ctx, req.PolicyName, req.PolicyNS, target)
		} else {
			w.fail(ctx, operationStart, msg, req, "provider does not support envoy config")
			return
		}

	case "policy-trace":
		if pt, ok := prov.(PolicyTracer); ok {
			var traceReq PolicyTraceRequest
			if req.TraceJSON != "" {
				if uerr := json.Unmarshal([]byte(req.TraceJSON), &traceReq); uerr != nil {
					w.fail(ctx, operationStart, msg, req, "invalid trace: %v", uerr)
					return
				}
			}
			traceResult, traceErr := pt.PolicyTrace(ctx, &traceReq, target)
			if traceErr != nil {
				err = traceErr
			} else {
				resultPayload, _ := json.Marshal(traceResult)
				opDuration.WithLabelValues(req.Provider, req.Action, "succeeded").Observe(time.Since(operationStart).Seconds())
				w.publishResult(ctx, msg.Subject()+".result", NetworkResultMessage{
					OperationID: req.OperationID, Status: "succeeded", Provider: req.Provider,
					Message: fmt.Sprintf("policy trace: %s", traceResult.Verdict), TraceResult: string(resultPayload),
				})
				msg.Ack()
				return
			}
		} else {
			w.fail(ctx, operationStart, msg, req, "provider does not support policy tracing")
			return
		}

	default:
		log.Printf("[%s] unsupported action: %s", w.workerID, req.Action)
		msg.Nak()
		return
	}

	if err != nil {
		opDuration.WithLabelValues(req.Provider, req.Action, "failed").Observe(time.Since(operationStart).Seconds())
		log.Printf("[%s] %s failed: %v", w.workerID, req.Action, err)
		w.publishResult(ctx, msg.Subject()+".result", NetworkResultMessage{
			OperationID: req.OperationID, Status: "failed", Provider: req.Provider, Error: err.Error(),
		})
		msg.Nak()
		return
	}

	opDuration.WithLabelValues(req.Provider, req.Action, "succeeded").Observe(time.Since(operationStart).Seconds())
	log.Printf("[%s] %s succeeded", w.workerID, req.Action)
	w.publishResult(ctx, msg.Subject()+".result", NetworkResultMessage{
		OperationID: req.OperationID, Status: "succeeded", Provider: req.Provider,
		Message: fmt.Sprintf("%s %s by %s", req.Action, req.Provider, w.workerID),
	})
	msg.Ack()
}

func (w *Worker) fail(ctx context.Context, start time.Time, msg jetstream.Msg, req NetworkRequestMessage, format string, args ...any) {
	opDuration.WithLabelValues(req.Provider, req.Action, "failed").Observe(time.Since(start).Seconds())
	log.Printf("[%s] %s failed: %s", w.workerID, req.Action, fmt.Sprintf(format, args...))
	w.publishResult(ctx, msg.Subject()+".result", NetworkResultMessage{
		OperationID: req.OperationID, Status: "failed", Provider: req.Provider,
		Error: fmt.Sprintf(format, args...),
	})
	msg.Ack()
}

func (w *Worker) publishResult(ctx context.Context, subject string, data any) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = w.js.Publish(ctx, subject, payload)
	return err
}