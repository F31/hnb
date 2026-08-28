package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/F31/hnb/pkg/appstore"
	"github.com/F31/hnb/pkg/appstore/helm"
	"github.com/F31/hnb/pkg/appstore/security"
	"github.com/F31/hnb/pkg/appstore/storage"
	"github.com/F31/hnb/pkg/appstore/store"
	"github.com/F31/hnb/pkg/iam"
	"github.com/F31/hnb/pkg/messaging"
)

const (
	uploadSessionTTL        = 3600
	uploadSessionPrefix     = "upload-"
	harborProject           = "hnb"
	uploadChunkSize         = 8 * 1024 * 1024
	uploadConcurrency       = 4
	uploadMaxRetries        = 3
	defaultTransferDir      = "/tmp/hnb-artifact-transfer"
	transferBackendLocal    = "local"
	transferBackendSharedFS = "shared-fs"
)

type uploadPolicy struct {
	Resumable        bool   `json:"resumable"`
	ChunkSize        int64  `json:"chunk_size"`
	MaxConcurrency   int    `json:"max_concurrency"`
	MaxRetries       int    `json:"max_retries"`
	ResumeStorageKey string `json:"resume_storage_key"`
}

type transferPolicy struct {
	Strategy         string `json:"strategy"`
	Endpoint         string `json:"endpoint"`
	Resumable        bool   `json:"resumable"`
	ChunkSize        int64  `json:"chunk_size"`
	MaxConcurrency   int    `json:"max_concurrency"`
	MaxRetries       int    `json:"max_retries"`
	ResumeStorageKey string `json:"resume_storage_key"`
}

type Config struct {
	ListenAddr             string
	DBDSN                  string
	NATSURL                string
	HarborURL              string
	HarborUser             string
	HarborPass             string
	MetricsAddr            string
	TokenIssuer            string
	TokenAudience          string
	TokenKeyManifestPath   string
	TokenKeyReloadInterval time.Duration
	TransferStagingDir     string
	TransferBackend        string
	TransferReadOnly       bool
	TransferReplicaCount   int
	TrivyEndpoint          string
}

var appMarketRoutes = []iam.RouteMetadata{
	{Method: http.MethodGet, Pattern: "/health", Public: true},
	{Method: http.MethodGet, Pattern: "/metrics", Public: true},
	{Method: http.MethodGet, Pattern: "/api/v1/publishers", ResourceKind: "publisher", Action: iam.ActionList},
	{Method: http.MethodPost, Pattern: "/api/v1/publishers", ResourceKind: "publisher", Action: iam.ActionCreate},
	{Method: http.MethodGet, Pattern: "/api/v1/products", ResourceKind: "product", Action: iam.ActionList},
	{Method: http.MethodPost, Pattern: "/api/v1/products", ResourceKind: "product", Action: iam.ActionCreate},
	{Method: http.MethodPatch, Pattern: "/api/v1/products/{id}", ResourceKind: "product", ResourceIDParam: "id", Action: iam.ActionUpdate},
	{Method: http.MethodDelete, Pattern: "/api/v1/products/{id}", ResourceKind: "product", ResourceIDParam: "id", Action: iam.ActionDelete},
	{Method: http.MethodGet, Pattern: "/api/v1/products/{productId}/releases", ResourceKind: "release", Action: iam.ActionList},
	{Method: http.MethodPost, Pattern: "/api/v1/products/{productId}/releases", ResourceKind: "release", Action: iam.ActionCreate},
	{Method: http.MethodPatch, Pattern: "/api/v1/releases/{id}", ResourceKind: "release", ResourceIDParam: "id", Action: iam.ActionUpdate},
	{Method: http.MethodDelete, Pattern: "/api/v1/releases/{id}", ResourceKind: "release", ResourceIDParam: "id", Action: iam.ActionDelete},
	{Method: http.MethodPost, Pattern: "/api/v1/releases/{id}/publish", ResourceKind: "release", ResourceIDParam: "id", Action: iam.ActionExecute},
	{Method: http.MethodGet, Pattern: "/api/v1/releases/{id}/artifacts", ResourceKind: "release", ResourceIDParam: "id", Action: iam.ActionRead},
	{Method: http.MethodPost, Pattern: "/api/v1/releases/{id}/artifacts/{artifactId}", ResourceKind: "release", ResourceIDParam: "id", Action: iam.ActionUpdate},
	{Method: http.MethodDelete, Pattern: "/api/v1/releases/{id}/artifacts/{artifactId}", ResourceKind: "release", ResourceIDParam: "id", Action: iam.ActionUpdate},
	{Method: http.MethodPost, Pattern: "/api/v1/applications", ResourceKind: "application", Action: iam.ActionCreate},
	{Method: http.MethodDelete, Pattern: "/api/v1/applications/{id}", ResourceKind: "application", ResourceIDParam: "id", Action: iam.ActionDelete},
	{Method: http.MethodGet, Pattern: "/api/v1/applications", ResourceKind: "application", Action: iam.ActionList},
	{Method: http.MethodGet, Pattern: "/api/v1/applications/groups", ResourceKind: "application", Action: iam.ActionList},
	{Method: http.MethodPost, Pattern: "/api/v1/applications/groups", ResourceKind: "application", Action: iam.ActionCreate},
	{Method: http.MethodGet, Pattern: "/api/v1/applications/groups/{id}", ResourceKind: "application", ResourceIDParam: "id", Action: iam.ActionRead},
	{Method: http.MethodPatch, Pattern: "/api/v1/applications/groups/{id}", ResourceKind: "application", ResourceIDParam: "id", Action: iam.ActionUpdate},
	{Method: http.MethodDelete, Pattern: "/api/v1/applications/groups/{id}", ResourceKind: "application", ResourceIDParam: "id", Action: iam.ActionDelete},
	{Method: http.MethodGet, Pattern: "/api/v1/applications/groups/{id}/applications", ResourceKind: "application", ResourceIDParam: "id", Action: iam.ActionList},
	{Method: http.MethodGet, Pattern: "/api/v1/artifacts", ResourceKind: "artifact", Action: iam.ActionList},
	{Method: http.MethodGet, Pattern: "/api/v1/artifacts/{id}", ResourceKind: "artifact", ResourceIDParam: "id", Action: iam.ActionRead},
	{Method: http.MethodPatch, Pattern: "/api/v1/artifacts/{id}", ResourceKind: "artifact", ResourceIDParam: "id", Action: iam.ActionUpdate},
	{Method: http.MethodGet, Pattern: "/api/v1/artifacts/{id}/references", ResourceKind: "artifact", ResourceIDParam: "id", Action: iam.ActionRead},
	{Method: http.MethodPost, Pattern: "/api/v1/artifacts/{id}/references", ResourceKind: "artifact", ResourceIDParam: "id", Action: iam.ActionCreate},
	{Method: http.MethodPost, Pattern: "/api/v1/artifacts/{id}/gc/preview", ResourceKind: "artifact", ResourceIDParam: "id", Action: iam.ActionRead},
	{Method: http.MethodPost, Pattern: "/api/v1/artifacts/{id}/gc", ResourceKind: "artifact", ResourceIDParam: "id", Action: iam.ActionExecute},
	{Method: http.MethodPost, Pattern: "/api/v1/artifacts/session", ResourceKind: "artifact", Action: iam.ActionCreate},
	{Method: http.MethodPost, Pattern: "/api/v1/artifacts/confirm", ResourceKind: "artifact", Action: iam.ActionCreate},
	{Method: http.MethodPost, Pattern: "/api/v1/artifacts/upload", ResourceKind: "artifact", Action: iam.ActionCreate},
	{Method: http.MethodGet, Pattern: "/api/v1/artifact-transfer/sessions/{id}", ResourceKind: "artifactTransfer", ResourceIDParam: "id", Action: iam.ActionRead},
	{Method: http.MethodPut, Pattern: "/api/v1/artifact-transfer/sessions/{id}/parts/{partNumber}", ResourceKind: "artifactTransfer", ResourceIDParam: "id", Action: iam.ActionUpdate},
	{Method: http.MethodPost, Pattern: "/api/v1/artifact-transfer/sessions/{id}/complete", ResourceKind: "artifactTransfer", ResourceIDParam: "id", Action: iam.ActionExecute},
	{Method: http.MethodPost, Pattern: "/api/v1/artifact-transfer/sessions/{id}/abort", ResourceKind: "artifactTransfer", ResourceIDParam: "id", Action: iam.ActionDelete},
	{Method: http.MethodGet, Pattern: "/api/v1/artifact-storage/profiles", ResourceKind: "artifactStorageProfile", Action: iam.ActionList},
	{Method: http.MethodPost, Pattern: "/api/v1/artifact-storage/profiles", ResourceKind: "artifactStorageProfile", Action: iam.ActionCreate},
	{Method: http.MethodGet, Pattern: "/api/v1/artifact-storage/profiles/{id}", ResourceKind: "artifactStorageProfile", ResourceIDParam: "id", Action: iam.ActionRead},
	{Method: http.MethodPatch, Pattern: "/api/v1/artifact-storage/profiles/{id}", ResourceKind: "artifactStorageProfile", ResourceIDParam: "id", Action: iam.ActionUpdate},
	{Method: http.MethodDelete, Pattern: "/api/v1/artifact-storage/profiles/{id}", ResourceKind: "artifactStorageProfile", ResourceIDParam: "id", Action: iam.ActionDelete},
	{Method: http.MethodPost, Pattern: "/api/v1/artifact-storage/profile-migrations", ResourceKind: "artifactStorageProfile", Action: iam.ActionExecute},
	{Method: http.MethodGet, Pattern: "/api/v1/artifact-distributions", ResourceKind: "artifactDistribution", Action: iam.ActionList},
	{Method: http.MethodGet, Pattern: "/api/v1/artifact-distributions/{id}", ResourceKind: "artifactDistribution", ResourceIDParam: "id", Action: iam.ActionRead},
	{Method: http.MethodPost, Pattern: "/api/v1/artifact-distributions/{id}/rebuild", ResourceKind: "artifactDistribution", ResourceIDParam: "id", Action: iam.ActionExecute},
	{Method: http.MethodPost, Pattern: "/api/v1/helm/sync", ResourceKind: "helmRepository", Action: iam.ActionExecute},
	{Method: http.MethodGet, Pattern: "/api/v1/recycle-bin/settings", ResourceKind: "recycleBin", Action: iam.ActionList},
	{Method: http.MethodPatch, Pattern: "/api/v1/recycle-bin/settings", ResourceKind: "recycleBin", Action: iam.ActionUpdate},
	{Method: http.MethodGet, Pattern: "/api/v1/recycle-bin/entries", ResourceKind: "recycleBin", Action: iam.ActionList},
	{Method: http.MethodPost, Pattern: "/api/v1/recycle-bin/entries/{id}/cancel", ResourceKind: "recycleBin", ResourceIDParam: "id", Action: iam.ActionExecute},
	{Method: http.MethodPost, Pattern: "/api/v1/recycle-bin/entries/{id}/purge", ResourceKind: "recycleBin", ResourceIDParam: "id", Action: iam.ActionExecute},
	{Method: http.MethodGet, Pattern: "/api/v1/security/profiles", ResourceKind: "securityProfile", Action: iam.ActionList},
	{Method: http.MethodPost, Pattern: "/api/v1/security/profiles", ResourceKind: "securityProfile", Action: iam.ActionCreate},
	{Method: http.MethodPatch, Pattern: "/api/v1/security/profiles/{id}", ResourceKind: "securityProfile", ResourceIDParam: "id", Action: iam.ActionUpdate},
	{Method: http.MethodDelete, Pattern: "/api/v1/security/profiles/{id}", ResourceKind: "securityProfile", ResourceIDParam: "id", Action: iam.ActionDelete},
	{Method: http.MethodPost, Pattern: "/api/v1/security/profiles/{id}/sync", ResourceKind: "securityProfile", ResourceIDParam: "id", Action: iam.ActionExecute},
	{Method: http.MethodPost, Pattern: "/api/v1/security/profiles/{id}/scan", ResourceKind: "securityProfile", ResourceIDParam: "id", Action: iam.ActionExecute},
	{Method: http.MethodGet, Pattern: "/api/v1/security/db-status", ResourceKind: "securityProfile", Action: iam.ActionList},
	{Method: http.MethodGet, Pattern: "/api/v1/security/reports", ResourceKind: "securityReport", Action: iam.ActionList},
	{Method: http.MethodGet, Pattern: "/api/v1/security/reports/{id}", ResourceKind: "securityReport", ResourceIDParam: "id", Action: iam.ActionRead},
}

