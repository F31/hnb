package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/F31/hnb/cmd/cluster-agent/internal/config"
	"github.com/F31/hnb/cmd/cluster-agent/internal/observer"
	"github.com/F31/hnb/pkg/iam"
	"github.com/F31/hnb/pkg/tunnel"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		fmt.Println()
		log.Println("shutting down...")
		cancel()
	}()

	client := tunnel.NewAgentClient(cfg.TunnelURL, iam.FileTokenSource{Path: cfg.TokenFile}, cfg.TenantID, cfg.ClusterID)

	// Start the RT-008 observation producer (Full/Delta target, capability,
	// and node inventory) against the platform ingest endpoint when configured.
	if cfg.ObservationIngestURL != "" {
		identity := observer.ObserverIdentity{
			TenantID:     cfg.TenantID,
			TargetID:     cfg.ClusterID,
			TargetKind:   "KubernetesTarget",
			ObserverID:   "agent-" + cfg.ClusterID,
			ObserverKind: "Agent",
		}
		producer := observer.NewProducer(identity, cfg.ObserverGeneration, 1, nil)
		kubeClient, err := newKubeHTTPClient(cfg.KubeCAFile, 15*time.Second)
		if err != nil {
			log.Fatalf("configure observer Kubernetes client: %v", err)
		}
		discovery := observer.NewKubeDiscoveryWithClient(cfg.KubeAPI, cfg.KubeToken, kubeClient)
		reporter := observer.NewReporter(cfg.ObservationIngestURL, cfg.ObserverTokenFile, producer, discovery)
		go reporter.Run(ctx, cfg.ObservationInterval)
		log.Printf("[agent] observation reporter enabled -> %s", cfg.ObservationIngestURL)
	}

	// Main loop with reconnection
	for {
		select {
		case <-ctx.Done():
			log.Println("cluster-agent stopped")
			return
		default:
		}

		if err := connectAndServe(ctx, client, cfg); err != nil {
			log.Printf("[agent] connection error: %v, reconnecting in %ds...", err, cfg.ReconnectInt)
			time.Sleep(time.Duration(cfg.ReconnectInt) * time.Second)
		}
	}
}

func connectAndServe(ctx context.Context, client *tunnel.AgentClient, cfg *config.Config) error {
	if err := client.Connect(); err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer client.Close()

	log.Printf("[agent] connected as %s to %s", cfg.ClusterID, cfg.TunnelURL)

	heartbeatTicker := time.NewTicker(30 * time.Second)
	defer heartbeatTicker.Stop()

	readErr := make(chan error, 1)
	go func() {
		for {
			msg, err := client.ReadMessage()
			if err != nil {
				readErr <- err
				return
			}

			switch msg.Type {
			case tunnel.MsgRequest:
				go handleRequest(client, msg, cfg)
			case tunnel.MsgHeartbeat:
				// Server heartbeat ACK
			case tunnel.MsgError:
				log.Printf("[agent] server error: %s", string(msg.Payload))
			default:
				log.Printf("[agent] unknown message type: %s", msg.Type)
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-readErr:
			return fmt.Errorf("read error: %w", err)
		case <-heartbeatTicker.C:
			payload := tunnel.HeartbeatPayload{
				ClusterID: cfg.ClusterID,
			}
			if err := client.SendHeartbeat(payload); err != nil {
				return fmt.Errorf("heartbeat: %w", err)
			}
		}
	}
}

func handleRequest(client *tunnel.AgentClient, msg *tunnel.Message, cfg *config.Config) {
	var req tunnel.RequestPayload
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		log.Printf("[agent] bad request: %v", err)
		client.SendResponse(req.RequestID, 400, nil, []byte(fmt.Sprintf("bad request: %v", err)))
		return
	}

	log.Printf("[agent] proxying %s %s", req.Method, req.Path)
	statusCode, headers, body := proxyToKubeAPI(req, cfg)
	client.SendResponse(req.RequestID, statusCode, headers, body)
}

func proxyToKubeAPI(req tunnel.RequestPayload, cfg *config.Config) (int, map[string]string, []byte) {
	if req.Method == "" {
		req.Method = http.MethodGet
	}
	if !kubeProxyAllowed(req.Method, req.Path) {
		return http.StatusForbidden, nil, []byte("kubernetes resource and method are not allowed")
	}
	if cfg.KubeAPI == "" {
		return 502, nil, []byte("kube-api is not configured")
	}
	targetURL := strings.TrimSuffix(cfg.KubeAPI, "/") + "/" + strings.TrimPrefix(req.Path, "/")
	target, err := url.Parse(targetURL)
	if err != nil {
		return 502, nil, []byte("parse kube request URL: " + err.Error())
	}
	target.RawQuery = req.RawQuery
	var body io.Reader
	if len(req.Body) > 0 {
		body = bytes.NewReader(req.Body)
	}
	kubeReq, err := http.NewRequest(req.Method, target.String(), body)
	if err != nil {
		return 502, nil, []byte("build kube request: " + err.Error())
	}
	if cfg.KubeToken != "" {
		kubeReq.Header.Set("Authorization", "Bearer "+cfg.KubeToken)
	}
	for name, value := range req.Headers {
		switch http.CanonicalHeaderKey(name) {
		case "Authorization", "Host":
			continue
		default:
			kubeReq.Header.Set(name, value)
		}
	}
	if kubeReq.Header.Get("Accept") == "" {
		kubeReq.Header.Set("Accept", "application/json")
	}
	if len(req.Body) > 0 && kubeReq.Header.Get("Content-Type") == "" {
		kubeReq.Header.Set("Content-Type", "application/json")
	}
	client, err := newKubeHTTPClient(cfg.KubeCAFile, 30*time.Second)
	if err != nil {
		return 502, nil, []byte(err.Error())
	}
	kubeResp, err := client.Do(kubeReq)
	if err != nil {
		log.Printf("[agent] kube proxy %s %s: %v", req.Method, target.Path, err)
		return 502, nil, []byte("kube proxy: " + err.Error())
	}
	defer kubeResp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(kubeResp.Body, 8<<20))
	if err != nil {
		return 502, nil, []byte("read kube response: " + err.Error())
	}
	headers := map[string]string{
		"Content-Type": kubeResp.Header.Get("Content-Type"),
	}
	return kubeResp.StatusCode, headers, respBody
}

