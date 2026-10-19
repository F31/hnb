// Lab-only route binding runner.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/F31/hnb/cmd/gateway-provider/internal/engine/gateway"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
)

func main() {
	kubeconfig := flag.String("kubeconfig", os.Getenv("KUBECONFIG"), "target kubeconfig")
	namespace := flag.String("namespace", "hnb-lab-probes", "route namespace")
	name := flag.String("name", "probe-app", "HTTPRoute name")
	gatewayName := flag.String("gateway", "hnb-lab", "Gateway name")
	gatewayNamespace := flag.String("gateway-namespace", "hnb-gateway", "Gateway namespace")
	hostname := flag.String("hostname", "app2.hnb.test", "route hostname")
	service := flag.String("service", "probe-server", "backend Service")
	port := flag.Int("port", 8080, "backend port")
	url := flag.String("url", "", "external probe URL")
	hostHeader := flag.String("host-header", "", "HTTP Host header")
	expected := flag.String("expected", "", "expected response body")
	timeout := flag.Duration("timeout", 60*time.Second, "route readiness timeout")
	flag.Parse()
	cfg, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err)
	}
	client, err := dynamic.NewForConfig(cfg)
	if err != nil {
		panic(err)
	}
	applier := gateway.NewRouteBindingApplier(client, nil)
	err = applier.ApplyAndProbe(context.Background(), gateway.RouteBinding{
		Namespace: *namespace, Name: *name, GatewayName: *gatewayName, GatewayNamespace: *gatewayNamespace,
		Hostname: *hostname, ServiceName: *service, ServicePort: int32(*port),
		ExternalURL: *url, HostHeader: *hostHeader, ExpectedBody: *expected, Timeout: *timeout,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("route accepted, gateway programmed, external probe passed")
}
