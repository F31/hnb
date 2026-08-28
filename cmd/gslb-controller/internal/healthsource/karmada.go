package healthsource

import (
	"context"
	"sync"
	"time"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	karmada "github.com/F31/hnb/cmd/gslb-controller/internal/karmada"
)

type KarmadaSource struct {
	client *karmada.Client
	mu     sync.RWMutex
	status map[string]string
}

func NewKarmadaSource(kubeconfig string) (*KarmadaSource, error) {
	var cfg *rest.Config
	var err error
	if kubeconfig != "" {
		cfg, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	} else {
		cfg, err = rest.InClusterConfig()
		if err != nil {
			cfg, err = clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)
		}
	}
	if err != nil {
		return nil, err
	}

	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	return &KarmadaSource{
		client: karmada.NewClient(dynClient),
		status: make(map[string]string),
	}, nil
}

func NewKarmadaSourceWithClient(client *karmada.Client) *KarmadaSource {
	return &KarmadaSource{
		client: client,
		status: make(map[string]string),
	}
}

func (s *KarmadaSource) Name() string {
	return "karmada"
}

func (s *KarmadaSource) Probe(ctx context.Context, targets []ClusterTarget) (map[string]HealthResult, error) {
	results := make(map[string]HealthResult, len(targets))

	karmadaClusters, err := s.client.ListClusters(ctx)
	if err != nil {
		return results, err
	}

	karmadaNames := make(map[string]bool, len(karmadaClusters))
	for _, kc := range karmadaClusters {
		karmadaNames[kc.GetName()] = true
	}

	for _, t := range targets {
		if !karmadaNames[t.Name] {
			results[t.Name] = HealthResult{
				Status:    "unknown",
				Source:    s.Name(),
				Timestamp: time.Now(),
				Details:   map[string]string{"reason": "not found in karmada"},
			}
			continue
		}

		status := s.client.GetClusterStatus(ctx, t.Name)
		results[t.Name] = HealthResult{
			Status:    status,
			Source:    s.Name(),
			Timestamp: time.Now(),
			Details: map[string]string{
				"karmada_status": status,
			},
		}
	}

	s.mu.Lock()
	for k, v := range results {
		s.status[k] = v.Status
	}
	s.mu.Unlock()

	return results, nil
}

func (s *KarmadaSource) GetStatus(name string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status[name]
}