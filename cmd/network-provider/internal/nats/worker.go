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

	"github.com/F31/hnb/cmd/network-provider/internal/metrics"
	"github.com/F31/hnb/cmd/network-provider/internal/provider"
)

type Worker struct {
	js        jetstream.JetStream
	providers map[string]provider.NetworkProvider
	workerID  string
}

func NewWorker(nc *natslib.Conn, providers map[string]provider.NetworkProvider) (*Worker, error) {
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
	}, nil
}

type NetworkRequestMessage struct {
	OperationID  string `json:"operation_id"`
	Action       string `json:"action"`
	Provider     string `json:"provider"`
	ProfileJSON  string `json:"profile_json,omitempty"`
	PolicyJSON   string `json:"policy_json,omitempty"`
	PolicyName   string `json:"policy_name,omitempty"`
	PolicyNS     string `json:"policy_namespace,omitempty"`
	TraceJSON    string `json:"trace_json,omitempty"`
	TargetID     string `json:"target_id,omitempty"`
	Version      string `json:"version,omitempty"`
}

type NetworkResultMessage struct {
	OperationID string `json:"operation_id"`
	Status      string `json:"status"`
	Provider    string `json:"provider"`
	Error       string `json:"error,omitempty"`
	Message     string `json:"message,omitempty"`
	TraceResult string `json:"trace_result,omitempty"`
}