func newKubeHTTPClient(caFile string, timeout time.Duration) (*http.Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if caFile != "" {
		caPEM, err := os.ReadFile(caFile)
		if err != nil {
			return nil, fmt.Errorf("read kube CA: %w", err)
		}
		roots := x509.NewCertPool()
		if !roots.AppendCertsFromPEM(caPEM) {
			return nil, fmt.Errorf("parse kube CA: no certificates found")
		}
		transport.TLSClientConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
	}
	return &http.Client{Timeout: timeout, Transport: transport}, nil
}

var kubeProxyAllowlist = map[string]map[string]struct{}{
	"/nodes":                                  {http.MethodGet: {}},
	"/services":                               {http.MethodGet: {}, http.MethodPost: {}, http.MethodPatch: {}, http.MethodDelete: {}},
	"/persistentvolumes":                      {http.MethodGet: {}},
	"/persistentvolumeclaims":                 {http.MethodGet: {}},
	"/configmaps":                             {http.MethodGet: {}, http.MethodPost: {}, http.MethodPatch: {}, http.MethodDelete: {}},
	"/secrets":                                {http.MethodGet: {}, http.MethodPost: {}, http.MethodDelete: {}},
	"/events":                                 {http.MethodGet: {}},
	"/pods":                                   {http.MethodGet: {}, http.MethodPatch: {}, http.MethodDelete: {}},
	"/pods/log":                               {http.MethodGet: {}},
	"apps/deployments":                        {http.MethodGet: {}, http.MethodPatch: {}, http.MethodDelete: {}},
	"apps/statefulsets":                       {http.MethodGet: {}, http.MethodPatch: {}, http.MethodDelete: {}},
	"apps/daemonsets":                         {http.MethodGet: {}, http.MethodPatch: {}, http.MethodDelete: {}},
	"batch/jobs":                              {http.MethodGet: {}, http.MethodPatch: {}, http.MethodDelete: {}},
	"batch/cronjobs":                          {http.MethodGet: {}, http.MethodPatch: {}, http.MethodDelete: {}},
	"networking.k8s.io/ingresses":             {http.MethodGet: {}, http.MethodPost: {}, http.MethodPatch: {}, http.MethodDelete: {}},
	"networking.k8s.io/networkpolicies":       {http.MethodGet: {}, http.MethodPost: {}, http.MethodPatch: {}, http.MethodDelete: {}},
	"storage.k8s.io/storageclasses":           {http.MethodGet: {}},
	"storage.k8s.io/csidrivers":               {http.MethodGet: {}},
	"storage.k8s.io/csinodes":                 {http.MethodGet: {}},
	"storage.k8s.io/csistoragecapacities":     {http.MethodGet: {}},
	"storage.k8s.io/volumeattachments":        {http.MethodGet: {}},
	"snapshot.storage.k8s.io/volumesnapshots": {http.MethodGet: {}},
	"snapshot.storage.k8s.io/volumesnapshotclasses":  {http.MethodGet: {}},
	"snapshot.storage.k8s.io/volumesnapshotcontents": {http.MethodGet: {}},
	"metallb.io/ipaddresspools":                      {http.MethodGet: {}, http.MethodPost: {}, http.MethodPatch: {}, http.MethodDelete: {}},
}

func kubeProxyAllowed(method, path string) bool {
	decoded, err := url.PathUnescape(strings.Trim(path, "/"))
	if err != nil || decoded == "" {
		return false
	}
	segments := strings.Split(decoded, "/")
	for _, segment := range segments {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}

	var group string
	var resourceIndex int
	switch segments[0] {
	case "api":
		if len(segments) < 3 {
			return false
		}
		resourceIndex = 2
	case "apis":
		if len(segments) < 4 {
			return false
		}
		group = segments[1]
		resourceIndex = 3
	default:
		return false
	}

	if segments[resourceIndex] == "namespaces" {
		resourceIndex += 2
		if len(segments) <= resourceIndex {
			return false
		}
	}
	remaining := len(segments) - resourceIndex
	if remaining < 1 || remaining > 3 {
		return false
	}
	key := group + "/" + segments[resourceIndex]
	if remaining == 3 {
		key += "/" + segments[resourceIndex+2]
	}
	methods, ok := kubeProxyAllowlist[key]
	if !ok {
		return false
	}
	_, ok = methods[strings.ToUpper(method)]
	return ok
}
