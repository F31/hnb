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
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/F31/hnb/cmd/gateway-provider/internal/engine/gateway"
	"github.com/F31/hnb/cmd/gateway-provider/internal/metrics"
)

type Worker struct {
	js       jetstream.JetStream
	executor *gateway.GatewayExecutor
	applier  *gateway.K8sApplier
	adapter  gateway.GatewayAdapter
	workerID string
}

func NewWorker(nc *natslib.Conn, kubeconfig, gatewayAdapter string) (*Worker, error) {
	js, err := jetstream.New(nc)
	if err != nil {
		return nil, fmt.Errorf("jetstream: %w", err)
	}

	id := uuid.New().String()
	if len(id) > 8 {
		id = id[:8]
	}

	var adapter gateway.GatewayAdapter
	switch gatewayAdapter {
	case "cilium":
		adapter = gateway.NewCiliumAdapter("cilium")
		log.Printf("[%s] using Cilium adapter", id)
	default:
		adapter = gateway.NewIstioAdapter("istio")
		log.Printf("[%s] using Istio adapter", id)
	}

	applier, err := gateway.NewK8sApplier(adapter, kubeconfig)
	if err != nil {
		log.Printf("[%s] k8s applier not available (gateway runs in shadow mode): %v", id, err)
	}

	return &Worker{
		js:       js,
		executor: gateway.NewGatewayExecutor(),
		applier:  applier,
		adapter:  adapter,
		workerID: fmt.Sprintf("gw-provider-%s", id),
	}, nil
}

type StepRequestMessage struct {
	OperationID    string `json:"operation_id"`
	StepID         string `json:"step_id"`
	StepType       string `json:"step_type"`
	IdempotencyKey string `json:"idempotency_key"`
	TenantID       string `json:"tenant_id"`
	ProfileJSON    string `json:"profile_json,omitempty"`
	GatewayName    string `json:"gateway_name,omitempty"`
	GatewayNS      string `json:"gateway_namespace,omitempty"`
}