func (w *Worker) Start(ctx context.Context) error {
	log.Printf("[%s] starting network provider", w.workerID)

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
		FilterSubject: "hnb.network.>",
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

	log.Printf("[%s] listening for network requests on hnb.network.>", w.workerID)
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

	log.Printf("[%s] request: %s/%s (action=%s, provider=%s)", w.workerID, req.OperationID, req.Action, req.Action, req.Provider)

	prov, ok := w.providers[req.Provider]
	if !ok {
		log.Printf("[%s] unknown provider: %s", w.workerID, req.Provider)
		w.publishResult(ctx, msg.Subject()+".result", NetworkResultMessage{
			OperationID: req.OperationID,
			Status:      "failed",
			Provider:    req.Provider,
			Error:       fmt.Sprintf("unknown provider: %s", req.Provider),
		})
		msg.Ack()
		return
	}

	target := &provider.RuntimeTarget{
		ID: req.TargetID,
	}

	operationStart := time.Now()
	var err error
	switch req.Action {
	case "install":
		var profile provider.NetworkProfile
		if req.ProfileJSON != "" {
			if uerr := json.Unmarshal([]byte(req.ProfileJSON), &profile); uerr != nil {
				log.Printf("[%s] bad profile_json: %v", w.workerID, uerr)
				w.publishResult(ctx, msg.Subject()+".result", NetworkResultMessage{
					OperationID: req.OperationID,
					Status:      "failed",
					Provider:    req.Provider,
					Error:       fmt.Sprintf("invalid profile: %v", uerr),
				})
				msg.Ack()
				return
			}
		}
		err = prov.Install(ctx, &profile, target)

	case "uninstall":
		var profile provider.NetworkProfile
		if req.ProfileJSON != "" {
			if uerr := json.Unmarshal([]byte(req.ProfileJSON), &profile); uerr != nil {
				log.Printf("[%s] bad profile_json: %v", w.workerID, uerr)
				w.publishResult(ctx, msg.Subject()+".result", NetworkResultMessage{
					OperationID: req.OperationID,
					Status:      "failed",
					Provider:    req.Provider,
					Error:       fmt.Sprintf("invalid profile: %v", uerr),
				})
				msg.Ack()
				return
			}
		}
		err = prov.Uninstall(ctx, &profile, target)

	case "upgrade":
		var profile provider.NetworkProfile
		if req.ProfileJSON != "" {
			if uerr := json.Unmarshal([]byte(req.ProfileJSON), &profile); uerr != nil {
				log.Printf("[%s] bad profile_json: %v", w.workerID, uerr)
				w.publishResult(ctx, msg.Subject()+".result", NetworkResultMessage{
					OperationID: req.OperationID,
					Status:      "failed",
					Provider:    req.Provider,
					Error:       fmt.Sprintf("invalid profile: %v", uerr),
				})
				msg.Ack()
				return
			}
		}
		err = prov.Upgrade(ctx, &profile, target, req.Version)

	case "health":
		err = prov.Health(ctx, target)

	case "apply-policy":
		pm, ok := prov.(provider.NetworkPolicyManager)
		if !ok {
			log.Printf("[%s] provider %s does not support policy management", w.workerID, req.Provider)
			w.publishResult(ctx, msg.Subject()+".result", NetworkResultMessage{
				OperationID: req.OperationID,
				Status:      "failed",
				Provider:    req.Provider,
				Error:       fmt.Sprintf("provider %s does not support policy management", req.Provider),
			})
			msg.Ack()
			return
		}
		var policy provider.NetworkPolicy
		if req.PolicyJSON != "" {
			if uerr := json.Unmarshal([]byte(req.PolicyJSON), &policy); uerr != nil {
				log.Printf("[%s] bad policy_json: %v", w.workerID, uerr)
				w.publishResult(ctx, msg.Subject()+".result", NetworkResultMessage{
					OperationID: req.OperationID,
					Status:      "failed",
					Provider:    req.Provider,
					Error:       fmt.Sprintf("invalid policy: %v", uerr),
				})
				msg.Ack()
				return
			}
		}
		err = pm.ApplyNetworkPolicy(ctx, &policy, target)

	case "delete-policy":
		pm, ok := prov.(provider.NetworkPolicyManager)
		if !ok {
			log.Printf("[%s] provider %s does not support policy management", w.workerID, req.Provider)
			w.publishResult(ctx, msg.Subject()+".result", NetworkResultMessage{
				OperationID: req.OperationID,
				Status:      "failed",
				Provider:    req.Provider,
				Error:       fmt.Sprintf("provider %s does not support policy management", req.Provider),
			})
			msg.Ack()
			return
		}
		err = pm.DeleteNetworkPolicy(ctx, req.PolicyName, req.PolicyNS, target)

	case "policy-trace":
		pt, ok := prov.(provider.PolicyTracer)
		if !ok {
			log.Printf("[%s] provider %s does not support policy tracing", w.workerID, req.Provider)
			w.publishResult(ctx, msg.Subject()+".result", NetworkResultMessage{
				OperationID: req.OperationID,
				Status:      "failed",
				Provider:    req.Provider,
				Error:       fmt.Sprintf("provider %s does not support policy tracing", req.Provider),
			})
			msg.Ack()
			return
		}
		var traceReq provider.PolicyTraceRequest
		if req.TraceJSON != "" {
			if uerr := json.Unmarshal([]byte(req.TraceJSON), &traceReq); uerr != nil {
				log.Printf("[%s] bad trace_json: %v", w.workerID, uerr)
				w.publishResult(ctx, msg.Subject()+".result", NetworkResultMessage{
					OperationID: req.OperationID,
					Status:      "failed",
					Provider:    req.Provider,
					Error:       fmt.Sprintf("invalid trace request: %v", uerr),
				})
				msg.Ack()
				return
			}
		}
		traceResult, traceErr := pt.PolicyTrace(ctx, &traceReq, target)
		if traceErr != nil {
			err = traceErr
		} else {
			resultPayload, _ := json.Marshal(traceResult)
			w.publishResult(ctx, msg.Subject()+".result", NetworkResultMessage{
				OperationID: req.OperationID,
				Status:      "succeeded",
				Provider:    req.Provider,
				Message:     fmt.Sprintf("policy trace: %s", traceResult.Verdict),
				TraceResult: string(resultPayload),
			})
			msg.Ack()
			return
		}

	case "apply-ccnp":
		ccm, ok := prov.(provider.ClusterwidePolicyManager)
		if !ok {
			log.Printf("[%s] provider %s does not support clusterwide policy", w.workerID, req.Provider)
			w.publishResult(ctx, msg.Subject()+".result", NetworkResultMessage{
				OperationID: req.OperationID,
				Status:      "failed",
				Provider:    req.Provider,
				Error:       fmt.Sprintf("provider %s does not support clusterwide policy", req.Provider),
			})
			msg.Ack()
			return
		}
		var ccnpPolicy provider.NetworkPolicy
		if req.PolicyJSON != "" {
			if uerr := json.Unmarshal([]byte(req.PolicyJSON), &ccnpPolicy); uerr != nil {
				log.Printf("[%s] bad policy_json: %v", w.workerID, uerr)
				w.publishResult(ctx, msg.Subject()+".result", NetworkResultMessage{
					OperationID: req.OperationID,
					Status:      "failed",
					Provider:    req.Provider,
					Error:       fmt.Sprintf("invalid policy: %v", uerr),
				})
				msg.Ack()
				return
			}
		}
		err = ccm.ApplyClusterwidePolicy(ctx, &ccnpPolicy, target)

	case "delete-ccnp":
		ccm, ok := prov.(provider.ClusterwidePolicyManager)
		if !ok {
			log.Printf("[%s] provider %s does not support clusterwide policy", w.workerID, req.Provider)
			w.publishResult(ctx, msg.Subject()+".result", NetworkResultMessage{
				OperationID: req.OperationID,
				Status:      "failed",
				Provider:    req.Provider,
				Error:       fmt.Sprintf("provider %s does not support clusterwide policy", req.Provider),
			})
			msg.Ack()
			return
		}
		err = ccm.DeleteClusterwidePolicy(ctx, req.PolicyName, target)

	case "apply-cec":
		ecm, ok := prov.(provider.EnvoyConfigManager)
		if !ok {
			log.Printf("[%s] provider %s does not support envoy config", w.workerID, req.Provider)
			w.publishResult(ctx, msg.Subject()+".result", NetworkResultMessage{
				OperationID: req.OperationID,
				Status:      "failed",
				Provider:    req.Provider,
				Error:       fmt.Sprintf("provider %s does not support envoy config", req.Provider),
			})
			msg.Ack()
			return
		}
		var envoyCfg provider.EnvoyConfig
		if req.PolicyJSON != "" {
			if uerr := json.Unmarshal([]byte(req.PolicyJSON), &envoyCfg); uerr != nil {
				log.Printf("[%s] bad policy_json: %v", w.workerID, uerr)
				w.publishResult(ctx, msg.Subject()+".result", NetworkResultMessage{
					OperationID: req.OperationID,
					Status:      "failed",
					Provider:    req.Provider,
					Error:       fmt.Sprintf("invalid envoy config: %v", uerr),
				})
				msg.Ack()
				return
			}
		}
		err = ecm.ApplyEnvoyConfig(ctx, &envoyCfg, target)

	case "delete-cec":
		ecm, ok := prov.(provider.EnvoyConfigManager)
		if !ok {
			log.Printf("[%s] provider %s does not support envoy config", w.workerID, req.Provider)
			w.publishResult(ctx, msg.Subject()+".result", NetworkResultMessage{
				OperationID: req.OperationID,
				Status:      "failed",
				Provider:    req.Provider,
				Error:       fmt.Sprintf("provider %s does not support envoy config", req.Provider),
			})
			msg.Ack()
			return
		}
		err = ecm.DeleteEnvoyConfig(ctx, req.PolicyName, req.PolicyNS, target)

	default:
		log.Printf("[%s] unsupported action: %s", w.workerID, req.Action)
		msg.Nak()
		return
	}

	if err != nil {
		duration := time.Since(operationStart).Seconds()
		metrics.OperationDuration.WithLabelValues(req.Provider, req.Action, "failed").Observe(duration)
		log.Printf("[%s] %s failed: %v", w.workerID, req.Action, err)
		w.publishResult(ctx, msg.Subject()+".result", NetworkResultMessage{
			OperationID: req.OperationID,
			Status:      "failed",
			Provider:    req.Provider,
			Error:       err.Error(),
		})
		msg.Nak()
		return
	}

	log.Printf("[%s] %s succeeded: %s/%s", w.workerID, req.Action, req.OperationID, req.Provider)
	duration := time.Since(operationStart).Seconds()
	metrics.OperationDuration.WithLabelValues(req.Provider, req.Action, "succeeded").Observe(duration)
	w.publishResult(ctx, msg.Subject()+".result", NetworkResultMessage{
		OperationID: req.OperationID,
		Status:      "succeeded",
		Provider:    req.Provider,
		Message:     fmt.Sprintf("%s %s by %s", req.Action, req.Provider, w.workerID),
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