func loadConfig() (*Config, error) {
	getEnv := func(key, def string) string {
		if v := os.Getenv(key); v != "" {
			return v
		}
		return def
	}
	getBool := func(key string) bool {
		v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
		return v == "1" || v == "true" || v == "yes" || v == "on"
	}
	getInt := func(key string, def int) int {
		v := strings.TrimSpace(os.Getenv(key))
		if v == "" {
			return def
		}
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed < 1 {
			return def
		}
		return parsed
	}
	cfg := &Config{
		ListenAddr:           getEnv("LISTEN_ADDR", ":8080"),
		DBDSN:                getEnv("DB_DSN", "postgres://postgres:postgres@localhost:5432/hnb?sslmode=disable"),
		NATSURL:              getEnv("NATS_URL", "nats://localhost:4222"),
		HarborURL:            getEnv("HARBOR_URL", ""),
		HarborUser:           getEnv("HARBOR_USERNAME", os.Getenv("HARBOR_USER")),
		HarborPass:           getEnv("HARBOR_PASSWORD", os.Getenv("HARBOR_PASS")),
		MetricsAddr:          getEnv("METRICS_ADDR", ":8081"),
		TokenIssuer:          os.Getenv("API_TOKEN_ISSUER"),
		TokenAudience:        getEnv("API_TOKEN_AUDIENCE", "hnb-app-market"),
		TokenKeyManifestPath: os.Getenv("API_TOKEN_KEY_MANIFEST_FILE"),
		TransferStagingDir:   getEnv("ARTIFACT_TRANSFER_STAGING_DIR", defaultTransferDir),
		TransferBackend:      getEnv("ARTIFACT_TRANSFER_BACKEND", transferBackendLocal),
		TransferReadOnly:     getBool("ARTIFACT_TRANSFER_READONLY"),
		TransferReplicaCount: getInt("ARTIFACT_TRANSFER_REPLICA_COUNT", 1),
		TrivyEndpoint:        getEnv("TRIVY_ENDPOINT", ""),
	}
	if cfg.TokenIssuer == "" || cfg.TokenAudience != "hnb-app-market" || cfg.TokenKeyManifestPath == "" {
		return nil, errors.New("API_TOKEN_ISSUER, API_TOKEN_AUDIENCE=hnb-app-market, and API_TOKEN_KEY_MANIFEST_FILE are required")
	}
	var err error
	cfg.TokenKeyReloadInterval, err = iam.ParseKeyReloadInterval(getEnv("API_TOKEN_KEY_RELOAD_INTERVAL", "5s"))
	if err != nil {
		return nil, err
	}
	if err := validateTransferConfig(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func validateTransferConfig(cfg *Config) error {
	switch cfg.TransferBackend {
	case transferBackendLocal, transferBackendSharedFS:
	case "s3-multipart":
		return errors.New("ARTIFACT_TRANSFER_BACKEND=s3-multipart is reserved for future implementation")
	default:
		return fmt.Errorf("unsupported ARTIFACT_TRANSFER_BACKEND %q", cfg.TransferBackend)
	}
	if cfg.TransferReplicaCount > 1 && cfg.TransferBackend == transferBackendLocal {
		return errors.New("ARTIFACT_TRANSFER_BACKEND=local is not allowed when ARTIFACT_TRANSFER_REPLICA_COUNT > 1; use shared-fs or s3-multipart")
	}
	if !cfg.TransferReadOnly {
		if err := os.MkdirAll(cfg.TransferStagingDir, 0o750); err != nil {
			return fmt.Errorf("artifact transfer staging directory is unavailable: %w", err)
		}
		probe, err := os.CreateTemp(cfg.TransferStagingDir, ".write-probe-*")
		if err != nil {
			return fmt.Errorf("artifact transfer staging directory is not writable: %w", err)
		}
		probeName := probe.Name()
		if err := probe.Close(); err != nil {
			return fmt.Errorf("close artifact transfer staging write probe: %w", err)
		}
		if err := os.Remove(probeName); err != nil {
			return fmt.Errorf("remove artifact transfer staging write probe: %w", err)
		}
	}
	return nil
}

func uploadPolicyForSize(sizeBytes int64, resumeKey string) uploadPolicy {
	policy := uploadPolicy{
		Resumable:        true,
		ChunkSize:        uploadChunkSize,
		MaxConcurrency:   uploadConcurrency,
		MaxRetries:       uploadMaxRetries,
		ResumeStorageKey: resumeKey,
	}
	const gib = int64(1024 * 1024 * 1024)
	if sizeBytes >= gib && sizeBytes < 10*gib {
		policy.ChunkSize = 32 * 1024 * 1024
		policy.MaxConcurrency = 4
		policy.MaxRetries = 5
	}
	if sizeBytes >= 10*gib {
		policy.ChunkSize = 64 * 1024 * 1024
		policy.MaxConcurrency = 3
		policy.MaxRetries = 5
	}
	return policy
}

func isAllowedArtifactFilename(filename, artifactType string) bool {
	name := strings.ToLower(filename)
	for _, suffix := range allowedArtifactSuffixes(artifactType) {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}

func allowedArtifactSuffixes(artifactType string) []string {
	switch appstore.ArtifactType(artifactType) {
	case appstore.ArtHelmChart:
		return []string{".tgz", ".tar.gz"}
	case appstore.ArtOCI:
		return []string{".tar", ".tar.gz", ".tgz", ".oci"}
	case appstore.ArtJAR:
		return []string{".jar"}
	case appstore.ArtWAR:
		return []string{".war"}
	case appstore.ArtOfflineBundle:
		return []string{".zip", ".tar", ".tar.gz", ".tgz", ".bundle"}
	case appstore.ArtGeneric:
		return []string{".zip", ".tar", ".tar.gz", ".tgz", ".yaml", ".yml", ".json", ".txt", ".bin", ".jar", ".war"}
	default:
		return []string{".zip", ".tar", ".tar.gz", ".tgz", ".yaml", ".yml", ".json", ".txt", ".bin", ".jar", ".war"}
	}
}

func ensureDefaultHarborProfile(cfg *Config, profileRepo *store.StorageProfileRepo, tenantID string) error {
	if cfg.HarborURL == "" {
		return nil
	}
	if tenantID == "" {
		return nil
	}
	profiles, err := profileRepo.List(tenantID, 100, 0)
	if err != nil {
		return err
	}
	for _, profile := range profiles {
		if profile.Backend == appstore.StorageOCI && strings.TrimRight(profile.Endpoint, "/") == strings.TrimRight(cfg.HarborURL, "/") {
			return nil
		}
	}
	return profileRepo.Create(&appstore.ArtifactStorageProfile{
		ID:              uuid.NewString(),
		TenantID:        tenantID,
		Name:            "default-harbor",
		Backend:         appstore.StorageOCI,
		ServiceTier:     appstore.StorageTierMinimal,
		AuthorityRole:   appstore.StorageAuthoritative,
		SecretReference: "env://HARBOR_USER,HARBOR_PASS",
		Endpoint:        cfg.HarborURL,
		LifecycleState:  appstore.StorageProfileActive,
		Metadata: map[string]any{
			"source":      "env",
			"description": "Default Harbor/OCI repository configured from app-market environment",
		},
		CreatedBy: "system:app-market",
	})
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	keySet, err := iam.LoadReloadingKeySet(context.Background(), iam.ReloadingKeySetConfig{ManifestPath: cfg.TokenKeyManifestPath, Issuer: cfg.TokenIssuer, OnSuccess: func(stats iam.KeyReloadStats) {
		log.Printf("identity key manifest generation %d loaded", stats.Generation)
	}})
	if err != nil {
		log.Fatalf("identity public keys: %v", err)
	}
	if err := keySet.StartPolling(context.Background(), cfg.TokenKeyReloadInterval, func(err error) { log.Printf("identity key manifest reload failed: %v", err) }); err != nil {
		log.Fatalf("identity key polling: %v", err)
	}
	verifier, err := iam.NewTokenVerifier(iam.TokenManagerConfig{
		Issuer: cfg.TokenIssuer, Audience: cfg.TokenAudience, AccessTTL: iam.MaxAccessTokenTTL,
	}, keySet)
	if err != nil {
		log.Fatalf("identity verifier: %v", err)
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

	// Initialize repositories
	pubRepo := store.NewPublisherRepo(db)
	prodRepo := store.NewProductRepo(db)
	relRepo := store.NewReleaseRepo(db)
	appRepo := store.NewApplicationRepo(db)
	groupRepo := store.NewApplicationGroupRepo(db)
	sessionRepo := store.NewUploadSessionRepo(db)
	artifactRepo := store.NewArtifactRepo(db)
	profileRepo := store.NewStorageProfileRepo(db)
	distributionRepo := store.NewDistributionRepo(db)
	gcRepo := store.NewGCRepo(db)
	recycleRepo := store.NewRecycleBinRepo(db)
	securityRepo := store.NewSecurityRepo(db)
	if err := ensureDefaultHarborProfile(cfg, profileRepo, "tenant-dev"); err != nil {
		log.Printf("[app-market] ensure default Harbor profile failed: %v", err)
	}

	// Harbor robot client + OCI storage (if Harbor configured)
	var robotClient *storage.RobotClient
	var oci *storage.OCIStorage
	if cfg.HarborURL != "" {
		robotClient = storage.NewRobotClient(storage.StorageConfig{
			RegistryURL: cfg.HarborURL,
			Username:    cfg.HarborUser,
			Password:    cfg.HarborPass,
		})
		oci = storage.NewOCIStorage(storage.StorageConfig{
			RegistryURL: cfg.HarborURL,
			Username:    cfg.HarborUser,
			Password:    cfg.HarborPass,
		})
		log.Printf("[app-market] OCI storage: %s", cfg.HarborURL)
	}

	// Background goroutine: clean up expired upload sessions
	if robotClient != nil {
		go cleanupExpiredSessions(robotClient, sessionRepo)
	}

	// Helm syncer
	helmSyncer := helm.NewSyncer()

	// HTTP Mux
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	// Health
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// Publishers
	mux.HandleFunc("GET /api/v1/publishers", func(w http.ResponseWriter, r *http.Request) {
		tenantID := trustedContext(r).TenantID
		publishers, err := pubRepo.List(tenantID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(publishers)
	})

	mux.HandleFunc("POST /api/v1/publishers", func(w http.ResponseWriter, r *http.Request) {
		var p appstore.Publisher
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		p.ID = uuid.NewString()
		applyPublisherIdentity(r, &p)
		p.Status = appstore.PubActive
		if err := pubRepo.Create(&p); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(p)
	})

	// Products
	mux.HandleFunc("GET /api/v1/products", func(w http.ResponseWriter, r *http.Request) {
		tenantID := trustedContext(r).TenantID
		publisherID := r.URL.Query().Get("publisher_id")
		if publisherID != "" {
			if _, err := pubRepo.Get(publisherID, tenantID); err != nil {
				writeMarketRepoError(w, err)
				return
			}
			products, err := prodRepo.List(publisherID, tenantID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			json.NewEncoder(w).Encode(products)
			return
		}
		// Search
		query := r.URL.Query().Get("q")
		category := r.URL.Query().Get("category")
		scope := r.URL.Query().Get("scope")
		if scope == "" {
			scope = "tenant"
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		pageSize, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
		if page < 1 {
			page = 1
		}
		if pageSize < 1 || pageSize > 100 {
			pageSize = 9
		}
		offset := (page - 1) * pageSize
		products, total, err := prodRepo.Search(tenantID, query, category, scope, pageSize, offset)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"items": products, "total": total, "page": page, "pageSize": pageSize})
	})

	mux.HandleFunc("POST /api/v1/products", func(w http.ResponseWriter, r *http.Request) {
		var p appstore.Product
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		tenantID := trustedContext(r).TenantID
		if p.PublisherID == "" {
			def, err := pubRepo.DefaultPublisher(tenantID)
			if err != nil {
				writeMarketRepoError(w, err)
				return
			}
			p.PublisherID = def.ID
		}
		p.ID = uuid.NewString()
		p.Status = appstore.ProdDraft
		if err := prodRepo.Create(&p, tenantID); err != nil {
			writeMarketRepoError(w, err)
			return
		}
		json.NewEncoder(w).Encode(p)
	})

	mux.HandleFunc("PATCH /api/v1/products/{id}", func(w http.ResponseWriter, r *http.Request) {
		product, err := prodRepo.Get(r.PathValue("id"), trustedContext(r).TenantID)
		if err != nil {
			writeMarketRepoError(w, err)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(product); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		product.ID = r.PathValue("id")
		if err := prodRepo.Update(product, trustedContext(r).TenantID, trustedContext(r).SubjectID); err != nil {
			writeMarketRepoError(w, err)
			return
		}
		json.NewEncoder(w).Encode(product)
	})

	mux.HandleFunc("DELETE /api/v1/products/{id}", func(w http.ResponseWriter, r *http.Request) {
		tenantID := trustedContext(r).TenantID
		product, err := prodRepo.Get(r.PathValue("id"), tenantID)
		if err != nil {
			writeMarketRepoError(w, err)
			return
		}
		settings, err := recycleRepo.EnsureSettings(tenantID)
		if err != nil {
			writeMarketRepoError(w, err)
			return
		}
		_, err = recycleRepo.Tombstone(tenantID, "product", product.ID, product.Name, settings.ProductRetention, "", trustedContext(r).SubjectID, "deleted from console", map[string]any{"publisher_id": product.PublisherID, "status": product.Status})
		if err != nil {
			writeMarketRepoError(w, err)
			return
		}
		_ = prodRepo.UpdateStatus(product.ID, tenantID, appstore.ProdArchived)
		w.WriteHeader(http.StatusAccepted)
	})

	// Releases
	mux.HandleFunc("GET /api/v1/products/{productId}/releases", func(w http.ResponseWriter, r *http.Request) {
		productID := r.PathValue("productId")
		tenantID := trustedContext(r).TenantID
		if _, err := prodRepo.Get(productID, tenantID); err != nil {
			writeMarketRepoError(w, err)
			return
		}
		releases, err := relRepo.ListByProduct(productID, tenantID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(releases)
	})

	mux.HandleFunc("POST /api/v1/products/{productId}/releases", func(w http.ResponseWriter, r *http.Request) {
		var rel appstore.Release
		if err := json.NewDecoder(r.Body).Decode(&rel); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		rel.ID = uuid.NewString()
		rel.ProductID = r.PathValue("productId")
		rel.Status = appstore.RelDraft
		applyReleaseIdentity(r, &rel)
		if err := relRepo.Create(&rel, trustedContext(r).TenantID); err != nil {
			writeMarketRepoError(w, err)
			return
		}
		json.NewEncoder(w).Encode(rel)
	})

	mux.HandleFunc("PATCH /api/v1/releases/{id}", func(w http.ResponseWriter, r *http.Request) {
		rel, err := relRepo.Get(r.PathValue("id"), trustedContext(r).TenantID)
		if err != nil {
			writeMarketRepoError(w, err)
			return
		}
		if rel.Status != appstore.RelDraft {
			http.Error(w, "published release is immutable", http.StatusConflict)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(rel); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		rel.ID = r.PathValue("id")
		rel.Status = appstore.RelDraft
		if err := relRepo.Update(rel, trustedContext(r).TenantID); err != nil {
			writeMarketRepoError(w, err)
			return
		}
		json.NewEncoder(w).Encode(rel)
	})

	mux.HandleFunc("DELETE /api/v1/releases/{id}", func(w http.ResponseWriter, r *http.Request) {
		rel, err := relRepo.Get(r.PathValue("id"), trustedContext(r).TenantID)
		if err != nil {
			writeMarketRepoError(w, err)
			return
		}
		if rel.Status != appstore.RelDraft {
			http.Error(w, "published release is immutable", http.StatusConflict)
			return
		}
		if err := relRepo.Delete(r.PathValue("id"), trustedContext(r).TenantID); err != nil {
			writeMarketRepoError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /api/v1/releases/{id}/publish", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if err := relRepo.Publish(id, trustedContext(r).TenantID); err != nil {
			writeMarketRepoError(w, err)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "published"})
	})

	mux.HandleFunc("GET /api/v1/releases/{id}/artifacts", func(w http.ResponseWriter, r *http.Request) {
		releaseID := r.PathValue("id")
		tenantID := trustedContext(r).TenantID
		if _, err := relRepo.Get(releaseID, tenantID); err != nil {
			writeMarketRepoError(w, err)
			return
		}
		artifacts, err := artifactRepo.ListByRelease(releaseID, tenantID)
		if err != nil {
			writeMarketRepoError(w, err)
			return
		}
		json.NewEncoder(w).Encode(artifacts)
	})

	mux.HandleFunc("POST /api/v1/releases/{id}/artifacts/{artifactId}", func(w http.ResponseWriter, r *http.Request) {
		artifact, err := artifactRepo.AttachToRelease(r.PathValue("id"), r.PathValue("artifactId"), trustedContext(r).TenantID)
		if err != nil {
			writeReleaseArtifactError(w, err)
			return
		}
		json.NewEncoder(w).Encode(artifact)
	})

	mux.HandleFunc("DELETE /api/v1/releases/{id}/artifacts/{artifactId}", func(w http.ResponseWriter, r *http.Request) {
		if err := artifactRepo.DetachFromRelease(r.PathValue("id"), r.PathValue("artifactId"), trustedContext(r).TenantID); err != nil {
			writeReleaseArtifactError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// Applications (install/upgrade/uninstall)
	mux.HandleFunc("POST /api/v1/applications", func(w http.ResponseWriter, r *http.Request) {
		var app appstore.Application
		if err := json.NewDecoder(r.Body).Decode(&app); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		app.ID = uuid.NewString()
		applyApplicationIdentity(r, &app)
		app.Status = appstore.AppDeploying
		if err := appRepo.Create(&app, trustedContext(r).TenantID); err != nil {
			writeMarketRepoError(w, err)
			return
		}

		// Publish install event to NATS
		publishMarketEvent(nc, "hnb.market.install", app, r.Context())

		json.NewEncoder(w).Encode(app)
	})

	mux.HandleFunc("DELETE /api/v1/applications/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		tenantID := trustedContext(r).TenantID
		if err := appRepo.UpdateStatus(id, tenantID, appstore.AppUninstalling); err != nil {
			writeMarketRepoError(w, err)
			return
		}

		publishMarketEvent(nc, "hnb.market.uninstall", map[string]string{"id": id, "tenant_id": tenantID}, r.Context())

		json.NewEncoder(w).Encode(map[string]string{"status": "uninstalling"})
	})

	mux.HandleFunc("GET /api/v1/applications", func(w http.ResponseWriter, r *http.Request) {
		tenantID := trustedContext(r).TenantID
		apps, err := appRepo.ListByTenant(tenantID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(apps)
	})

	// Application group handlers
	mux.HandleFunc("GET /api/v1/applications/groups", func(w http.ResponseWriter, r *http.Request) {
		tenantID := trustedContext(r).TenantID
		groups, err := groupRepo.List(tenantID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(groups)
	})

	mux.HandleFunc("POST /api/v1/applications/groups", func(w http.ResponseWriter, r *http.Request) {
		var group appstore.ApplicationGroup
		if err := json.NewDecoder(r.Body).Decode(&group); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		group.ID = uuid.NewString()
		group.TenantID = trustedContext(r).TenantID
		if group.Status == "" {
			group.Status = "active"
		}
		if group.GroupType == "" {
			group.GroupType = appstore.GroupCustom
		}
		group.CreatedAt = time.Now()
		group.UpdatedAt = time.Now()
		if err := groupRepo.Create(&group); err != nil {
			writeMarketRepoError(w, err)
			return
		}
		json.NewEncoder(w).Encode(group)
	})

	mux.HandleFunc("GET /api/v1/applications/groups/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		tenantID := trustedContext(r).TenantID
		group, err := groupRepo.Get(id, tenantID)
		if err != nil {
			writeMarketRepoError(w, err)
			return
		}
		json.NewEncoder(w).Encode(group)
	})

	mux.HandleFunc("PATCH /api/v1/applications/groups/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		tenantID := trustedContext(r).TenantID
		var req appstore.ApplicationGroup
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		req.ID = id
		req.TenantID = tenantID
		if err := groupRepo.Update(&req); err != nil {
			writeMarketRepoError(w, err)
			return
		}
		json.NewEncoder(w).Encode(req)
	})

	mux.HandleFunc("DELETE /api/v1/applications/groups/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		tenantID := trustedContext(r).TenantID
		if err := groupRepo.Delete(id, tenantID); err != nil {
			writeMarketRepoError(w, err)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
	})

	mux.HandleFunc("GET /api/v1/applications/groups/{id}/applications", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		tenantID := trustedContext(r).TenantID
		apps, err := appRepo.ListByGroup(id, tenantID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(apps)
	})

	mux.HandleFunc("GET /api/v1/artifacts", func(w http.ResponseWriter, r *http.Request) {
		tenantID := trustedContext(r).TenantID
		var artifacts []appstore.ArtifactDescriptor
		var err error
		if r.URL.Query().Get("unassigned") == "true" {
			artifacts, err = artifactRepo.ListUnassigned(tenantID, 100, 0)
		} else {
			artifacts, err = artifactRepo.List(tenantID, 100, 0)
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(artifacts)
	})

	mux.HandleFunc("GET /api/v1/artifacts/{id}", func(w http.ResponseWriter, r *http.Request) {
		artifact, err := artifactRepo.Get(r.PathValue("id"), trustedContext(r).TenantID)
		if err != nil {
			writeMarketRepoError(w, err)
			return
		}
		json.NewEncoder(w).Encode(artifact)
	})

	mux.HandleFunc("PATCH /api/v1/artifacts/{id}", func(w http.ResponseWriter, r *http.Request) {
		// Artifacts are immutable: digest, size, repository and artifact_type are
		// derived from the upload. To change anything, callers must delete and
		// re-upload via the Transfer Gateway. See POST /api/v1/artifacts/{id}/gc.
		w.Header().Set("Allow", "GET, POST /gc, DELETE")
		http.Error(w, "artifacts are immutable; delete and re-upload to change", http.StatusMethodNotAllowed)
	})

	mux.HandleFunc("GET /api/v1/artifacts/{id}/references", func(w http.ResponseWriter, r *http.Request) {
		refs, err := gcRepo.ListReferences(r.PathValue("id"), trustedContext(r).TenantID)
		if err != nil {
			writeMarketRepoError(w, err)
			return
		}
		json.NewEncoder(w).Encode(refs)
	})

	mux.HandleFunc("POST /api/v1/artifacts/{id}/references", func(w http.ResponseWriter, r *http.Request) {
		var ref appstore.ArtifactReference
		if err := json.NewDecoder(r.Body).Decode(&ref); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		trusted := trustedContext(r)
		ref.ID = uuid.NewString()
		ref.TenantID = trusted.TenantID
		ref.ArtifactID = r.PathValue("id")
		ref.CreatedBy = trusted.SubjectID
		if err := gcRepo.RegisterReference(&ref); err != nil {
			writeMarketRepoError(w, err)
			return
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(ref)
	})

	mux.HandleFunc("POST /api/v1/artifacts/{id}/gc/preview", func(w http.ResponseWriter, r *http.Request) {
		preview, err := gcRepo.Preview(r.PathValue("id"), trustedContext(r).TenantID, 24*time.Hour)
		if err != nil {
			writeMarketRepoError(w, err)
			return
		}
		json.NewEncoder(w).Encode(preview)
	})

	mux.HandleFunc("POST /api/v1/artifacts/{id}/gc", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			OperationID string `json:"operation_id"`
			Reason      string `json:"reason"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		trusted := trustedContext(r)
		tombstone, err := gcRepo.Execute(r.PathValue("id"), trusted.TenantID, uuid.NewString(), req.OperationID, trusted.SubjectID, req.Reason, 24*time.Hour)
		if err != nil {
			if errors.Is(err, store.ErrArtifactGCBlocked) {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			writeMarketRepoError(w, err)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(tombstone)
	})

	mux.HandleFunc("POST /api/v1/artifacts/upload", deprecatedArtifactUploadHandler)

	mux.HandleFunc("GET /api/v1/artifact-storage/profiles", func(w http.ResponseWriter, r *http.Request) {
		trusted := trustedContext(r)
		if err := ensureDefaultHarborProfile(cfg, profileRepo, trusted.TenantID); err != nil {
			log.Printf("[app-market] ensure tenant default Harbor profile failed: tenant=%s err=%v", trusted.TenantID, err)
		}
		profiles, err := profileRepo.List(trusted.TenantID, 100, 0)
		if err != nil {
			writeMarketRepoError(w, err)
			return
		}
		log.Printf("[app-market] list storage profiles tenant=%s count=%d", trusted.TenantID, len(profiles))
		json.NewEncoder(w).Encode(profiles)
	})

	mux.HandleFunc("GET /api/v1/artifact-storage/profiles/{id}", func(w http.ResponseWriter, r *http.Request) {
		profile, err := profileRepo.Get(r.PathValue("id"), trustedContext(r).TenantID)
		if err != nil {
			writeMarketRepoError(w, err)
			return
		}
		json.NewEncoder(w).Encode(profile)
	})

	mux.HandleFunc("POST /api/v1/artifact-storage/profiles", func(w http.ResponseWriter, r *http.Request) {
		var profile appstore.ArtifactStorageProfile
		if err := json.NewDecoder(r.Body).Decode(&profile); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		trusted := trustedContext(r)
		profile.ID = uuid.NewString()
		profile.TenantID = trusted.TenantID
		profile.CreatedBy = trusted.SubjectID
		if err := profileRepo.Create(&profile); err != nil {
			if errors.Is(err, store.ErrInvalidStorageProfile) {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			writeMarketRepoError(w, err)
			return
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(profile)
	})

	mux.HandleFunc("PATCH /api/v1/artifact-storage/profiles/{id}", func(w http.ResponseWriter, r *http.Request) {
		profile, err := profileRepo.Get(r.PathValue("id"), trustedContext(r).TenantID)
		if err != nil {
			writeMarketRepoError(w, err)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(profile); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		trusted := trustedContext(r)
		profile.ID = r.PathValue("id")
		profile.TenantID = trusted.TenantID
		profile.CreatedBy = trusted.SubjectID
		if err := profileRepo.Update(profile); err != nil {
			if errors.Is(err, store.ErrInvalidStorageProfile) {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			writeMarketRepoError(w, err)
			return
		}
		json.NewEncoder(w).Encode(profile)
	})

	mux.HandleFunc("DELETE /api/v1/artifact-storage/profiles/{id}", func(w http.ResponseWriter, r *http.Request) {
		if err := profileRepo.Delete(r.PathValue("id"), trustedContext(r).TenantID); err != nil {
			writeMarketRepoError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /api/v1/artifact-storage/profile-migrations", func(w http.ResponseWriter, r *http.Request) {
		var migration appstore.ArtifactProfileMigration
		if err := json.NewDecoder(r.Body).Decode(&migration); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		trusted := trustedContext(r)
		migration.ID = uuid.NewString()
		migration.TenantID = trusted.TenantID
		migration.Status = "requested"
		migration.RequestedBy = trusted.SubjectID
		if migration.IdempotencyKey == "" {
			migration.IdempotencyKey = fmt.Sprintf("artifact-profile-migration:%s:%s", migration.ArtifactID, migration.TargetProfileID)
		}
		if err := profileRepo.RequestMigration(&migration); err != nil {
			writeMarketRepoError(w, err)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(migration)
	})

	mux.HandleFunc("GET /api/v1/artifact-distributions", func(w http.ResponseWriter, r *http.Request) {
		targets, err := distributionRepo.List(trustedContext(r).TenantID, 100, 0)
		if err != nil {
			writeMarketRepoError(w, err)
			return
		}
		json.NewEncoder(w).Encode(targets)
	})

	mux.HandleFunc("GET /api/v1/artifact-distributions/{id}", func(w http.ResponseWriter, r *http.Request) {
		target, err := distributionRepo.Get(r.PathValue("id"), trustedContext(r).TenantID)
		if err != nil {
			writeMarketRepoError(w, err)
			return
		}
		json.NewEncoder(w).Encode(target)
	})

	mux.HandleFunc("POST /api/v1/artifact-distributions/{id}/rebuild", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			OperationID    string `json:"operation_id"`
			IdempotencyKey string `json:"idempotency_key"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, context.Canceled) {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		cmd, err := distributionRepo.RequestRebuild(r.PathValue("id"), trustedContext(r).TenantID, req.OperationID, req.IdempotencyKey)
		if err != nil {
			writeMarketRepoError(w, err)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(cmd)
	})

	transferGateway := newLocalTransferGateway(cfg.TransferBackend, cfg.TransferStagingDir, sessionRepo, artifactRepo, robotClient, oci)
	transferGateway.Register(mux)

	// Recycle bin
	mux.HandleFunc("GET /api/v1/recycle-bin/settings", func(w http.ResponseWriter, r *http.Request) {
		setting, err := recycleRepo.EnsureSettings(trustedContext(r).TenantID)
		if err != nil {
			writeMarketRepoError(w, err)
			return
		}
		json.NewEncoder(w).Encode(setting)
	})
	mux.HandleFunc("PATCH /api/v1/recycle-bin/settings", func(w http.ResponseWriter, r *http.Request) {
		var s appstore.RecycleBinSettings
		if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		if err := recycleRepo.UpdateSettings(trustedContext(r).TenantID, &s); err != nil {
			writeMarketRepoError(w, err)
			return
		}
		json.NewEncoder(w).Encode(s)
	})
	mux.HandleFunc("GET /api/v1/recycle-bin/entries", func(w http.ResponseWriter, r *http.Request) {
		tenantID := trustedContext(r).TenantID
		resourceType := r.URL.Query().Get("type")
		state := r.URL.Query().Get("state")
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		pageSize, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
		if page < 1 {
			page = 1
		}
		if pageSize < 1 || pageSize > 100 {
			pageSize = 20
		}
		entries, total, err := recycleRepo.ListByTenant(tenantID, resourceType, state, page, pageSize)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"items": entries, "total": total, "page": page, "pageSize": pageSize})
	})
	mux.HandleFunc("POST /api/v1/recycle-bin/entries/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		if err := recycleRepo.Cancel(r.PathValue("id"), trustedContext(r).TenantID); err != nil {
			writeMarketRepoError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/v1/recycle-bin/entries/{id}/purge", func(w http.ResponseWriter, r *http.Request) {
		if err := recycleRepo.PurgeNow(r.PathValue("id"), trustedContext(r).TenantID); err != nil {
			writeMarketRepoError(w, err)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	})

	// Background sweep worker
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			entries, err := recycleRepo.SweepPending(time.Now())
			if err != nil {
				log.Printf("[app-market] recycle sweep: %v", err)
				continue
			}
			for _, e := range entries {
				if err := recycleRepo.MarkDeleting(e.ID, e.TenantID); err != nil {
					log.Printf("[app-market] recycle sweep mark deleting: %v", err)
					continue
				}
				if err := recycleRepo.MarkDeleted(e.ID, e.TenantID); err != nil {
					log.Printf("[app-market] recycle sweep mark deleted: %v", err)
				}
			}
		}
	}()

	// Security scan worker
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			reports, err := securityRepo.ClaimQueued(5)
			if err != nil {
				log.Printf("[app-market] scan worker claim: %v", err)
				continue
			}
scanner := security.NewScannerFromHarbor(cfg.HarborURL, cfg.HarborUser, cfg.HarborPass)
			for _, report := range reports {
				_ = securityRepo.UpdateReportState(report.ID, report.TenantID, "running", nil, nil, "")
				artifact, err := artifactRepo.Get(report.ArtifactID, report.TenantID)
				if err != nil {
					_ = securityRepo.UpdateReportState(report.ID, report.TenantID, "failed", nil, nil, err.Error())
					continue
				}
				findings, err := scanner.Scan(context.Background(), artifact.RegistryURL, artifact.Digest)
				if err != nil {
					_ = securityRepo.UpdateReportState(report.ID, report.TenantID, "failed", nil, nil, err.Error())
					continue
				}
				for i := range findings.Findings {
					cnt, _ := securityRepo.CountAffectedImages(report.TenantID, findings.Findings[i].CVE)
					findings.Findings[i].AffectedImages = cnt + 1
				}
				summaryJSON, _ := json.Marshal(findings.SeveritySummary)
				findingsJSON, _ := json.Marshal(findings.Findings)
				_ = securityRepo.UpdateReportState(report.ID, report.TenantID, "completed", summaryJSON, findingsJSON, "")
			}
		}
	}()

	// Security routes
	scanner := security.NewScanner(cfg.TrivyEndpoint)
	mux.HandleFunc("GET /api/v1/security/profiles", func(w http.ResponseWriter, r *http.Request) {
		profiles, err := securityRepo.ListProfiles(trustedContext(r).TenantID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(profiles)
	})
	mux.HandleFunc("POST /api/v1/security/profiles", func(w http.ResponseWriter, r *http.Request) {
		var p appstore.ScanProfile
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		trusted := trustedContext(r)
		p.TenantID = trusted.TenantID
		p.CreatedBy = trusted.SubjectID
		if err := securityRepo.CreateProfile(&p); err != nil {
			writeMarketRepoError(w, err)
			return
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(p)
	})
	mux.HandleFunc("PATCH /api/v1/security/profiles/{id}", func(w http.ResponseWriter, r *http.Request) {
		var p appstore.ScanProfile
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		if err := securityRepo.UpdateProfile(r.PathValue("id"), trustedContext(r).TenantID, &p); err != nil {
			writeMarketRepoError(w, err)
			return
		}
		json.NewEncoder(w).Encode(p)
	})
	mux.HandleFunc("DELETE /api/v1/security/profiles/{id}", func(w http.ResponseWriter, r *http.Request) {
		if err := securityRepo.DeleteProfile(r.PathValue("id"), trustedContext(r).TenantID); err != nil {
			writeMarketRepoError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/v1/security/profiles/{id}/sync", func(w http.ResponseWriter, r *http.Request) {
		dbLabel, err := scanner.UpdateDB(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"db_label": dbLabel})
	})
	mux.HandleFunc("POST /api/v1/security/profiles/{id}/scan", func(w http.ResponseWriter, r *http.Request) {
		trusted := trustedContext(r)
		var req struct {
			ArtifactID string `json:"artifact_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		report, err := securityRepo.EnqueueScan(trusted.TenantID, req.ArtifactID, r.PathValue("id"), trusted.SubjectID)
		if err != nil {
			writeMarketRepoError(w, err)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(report)
	})
	mux.HandleFunc("GET /api/v1/security/db-status", func(w http.ResponseWriter, r *http.Request) {
		status, err := securityRepo.GetDBStatus(trustedContext(r).TenantID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(status)
	})
	mux.HandleFunc("GET /api/v1/security/reports", func(w http.ResponseWriter, r *http.Request) {
		tenantID := trustedContext(r).TenantID
		artifactID := r.URL.Query().Get("artifact_id")
		profileID := r.URL.Query().Get("profile_id")
		state := r.URL.Query().Get("state")
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		pageSize, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
		if page < 1 {
			page = 1
		}
		if pageSize < 1 || pageSize > 100 {
			pageSize = 20
		}
		offset := (page - 1) * pageSize
		reports, total, err := securityRepo.ListReports(tenantID, artifactID, profileID, state, pageSize, offset)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"items": reports, "total": total, "page": page, "pageSize": pageSize})
	})
	mux.HandleFunc("GET /api/v1/security/reports/{id}", func(w http.ResponseWriter, r *http.Request) {
		report, err := securityRepo.GetReport(r.PathValue("id"), trustedContext(r).TenantID)
		if err != nil {
			writeMarketRepoError(w, err)
			return
		}
		json.NewEncoder(w).Encode(report)
	})

	mux.HandleFunc("GET /api/v1/harbor/projects", func(w http.ResponseWriter, r *http.Request) {
		if cfg.HarborURL == "" {
			json.NewEncoder(w).Encode([]string{})
			return
		}
		req, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, cfg.HarborURL+"/api/v2.0/projects?page=1&page_size=100", nil)
		req.SetBasicAuth(cfg.HarborUser, cfg.HarborPass)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		var projects []struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&projects); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		names := make([]string, len(projects))
		for i, p := range projects {
			names[i] = p.Name
		}
		json.NewEncoder(w).Encode(names)
	})

	// Artifact upload session. The default path returns a unified transfer endpoint;
	// Harbor credentials are included only as a legacy direct-upload compatibility path.
	mux.HandleFunc("POST /api/v1/artifacts/session", func(w http.ResponseWriter, r *http.Request) {
		if cfg.TransferReadOnly {
			http.Error(w, "artifact transfer is read-only", http.StatusLocked)
			return
		}
		if cfg.HarborURL == "" {
			http.Error(w, "artifact repository is not configured; connect and configure Harbor/OCI storage before uploading artifacts", http.StatusConflict)
			return
		}
		var req struct {
			Filename     string  `json:"filename"`
			ArtifactType string  `json:"artifact_type"`
			SizeBytes    int64   `json:"size_bytes"`
			ReleaseID    *string `json:"release_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		if req.Filename == "" {
			http.Error(w, "filename required", http.StatusBadRequest)
			return
		}
		if !isAllowedArtifactFilename(req.Filename, req.ArtifactType) {
			http.Error(w, fmt.Sprintf("unsupported artifact format for %s", req.ArtifactType), http.StatusBadRequest)
			return
		}

		sessionID := uuid.NewString()
		tenantID := trustedContext(r).TenantID
		if req.ReleaseID != nil {
			if _, err := uuid.Parse(*req.ReleaseID); err != nil {
				http.Error(w, "invalid release_id", http.StatusBadRequest)
				return
			}
			release, err := relRepo.Get(*req.ReleaseID, tenantID)
			if err != nil {
				writeMarketRepoError(w, err)
				return
			}
			if release.Status != appstore.RelDraft {
				http.Error(w, store.ErrUploadReleaseState.Error(), http.StatusConflict)
				return
			}
		}

		repoName := resolveArtifactRepository(oci, req.Filename)
		robotID := 0
		robotName := ""
		robotToken := ""
		if robotClient != nil {
			robot, err := robotClient.CreateRobot(r.Context(), uploadSessionPrefix+sessionID[:8], harborProject, uploadSessionTTL)
			if err != nil {
				log.Printf("[app-market] create robot failed; continuing with transfer gateway only: %v", err)
			} else {
				robotID = robot.ID
				robotName = robot.Name
				robotToken = robot.Token
			}
		}

		now := time.Now()
		session := &appstore.UploadSession{
			ID:           sessionID,
			TenantID:     tenantID,
			ReleaseID:    req.ReleaseID,
			Filename:     req.Filename,
			ArtifactType: req.ArtifactType,
			SizeBytes:    req.SizeBytes,
			Status:       appstore.SessionPending,
			HarborURL:    cfg.HarborURL,
			Repository:   repoName,
			RobotID:      robotID,
			RobotName:    robotName,
			ExpiresAt:    now.Add(uploadSessionTTL * time.Second),
		}
		if err := sessionRepo.Create(session); err != nil {
			if robotClient != nil && robotID != 0 {
				robotClient.DeleteRobot(r.Context(), robotID)
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		resumeKey := fmt.Sprintf("hnb:artifact-upload:%s:%s", tenantID, sessionID)
		policy := uploadPolicyForSize(req.SizeBytes, resumeKey)
		json.NewEncoder(w).Encode(map[string]any{
			"session_id":           sessionID,
			"transfer_endpoint":    fmt.Sprintf("/api/v1/market/artifact-transfer/sessions/%s", sessionID),
			"strategy":             "multipart",
			"harbor_url":           cfg.HarborURL,
			"repository":           repoName,
			"robot_name":           robotName,
			"robot_token":          robotToken,
			"expires_at":           session.ExpiresAt,
			"artifact_target":      map[string]string{"backend": cfg.TransferBackend, "repository": repoName},
			"transfer_endpoint_v1": fmt.Sprintf("/api/v1/market/artifact-transfer/sessions/%s", sessionID),
			"upload_policy":        policy,
			"transfer": transferPolicy{
				Strategy:         "multipart",
				Endpoint:         fmt.Sprintf("/api/v1/market/artifact-transfer/sessions/%s", sessionID),
				Resumable:        true,
				ChunkSize:        policy.ChunkSize,
				MaxConcurrency:   policy.MaxConcurrency,
				MaxRetries:       policy.MaxRetries,
				ResumeStorageKey: resumeKey,
			},
		})
	})

	// Legacy Harbor direct upload confirmation remains available when Harbor is configured.
	if robotClient != nil && oci != nil {

		// Artifact upload confirmation
		mux.HandleFunc("POST /api/v1/artifacts/confirm", func(w http.ResponseWriter, r *http.Request) {
			var req struct {
				SessionID string `json:"session_id"`
				Digest    string `json:"digest"`
				SizeBytes int64  `json:"size_bytes"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "invalid body", http.StatusBadRequest)
				return
			}
			if req.SessionID == "" || req.Digest == "" {
				http.Error(w, "session_id and digest required", http.StatusBadRequest)
				return
			}
			if !storage.IsSHA256Digest(req.Digest) {
				http.Error(w, "invalid digest format (expected sha256:hex64)", http.StatusBadRequest)
				return
			}

			tenantID := trustedContext(r).TenantID
			session, err := sessionRepo.Get(req.SessionID, tenantID)
			if err != nil {
				writeMarketRepoError(w, err)
				return
			}
			if session.Status == appstore.SessionCompleted && session.ArtifactID != nil {
				artifact, err := artifactRepo.Get(*session.ArtifactID, tenantID)
				if err != nil {
					writeMarketRepoError(w, err)
					return
				}
				json.NewEncoder(w).Encode(map[string]any{
					"artifact_id": artifact.ID, "digest": artifact.Digest,
					"size_bytes": artifact.SizeBytes, "registry_url": artifact.RegistryURL,
				})
				return
			}
			if session.Status != appstore.SessionPending && session.Status != appstore.SessionUploading {
				http.Error(w, "session not in uploadable state", http.StatusConflict)
				return
			}
			if !session.ExpiresAt.After(time.Now()) {
				http.Error(w, "upload session expired", http.StatusConflict)
				return
			}

			verified, err := oci.VerifyManifest(r.Context(), session.Repository, req.Digest)
			if err != nil {
				if errors.Is(err, storage.ErrManifestNotFound) || errors.Is(err, storage.ErrDigestMismatch) {
					http.Error(w, err.Error(), http.StatusConflict)
				} else {
					http.Error(w, err.Error(), http.StatusServiceUnavailable)
				}
				return
			}
			if session.SizeBytes > 0 && verified.SizeBytes > 0 && session.SizeBytes != verified.SizeBytes {
				http.Error(w, "uploaded size does not match session", http.StatusConflict)
				return
			}
			if req.SizeBytes > 0 && verified.SizeBytes > 0 && req.SizeBytes != verified.SizeBytes {
				http.Error(w, "confirmed size does not match Harbor", http.StatusConflict)
				return
			}

			artifact, err := artifactRepo.ConfirmUpload(req.SessionID, tenantID, &appstore.ArtifactDescriptor{
				ID: uuid.NewString(), TenantID: tenantID, Name: session.Filename,
				ArtifactType: appstore.ArtifactType(session.ArtifactType), MediaType: verified.MediaType,
				Repository: session.Repository, RegistryURL: verified.RegistryURL, Digest: verified.Digest,
				SizeBytes: verified.SizeBytes, VerificationStatus: appstore.ArtifactVerified,
				LifecycleState: appstore.ArtifactActive,
			})
			if err != nil {
				if errors.Is(err, store.ErrUploadSessionExpired) || errors.Is(err, store.ErrUploadSessionState) || errors.Is(err, store.ErrUploadReleaseState) {
					http.Error(w, err.Error(), http.StatusConflict)
					return
				}
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			if err := robotClient.DeleteRobot(r.Context(), session.RobotID); err != nil {
				log.Printf("[app-market] delete upload robot failed: session=%s robot=%d err=%v", req.SessionID, session.RobotID, err)
			}

			log.Printf("[app-market] artifact confirmed: session=%s artifact=%s digest=%s", req.SessionID, artifact.ID, artifact.Digest)

			json.NewEncoder(w).Encode(map[string]any{
				"artifact_id":  artifact.ID,
				"digest":       artifact.Digest,
				"size_bytes":   artifact.SizeBytes,
				"registry_url": artifact.RegistryURL,
			})
		})
	}

	// Helm repo sync
	mux.HandleFunc("POST /api/v1/helm/sync", func(w http.ResponseWriter, r *http.Request) {
		var repo helm.RepoEntry
		if err := json.NewDecoder(r.Body).Decode(&repo); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}

		go func() {
			helmSyncer.Sync(&repo, func(name, version string, cv *helm.ChartVersion) error {
				log.Printf("[helm-sync] chart=%s version=%s", name, version)
				return nil
			})
		}()

		json.NewEncoder(w).Encode(map[string]string{"status": "syncing"})
	})

	// Metrics
	go func() {
		log.Printf("[metrics] serving on %s/metrics", cfg.MetricsAddr)
		http.ListenAndServe(cfg.MetricsAddr, promhttp.Handler())
	}()

	httpServer := &http.Server{Addr: cfg.ListenAddr, Handler: appMarketHTTPHandler(verifier, mux)}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		fmt.Println()
		log.Println("shutting down...")
		httpServer.Shutdown(context.Background())
	}()

	log.Printf("app-market listening on %s", cfg.ListenAddr)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("serve: %v", err)
	}
	log.Println("app-market stopped")
}

func resolveArtifactRepository(oci *storage.OCIStorage, filename string) string {
	if oci != nil {
		return oci.ResolveRepository(filename)
	}
	name := strings.ToLower(filename)
	name = strings.TrimSuffix(name, filepath.Ext(name))
	name = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '/' {
			return r
		}
		return '-'
	}, name)
	name = strings.Trim(name, "-/")
	if name == "" {
		name = "artifact"
	}
	return "hnb/" + name
}

type localTransferGateway struct {
	backend      string
	root         string
	sessionRepo  *store.UploadSessionRepo
	artifactRepo *store.ArtifactRepo
	robotClient  *storage.RobotClient
	oci          *storage.OCIStorage
	promotionCh  chan promotionJob
}

func newLocalTransferGateway(backend, root string, sessionRepo *store.UploadSessionRepo, artifactRepo *store.ArtifactRepo, robotClient *storage.RobotClient, oci *storage.OCIStorage) *localTransferGateway {
	if root == "" {
		root = defaultTransferDir
	}
	g := &localTransferGateway{backend: backend, root: root, sessionRepo: sessionRepo, artifactRepo: artifactRepo, robotClient: robotClient, oci: oci}
	if oci != nil {
		g.promotionCh = make(chan promotionJob, 64)
		go g.promotionWorker()
	}
	return g
}

type promotionJob struct {
	artifactID  string
	tenantID    string
	repository  string
	tag         string
	stagingPath string
	contentSize int64
}

func (g *localTransferGateway) promotionWorker() {
	for job := range g.promotionCh {
		g.runPromotion(job)
	}
}

func (g *localTransferGateway) runPromotion(job promotionJob) {
	ctx := context.Background()
	file, err := os.Open(job.stagingPath)
	if err != nil {
		log.Printf("[app-market] promotion open staging failed: artifact=%s err=%v", job.artifactID, err)
		_ = g.artifactRepo.MarkPromoted(job.artifactID, job.tenantID, "", appstore.ArtifactFailed)
		return
	}
	defer file.Close()

	annotations := map[string]string{
		"org.opencontainers.image.title": filepath.Base(job.stagingPath),
	}
	registryURL, err := g.oci.Push(ctx, job.repository, job.tag, file, job.contentSize, annotations)
	if err != nil {
		log.Printf("[app-market] promotion push failed: artifact=%s err=%v", job.artifactID, err)
		_ = g.artifactRepo.MarkPromoted(job.artifactID, job.tenantID, "", appstore.ArtifactFailed)
		return
	}

	if err := g.artifactRepo.MarkPromoted(job.artifactID, job.tenantID, registryURL, appstore.ArtifactVerified); err != nil {
		log.Printf("[app-market] promotion update failed: artifact=%s err=%v", job.artifactID, err)
		return
	}
	log.Printf("[app-market] promotion completed: artifact=%s registry_url=%s", job.artifactID, registryURL)
}

func (g *localTransferGateway) schedulePromotion(job promotionJob) {
	if g.promotionCh == nil {
		return
	}
	select {
	case g.promotionCh <- job:
		log.Printf("[app-market] promotion scheduled: artifact=%s repository=%s", job.artifactID, job.repository)
	default:
		log.Printf("[app-market] promotion queue full: artifact=%s", job.artifactID)
	}
}

func (g *localTransferGateway) cleanupRobot(ctx context.Context, session *appstore.UploadSession) {
	if g.robotClient == nil || session.RobotID == 0 {
		return
	}
	if err := g.robotClient.DeleteRobot(ctx, session.RobotID); err != nil {
		log.Printf("[app-market] delete upload robot failed: session=%s robot=%d err=%v", session.ID, session.RobotID, err)
	}
}

func (g *localTransferGateway) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/artifact-transfer/sessions/{id}", g.handleStatus)
	mux.HandleFunc("PUT /api/v1/artifact-transfer/sessions/{id}/parts/{partNumber}", g.handlePart)
	mux.HandleFunc("POST /api/v1/artifact-transfer/sessions/{id}/complete", g.handleComplete)
	mux.HandleFunc("POST /api/v1/artifact-transfer/sessions/{id}/abort", g.handleAbort)
}

// sanitizePathSegment replaces path separators and parent-directory sequences
// in a single path segment so it can never escape the staging root when joined.
func sanitizePathSegment(value string) string {
	value = strings.ReplaceAll(value, "\\", "/")
	cleaned := filepath.Clean(value)
	cleaned = strings.ReplaceAll(cleaned, "..", "_")
	cleaned = strings.NewReplacer("/", "_", "\\", "_", ":", "_").Replace(cleaned)
	if cleaned == "" || cleaned == "." {
		return "_"
	}
	return cleaned
}

func (g *localTransferGateway) sessionDir(tenantID, sessionID string) string {
	safeTenant := sanitizePathSegment(tenantID)
	safeSession := sanitizePathSegment(sessionID)
	dir := filepath.Join(g.root, safeTenant, safeSession)
	if !strings.HasPrefix(dir, filepath.Clean(g.root)+string(filepath.Separator)) {
		return filepath.Join(g.root, "restricted")
	}
	return dir
}

func (g *localTransferGateway) partPath(tenantID, sessionID string, partNumber int) string {
	return filepath.Join(g.sessionDir(tenantID, sessionID), "parts", fmt.Sprintf("%08d.part", partNumber))
}

func (g *localTransferGateway) getSession(w http.ResponseWriter, r *http.Request) (*appstore.UploadSession, string, bool) {
	tenantID := trustedContext(r).TenantID
	session, err := g.sessionRepo.Get(r.PathValue("id"), tenantID)
	if err != nil {
		writeMarketRepoError(w, err)
		return nil, "", false
	}
	if !session.ExpiresAt.After(time.Now()) {
		http.Error(w, "upload session expired", http.StatusConflict)
		return nil, "", false
	}
	return session, tenantID, true
}

func (g *localTransferGateway) handleStatus(w http.ResponseWriter, r *http.Request) {
	session, tenantID, ok := g.getSession(w, r)
	if !ok {
		return
	}
	parts, uploaded, err := listUploadedParts(filepath.Join(g.sessionDir(tenantID, session.ID), "parts"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(map[string]any{
		"session_id":      session.ID,
		"status":          session.Status,
		"uploaded_bytes":  uploaded,
		"total_bytes":     session.SizeBytes,
		"completed_parts": parts,
		"expires_at":      session.ExpiresAt,
	})
}

func (g *localTransferGateway) handlePart(w http.ResponseWriter, r *http.Request) {
	session, tenantID, ok := g.getSession(w, r)
	if !ok {
		return
	}
	if session.Status != appstore.SessionPending && session.Status != appstore.SessionUploading {
		http.Error(w, "session not uploadable", http.StatusConflict)
		return
	}
	partNumber, err := strconv.Atoi(r.PathValue("partNumber"))
	if err != nil || partNumber < 1 {
		http.Error(w, "invalid part number", http.StatusBadRequest)
		return
	}
	path := g.partPath(tenantID, session.ID, partNumber)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		log.Printf("[app-market] transfer mkdir failed: session=%s part=%d path=%s err=%v", session.ID, partNumber, path, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tmp := fmt.Sprintf("%s.%s.tmp", path, uuid.NewString())
	file, err := os.Create(tmp)
	if err != nil {
		log.Printf("[app-market] transfer create part failed: session=%s part=%d path=%s err=%v", session.ID, partNumber, tmp, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	written, copyErr := io.Copy(file, r.Body)
	closeErr := file.Close()
	if copyErr != nil {
		os.Remove(tmp)
		log.Printf("[app-market] transfer copy part failed: session=%s part=%d path=%s err=%v", session.ID, partNumber, tmp, copyErr)
		http.Error(w, copyErr.Error(), http.StatusInternalServerError)
		return
	}
	if closeErr != nil {
		os.Remove(tmp)
		log.Printf("[app-market] transfer close part failed: session=%s part=%d path=%s err=%v", session.ID, partNumber, tmp, closeErr)
		http.Error(w, closeErr.Error(), http.StatusInternalServerError)
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		log.Printf("[app-market] transfer rename part failed: session=%s part=%d tmp=%s path=%s err=%v", session.ID, partNumber, tmp, path, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if session.Status == appstore.SessionPending {
		if err := g.sessionRepo.UpdateStatus(session.ID, tenantID, appstore.SessionUploading); err != nil {
			log.Printf("[app-market] mark transfer uploading failed: session=%s err=%v", session.ID, err)
		}
	}
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]any{"part_number": partNumber, "size_bytes": written})
}

func (g *localTransferGateway) handleComplete(w http.ResponseWriter, r *http.Request) {
	session, tenantID, ok := g.getSession(w, r)
	if !ok {
		return
	}
	if session.Status == appstore.SessionCompleted && session.ArtifactID != nil {
		g.cleanupRobot(r.Context(), session)
		artifact, err := g.artifactRepo.Get(*session.ArtifactID, tenantID)
		if err != nil {
			writeMarketRepoError(w, err)
			return
		}
		json.NewEncoder(w).Encode(transferCompleteResponse(artifact))
		return
	}
	if session.Status != appstore.SessionPending && session.Status != appstore.SessionUploading {
		http.Error(w, "session not completable", http.StatusConflict)
		return
	}
	assembledPath := filepath.Join(g.sessionDir(tenantID, session.ID), "artifact.bin")
	digest, size, err := assembleParts(filepath.Join(g.sessionDir(tenantID, session.ID), "parts"), assembledPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if session.SizeBytes > 0 && session.SizeBytes != size {
		http.Error(w, "uploaded size does not match session", http.StatusConflict)
		return
	}
	artifact, err := g.artifactRepo.ConfirmUpload(session.ID, tenantID, &appstore.ArtifactDescriptor{
		ID: uuid.NewString(), TenantID: tenantID, Name: session.Filename,
		ArtifactType: appstore.ArtifactType(session.ArtifactType), MediaType: "application/octet-stream",
		Repository: session.Repository, RegistryURL: "staging://" + session.ID, Digest: digest,
		SizeBytes: size, VerificationStatus: appstore.ArtifactPending, LifecycleState: appstore.ArtifactActive,
		Metadata: map[string]any{"transfer_backend": g.backend, "staging_path": assembledPath},
	})
	if err != nil {
		if errors.Is(err, store.ErrUploadSessionExpired) || errors.Is(err, store.ErrUploadSessionState) || errors.Is(err, store.ErrUploadReleaseState) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	g.cleanupRobot(r.Context(), session)
	g.schedulePromotion(promotionJob{
		artifactID:  artifact.ID,
		tenantID:    tenantID,
		repository:  session.Repository,
		tag:         fmt.Sprintf("v1-%s", strings.ReplaceAll(uuid.NewString()[:8], "-", "")),
		stagingPath: assembledPath,
		contentSize: size,
	})
	json.NewEncoder(w).Encode(transferCompleteResponse(artifact))
}

func (g *localTransferGateway) handleAbort(w http.ResponseWriter, r *http.Request) {
	session, tenantID, ok := g.getSession(w, r)
	if !ok {
		return
	}
	if err := os.RemoveAll(g.sessionDir(tenantID, session.ID)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := g.sessionRepo.UpdateStatus(session.ID, tenantID, appstore.SessionFailed); err != nil {
		writeMarketRepoError(w, err)
		return
	}
	g.cleanupRobot(r.Context(), session)
	w.WriteHeader(http.StatusNoContent)
}

func listUploadedParts(dir string) ([]int, int64, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []int{}, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	parts := make([]int, 0, len(entries))
	var total int64
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".part") {
			continue
		}
		part, err := strconv.Atoi(strings.TrimSuffix(entry.Name(), ".part"))
		if err != nil {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, 0, err
		}
		parts = append(parts, part)
		total += info.Size()
	}
	return parts, total, nil
}

func assembleParts(partsDir, target string) (string, int64, error) {
	parts, _, err := listUploadedParts(partsDir)
	if err != nil {
		return "", 0, err
	}
	if len(parts) == 0 {
		return "", 0, errors.New("no uploaded parts")
	}
	partSet := make(map[int]struct{}, len(parts))
	for _, part := range parts {
		partSet[part] = struct{}{}
	}
	for i := 1; i <= len(parts); i++ {
		if _, ok := partSet[i]; !ok {
			return "", 0, fmt.Errorf("missing part %d", i)
		}
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", 0, err
	}
	out, err := os.Create(target)
	if err != nil {
		return "", 0, err
	}
	defer out.Close()
	hash := sha256.New()
	writer := io.MultiWriter(out, hash)
	var total int64
	for part := 1; part <= len(parts); part++ {
		in, err := os.Open(filepath.Join(partsDir, fmt.Sprintf("%08d.part", part)))
		if err != nil {
			return "", 0, err
		}
		written, copyErr := io.Copy(writer, in)
		closeErr := in.Close()
		if copyErr != nil {
			return "", 0, copyErr
		}
		if closeErr != nil {
			return "", 0, closeErr
		}
		total += written
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), total, nil
}

func transferCompleteResponse(artifact *appstore.ArtifactDescriptor) map[string]any {
	return map[string]any{
		"artifact_id":  artifact.ID,
		"digest":       artifact.Digest,
		"size_bytes":   artifact.SizeBytes,
		"registry_url": artifact.RegistryURL,
	}
}

func appMarketHTTPHandler(authenticator iam.AccessAuthenticator, handler http.Handler) http.Handler {
	return appMarketHTTPHandlerWithRoutes(authenticator, handler, appMarketRoutes)
}

func appMarketHTTPHandlerWithRoutes(authenticator iam.AccessAuthenticator, handler http.Handler, routes []iam.RouteMetadata) http.Handler {
	authorized := iam.AuthorizeRoutes(iam.NewEvaluator(), routes, handler)
	return iam.TrustedHTTPMiddleware(authenticator, "/health", "/metrics")(authorized)
}

func trustedContext(r *http.Request) iam.TrustedContext {
	trusted, _ := iam.TrustedContextFrom(r.Context())
	return trusted
}

func applyPublisherIdentity(r *http.Request, publisher *appstore.Publisher) {
	publisher.TenantID = trustedContext(r).TenantID
}

func applyReleaseIdentity(r *http.Request, release *appstore.Release) {
	release.CreatedBy = trustedContext(r).SubjectID
}

func applyApplicationIdentity(r *http.Request, application *appstore.Application) {
	application.TenantID = trustedContext(r).TenantID
}

func writeMarketRepoError(w http.ResponseWriter, err error) {
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

func writeReleaseArtifactError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrUploadReleaseState) || errors.Is(err, store.ErrArtifactNotAttachable) {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeMarketRepoError(w, err)
}

func deprecatedArtifactUploadHandler(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "proxy upload removed; use POST /api/v1/artifacts/session and upload directly to Harbor", http.StatusGone)
}

// publishMarketEvent serializes payload to JSON and publishes it to NATS.
// Errors are logged but do not fail the originating HTTP request because the
// control-plane write (DB) has already succeeded. The downstream worker will
// reconcile by polling the application status, so a missed event is recoverable.
func publishMarketEvent(nc *nats.Conn, subject string, payload any, ctx context.Context) {
	if nc == nil || nc.Status() != nats.CONNECTED {
		log.Printf("[app-market] nats not connected; skipping publish subject=%s", subject)
		return
	}
	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[app-market] nats publish marshal failed: subject=%s err=%v", subject, err)
		return
	}
	if err := nc.Publish(subject, data); err != nil {
		log.Printf("[app-market] nats publish failed: subject=%s err=%v", subject, err)
		return
	}
	if err := nc.Flush(); err != nil {
		log.Printf("[app-market] nats flush failed: subject=%s err=%v", subject, err)
	}
}

func cleanupExpiredSessions(robotClient *storage.RobotClient, sessionRepo *store.UploadSessionRepo) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		sessions, err := sessionRepo.ListExpired(50)
		if err != nil {
			log.Printf("[app-market] list expired sessions failed: %v", err)
			continue
		}
		for _, s := range sessions {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			if err := robotClient.DeleteRobot(ctx, s.RobotID); err != nil {
				log.Printf("[app-market] delete robot %d for session %s failed: %v", s.RobotID, s.ID, err)
			} else {
				log.Printf("[app-market] expired session %s cleaned (robot %d deleted)", s.ID, s.RobotID)
			}
			sessionRepo.UpdateStatus(s.ID, s.TenantID, appstore.SessionExpired)
			cancel()
		}
	}
}