func (w *Worker) Start(ctx context.Context) error {
	log.Printf("[%s] starting gateway provider", w.workerID)

	_, err := w.js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:        "GATEWAY",
		Subjects:    []string{"hnb.gateway.*"},
		Storage:     jetstream.FileStorage,
		MaxMsgs:     10000,
		Discard:     jetstream.DiscardNew,
		MaxAge:      72 * time.Hour,
		Duplicates:  5 * time.Minute,
	})
	if err != nil {
		return fmt.Errorf("stream: %w", err)
	}

	cons, err := w.js.CreateOrUpdateConsumer(ctx, "GATEWAY", jetstream.ConsumerConfig{
		Name:         "gateway-worker",
		Durable:      "gateway-worker",
		AckPolicy:    jetstream.AckExplicitPolicy,
		MaxDeliver:   3,
		BackOff:      []time.Duration{5 * time.Second, 30 * time.Second, 5 * time.Minute},
		FilterSubject: "hnb.gateway.>",
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

	log.Printf("[%s] listening for gateway requests on hnb.gateway.>", w.workerID)
	<-ctx.Done()
	cc.Stop()
	return nil
}

func (w *Worker) handleMessage(ctx context.Context, msg jetstream.Msg) {
	var req StepRequestMessage
	if err := json.Unmarshal(msg.Data(), &req); err != nil {
		log.Printf("[%s] bad message: %v", w.workerID, err)
		msg.Nak()
		return
	}

	log.Printf("[%s] request: %s/%s (type=%s)", w.workerID, req.OperationID, req.StepID, req.StepType)

	if w.applier == nil {
		w.handleShadowMode(ctx, msg, &req)
		return
	}

	switch req.StepType {
	case "configure_gateway":
		w.handleConfigure(ctx, msg, &req)
	case "deconfigure_gateway":
		w.handleDeconfigure(ctx, msg, &req)
	default:
		log.Printf("[%s] unsupported step type: %s", w.workerID, req.StepType)
		msg.Nak()
	}
}

func (w *Worker) handleShadowMode(ctx context.Context, msg jetstream.Msg, req *StepRequestMessage) {
	if req.ProfileJSON == "" {
		log.Printf("[%s] shadow mode: missing profile_json for %s/%s", w.workerID, req.OperationID, req.StepID)
		w.publishResult(ctx, "hnb.gateway.configured", map[string]any{
			"operation_id": req.OperationID, "step_id": req.StepID,
			"status": "failed", "error": "missing profile_json",
		})
		msg.Ack()
		return
	}

	var profile gateway.GatewayProfile
	if err := json.Unmarshal([]byte(req.ProfileJSON), &profile); err != nil {
		log.Printf("[%s] shadow mode: bad profile_json: %v", w.workerID, err)
		w.publishResult(ctx, "hnb.gateway.configured", map[string]any{
			"operation_id": req.OperationID, "step_id": req.StepID,
			"status": "failed", "error": fmt.Sprintf("invalid profile: %v", err),
		})
		msg.Ack()
		return
	}

	pv := gateway.NewProfileValidator()
	if errs := pv.Validate(&profile); len(errs) > 0 {
		log.Printf("[%s] shadow mode: validation failed: %v", w.workerID, errs)
		w.publishResult(ctx, "hnb.gateway.configured", map[string]any{
			"operation_id": req.OperationID, "step_id": req.StepID,
			"status": "failed", "error": fmt.Sprintf("validation: %v", errs),
		})
		msg.Ack()
		return
	}

	log.Printf("[%s] shadow mode: would configure gateway %s for %s/%s", w.workerID, profile.Name, req.OperationID, req.StepID)
	result := w.publishResult(ctx, "hnb.gateway.configured", map[string]any{
		"operation_id": req.OperationID, "step_id": req.StepID,
		"status": "succeeded", "message": fmt.Sprintf("gateway %s validated (shadow) by %s", profile.Name, w.workerID),
	})
	if result != nil {
		log.Printf("[%s] publish result failed: %v", w.workerID, result)
	}
	msg.Ack()
}

func (w *Worker) handleConfigure(ctx context.Context, msg jetstream.Msg, req *StepRequestMessage) {
	metrics.OperationsActive.Inc()
	defer metrics.OperationsActive.Dec()
	start := time.Now()
	metrics.MessagesReceived.WithLabelValues("configure_gateway", "received").Inc()
	if req.ProfileJSON == "" {
		log.Printf("[%s] missing profile_json for %s/%s", w.workerID, req.OperationID, req.StepID)
		w.publishResult(ctx, "hnb.gateway.configured", map[string]any{
			"operation_id": req.OperationID, "step_id": req.StepID,
			"status": "failed", "error": "missing profile_json in request",
		})
		msg.Ack()
		return
	}

	var profile gateway.GatewayProfile
	if err := json.Unmarshal([]byte(req.ProfileJSON), &profile); err != nil {
		log.Printf("[%s] bad profile_json: %v", w.workerID, err)
		metrics.MessagesReceived.WithLabelValues("configure_gateway", "failed").Inc()
		metrics.ApplyTotal.WithLabelValues(w.adapter.Name(), "configure", "failed").Inc()
		metrics.ApplyDuration.WithLabelValues(w.adapter.Name(), "configure").Observe(time.Since(start).Seconds())
		w.publishResult(ctx, "hnb.gateway.configured", map[string]any{
			"operation_id": req.OperationID, "step_id": req.StepID,
			"status": "failed", "error": fmt.Sprintf("invalid profile: %v", err),
		})
		msg.Ack()
		return
	}

	tenantID := req.TenantID
	if tenantID == "" {
		tenantID = profile.TenantID
	}

	ns := w.adapter.ToGatewayNamespace(&profile, tenantID)
	gw := &gateway.Gateway{
		ID: profile.ID, Name: profile.Name, Namespace: ns,
		Type: profile.Type, Status: gateway.GwPending,
	}
	gw.Listeners = make([]gateway.Listener, len(profile.Listeners))
	for i, l := range profile.Listeners {
		gw.Listeners[i] = gateway.Listener{
			Name: l.Name, Port: l.Port, Protocol: l.Protocol,
			Hostname: l.Hostname, AllowRoute: l.AllowRoute,
		}
		if l.TLS != nil {
			tls := &gateway.TLSConfig{Mode: l.TLS.Mode, CertificateRef: l.TLS.CertificateRef}
			gw.Listeners[i].TLS = tls
		}
	}

	reqRoutes := []string{"HTTPRoute"}
	reqFeatures := w.collectRequiredFeatures(&profile)

	cap := &gateway.GatewayCapabilitySnapshot{
		ProviderName:    w.adapter.Name(),
		SupportedRoutes: []string{"HTTPRoute", "GRPCRoute", "TCPRoute", "TLSRoute"},
		CoreFeatures:    []string{"HTTPRoute", "HTTPRoute.HostRewrite", "HTTPRoute.PathRewrite", "HTTPRoute.RequestMirror", "HTTPRoute.RequestHeaderModifier", "HTTPRoute.ResponseHeaderModifier", "HTTPRoute.URLRewrite"},
		ExtendedFeatures: []string{"GRPCRoute", "HTTPRoute.RequestRedirect", "HTTPRoute.RequestTimeout", "HTTPRoute.BackendTimeout", "HTTPRoute.WeightedSplit", "TCPRoute", "TLSRoute"},
	}

	if err := w.executor.ValidateAndPrepare(&profile, &gateway.GatewayRequirements{RequiredRoutes: reqRoutes, RequiredFeatures: reqFeatures}, cap, gw); err != nil {
		log.Printf("[%s] validation failed: %v", w.workerID, err)
		w.publishResult(ctx, "hnb.gateway.configured", map[string]any{
			"operation_id": req.OperationID, "step_id": req.StepID,
			"status": "failed", "error": fmt.Sprintf("validation: %v", err),
		})
		msg.Ack()
		return
	}

	if err := w.applier.ApplyGateway(ctx, &profile, tenantID); err != nil {
		log.Printf("[%s] apply gateway failed: %v", w.workerID, err)
		msg.Nak()
		return
	}

	if err := w.applier.ApplyHTTPRoute(ctx, &profile, tenantID); err != nil {
		log.Printf("[%s] apply httproute failed, rolling back gateway: %v", w.workerID, err)
		if rollbackErr := w.applier.DeleteGateway(ctx, profile.Name, ns); rollbackErr != nil {
			log.Printf("[%s] rollback gateway failed: %v", w.workerID, rollbackErr)
		}
		msg.Nak()
		return
	}

	if vsProvider, ok := w.adapter.(gateway.VirtualServiceProvider); ok {
		vsSpec := vsProvider.ToVirtualService(&profile, tenantID)
		if err := w.applier.ApplyVirtualService(ctx, vsSpec, ns); err != nil {
			log.Printf("[%s] apply virtualservice failed, rolling back: %v", w.workerID, err)
			w.applier.DeleteGateway(ctx, profile.Name, ns)
			w.applier.DeleteHTTPRoute(ctx, profile.Name+"-httproute", ns)
			msg.Nak()
			return
		}
	}

	log.Printf("[%s] gateway configured: %s/%s", w.workerID, req.OperationID, req.StepID)
	metrics.MessagesReceived.WithLabelValues("configure_gateway", "succeeded").Inc()
	metrics.ApplyTotal.WithLabelValues(w.adapter.Name(), "configure", "succeeded").Inc()
	metrics.ApplyDuration.WithLabelValues(w.adapter.Name(), "configure").Observe(time.Since(start).Seconds())
	if result := w.publishResult(ctx, "hnb.gateway.configured", map[string]any{
		"operation_id": req.OperationID, "step_id": req.StepID,
		"status": "succeeded", "message": fmt.Sprintf("gateway %s configured by %s", profile.Name, w.workerID),
		"gateway": profile.Name, "namespace": ns,
	}); result != nil {
		log.Printf("[%s] publish result failed: %v", w.workerID, result)
	}
	msg.Ack()
}

func (w *Worker) handleDeconfigure(ctx context.Context, msg jetstream.Msg, req *StepRequestMessage) {
	metrics.OperationsActive.Inc()
	defer metrics.OperationsActive.Dec()
	start := time.Now()
	metrics.MessagesReceived.WithLabelValues("deconfigure_gateway", "received").Inc()
	gwName := req.GatewayName
	gwNS := req.GatewayNS
	if gwName == "" && req.ProfileJSON != "" {
		var profile gateway.GatewayProfile
		if err := json.Unmarshal([]byte(req.ProfileJSON), &profile); err == nil {
			gwName = profile.Name
			tenantID := req.TenantID
			if tenantID == "" {
				tenantID = profile.TenantID
			}
			gwNS = w.adapter.ToGatewayNamespace(&profile, tenantID)
		}
	}
	if gwName == "" {
		log.Printf("[%s] cannot determine gateway name for %s/%s", w.workerID, req.OperationID, req.StepID)
		msg.Ack()
		return
	}

	log.Printf("[%s] deconfiguring gateway %s/%s", w.workerID, gwNS, gwName)

	httpRouteName := gwName + "-httproute"

	if err := w.applier.DeleteHTTPRoute(ctx, httpRouteName, gwNS); err != nil {
		if !containsNotFound(err) {
			log.Printf("[%s] delete httproute failed: %v", w.workerID, err)
		}
	}

	if err := w.applier.DeleteGateway(ctx, gwName, gwNS); err != nil {
		if !containsNotFound(err) {
			log.Printf("[%s] delete gateway failed: %v", w.workerID, err)
			msg.Nak()
			return
		}
	}

	log.Printf("[%s] gateway deconfigured: %s/%s", w.workerID, gwNS, gwName)
	metrics.MessagesReceived.WithLabelValues("deconfigure_gateway", "succeeded").Inc()
	metrics.ApplyTotal.WithLabelValues(w.adapter.Name(), "deconfigure", "succeeded").Inc()
	metrics.ApplyDuration.WithLabelValues(w.adapter.Name(), "deconfigure").Observe(time.Since(start).Seconds())
	if result := w.publishResult(ctx, "hnb.gateway.configured", map[string]any{
		"operation_id": req.OperationID, "step_id": req.StepID,
		"status": "succeeded", "message": fmt.Sprintf("gateway %s deconfigured by %s", gwName, w.workerID),
	}); result != nil {
		log.Printf("[%s] publish result failed: %v", w.workerID, result)
	}
	msg.Ack()
}

func containsNotFound(err error) bool {
	return err == nil || apierrors.IsNotFound(err)
}

func (w *Worker) collectRequiredFeatures(profile *gateway.GatewayProfile) []string {
	var features []string
	seen := map[string]bool{}
	for _, rule := range profile.Rules {
		if rule.Redirect != nil && !seen["HTTPRoute.RequestRedirect"] {
			features = append(features, "HTTPRoute.RequestRedirect")
			seen["HTTPRoute.RequestRedirect"] = true
		}
		if rule.Rewrite != nil && !seen["HTTPRoute.URLRewrite"] {
			features = append(features, "HTTPRoute.URLRewrite")
			seen["HTTPRoute.URLRewrite"] = true
		}
		if rule.Mirror != nil && !seen["HTTPRoute.RequestMirror"] {
			features = append(features, "HTTPRoute.RequestMirror")
			seen["HTTPRoute.RequestMirror"] = true
		}
		if rule.Headers != nil {
			if len(rule.Headers.Set) > 0 && !seen["HTTPRoute.RequestHeaderModifier"] {
				features = append(features, "HTTPRoute.RequestHeaderModifier")
				seen["HTTPRoute.RequestHeaderModifier"] = true
			}
		}
		if rule.Timeout != "" && !seen["HTTPRoute.RequestTimeout"] {
			features = append(features, "HTTPRoute.RequestTimeout")
			seen["HTTPRoute.RequestTimeout"] = true
		}
		if len(rule.Backends) > 1 && !seen["HTTPRoute.WeightedSplit"] {
			features = append(features, "HTTPRoute.WeightedSplit")
			seen["HTTPRoute.WeightedSplit"] = true
		}
	}
	return features
}

func (w *Worker) publishResult(ctx context.Context, subject string, data any) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = w.js.Publish(ctx, subject, payload)
	return err
}

func (w *Worker) GetNamespace(tenantID string, profile *gateway.GatewayProfile) string {
	if w.applier != nil {
		return w.applier.GetNamespace(tenantID, profile)
	}
	return "hnb-" + tenantID
}

func (w *Worker) Applier() *gateway.K8sApplier {
	return w.applier
}