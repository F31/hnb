package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	_ "github.com/lib/pq"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/F31/hnb/cmd/apiserver/internal/config"
	"github.com/F31/hnb/cmd/apiserver/internal/leader"
	"github.com/F31/hnb/cmd/apiserver/internal/metrics"
	"github.com/F31/hnb/cmd/apiserver/internal/middleware"
	"github.com/F31/hnb/cmd/apiserver/internal/capability"
	"github.com/F31/hnb/cmd/apiserver/internal/router"
	"github.com/F31/hnb/cmd/apiserver/internal/server"
	"github.com/F31/hnb/pkg/audit"
	"github.com/F31/hnb/pkg/iam"
	"github.com/F31/hnb/pkg/messaging"
	"github.com/F31/hnb/pkg/tunnel"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	db, err := sql.Open("postgres", cfg.DBDSN)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("db ping: %v", err)
	}

	nc, err := messaging.ConnectNATSFromEnv(cfg.NATSURL)
	if err != nil {
		log.Fatalf("nats: %v", err)
	}
	defer nc.Close()

	js, err := jetstream.New(nc)
	if err != nil {
		log.Fatalf("jetstream: %v", err)
	}

	iamStore := iam.NewIAMDBStore(db, cfg.TokenIssuer)
	if err := iamStore.Migrate(); err != nil {
		log.Fatalf("iam migrate: %v", err)
	}
	if cfg.BootstrapAdminPassword != "" {
		if err := bootstrapAdmin(context.Background(), db, cfg.BootstrapAdminPassword, cfg.TokenIssuer); err != nil {
			log.Fatalf("bootstrap: %v", err)
		}
	}
	keySet, err := iam.LoadReloadingKeySet(context.Background(), iam.ReloadingKeySetConfig{
		ManifestPath: cfg.TokenKeyManifestPath, ActivePrivateKeyPath: cfg.TokenPrivateKeyPath,
		Issuer: cfg.TokenIssuer, Recorder: iamStore,
		OnSuccess: func(stats iam.KeyReloadStats) {
			log.Printf("[identity] key manifest generation %d loaded", stats.Generation)
		},
	})
	if err != nil {
		log.Fatalf("identity keys: %v", err)
	}
	if err := keySet.StartPolling(context.Background(), cfg.TokenKeyReloadInterval, func(err error) {
		log.Printf("[identity] key manifest reload failed: %v", err)
	}); err != nil {
		log.Fatalf("identity key polling: %v", err)
	}

	auditStore := audit.NewStore(db)
	if err := auditStore.Migrate(); err != nil {
		log.Fatalf("audit migrate: %v", err)
	}

	rbac := iam.NewRBACEngine()
	loadBindings(rbac, iamStore)

	tokenManager, err := iam.NewTokenManager(iam.TokenManagerConfig{
		Issuer: cfg.TokenIssuer, Audience: cfg.TokenAudience, Audiences: cfg.TokenAudiences,
		AccessTTL: iam.MaxAccessTokenTTL, RefreshTTL: 24 * time.Hour,
	}, keySet, keySet, iamStore, iamStore, iamStore)
	if err != nil {
		log.Fatalf("token manager: %v", err)
	}
	delegationSigner, err := iam.NewDelegationSigner(iam.DelegationConfig{
		Issuer: cfg.TokenIssuer, Audience: "hnb-platform-api", ServiceSubject: "hnb-apiserver", TTL: 30 * time.Second,
	}, keySet)
	if err != nil {
		log.Fatalf("delegation signer: %v", err)
	}
	authMW := middleware.NewAuth(tokenManager, []string{
		"/health", "/ready", "/openapi.json",
		"/api/v1/auth/login", "/api/v1/auth/refresh",
	})

	tunnelVerifier, err := iam.NewTokenVerifier(iam.TokenManagerConfig{
		Issuer: cfg.TokenIssuer, Audience: "hnb-apiserver-tunnel", AccessTTL: iam.MaxAccessTokenTTL,
	}, keySet)
	if err != nil {
		log.Fatalf("tunnel token verifier: %v", err)
	}
	tunnelAuthenticator, err := iam.NewServiceAuthenticator(tunnelVerifier)
	if err != nil {
		log.Fatalf("tunnel service identity: %v", err)
	}
	agentTunnelConfig := iam.AgentTunnelTokenConfig{
		Issuer: cfg.TokenIssuer, Audience: "hnb-apiserver-tunnel", TTL: iam.MaxAgentTunnelTokenTTL,
	}
	agentTunnelVerifier, err := iam.NewAgentTunnelTokenVerifier(agentTunnelConfig, keySet)
	if err != nil {
		log.Fatalf("agent tunnel token verifier: %v", err)
	}
	agentTunnelSigner, err := iam.NewAgentTunnelTokenSigner(agentTunnelConfig, keySet)
	if err != nil {
		log.Fatalf("agent tunnel token signer: %v", err)
	}
	verifyToken := func(ctx context.Context, token, tenantID, clusterID string) (time.Time, error) {
		// 优先校验专用的 agent-tunnel token profile（接入指引签发的长 TTL 令牌，
		// 覆盖 kubectl apply + 首次 Pod 启动窗口）；回退到短 TTL 访问令牌路径
		// （保留既有手动签发/轮换流程的兼容性）。
		if expiry, err := agentTunnelVerifier.Verify(ctx, token, tenantID, clusterID); err == nil {
			return expiry, nil
		}
		trusted, err := tunnelAuthenticator.Authenticate(ctx, token, iam.ActionExecute, tenantID, "cluster", clusterID)
		return trusted.ExpiresAt, err
	}
	tunnelServer := tunnel.NewTunnelServer(verifyToken)

	routesPath := filepath.Join(cfg.ConfigDir, "routes.yaml")
	routeRegistry := router.NewRegistry(func(rf *router.RouteFile) {
		log.Printf("[router] extension routes reloaded")
	})
	if extCfg, err := config.LoadRoutes(routesPath); err == nil {
		routeRegistry.Load(extCfg.ToRouteFile())
		routeRegistry.Watch(routesPath)
	}

	metrics.Serve(cfg.MetricsAddr)

	httpHandler := router.NewWithCapabilities(db, tunnelServer, authMW, tokenManager, iamStore, auditStore, rbac, routeRegistry, delegationSigner, agentTunnelSigner, cfg.PlatformAPIURL, cfg.ClusterProjectionMode, cfg.AppMarketURL, cfg.HarborURL, cfg.HarborUser, cfg.HarborPass, cfg.PublicBaseURL, cfg.AgentImage, capability.FromCSV(cfg.ClusterCapabilities))

	hostname, _ := os.Hostname()
	leaderElection := leader.New(nc, js, "apiserver", hostname)
	go leaderElection.Start(context.Background())

	srv := server.New(cfg.ListenAddr, httpHandler)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		fmt.Println()
		log.Println("shutting down...")
		leaderElection.Stop()
		srv.Shutdown(context.Background())
	}()

	log.Printf("apiserver listening on %s", cfg.ListenAddr)
	if err := srv.Start(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("serve: %v", err)
	}
	log.Println("apiserver stopped")
}

func loadBindings(rbac *iam.RBACEngine, store *iam.IAMDBStore) {
	bindings, err := store.ListRoleBindings()
	if err != nil {
		log.Printf("[rbac] load bindings: %v", err)
		return
	}
	for _, b := range bindings {
		b := b
		rbac.BindRole(&b)
	}
	log.Printf("[rbac] loaded %d role bindings", len(bindings))
}
