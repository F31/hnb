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
	"strings"
	"syscall"
	"time"

	_ "github.com/lib/pq"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/F31/hnb/cmd/apiserver/internal/capability"
	"github.com/F31/hnb/cmd/apiserver/internal/config"
	"github.com/F31/hnb/cmd/apiserver/internal/handler"
	"github.com/F31/hnb/cmd/apiserver/internal/leader"
	"github.com/F31/hnb/cmd/apiserver/internal/metrics"
	"github.com/F31/hnb/cmd/apiserver/internal/middleware"
	"github.com/F31/hnb/cmd/apiserver/internal/router"
	"github.com/F31/hnb/cmd/apiserver/internal/server"
	"github.com/F31/hnb/pkg/audit"
	dbpkg "github.com/F31/hnb/pkg/db"
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

	if err := dbpkg.WaitForReady(context.Background(), db); err != nil {
		log.Fatalf("db ping: %v", err)
	}

	// Schema readiness gate (PINST-004): when the operator declares a migration
	// directory, the server refuses to start unless the platform owner's schema
	// is fully migrated and checksum-verified. An incompatible schema must keep
	// the platform out of readiness instead of starting against a half-migrated
	// database.
	if cfg.MigrationsDir != "" {
		files, loadErr := dbpkg.LoadMigrations(cfg.MigrationsDir)
		if loadErr != nil {
			log.Fatalf("schema readiness: cannot load migrations from %s: %v", cfg.MigrationsDir, loadErr)
		}
		migrator := dbpkg.Migrator{DB: db, Owner: dbpkg.OwnerPlatform}
		ready, problems, verifyErr := migrator.VerifySchemaForService(context.Background(), files)
		if verifyErr != nil {
			log.Fatalf("schema readiness: %v", verifyErr)
		}
		if !ready {
			log.Fatalf("schema readiness blocked: platform schema is not migrated or incompatible: %s",
				strings.Join(problems, "; "))
		}
		log.Printf("schema readiness: platform owner schema is migrated and verified (%d migrations)", len(files))
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
	// Existing deployments predate the default workspace; ensure it exists so
	// login has a workspace to land on.
	if err := ensureDefaultWorkspace(context.Background(), db); err != nil {
		log.Printf("ensure default workspace: %v", err)
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
		"/api/v1/service-credentials:renew",
		"/api/v1/agent-enrollments:exchange",
		"/api/v1/agent-sessions:renew",
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
	serviceIdentityStore := iam.NewServiceIdentityDBStore(db)
	verifyToken := func(ctx context.Context, token, tenantID, clusterID string) (time.Time, error) {
		// 优先校验专用的 agent-tunnel token profile；若 token 绑定 session，
		// 还必须核对持久化 session 未撤销、未过期且 generation 一致。
		if claims, err := agentTunnelVerifier.VerifyClaims(ctx, token, tenantID, clusterID); err == nil {
			if claims.SessionID != "" {
				session, err := serviceIdentityStore.GetAgentSession(ctx, tenantID, clusterID, claims.SessionID)
				if err != nil || session.RevokedAt != nil || !session.ExpiresAt.After(time.Now()) || session.Generation != claims.Generation {
					return time.Time{}, fmt.Errorf("agent session unavailable")
				}
			}
			return time.Unix(claims.ExpiresAt, 0).UTC(), nil
		}
		trusted, err := tunnelAuthenticator.Authenticate(ctx, token, iam.ActionExecute, tenantID, "cluster", clusterID)
		return trusted.ExpiresAt, err
	}
	tunnelServer := tunnel.NewTunnelServer(verifyToken)
	serviceIdentityAuthority, err := iam.NewServiceCredentialAuthority(
		serviceIdentityStore, keySet, keySet, cfg.TokenIssuer, time.Now,
		func(_ context.Context, event iam.ServiceCredentialAuditEvent) {
			_ = auditStore.Create(&audit.Event{
				ID:               fmt.Sprintf("audit-%d", time.Now().UnixNano()),
				Timestamp:        time.Now().UTC(),
				UserID:           event.ActorSubjectID,
				TenantID:         event.TenantID,
				ServiceSubjectID: event.ServiceSubjectID,
				Action:           event.Action,
				ResourceType:     "serviceIdentity",
				ResourceID:       event.ServiceIdentityID,
				StatusCode:       http.StatusOK,
			})
		})
	if err != nil {
		log.Fatalf("service credential authority: %v", err)
	}
	serviceIdentityHandler := handler.NewServiceIdentityHandler(serviceIdentityAuthority, serviceIdentityStore)
	agentEnrollmentHandler := handler.NewAgentEnrollmentHandler(
		serviceIdentityStore, serviceIdentityStore, handler.NewSQLAgentTargetResolver(db),
		agentTunnelSigner, agentTunnelVerifier, handler.DefaultAgentSessionTTL, tunnelServer.Revoke, time.Now,
		func(_ context.Context, event handler.AgentEnrollmentAuditEvent) {
			_ = auditStore.Create(&audit.Event{
				ID:               fmt.Sprintf("audit-%d", time.Now().UnixNano()),
				Timestamp:        time.Now().UTC(),
				UserID:           event.ActorSubjectID,
				TenantID:         event.TenantID,
				ServiceSubjectID: event.ServiceSubjectID,
				Action:           event.Action,
				ResourceType:     "cluster",
				ResourceID:       event.TargetID,
				StatusCode:       http.StatusOK,
			})
		})

	routesPath := filepath.Join(cfg.ConfigDir, "routes.yaml")
	routeRegistry := router.NewRegistry(func(rf *router.RouteFile) {
		log.Printf("[router] extension routes reloaded")
	})
	if extCfg, err := config.LoadRoutes(routesPath); err == nil {
		routeRegistry.Load(extCfg.ToRouteFile())
		routeRegistry.Watch(routesPath)
	}

	metrics.Serve(cfg.MetricsAddr)

	httpHandler := router.NewWithCapabilities(db, tunnelServer, authMW, tokenManager, iamStore, auditStore, rbac, routeRegistry, delegationSigner, agentTunnelSigner, serviceIdentityHandler, agentEnrollmentHandler, cfg.PlatformAPIURL, cfg.ClusterProjectionMode, cfg.AppMarketURL, cfg.HarborURL, cfg.HarborUser, cfg.HarborPass, cfg.PublicBaseURL, cfg.AgentImage, capability.FromCSV(cfg.ClusterCapabilities))

	hostname, _ := os.Hostname()
	leaderElection := leader.New(nc, js, "apiserver", hostname)
	go leaderElection.Start(context.Background())

	srv := server.New(cfg.ListenAddr, dbpkg.WithHealth(httpHandler, db, nc.IsConnected))

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
