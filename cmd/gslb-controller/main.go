package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/lib/pq"

	"github.com/google/uuid"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/F31/hnb/cmd/gslb-controller/internal/consumer"
	"github.com/F31/hnb/cmd/gslb-controller/internal/dns"
	"github.com/F31/hnb/cmd/gslb-controller/internal/executor"
	"github.com/F31/hnb/cmd/gslb-controller/internal/healthsource"
	"github.com/F31/hnb/cmd/gslb-controller/internal/provider"
	"github.com/F31/hnb/cmd/gslb-controller/internal/reconciler"
	"github.com/F31/hnb/cmd/gslb-controller/internal/store"
)

type Config struct {
	ListenAddr        string
	ProbeInterval     time.Duration
	ProbeTimeout      time.Duration
	Domain            string
	DBHost            string
	DBPort            string
	DBUser            string
	DBPassword        string
	DBName            string
	DBSSLMode         string
	Kubeconfig        string
	DNSNamespace      string
	ReconcileInterval time.Duration
	HealthSources     string
	KarmadaKubeconfig string
	MergePolicy       string
	NATSURL           string
	DNSTTL            int
}

func loadConfig() *Config {
	return &Config{
		ListenAddr:        getEnv("LISTEN_ADDR", ":8080"),
		ProbeInterval:     durationEnv("PROBE_INTERVAL", 30*time.Second),
		ProbeTimeout:      durationEnv("PROBE_TIMEOUT", 5*time.Second),
		Domain:            getEnv("DNS_DOMAIN", "hnb.cloud"),
		DBHost:            getEnv("DB_HOST", "localhost"),
		DBPort:            getEnv("DB_PORT", "5432"),
		DBUser:            getEnv("DB_USER", "hnb"),
		DBPassword:        getEnv("DB_PASSWORD", ""),
		DBName:            getEnv("DB_NAME", "hnb"),
		DBSSLMode:         getEnv("DB_SSLMODE", "disable"),
		Kubeconfig:        getEnv("KUBECONFIG", ""),
		DNSNamespace:      getEnv("DNS_NAMESPACE", "gslb-system"),
		ReconcileInterval: durationEnv("RECONCILE_INTERVAL", 60*time.Second),
		HealthSources:     getEnv("GSLB_HEALTH_SOURCES", "http"),
		KarmadaKubeconfig: getEnv("KARMADA_KUBECONFIG", ""),
		MergePolicy:       getEnv("HEALTH_MERGE_POLICY", "all-healthy"),
		NATSURL:           getEnv("NATS_URL", "nats://localhost:4222"),
		DNSTTL:            intEnv("GSLB_DNS_TTL", 300),
	}
}

func intEnv(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n > 0 {
			return n
		}
	}
	return fallback
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func durationEnv(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		d, err := time.ParseDuration(v)
		if err == nil {
			return d
		}
	}
	return fallback
}

type ClusterTarget struct {
	Name           string   `json:"name"`
	Endpoint       string   `json:"endpoint"`
	Weight         int      `json:"weight"`
	TrafficTargets []string `json:"traffic_targets,omitempty"`
	DNSName        string   `json:"dns_name,omitempty"`
}

func openDB(cfg *Config) (*sql.DB, error) {
	dsn := "host=" + cfg.DBHost +
		" port=" + cfg.DBPort +
		" user=" + cfg.DBUser +
		" dbname=" + cfg.DBName +
		" sslmode=" + cfg.DBSSLMode
	if cfg.DBPassword != "" {
		dsn += " password=" + cfg.DBPassword
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)

	if err := db.Ping(); err != nil {
		return nil, err
	}
	return db, nil
}

func kubernetesConfig(kubeconfig string) (*rest.Config, error) {
	if kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		return rest.InClusterConfig()
	}
	return clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)
}

