package nats

import (
	"context"
	"log"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	"github.com/F31/hnb/cmd/gateway-provider/internal/engine/gateway"
)

type GatewayReconciler struct {
	applier  *gateway.K8sApplier
	interval time.Duration
}

func NewGatewayReconciler(applier *gateway.K8sApplier, interval time.Duration) *GatewayReconciler {
	return &GatewayReconciler{applier: applier, interval: interval}
}

func (r *GatewayReconciler) Start(ctx context.Context) {
	if r.applier == nil {
		log.Println("[reconciler] no applier, skipping")
		return
	}
	log.Printf("[reconciler] starting (interval=%s)", r.interval)
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	r.reconcile(ctx)
	for {
		select {
		case <-ctx.Done():
			log.Println("[reconciler] stopped")
			return
		case <-ticker.C:
			r.reconcile(ctx)
		}
	}
}

func (r *GatewayReconciler) reconcile(ctx context.Context) {
	namespaces, err := r.listGatewayNamespaces(ctx)
	if err != nil {
		klog.V(2).Infof("[reconciler] list namespaces: %v", err)
		return
	}
	for _, ns := range namespaces {
		r.reconcileNamespace(ctx, ns)
	}
}

func (r *GatewayReconciler) listGatewayNamespaces(ctx context.Context) ([]string, error) {
	gwList, err := r.applier.ListGateways(ctx, "", metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var namespaces []string
	for _, gw := range gwList {
		ns := gw.GetNamespace()
		if !seen[ns] {
			seen[ns] = true
			namespaces = append(namespaces, ns)
		}
	}
	return namespaces, nil
}

func (r *GatewayReconciler) reconcileNamespace(ctx context.Context, namespace string) {
	gwList, err := r.applier.ListGateways(ctx, namespace, metav1.ListOptions{})
	if err != nil {
		klog.V(2).Infof("[reconciler] list gateways in %s: %v", namespace, err)
		return
	}
	for _, gw := range gwList {
		labels := gw.GetLabels()
		if labels == nil || labels["hnb.cloud/managed-by"] != "hnb-gateway-provider" {
			continue
		}
		routeName := gw.GetName() + "-httproute"
		_, err := r.applier.GetHTTPRoute(ctx, routeName, namespace)
		if err != nil {
			klog.V(2).Infof("[reconciler] gateway %s/%s missing HTTPRoute %s: %v", namespace, gw.GetName(), routeName, err)
		}
	}
}