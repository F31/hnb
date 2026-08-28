//go:build ignore

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/F31/hnb/cmd/cluster-agent/internal/observer"
)

func main() {
	discovery := observer.NewKubeDiscovery("https://127.0.0.1:43398", os.Getenv("KUBE_TOKEN"))
	identity := observer.ObserverIdentity{
		TenantID: "tenant-dev", TargetID: "79eb7403-2e06-4502-901a-420e3c40cd55",
		TargetKind: "KubernetesTarget", ObserverID: "agent-79eb7403", ObserverKind: "Agent",
	}
	producer := observer.NewProducer(identity, 1, 1, nil)
	reporter := observer.NewReporter(os.Getenv("INGEST_URL"), os.Getenv("OBSERVER_TOKEN_FILE"), producer, discovery)
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	err := reporter.ReportOnce(ctx)
	if err != nil {
		fmt.Println("REPORT ERROR:", err)
		os.Exit(1)
	}
	fmt.Println("REPORT OK")
}