func main() {
	cfg := loadConfig()

	db, err := openDB(cfg)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()
	log.Printf("connected to postgresql at %s:%s/%s", cfg.DBHost, cfg.DBPort, cfg.DBName)

	restConfig, err := kubernetesConfig(cfg.Kubeconfig)
	if err != nil {
		log.Fatalf("kubernetes config: %v", err)
	}

	kubeClient, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		log.Fatalf("kubernetes client: %v", err)
	}

	dynClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		log.Fatalf("dynamic client: %v", err)
	}

	clusterStore := store.NewClusterStore(db)

	sources, err := healthsource.ParseSourcesWithKarmada(
		cfg.HealthSources,
		cfg.KarmadaKubeconfig,
		cfg.ProbeInterval,
		cfg.ProbeTimeout,
	)
	if err != nil {
		log.Fatalf("health sources: %v", err)
	}
	mergePolicy := healthsource.ParseMergePolicy(cfg.MergePolicy)
	healthMgr := healthsource.NewHealthManager(sources, mergePolicy)

	log.Printf("health sources: %v, merge policy: %s",
		sourceNames(healthMgr.Sources()), cfg.MergePolicy)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("shutting down...")
		cancel()
	}()

	switchStore := store.NewSwitchRequestStore(db)
	rec := reconciler.New(
		clusterStore,
		healthMgr,
		dynClient,
		kubeClient,
		switchStore,
		switchStore,
		cfg.Domain,
		cfg.ReconcileInterval,
	)

	// GSLB-005：DNS 数据面唯一写入口 = executor（经 NATS 执行命令驱动）。
	// reconciler 不再持有任何 DNS 写能力。
	dnsManager := dns.NewManager(dynClient, cfg.DNSNamespace)
	// gslb-dns-provider SPI：内置 ExternalDNS 参考实现（GSLB-006）
	planExecutor := executor.NewExecutor(provider.NewExternalDNS(dnsManager), cfg.DNSTTL)

	js, cleanupNATS, err := consumer.Connect(ctx, cfg.NATSURL)
	if err != nil {
		log.Fatalf("connect nats: %v", err)
	}
	defer cleanupNATS()
	gslbConsumer := consumer.New(js, switchStore, planExecutor)
	go func() {
		if err := gslbConsumer.Start(ctx); err != nil {
			log.Printf("[gslb-consumer] stopped: %v", err)
		}
	}()

	go rec.Start(ctx)

	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	mux.HandleFunc("/api/v1/targets", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var targets []ClusterTarget
		if err := json.NewDecoder(r.Body).Decode(&targets); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		hsTargets := make([]healthsource.ClusterTarget, len(targets))
		weights := make(map[string]int)
		dnsNames := make(map[string]string)
		for i, t := range targets {
			hsTargets[i] = healthsource.ClusterTarget{
				Name:     t.Name,
				Endpoint: t.Endpoint,
			}
			weights[t.Name] = t.Weight
			if t.DNSName != "" {
				dnsNames[t.Name] = t.DNSName
			}
		}

		merged := healthMgr.ProbeAll(ctx, hsTargets)
		healthy := healthMgr.HealthyTargets(hsTargets)
		records := healthsource.GenerateDNSRecords(cfg.Domain, healthy, weights, dnsNames)

		statuses := make(map[string]string)
		for name, hr := range merged {
			statuses[name] = hr.Status
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"records":       records,
			"healthy":       len(healthy),
			"total":         len(targets),
			"statuses":      statuses,
			"merge_policy":  cfg.MergePolicy,
			"health_sources": sourceNames(healthMgr.Sources()),
		})
	})

	mux.HandleFunc("/api/v1/status", func(w http.ResponseWriter, r *http.Request) {
		allStatuses := healthMgr.GetAllStatuses()
		mergedStatuses := healthMgr.GetAllMergedStatuses()

		detailed := make(map[string]map[string]string)
		for name, sources := range allStatuses {
			entry := make(map[string]string)
			for src, st := range sources {
				entry[src] = st
			}
			entry["merged"] = mergedStatuses[name]
			detailed[name] = entry
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"clusters":      detailed,
			"merge_policy":  cfg.MergePolicy,
			"health_sources": sourceNames(healthMgr.Sources()),
		})
	})

	mux.HandleFunc("/api/v1/clusters", func(w http.ResponseWriter, r *http.Request) {
		clusters, err := clusterStore.ListAll(ctx)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(clusters)
	})

	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("gslb-controller [%s] listening on %s", uuid.New().String()[:8], cfg.ListenAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("serve: %v", err)
		}
	}()

	<-ctx.Done()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	server.Shutdown(shutdownCtx)
}

func sourceNames(sources []healthsource.HealthSource) []string {
	names := make([]string, len(sources))
	for i, s := range sources {
		names[i] = s.Name()
	}
	return names
}