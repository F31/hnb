package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/lib/pq"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"

	"github.com/F31/hnb/cmd/rbac-syncer/internal/config"
	"github.com/F31/hnb/cmd/rbac-syncer/internal/health"
	"github.com/F31/hnb/cmd/rbac-syncer/internal/informer"
	"github.com/F31/hnb/cmd/rbac-syncer/internal/metrics"
	"github.com/F31/hnb/cmd/rbac-syncer/internal/reconciler"
	"github.com/F31/hnb/cmd/rbac-syncer/internal/watcher"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		klog.Fatalf("Failed to load config: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	db, err := openDB(cfg)
	if err != nil {
		klog.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	clientset, err := newK8sClientset(cfg)
	if err != nil {
		klog.Fatalf("Failed to create K8s clientset: %v", err)
	}

	klog.Infof("Starting RBAC Syncer (shadow=%v, interval=%s)", cfg.ShadowMode, cfg.PollInterval)

	healthServer := health.NewHealthServer(cfg.HealthAddr)
	healthServer.RegisterCheck("database", func(ctx context.Context) error {
		return db.PingContext(ctx)
	})
	healthServer.RegisterCheck("k8s-api", func(ctx context.Context) error {
		_, err := clientset.Discovery().ServerVersion()
		return err
	})
	go healthServer.Start(ctx)

	auditLogger := metrics.NewAuditLogger(db)

	userRoleWatcher := watcher.NewUserRoleWatcher(db, cfg.PollInterval)
	namespaceWatcher := watcher.NewNamespaceWatcher(db, cfg.PollInterval)
	roleBindingInformer := informer.NewRoleBindingInformer(clientset)

	syncer := reconciler.NewSyncer(cfg, clientset, db, userRoleWatcher, namespaceWatcher, roleBindingInformer, auditLogger)

	go syncer.Start(ctx)

	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				metrics.LogMetricsSummary()
			}
		}
	}()

	<-sigCh
	klog.Info("Shutting down...")
	cancel()
}

func openDB(cfg *config.Config) (*sql.DB, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s dbname=%s sslmode=disable",
		cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBName,
	)
	if cfg.DBPassword != "" {
		dsn += fmt.Sprintf(" password=%s", cfg.DBPassword)
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("sql open: %w", err)
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("db ping: %w", err)
	}
	return db, nil
}

func newK8sClientset(cfg *config.Config) (*kubernetes.Clientset, error) {
	var restCfg *rest.Config
	var err error

	if cfg.KubeConfigPath != "" {
		restCfg, err = clientcmd.BuildConfigFromFlags("", cfg.KubeConfigPath)
	} else {
		restCfg, err = rest.InClusterConfig()
	}
	if err != nil {
		return nil, fmt.Errorf("k8s config: %w", err)
	}

	restCfg.QPS = cfg.KubeQPS
	restCfg.Burst = cfg.KubeBurst

	return kubernetes.NewForConfig(restCfg)
}
