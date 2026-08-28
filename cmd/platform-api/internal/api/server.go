package api

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/F31/hnb/cmd/platform-api/internal/api/middleware"
	"github.com/F31/hnb/cmd/platform-api/internal/engine"
	"github.com/F31/hnb/cmd/platform-api/internal/service"
	stalepolicy "github.com/F31/hnb/cmd/platform-api/internal/stale"
	"github.com/F31/hnb/cmd/platform-api/internal/store"
	"github.com/F31/hnb/pkg/core"
	"github.com/F31/hnb/pkg/iam"
)

const (
	maxRequestBodyBytes = 1 << 20
	defaultListLimit    = 50
	maxListLimit        = 200
)

type Server struct {
	store              store.Store
	service            service.ClusterService
	engine             *engine.Engine
	mux                *http.ServeMux
	auth               iam.AccessAuthenticator
	authz              *iam.Evaluator
	auditWriter        auditLogWriter
	permissionResolver currentPermissionResolver
	staleSigner        *stalepolicy.Signer
	stalePolicy        stalepolicy.Policy
	delegationVerifier *iam.DelegationVerifier
	observerIngest     http.Handler
	secretCipher       secretCipher
	startupTime        time.Time
}

type currentPermissionResolver interface {
	ResolvePermissions(context.Context, string, string, string) (string, []iam.ScopedPermission, error)
}

// consoleCapabilities lists what this platform instance supports.
var consoleCapabilities = []map[string]string{
	{"id": "kubernetes_targets", "version": "v1"},
	{"id": "edge_targets", "version": "v1"},
	{"id": "helm_operations", "version": "v1"},
	{"id": "policy_enforcement", "version": "v1"},
	{"id": "runtime_intents", "version": "v1"},
}

// BootstrapSubject is the verified identity returned by console bootstrap.
type BootstrapSubject struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	DisplayName string `json:"displayName"`
}

// BootstrapMembership records a tenant membership derived from scoped permissions.
type BootstrapMembership struct {
	MembershipID string `json:"membershipId"`
	TenantID     string `json:"tenantId"`
	TenantName   string `json:"tenantName"`
}

// BootstrapPermission mirrors iam.ScopedPermission for the console contract.
type BootstrapPermission struct {
	TenantID     string `json:"tenantId"`
	ResourceKind string `json:"resourceKind"`
	ResourceID   string `json:"resourceId,omitempty"`
	Action       string `json:"action"`
}

var platformRoutes = []platformRoute{
	{Method: http.MethodGet, Pattern: "/healthz", ResourceKind: "", Action: "", Public: true},
	{Method: http.MethodGet, Pattern: "/livez", ResourceKind: "", Action: "", Public: true},
	{Method: http.MethodGet, Pattern: "/startupz", ResourceKind: "", Action: "", Public: true},
	{Method: http.MethodGet, Pattern: "/readyz", ResourceKind: "", Action: "", Public: true},
	{Method: http.MethodPost, Pattern: "/v1/intents", ResourceKind: string(iam.ResourceIntent), Action: iam.ActionCreate},
	{Method: http.MethodPost, Pattern: "/v1/runtime-intent-batches", ResourceKind: string(iam.ResourceCluster), Action: iam.ActionDelete},
	{Method: http.MethodPost, Pattern: "/v1/secrets:register", ResourceKind: string(iam.ResourceSecret), Action: iam.ActionCreate},
	{Method: http.MethodGet, Pattern: "/v1/console/bootstrap", ResourceKind: "consoleBootstrap", Public: true},
	{Method: http.MethodGet, Pattern: "/v1/session/bootstrap", ResourceKind: "consoleBootstrap", Public: true},
	{Method: http.MethodPost, Pattern: "/v1/operations", ResourceKind: string(iam.ResourceOperation), Action: iam.ActionExecute},
	{Method: http.MethodGet, Pattern: "/v1/operations", ResourceKind: string(iam.ResourceOperation), Action: iam.ActionList},
	{Method: http.MethodGet, Pattern: "/v1/operations/{id}", ResourceKind: string(iam.ResourceOperation), ResourceIDParam: "id", Action: iam.ActionRead},
	{Method: http.MethodPost, Pattern: "/v1/operations/{id}/approve", ResourceKind: string(iam.ResourceOperation), ResourceIDParam: "id", Action: iam.ActionApprove},
	{Method: http.MethodPost, Pattern: "/v1/operations/{id}/reject", ResourceKind: string(iam.ResourceOperation), ResourceIDParam: "id", Action: iam.ActionReject},
	{Method: http.MethodPost, Pattern: "/v1/operations/{id}/cancel", ResourceKind: string(iam.ResourceOperation), ResourceIDParam: "id", Action: iam.ActionCancel},
	{Method: http.MethodPost, Pattern: "/v1/targets", ResourceKind: "runtimeTarget", Action: iam.ActionCreate},
	{Method: http.MethodGet, Pattern: "/v1/targets", ResourceKind: "runtimeTarget", Action: iam.ActionList},
	{Method: http.MethodGet, Pattern: "/v1/targets/{id}", ResourceKind: "runtimeTarget", ResourceIDParam: "id", Action: iam.ActionRead},
	{Method: http.MethodDelete, Pattern: "/v1/targets/{id}", ResourceKind: "runtimeTarget", ResourceIDParam: "id", Action: iam.ActionDelete},
	{Method: http.MethodPost, Pattern: "/v1/clusters", ResourceKind: "cluster", Action: iam.ActionCreate},
	{Method: http.MethodGet, Pattern: "/v1/clusters", ResourceKind: "cluster", Action: iam.ActionList},
	{Method: http.MethodGet, Pattern: "/v1/clusters/{id}", ResourceKind: "cluster", ResourceIDParam: "id", Action: iam.ActionRead},
	{Method: http.MethodPut, Pattern: "/v1/clusters/{id}", ResourceKind: "cluster", ResourceIDParam: "id", Action: iam.ActionExecute},
	{Method: http.MethodDelete, Pattern: "/v1/clusters/{id}", ResourceKind: "cluster", ResourceIDParam: "id", Action: iam.ActionDelete},
	{Method: http.MethodPatch, Pattern: "/v1/clusters/{id}/description", ResourceKind: string(iam.ResourceCluster), ResourceIDParam: "id", Action: iam.ActionUpdate},
	{Method: http.MethodPost, Pattern: "/v1/clusters/{id}/kubeconfig:issue", ResourceKind: string(iam.ResourceCluster), ResourceIDParam: "id", Action: iam.ActionRead},
	{Method: http.MethodPut, Pattern: "/api/v1/clusters/{id}/heartbeat", ResourceKind: "cluster", Action: iam.ActionExecute},
	{Method: http.MethodPost, Pattern: "/v1/providers/{id}/manifest", ResourceKind: "provider", Action: iam.ActionExecute},
	{Method: http.MethodGet, Pattern: "/v1/providers/{id}/manifest", ResourceKind: "provider", Action: iam.ActionRead},
	{Method: http.MethodDelete, Pattern: "/v1/providers/{id}/manifest", ResourceKind: "provider", Action: iam.ActionDelete},
	{Method: http.MethodPost, Pattern: "/v1/compatibility", ResourceKind: "provider", Action: iam.ActionExecute},
	{Method: http.MethodGet, Pattern: "/v1/compatibility", ResourceKind: "provider", Action: iam.ActionRead},
}

type platformRoute struct {
	Method          string
	Pattern         string
	ResourceKind    string
	Action          iam.AuthorizationAction
	ResourceIDParam string
	Public          bool
}

func NewServer(st store.Store, authenticator iam.AccessAuthenticator, permissionResolvers ...currentPermissionResolver) *Server {
	s := &Server{
		store:       st,
		service:     service.NewClusterService(st),
		engine:      engine.NewEngine(lifecycleProviderResolver{store: st}),
		mux:         http.NewServeMux(),
		auth:        authenticator,
		authz:       iam.NewEvaluator(),
		startupTime: time.Now().UTC(),
	}
	if len(permissionResolvers) > 0 {
		s.permissionResolver = permissionResolvers[0]
	}
	s.stalePolicy = stalepolicy.DefaultPolicy()
	s.mux.HandleFunc("GET /healthz", s.handleHealth)
	s.mux.HandleFunc("GET /livez", s.handleLivez)
	s.mux.HandleFunc("GET /startupz", s.handleStartupz)
	s.mux.HandleFunc("GET /readyz", s.handleReadyz)
	s.mux.HandleFunc("POST /v1/intents", s.handleSubmitIntent)
	s.mux.HandleFunc("POST /v1/runtime-intent-batches", s.handleCreateRuntimeIntentBatch)
	s.mux.HandleFunc("POST /v1/secrets:register", s.handleRegisterSecret)
	s.mux.HandleFunc("GET /v1/console/bootstrap", s.handleConsoleBootstrap)
	s.mux.HandleFunc("GET /v1/session/bootstrap", s.handleConsoleBootstrap)
	s.mux.HandleFunc("POST /v1/operations", s.handleSubmitOperation)
	s.mux.HandleFunc("GET /v1/operations", s.handleListOperations)
	s.mux.HandleFunc("GET /v1/operations/{id}", s.handleGetOperation)
	s.mux.HandleFunc("POST /v1/operations/{id}/approve", s.handleApprove)
	s.mux.HandleFunc("POST /v1/operations/{id}/reject", s.handleReject)
	s.mux.HandleFunc("POST /v1/operations/{id}/cancel", s.handleCancel)
	s.mux.HandleFunc("POST /v1/targets", s.handleCreateRuntimeTarget)
	s.mux.HandleFunc("GET /v1/targets", s.handleListRuntimeTargets)
	s.mux.HandleFunc("GET /v1/targets/{id}", s.handleGetRuntimeTarget)
	s.mux.HandleFunc("DELETE /v1/targets/{id}", s.handleDeleteRuntimeTarget)
	s.mux.HandleFunc("POST /v1/clusters", s.handleCreateCluster)
	s.mux.HandleFunc("GET /v1/clusters", s.handleGetClusters)
	s.mux.HandleFunc("GET /v1/clusters/{id}", s.handleGetCluster)
	s.mux.HandleFunc("PUT /v1/clusters/{id}", s.handleUpdateCluster)
	s.mux.HandleFunc("DELETE /v1/clusters/{id}", s.handleDeleteCluster)
	s.mux.HandleFunc("PATCH /v1/clusters/{id}/description", s.handleUpdateClusterDescription)
	s.mux.HandleFunc("POST /v1/clusters/{id}/kubeconfig:issue", s.handleIssueClusterKubeconfig)
	s.mux.HandleFunc("PUT /api/v1/clusters/{id}/heartbeat", s.handleHeartbeatCluster)
	s.mux.HandleFunc("POST /v1/providers/{id}/manifest", s.handleSaveManifest)
	s.mux.HandleFunc("GET /v1/providers/{id}/manifest", s.handleGetManifest)
	s.mux.HandleFunc("DELETE /v1/providers/{id}/manifest", s.handleDeleteManifest)
	s.mux.HandleFunc("POST /v1/compatibility", s.handleSaveCompatibility)
	s.mux.HandleFunc("GET /v1/compatibility", s.handleCheckCompatibility)
	return s
}

func (s *Server) ConfigureStaleAdmission(signer *stalepolicy.Signer, policy stalepolicy.Policy) {
	s.staleSigner = signer
	s.stalePolicy = policy
}

func (s *Server) ConfigureIntentDelegation(verifier *iam.DelegationVerifier) {
	s.delegationVerifier = verifier
}

// ConfigureObserverIngest registers the runtime-target observation ingest
// routes. These routes self-authenticate via observer identity JWTs and are
// dispatched before the access-token middleware.
func (s *Server) ConfigureObserverIngest(handler http.Handler) {
	s.observerIngest = handler
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	r = withTraceHeaders(w, r)
	if s.observerIngest != nil && r.Method == http.MethodPost &&
		(r.URL.Path == "/v1/observations" || r.URL.Path == "/v1/observations/reset") {
		s.observerIngest.ServeHTTP(w, r)
		return
	}
	guarded := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		route := matchRoute(platformRoutes, r.Method, r.URL.Path)
		if route == nil {
			writeError(w, http.StatusForbidden, "forbidden", "permission denied")
			return
		}
		if !route.Public {
			if _, ok := iam.TrustedContextFrom(r.Context()); !ok {
				writeError(w, http.StatusUnauthorized, "unauthorized", "trusted context is required")
				return
			}
		}
		// AI-004: AI source writes must reference an operation
		if r.Header.Get("X-AI-Source") == "copilot" || r.Header.Get("X-AI-Source") == "aiops" {
			if isWriteMethod(r.Method) && r.Header.Get("X-Operation-Id") == "" {
				writeError(w, http.StatusBadRequest, "ai_bypass", "AI-initiated writes must include X-Operation-Id header")
				return
			}
		}
		s.mux.ServeHTTP(w, r)
	})
	accessTokenMiddleware := iam.TrustedHTTPMiddleware(s.auth, "/healthz", "/livez", "/startupz", "/readyz")
	var authed http.Handler
	if (r.Method == http.MethodPost && (r.URL.Path == "/v1/intents" || r.URL.Path == "/v1/secrets:register" || isClusterKubeconfigIssuePath(r.URL.Path))) ||
		(r.Method == http.MethodPatch && isClusterDescriptionPath(r.URL.Path)) {
		authed = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if s.delegationVerifier == nil {
				writeError(w, http.StatusUnauthorized, "invalid_delegation", "trusted service delegation is required")
				return
			}
			values := r.Header.Values("Authorization")
			if len(values) != 1 || !strings.HasPrefix(values[0], "Bearer ") {
				writeError(w, http.StatusUnauthorized, "invalid_delegation", "trusted service delegation is required")
				return
			}
			claims, trusted, err := s.delegationVerifier.Verify(r.Context(), strings.TrimPrefix(values[0], "Bearer "))
			if err != nil {
				writeError(w, http.StatusUnauthorized, "invalid_delegation", "trusted service delegation is required")
				return
			}
			r.Header.Del("Authorization")
			ctx := iam.WithTrustedContext(r.Context(), trusted)
			ctx = iam.WithDelegationClaims(ctx, *claims)
			guarded.ServeHTTP(w, r.WithContext(ctx))
		})
	} else if isOperationDelegationPath(r.Method, r.URL.Path) {
		authed = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Operation BFF forwarding uses a trusted service delegation when
			// presented; otherwise the request falls through to the normal
			// access-token middleware so browser clients keep working.
			if s.delegationVerifier != nil {
				values := r.Header.Values("Authorization")
				if len(values) == 1 && strings.HasPrefix(values[0], "Bearer ") {
					if claims, trusted, err := s.delegationVerifier.Verify(r.Context(), strings.TrimPrefix(values[0], "Bearer ")); err == nil {
						r.Header.Del("Authorization")
						ctx := iam.WithTrustedContext(r.Context(), trusted)
						ctx = iam.WithDelegationClaims(ctx, *claims)
						guarded.ServeHTTP(w, r.WithContext(ctx))
						return
					}
				}
			}
			accessTokenMiddleware(guarded).ServeHTTP(w, r)
		})
	} else {
		authed = accessTokenMiddleware(guarded)
	}
	// Apply error handling middleware
	handler := middleware.ErrorHandler(authed)
	handler.ServeHTTP(w, r)
}

// isOperationDelegationPath reports whether the request is an operation BFF
// forwarding (list, detail, or action) that presents a trusted service
// delegation instead of a browser access token.
func isOperationDelegationPath(method, path string) bool {
	if method == http.MethodGet && (path == "/v1/operations" || strings.HasPrefix(path, "/v1/operations/")) {
		return true
	}
	if method != http.MethodPost || !strings.HasPrefix(path, "/v1/operations/") {
		return false
	}
	rest := strings.TrimPrefix(path, "/v1/operations/")
	switch {
	case strings.HasSuffix(rest, "/approve"), strings.HasSuffix(rest, "/reject"), strings.HasSuffix(rest, "/cancel"):
		return true
	default:
		return false
	}
}

func isWriteMethod(method string) bool {
	return method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch || method == http.MethodDelete
}

// isClusterDescriptionPath reports whether the request targets the cluster
// description update endpoint (BFF-forwarded with a trusted service delegation).
func isClusterDescriptionPath(path string) bool {
	return strings.HasPrefix(path, "/v1/clusters/") && strings.HasSuffix(path, "/description")
}

// isClusterKubeconfigIssuePath reports whether the request targets the cluster
// kubeconfig issue endpoint (BFF-forwarded with a trusted service delegation).
func isClusterKubeconfigIssuePath(path string) bool {
	return strings.HasPrefix(path, "/v1/clusters/") && strings.HasSuffix(path, "/kubeconfig:issue")
}

func matchRoute(routes []platformRoute, method, path string) *platformRoute {
	for i := range routes {
		r := &routes[i]
		if r.Method != method {
			continue
		}
		if strings.Contains(r.Pattern, "{") || strings.Contains(r.Pattern, "*") {
			prefix := r.Pattern
			for {
				idx := strings.Index(prefix, "{")
				if idx < 0 {
					break
				}
				prefix = prefix[:idx]
			}
			if !strings.HasPrefix(path, prefix) {
				continue
			}
			// Also verify method matches.
		} else {
			if path != r.Pattern {
				continue
			}
		}
		return r
	}
	return nil
}

func (s *Server) authorize(w http.ResponseWriter, r *http.Request, resourceID, projectID, environmentID, namespaceID string) bool {
	route := matchRoute(platformRoutes, r.Method, r.URL.Path)
	if route == nil {
		writeError(w, http.StatusForbidden, "forbidden", "permission denied")
		return false
	}
	if route.Public {
		return true
	}
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "trusted context is required")
		return false
	}
	if resourceID == "" {
		resourceID = extractPathParam(r, route.Pattern, "id")
	}
	decision := s.authz.Evaluate(trusted, iam.AuthorizationRequest{
		SubjectID: trusted.SubjectID, TenantID: trusted.TenantID,
		ResourceKind: route.ResourceKind, ResourceID: resourceID, Action: route.Action,
		ProjectID: projectID, EnvironmentID: environmentID, NamespaceID: namespaceID,
	})
	if !decision.Allowed {
		writeError(w, http.StatusForbidden, decision.ReasonCode, "permission denied")
		return false
	}
	return true
}

func extractPathParam(r *http.Request, pattern, name string) string {
	if name == "" {
		return ""
	}
	idx := strings.Index(pattern, "{"+name+"}")
	if idx < 0 {
		return ""
	}
	return strings.TrimPrefix(r.URL.Path, pattern[:idx])
}

func (s *Server) hasAuthorizationCandidate(w http.ResponseWriter, r *http.Request, resourceID string) bool {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	route := matchRoute(platformRoutes, r.Method, r.URL.Path)
	if !ok || route == nil || route.Public || iam.ValidatePermissionSnapshot(trusted.PolicyVersion, trusted.ScopedPermissions, trusted.TenantID) != nil {
		writeError(w, http.StatusForbidden, "invalid_policy_snapshot", "permission denied")
		return false
	}
	for _, permission := range trusted.ScopedPermissions {
		if permission.TenantID == trusted.TenantID && permission.Action == route.Action &&
			(permission.ResourceKind == "*" || permission.ResourceKind == route.ResourceKind) &&
			(permission.ResourceID == "" || permission.ResourceID == resourceID) {
			return true
		}
	}
	writeError(w, http.StatusForbidden, "permission_denied", "permission denied")
	return false
}

// auditLogWriter is an optional interface for intent-level audit logging.
type auditLogWriter interface{}

// --- Handlers ---

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.store.Ping(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, "unhealthy", "database is not reachable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleLivez(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "alive"})
}

func (s *Server) handleStartupz(w http.ResponseWriter, _ *http.Request) {
	if s.startupTime.IsZero() {
		writeError(w, http.StatusServiceUnavailable, "starting_up", "server still initializing")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "started",
		"since":  s.startupTime.Format(time.RFC3339),
	})
}

func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.store.Ready(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, "not_ready", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

// handleSubmitIntent is the canonical entry point for RuntimeIntent submission.
func (s *Server) handleSubmitIntent(w http.ResponseWriter, r *http.Request) {
	trusted, _ := iam.TrustedContextFrom(r.Context())
	delegation, ok := iam.DelegationClaimsFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid_delegation", "trusted service delegation is required")
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBodyBytes))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "failed to read request body")
		return
	}

	intent, err := engine.ParseRuntimeIntent(body)
	if err != nil {
		var ve *engine.ValidationError
		if errors.As(err, &ve) {
			writeValidationProblem(w, r, ve)
			return
		}
		writeError(w, http.StatusBadRequest, "invalid_intent", err.Error())
		return
	}

	if err := intent.ValidateNoExtraFields(body); err != nil {
		writeError(w, http.StatusBadRequest, "forbidden_field_injection", err.Error())
		return
	}
	intentDigest := intent.ComputeIntentDigest()
	if supplied := strings.TrimSpace(r.Header.Get("X-Semantic-Digest")); supplied != "" && supplied != intentDigest {
		writeError(w, http.StatusBadRequest, "semantic-digest-mismatch", "semantic digest does not match the request")
		return
	}
	commitmentAction := string(iam.ActionCreate)
	if action, ok := iam.ClusterActionForIntentKind(string(intent.Kind)); ok {
		commitmentAction = string(action)
	} else if action, ok := iam.StorageBindingActionForIntentKind(string(intent.Kind)); ok {
		commitmentAction = string(action)
	} else if action, ok := iam.StorageDriverActionForIntentKind(string(intent.Kind)); ok {
		commitmentAction = string(action)
	} else if action, ok := iam.RetainedVolumeActionForIntentKind(string(intent.Kind)); ok {
		commitmentAction = string(action)
	}
	resourceKind := string(iam.ResourceCluster)
	resourceID := intent.Spec.TargetID
	if engine.IsStorageIntentKind(intent.Kind) {
		if engine.IsStorageDriverIntentKind(intent.Kind) {
			resourceKind = string(iam.ResourceStorageDriverInstallation)
			resourceID = intent.Spec.InstallationID
		} else if engine.IsRetainedVolumeIntentKind(intent.Kind) {
			resourceKind = string(iam.ResourceRetainedVolume)
			resourceID = intent.Spec.VolumeID
		} else {
			resourceKind = string(iam.ResourceStorageClassBinding)
			resourceID = intent.Spec.BindingID
			if intent.Kind == engine.IntentImportStorageClassBinding {
				resourceID = intent.Spec.OfferingID
			}
		}
	}
	if delegation.ActorSubject != trusted.SubjectID || delegation.MembershipID != trusted.MembershipID || delegation.TenantID != trusted.TenantID ||
		delegation.IntentKind != string(intent.Kind) || delegation.Action != iam.AuthorizationAction(commitmentAction) ||
		delegation.Scope.ResourceKind != resourceKind || delegation.Scope.ResourceID != resourceID ||
		delegation.SemanticDigest != intentDigest || delegation.CorrelationID != trusted.CorrelationID ||
		delegation.CorrelationID != strings.TrimSpace(r.Header.Get("X-Correlation-ID")) ||
		(intent.Metadata.CorrelationID != "" && delegation.CorrelationID != intent.Metadata.CorrelationID) {
		writeError(w, http.StatusUnauthorized, "delegation_evidence_mismatch", "delegation evidence does not match the request")
		return
	}
	if engine.IsClusterIntentKind(intent.Kind) {
		var allowed bool
		trusted, allowed = s.authorizeClusterCommitment(w, r, trusted, intent, iam.AuthorizationAction(commitmentAction))
		if !allowed {
			return
		}
	} else if engine.IsStorageIntentKind(intent.Kind) {
		var allowed bool
		trusted, allowed = s.authorizeStorageCommitment(w, r, trusted, resourceKind, resourceID, iam.AuthorizationAction(commitmentAction))
		if !allowed {
			return
		}
	} else {
		if s.permissionResolver != nil {
			policyVersion, permissions, err := s.permissionResolver.ResolvePermissions(r.Context(), trusted.SubjectID, trusted.MembershipID, trusted.TenantID)
			if err != nil {
				writeError(w, http.StatusForbidden, "permission_denied", "permission denied")
				return
			}
			trusted.PolicyVersion, trusted.ScopedPermissions = policyVersion, permissions
			r = r.WithContext(iam.WithTrustedContext(r.Context(), trusted))
		}
		if !s.authorize(w, r, "", "", "", "") {
			return
		}
	}
	if commitment, err := s.store.GetIntentCommitment(r.Context(), trusted.TenantID, string(intent.Kind), commitmentAction, intent.Metadata.IdempotencyKey); err == nil {
		if commitment.SemanticDigest != intentDigest {
			writeError(w, http.StatusConflict, "idempotency-conflict", "idempotency key is already committed to a different request")
			return
		}
		writeJSON(w, http.StatusAccepted, intentCommitmentResponse(commitment, true))
		return
	} else if !errors.Is(err, store.ErrNotFound) {
		log.Printf("platform-api: lookup intent commitment: %v", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal server error")
		return
	}

	var target *store.RuntimeTarget
	policyOutcome := stalepolicy.Outcome("")
	if engine.IsClusterIntentKind(intent.Kind) {
		var admitted bool
		trusted, target, admitted = s.admitClusterIntent(w, r, trusted, intent)
		if !admitted {
			return
		}
		policyOutcome, admitted = s.admitStaleTarget(w, r, trusted, target, intent)
		if !admitted {
			return
		}
		if !s.validateClusterSecretReferences(w, r, intent) {
			return
		}
	} else if engine.IsStorageIntentKind(intent.Kind) {
		var admitted bool
		target, admitted = s.admitStorageIntent(w, r, trusted, intent)
		if !admitted {
			return
		}
		if engine.IsStorageDriverIntentKind(intent.Kind) {
			if !s.validateStorageDriverSecretReferences(w, r, intent) {
				return
			}
		}
	}

	plan, err := s.engine.ProcessWithContext(r.Context(), intent, trusted.TenantID)
	if err != nil {
		if code, ok := engine.CompatibilityErrorCode(err); ok {
			status := http.StatusServiceUnavailable
			if code == engine.CodeTargetActionUnsupported {
				status = http.StatusConflict
			}
			writeError(w, status, code, err.Error())
			return
		}
		log.Printf("platform-api: intent planning failed: %v", err)
		writeError(w, http.StatusBadRequest, "planning_failed", err.Error())
		return
	}

	correlationID := trusted.CorrelationID

	cmd := store.IntentSubmitCommand{
		Intent:             intent,
		ExecutionPlan:      plan,
		TenantID:           trusted.TenantID,
		SubjectID:          trusted.SubjectID,
		CorrelationID:      correlationID,
		PolicyVersion:      trusted.PolicyVersion,
		InitiatedBy:        trusted.SubjectID,
		MembershipID:       trusted.MembershipID,
		CommitmentAction:   commitmentAction,
		ServiceSubject:     delegation.ServiceSubject,
		DelegationTokenID:  delegation.TokenID,
		DelegationKeyID:    delegation.KeyID,
		TraceID:            r.Header.Get("X-Trace-Id"),
		AuthorizationScope: delegation.Scope,
	}
	if target != nil {
		cmd.RuntimeTargetID = target.ID
		cmd.ExpectedTargetVersion = intent.Spec.ExpectedVersion
	}
	// For cluster create/import, atomically provision a runtime_targets row so
	// the console read model shows the cluster (PROVISIONING / REGISTERING)
	// before the lifecycle provider and observer report the live state.
	if target == nil && engine.IsClusterIntentKind(intent.Kind) &&
		(intent.Kind == engine.IntentCreateKubernetesTarget || intent.Kind == engine.IntentImportRuntimeTarget) &&
		plan.TargetSnapshot != nil && plan.TargetSnapshot.TargetID != "" {
		lifecycleState := "REGISTERING"
		source := "imported"
		if intent.Kind == engine.IntentCreateKubernetesTarget {
			lifecycleState = "PROVISIONING"
			source = "created"
		}
		targetType := "kubernetes"
		connectionType := "agent"
		connectionEndpoint := ""
		if intent.Spec.TargetKind == "EdgeRuntimeTarget" {
			targetType = "edge_runtime"
			connectionType = "cloudhub"
			connectionEndpoint = intent.Spec.CloudCoreEndpoint
		}
		cmd.ProvisionTarget = &store.ProvisionRuntimeTarget{
			TargetID:           plan.TargetSnapshot.TargetID,
			Name:               plan.TargetSnapshot.TargetID,
			DisplayName:        intent.Spec.DisplayName,
			TargetType:         targetType,
			ConnectionType:     connectionType,
			ConnectionEndpoint: connectionEndpoint,
			LifecycleState:     lifecycleState,
			Source:             source,
		}
	}
	if intent.Spec.RiskConfirmation != nil && policyOutcome != "" {
		cmd.StalePolicyOutcome = string(policyOutcome)
		cmd.ConfirmationAccepted = true
		cmd.ConfirmationReason = intent.Spec.RiskConfirmation.Reason
	}
	switch policyOutcome {
	case stalepolicy.OutcomeRequireApproval:
		cmd.InitialStatus = store.StatusPendingApproval
	case stalepolicy.OutcomeQueuedOffline:
		cmd.InitialStatus = store.StatusQueuedOffline
	case stalepolicy.OutcomeAllow:
		cmd.InitialStatus = store.StatusQueued
	}

	op, created, err := s.store.SubmitIntent(r.Context(), cmd)
	if err != nil {
		s.writeStoreError(w, r, err)
		return
	}

	resp := IntentResponse{
		IntentID:       op.IntentID,
		OperationID:    op.ID,
		PlanID:         op.PlanID,
		Kind:           string(intent.Kind),
		Status:         op.Status,
		CorrelationID:  correlationID,
		CreatedAt:      op.CreatedAt.Format(time.RFC3339),
		SemanticDigest: intentDigest,
		Replayed:       false,
	}
	if !created {
		resp.Replayed = true
	}
	writeJSON(w, http.StatusAccepted, resp)
}

func (s *Server) authorizeClusterCommitment(w http.ResponseWriter, r *http.Request, trusted iam.TrustedContext, intent *engine.RuntimeIntent, action iam.AuthorizationAction) (iam.TrustedContext, bool) {
	if s.permissionResolver != nil {
		policyVersion, permissions, err := s.permissionResolver.ResolvePermissions(r.Context(), trusted.SubjectID, trusted.MembershipID, trusted.TenantID)
		if err != nil {
			writeError(w, http.StatusForbidden, "permission_denied", "permission denied")
			return trusted, false
		}
		trusted.PolicyVersion, trusted.ScopedPermissions = policyVersion, permissions
	}
	decision := s.authz.Evaluate(trusted, iam.AuthorizationRequest{
		SubjectID: trusted.SubjectID, TenantID: trusted.TenantID, ResourceKind: string(iam.ResourceCluster),
		ResourceID: intent.Spec.TargetID, Action: action, ProjectID: trusted.ProjectID,
		EnvironmentID: trusted.EnvironmentID, NamespaceID: trusted.NamespaceID,
	})
	if !decision.Allowed {
		writeError(w, http.StatusForbidden, decision.ReasonCode, "permission denied")
		return trusted, false
	}
	return trusted, true
}

func (s *Server) authorizeStorageCommitment(w http.ResponseWriter, r *http.Request, trusted iam.TrustedContext, resourceKind, resourceID string, action iam.AuthorizationAction) (iam.TrustedContext, bool) {
	if s.permissionResolver != nil {
		policyVersion, permissions, err := s.permissionResolver.ResolvePermissions(r.Context(), trusted.SubjectID, trusted.MembershipID, trusted.TenantID)
		if err != nil {
			writeError(w, http.StatusForbidden, "permission_denied", "permission denied")
			return trusted, false
		}
		trusted.PolicyVersion, trusted.ScopedPermissions = policyVersion, permissions
	}
	decision := s.authz.Evaluate(trusted, iam.AuthorizationRequest{SubjectID: trusted.SubjectID, TenantID: trusted.TenantID, ResourceKind: resourceKind, ResourceID: resourceID, Action: action, ProjectID: trusted.ProjectID, EnvironmentID: trusted.EnvironmentID, NamespaceID: trusted.NamespaceID})
	if !decision.Allowed {
		writeError(w, http.StatusForbidden, decision.ReasonCode, "permission denied")
		return trusted, false
	}
	return trusted, true
}

func (s *Server) validateStorageDriverSecretReferences(w http.ResponseWriter, r *http.Request, intent *engine.RuntimeIntent) bool {
	trusted, _ := iam.TrustedContextFrom(r.Context())
	for _, ref := range intent.Spec.SecretReferences {
		metadata, err := s.store.ResolveSecretReference(r.Context(), trusted.TenantID, ref.Provider, ref.Scope, ref.Name, ref.Version)
		if err != nil || metadata.Purpose != "storage-driver" || metadata.AllowedLifecycleProviderID != intent.Spec.PackageID {
			writeError(w, http.StatusForbidden, "secret-reference-denied", "secret reference is not authorized for this storage driver")
			return false
		}
	}
	return true
}

func intentCommitmentResponse(commitment *store.IntentCommitment, replayed bool) IntentResponse {
	return IntentResponse{
		IntentID: commitment.IntentID, OperationID: commitment.OperationID,
		PlanID: commitment.ExecutionPlanID, Kind: commitment.Kind, Status: commitment.AcceptedStatus,
		CorrelationID: commitment.CorrelationID, CreatedAt: commitment.CreatedAt.UTC().Format(time.RFC3339),
		SemanticDigest: commitment.SemanticDigest, Replayed: replayed,
	}
}

func (s *Server) admitStaleTarget(w http.ResponseWriter, r *http.Request, trusted iam.TrustedContext, target *store.RuntimeTarget, intent *engine.RuntimeIntent) (stalepolicy.Outcome, bool) {
	if target == nil || (intent.Kind != engine.IntentUpgradeRuntimeTarget && intent.Kind != engine.IntentDeleteRuntimeTarget) {
		return "", true
	}
	staleTarget := target.ObservedAt == nil || time.Since(*target.ObservedAt) > time.Duration(target.StaleThresholdSec)*time.Second
	if !staleTarget {
		return "", true
	}
	if s.staleSigner == nil {
		writeError(w, http.StatusServiceUnavailable, "stale-policy-unavailable", "stale target policy is unavailable")
		return "", false
	}
	claims := staleChallengeClaims(trusted, target, intent)
	confirmation := intent.Spec.RiskConfirmation
	if confirmation == nil {
		token, err := s.staleSigner.Issue(claims)
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "stale-policy-unavailable", "stale target policy is unavailable")
			return "", false
		}
		if !s.appendStaleAudit(r, trusted, target, intent, "stale_challenge_issued", "not_applicable", "stale-confirmation-required", string(s.stalePolicy.Evaluate(string(intent.Kind))), false, "") {
			writeError(w, http.StatusServiceUnavailable, "audit-unavailable", "security audit is unavailable")
			return "", false
		}
		w.Header().Set("Content-Type", "application/problem+json")
		problem := map[string]any{
			"type": "https://hnb.example/problems/stale-confirmation-required", "title": "Stale confirmation required",
			"status": http.StatusConflict, "code": "STALE_CONFIRMATION_REQUIRED",
			"correlationId": w.Header().Get("X-Correlation-ID"), "traceId": canonicalTraceID(w.Header().Get("X-Trace-Id"), w.Header().Get("X-Correlation-ID")),
			"targetId": target.ID, "action": string(intent.Kind),
			"lifecycleState": target.LifecycleState, "healthState": target.HealthState, "connectivityState": target.ConnectivityState,
			"policyOutcome": s.stalePolicy.Evaluate(string(intent.Kind)), "confirmation": token,
		}
		if lastKnownStateAt := formatOptionalTime(target.LastKnownStateAt); lastKnownStateAt != "" {
			problem["lastKnownStateAt"] = lastKnownStateAt
		}
		writeJSON(w, http.StatusConflict, problem)
		return "", false
	}
	if err := s.staleSigner.Verify(confirmation.Confirmation, claims); err != nil {
		if !s.appendStaleAudit(r, trusted, target, intent, "stale_confirmation_rejected", "deny", "stale-confirmation-expired", "deny", false, "") {
			writeError(w, http.StatusServiceUnavailable, "audit-unavailable", "security audit is unavailable")
			return "", false
		}
		writeError(w, http.StatusConflict, "stale-confirmation-expired", "stale confirmation is invalid or expired")
		return "", false
	}
	outcome := s.stalePolicy.Evaluate(string(intent.Kind))
	if outcome == stalepolicy.OutcomeDeny {
		if !s.appendStaleAudit(r, trusted, target, intent, "stale_policy_decided", "deny", "stale-policy-denied", string(outcome), true, confirmation.Reason) {
			writeError(w, http.StatusServiceUnavailable, "audit-unavailable", "security audit is unavailable")
			return "", false
		}
		writeError(w, http.StatusConflict, "stale-policy-denied", "stale target policy denied the action")
		return outcome, false
	}
	return outcome, true
}

func (s *Server) appendStaleAudit(r *http.Request, trusted iam.TrustedContext, target *store.RuntimeTarget, intent *engine.RuntimeIntent, eventType, decision, reasonCode, outcome string, confirmed bool, reason string) bool {
	detail := map[string]any{
		"targetKind": intent.Spec.TargetKind, "projectionVersion": target.ProjectionVersion,
		"observationGeneration": target.ObservationGeneration, "observationRevision": target.ObservationRevision,
		"freshness": "STALE", "policyOutcome": outcome, "confirmationAccepted": confirmed,
	}
	if reason != "" {
		detail["reason"] = reason
	}
	err := s.store.AppendSecurityAudit(r.Context(), store.SecurityAuditRecord{
		TenantID: trusted.TenantID, SubjectID: trusted.SubjectID, EventType: eventType,
		Decision: decision, ReasonCode: reasonCode, Action: string(intent.Kind), ResourceID: target.ID,
		CorrelationID: trusted.CorrelationID, TraceID: r.Header.Get("X-Trace-Id"), Outcome: outcome, Detail: detail,
	})
	return err == nil
}

func staleChallengeClaims(trusted iam.TrustedContext, target *store.RuntimeTarget, intent *engine.RuntimeIntent) stalepolicy.ChallengeClaims {
	observedAt := int64(0)
	if target.ObservedAt != nil {
		observedAt = target.ObservedAt.Unix()
	}
	return stalepolicy.ChallengeClaims{
		TenantID: trusted.TenantID, ActorID: trusted.SubjectID, TargetID: target.ID,
		TargetKind: intent.Spec.TargetKind, IntentKind: string(intent.Kind), IntentDigest: intent.ComputeIntentDigest(),
		ProjectionVersion: target.ProjectionVersion, ObservationGeneration: target.ObservationGeneration,
		ObservationRevision: target.ObservationRevision, ObservedAt: observedAt,
	}
}

func formatOptionalTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func (s *Server) validateClusterSecretReferences(w http.ResponseWriter, r *http.Request, intent *engine.RuntimeIntent) bool {
	references := append([]engine.SecretReferenceEntry(nil), intent.Spec.SecretReferences...)
	if intent.Spec.CredentialSecretRef != nil {
		references = append(references, *intent.Spec.CredentialSecretRef)
	}
	if len(references) == 0 {
		return true
	}
	purpose := "kubeconfig"
	providerID := "runtime-target.lifecycle.kubernetes"
	if intent.Spec.TargetKind == "EdgeRuntimeTarget" {
		purpose = "cloudcore-client"
		providerID = "runtime-target.lifecycle.edge"
	}
	trusted, _ := iam.TrustedContextFrom(r.Context())
	for _, ref := range references {
		metadata, err := s.store.ResolveSecretReference(r.Context(), trusted.TenantID, ref.Provider, ref.Scope, ref.Name, ref.Version)
		if err != nil || metadata.Purpose != purpose || metadata.AllowedLifecycleProviderID != providerID {
			writeError(w, http.StatusForbidden, "secret-reference-denied", "secret reference is not authorized for this action")
			return false
		}
	}
	return true
}

// handleSubmitOperation remains for backward compatibility but is deprecated
// in favor of POST /v1/intents.
func (s *Server) handleSubmitOperation(w http.ResponseWriter, r *http.Request) {
	trusted, _ := iam.TrustedContextFrom(r.Context())
	var req submitOperationRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	req.TenantID = trusted.TenantID
	req.InitiatedBy = trusted.SubjectID
	req.CorrelationID = trusted.CorrelationID
	cmd, err := toSubmitCommand(&req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if !s.authorize(w, r, "", cmd.ProjectID, cmd.EnvironmentID, cmd.NamespaceID) {
		return
	}
	op, created, err := s.store.SubmitOperation(r.Context(), cmd)
	if err != nil {
		s.writeStoreError(w, r, err)
		return
	}
	status := http.StatusCreated
	if !created {
		status = http.StatusOK
	}
	writeJSON(w, status, toOperationResponse(op))
}

func (s *Server) handleConsoleBootstrap(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "trusted context is required")
		return
	}

	if !s.authorize(w, r, "", "", "", "") {
		return
	}

	subjectType := "user"
	switch trusted.SubjectType {
	case "service", "workload":
		subjectType = trusted.SubjectType
	}

	sub := BootstrapSubject{
		ID:          trusted.SubjectID,
		Type:        subjectType,
		DisplayName: trusted.SubjectID,
	}

	memberships := []BootstrapMembership{}
	for _, perm := range trusted.ScopedPermissions {
		if !containsMember(memberships, perm.TenantID) {
			memberships = append(memberships, BootstrapMembership{
				MembershipID: trusted.MembershipID,
				TenantID:     perm.TenantID,
				TenantName:   perm.TenantID,
			})
		}
	}
	if len(memberships) == 0 {
		memberships = append(memberships, BootstrapMembership{
			MembershipID: trusted.MembershipID,
			TenantID:     trusted.TenantID,
			TenantName:   trusted.TenantID,
		})
	}

	selectedTenant := trusted.TenantID
	if selectedTenant == "" && len(memberships) > 0 {
		selectedTenant = memberships[0].TenantID
	}

	capabilities := make([]map[string]string, len(consoleCapabilities))
	for i, c := range consoleCapabilities {
		capabilities[i] = c
	}
	for _, decision := range s.engine.AvailableLifecycleDecisions(r.Context()) {
		kind := "kubernetes"
		if decision.TargetKind == "EdgeRuntimeTarget" {
			kind = "edge"
		}
		capabilities = append(capabilities, map[string]string{
			"id": "runtime-target.lifecycle." + kind + "." + decision.Action, "version": decision.MatrixVersion,
		})
	}

	permissions := make([]BootstrapPermission, 0, len(trusted.ScopedPermissions))
	for _, p := range trusted.ScopedPermissions {
		permissions = append(permissions, BootstrapPermission{
			TenantID:     p.TenantID,
			ResourceKind: p.ResourceKind,
			ResourceID:   p.ResourceID,
			Action:       string(p.Action),
		})
	}

	resp := map[string]any{
		"subject":           sub,
		"selectedTenantId":  selectedTenant,
		"memberships":       memberships,
		"capabilities":      capabilities,
		"permissions":       permissions,
		"policyVersion":     trusted.PolicyVersion,
		"permissionVersion": trusted.PolicyVersion,
	}
	writeJSON(w, http.StatusOK, resp)
}

func containsMember(list []BootstrapMembership, tenantID string) bool {
	for _, m := range list {
		if m.TenantID == tenantID {
			return true
		}
	}
	return false
}

func (s *Server) handleApprove(w http.ResponseWriter, r *http.Request) {
	s.handleAction(w, r, s.store.ApproveOperation)
}

func (s *Server) handleReject(w http.ResponseWriter, r *http.Request) {
	s.handleAction(w, r, s.store.RejectOperation)
}

func (s *Server) handleCancel(w http.ResponseWriter, r *http.Request) {
	s.handleAction(w, r, s.store.CancelOperation)
}

// delegationOperationEvidence verifies a presented operation delegation (from
// the apiserver BFF) matches the request and re-resolves the actor's current
// permissions as the authority. When no delegation is present it returns the
// request unchanged and ok=true so the access-token path proceeds. On success
// it returns a request whose context carries the re-authorized trusted context.
func (s *Server) delegationOperationEvidence(w http.ResponseWriter, r *http.Request, id, expectedAction string) (*http.Request, bool) {
	trusted, _ := iam.TrustedContextFrom(r.Context())
	delegation, ok := iam.DelegationClaimsFrom(r.Context())
	if !ok {
		return r, true
	}
	if delegation.Scope.ResourceKind != string(iam.ResourceOperation) ||
		delegation.Scope.ResourceID != id ||
		delegation.Action != iam.AuthorizationAction(expectedAction) ||
		delegation.TenantID != trusted.TenantID || delegation.ActorSubject != trusted.SubjectID ||
		delegation.CorrelationID != trusted.CorrelationID ||
		delegation.CorrelationID != strings.TrimSpace(r.Header.Get("X-Correlation-ID")) {
		writeError(w, http.StatusUnauthorized, "delegation_evidence_mismatch", "delegation evidence does not match the request")
		return r, false
	}
	if s.permissionResolver != nil {
		policyVersion, permissions, err := s.permissionResolver.ResolvePermissions(r.Context(), trusted.SubjectID, trusted.MembershipID, trusted.TenantID)
		if err != nil {
			writeError(w, http.StatusForbidden, "permission_denied", "permission denied")
			return r, false
		}
		trusted.PolicyVersion, trusted.ScopedPermissions = policyVersion, permissions
	}
	decision := s.authz.Evaluate(trusted, iam.AuthorizationRequest{
		SubjectID: trusted.SubjectID, TenantID: trusted.TenantID,
		ResourceKind: string(iam.ResourceOperation), ResourceID: id, Action: iam.AuthorizationAction(expectedAction),
		ProjectID: trusted.ProjectID, EnvironmentID: trusted.EnvironmentID, NamespaceID: trusted.NamespaceID,
	})
	if !decision.Allowed {
		writeError(w, http.StatusForbidden, decision.ReasonCode, "permission denied")
		return r, false
	}
	return r.WithContext(iam.WithTrustedContext(r.Context(), trusted)), true
}

type actionFunc func(ctx context.Context, id, tenantID, actorID, reason string) (*store.Operation, error)

func (s *Server) handleAction(w http.ResponseWriter, r *http.Request, action actionFunc) {
	trusted, _ := iam.TrustedContextFrom(r.Context())
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	expectedAction := string(iam.ActionExecute)
	switch {
	case strings.HasSuffix(r.URL.Path, "/approve"):
		expectedAction = string(iam.ActionApprove)
	case strings.HasSuffix(r.URL.Path, "/reject"):
		expectedAction = string(iam.ActionReject)
	case strings.HasSuffix(r.URL.Path, "/cancel"):
		expectedAction = string(iam.ActionCancel)
	}
	r, ok = s.delegationOperationEvidence(w, r, id, expectedAction)
	if !ok {
		return
	}
	trusted, _ = iam.TrustedContextFrom(r.Context())
	if !s.hasAuthorizationCandidate(w, r, id) {
		return
	}
	op, err := s.store.GetOperation(r.Context(), id, trusted.TenantID)
	if err != nil {
		s.writeStoreError(w, r, err)
		return
	}
	if !s.authorize(w, r, id, op.ProjectID, op.EnvironmentID, op.NamespaceID) {
		return
	}
	var req actionRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	op, err = action(r.Context(), id, trusted.TenantID, trusted.SubjectID, req.Reason)
	if err != nil {
		s.writeStoreError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toOperationResponse(op))
}

func (s *Server) handleGetOperation(w http.ResponseWriter, r *http.Request) {
	trusted, _ := iam.TrustedContextFrom(r.Context())
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	r, ok = s.delegationOperationEvidence(w, r, id, string(iam.ActionRead))
	if !ok {
		return
	}
	trusted, _ = iam.TrustedContextFrom(r.Context())
	if !s.hasAuthorizationCandidate(w, r, id) {
		return
	}
	op, err := s.store.GetOperation(r.Context(), id, trusted.TenantID)
	if err != nil {
		s.writeStoreError(w, r, err)
		return
	}
	if !s.authorize(w, r, id, op.ProjectID, op.EnvironmentID, op.NamespaceID) {
		return
	}
	writeJSON(w, http.StatusOK, toOperationResponse(op))
}

func (s *Server) handleListOperations(w http.ResponseWriter, r *http.Request) {
	trusted, _ := iam.TrustedContextFrom(r.Context())
	r, ok := s.delegationOperationEvidence(w, r, "", string(iam.ActionList))
	if !ok {
		return
	}
	trusted, _ = iam.TrustedContextFrom(r.Context())
	if !s.authorize(w, r, "", "", "", "") {
		return
	}
	query := r.URL.Query()
	q := store.ListQuery{
		TenantID:      trusted.TenantID,
		Status:        query.Get("status"),
		OperationType: query.Get("type"),
		Limit:         defaultListLimit,
	}
	if q.Status != "" && !store.IsValidStatus(q.Status) {
		writeError(w, http.StatusBadRequest, "invalid_request", "unknown status filter")
		return
	}
	if q.OperationType != "" && !store.IsValidOperationType(q.OperationType) {
		writeError(w, http.StatusBadRequest, "invalid_request", "unknown type filter")
		return
	}
	if raw := query.Get("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > maxListLimit {
			writeError(w, http.StatusBadRequest, "invalid_request", "limit must be between 1 and 200")
			return
		}
		q.Limit = limit
	}
	if raw := query.Get("offset"); raw != "" {
		offset, err := strconv.Atoi(raw)
		if err != nil || offset < 0 {
			writeError(w, http.StatusBadRequest, "invalid_request", "offset must be a non-negative integer")
			return
		}
		q.Offset = offset
	}

	summaries, total, err := s.store.ListOperations(r.Context(), q)
	if err != nil {
		s.writeStoreError(w, r, err)
		return
	}
	resp := listOperationsResponse{
		Operations: make([]operationSummaryResponse, 0, len(summaries)),
		Total:      total,
		Limit:      q.Limit,
		Offset:     q.Offset,
	}
	for _, item := range summaries {
		resp.Operations = append(resp.Operations, toSummaryResponse(item))
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) writeStoreError(w http.ResponseWriter, r *http.Request, err error) {
	var validation *validationError
	switch {
	case errors.As(err, &validation):
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "not found")
	case errors.Is(err, store.ErrInvalidState):
		writeError(w, http.StatusConflict, "invalid_state", err.Error())
	case errors.Is(err, store.ErrTargetNotFound):
		writeError(w, http.StatusNotFound, "target_not_found", "runtime target not found")
	case errors.Is(err, store.ErrTargetVersionConflict):
		writeError(w, http.StatusConflict, "target-version-conflict", "runtime target version changed")
	case errors.Is(err, store.ErrStorageBindingConflict):
		writeError(w, http.StatusConflict, "storage-binding-conflict", "storage binding version or identity changed")
	case errors.Is(err, store.ErrStorageObservationConflict):
		writeError(w, http.StatusConflict, "storage-observation-conflict", "StorageClass observation is stale")
	case errors.Is(err, store.ErrIdempotencyConflict):
		writeError(w, http.StatusConflict, "idempotency-conflict", "idempotency key is already committed to a different request")
	default:
		log.Printf("platform-api: %s %s: %v", r.Method, r.URL.Path, err)
		writeError(w, http.StatusInternalServerError, "internal", "internal server error")
	}
}

func (s *Server) admitClusterIntent(w http.ResponseWriter, r *http.Request, trusted iam.TrustedContext, intent *engine.RuntimeIntent) (iam.TrustedContext, *store.RuntimeTarget, bool) {
	var target *store.RuntimeTarget
	if intent.Kind == engine.IntentUpgradeRuntimeTarget || intent.Kind == engine.IntentDeleteRuntimeTarget {
		var err error
		target, err = s.store.GetRuntimeTarget(r.Context(), intent.Spec.TargetID, trusted.TenantID)
		if err != nil {
			if !errors.Is(err, store.ErrTargetNotFound) {
				log.Printf("platform-api: resolve runtime target: %v", err)
			}
			writeError(w, http.StatusNotFound, "target_not_found", "runtime target not found")
			return trusted, nil, false
		}
		if runtimeTargetKind(target.TargetType) != intent.Spec.TargetKind {
			writeError(w, http.StatusNotFound, "target_not_found", "runtime target not found")
			return trusted, nil, false
		}
		if target.ProjectionVersion != intent.Spec.ExpectedVersion {
			writeError(w, http.StatusConflict, "target-version-conflict", "runtime target version changed")
			return trusted, nil, false
		}
	}

	if s.permissionResolver != nil {
		policyVersion, permissions, err := s.permissionResolver.ResolvePermissions(
			r.Context(), trusted.SubjectID, trusted.MembershipID, trusted.TenantID,
		)
		if err != nil {
			writeError(w, http.StatusForbidden, "permission_denied", "permission denied")
			return trusted, nil, false
		}
		trusted.PolicyVersion = policyVersion
		trusted.ScopedPermissions = permissions
	}
	action, ok := iam.ClusterActionForIntentKind(string(intent.Kind))
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_intent", "unsupported cluster intent kind")
		return trusted, nil, false
	}
	resourceID := ""
	if target != nil {
		resourceID = target.ID
	}
	decision := s.authz.Evaluate(trusted, iam.AuthorizationRequest{
		SubjectID: trusted.SubjectID, TenantID: trusted.TenantID,
		ResourceKind: string(iam.ResourceCluster), ResourceID: resourceID, Action: action,
		ProjectID: trusted.ProjectID, EnvironmentID: trusted.EnvironmentID, NamespaceID: trusted.NamespaceID,
	})
	if !decision.Allowed {
		writeError(w, http.StatusForbidden, decision.ReasonCode, "permission denied")
		return trusted, nil, false
	}
	return trusted, target, true
}

func (s *Server) admitStorageIntent(w http.ResponseWriter, r *http.Request, trusted iam.TrustedContext, intent *engine.RuntimeIntent) (*store.RuntimeTarget, bool) {
	target, err := s.store.GetRuntimeTarget(r.Context(), intent.Spec.TargetID, trusted.TenantID)
	if err != nil || runtimeTargetKind(target.TargetType) != "KubernetesTarget" {
		writeError(w, http.StatusNotFound, "target_not_found", "runtime target not found")
		return nil, false
	}
	if target.ProjectionVersion != intent.Spec.ExpectedVersion {
		writeError(w, http.StatusConflict, "target-version-conflict", "runtime target version changed")
		return nil, false
	}
	if engine.IsStorageDriverIntentKind(intent.Kind) {
		if target.KubernetesVersion == "" || strings.TrimPrefix(target.KubernetesVersion, "v") != strings.TrimPrefix(intent.Spec.KubernetesVersion, "v") {
			writeError(w, http.StatusConflict, "target-capability-conflict", "target Kubernetes capability is unknown or changed")
			return nil, false
		}
		expectedInstallationID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("hnb.storage-driver-installation\x00"+trusted.TenantID+"\x00"+intent.Spec.TargetID+"\x00"+intent.Spec.PackageID)).String()
		if intent.Spec.InstallationID != expectedInstallationID {
			writeError(w, http.StatusNotFound, "storage_driver_installation_not_found", "storage driver installation was not found")
			return nil, false
		}
	} else if engine.IsRetainedVolumeIntentKind(intent.Kind) && (target.ObservedAt == nil || time.Since(target.ObservedAt.UTC()) > time.Duration(target.StaleThresholdSec)*time.Second) {
		writeError(w, http.StatusConflict, "storage-observation-stale", "fresh retained-volume dependency evidence is required")
		return nil, false
	}
	return target, true
}

func runtimeTargetKind(targetType string) string {
	switch targetType {
	case "kubernetes":
		return "KubernetesTarget"
	case "edge_runtime":
		return "EdgeRuntimeTarget"
	default:
		return ""
	}
}

func (s *Server) handleCreateRuntimeTarget(w http.ResponseWriter, r *http.Request) {
	trusted, _ := iam.TrustedContextFrom(r.Context())
	var req createRuntimeTargetRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	req.TenantID = trusted.TenantID
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "name is required")
		return
	}
	if req.TargetType == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "targetType is required")
		return
	}
	if !s.authorize(w, r, "", "", "", "") {
		return
	}

	rt := &store.RuntimeTarget{
		TenantID:           req.TenantID,
		Name:               req.Name,
		DisplayName:        req.DisplayName,
		TargetType:         req.TargetType,
		Distribution:       req.Distribution,
		EdgeType:           req.EdgeType,
		EdgeConfig:         req.EdgeConfig,
		ConnectionType:     req.ConnectionType,
		ConnectionEndpoint: req.ConnectionEndpoint,
		Labels:             req.Labels,
		StaleThresholdSec:  req.StaleThresholdSec,
	}

	if err := s.store.CreateRuntimeTarget(r.Context(), rt); err != nil {
		log.Printf("platform-api: create runtime target: %v", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to create runtime target")
		return
	}

	writeJSON(w, http.StatusCreated, toRuntimeTargetResponse(rt))
}

func (s *Server) handleListRuntimeTargets(w http.ResponseWriter, r *http.Request) {
	trusted, _ := iam.TrustedContextFrom(r.Context())
	if !s.authorize(w, r, "", "", "", "") {
		return
	}

	targets, err := s.store.ListRuntimeTargets(r.Context(), trusted.TenantID)
	if err != nil {
		log.Printf("platform-api: list runtime targets: %v", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to list runtime targets")
		return
	}

	resp := listRuntimeTargetsResponse{
		Targets: make([]runtimeTargetResponse, 0, len(targets)),
		Total:   len(targets),
	}
	for _, rt := range targets {
		resp.Targets = append(resp.Targets, toRuntimeTargetResponse(rt))
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleGetRuntimeTarget(w http.ResponseWriter, r *http.Request) {
	trusted, _ := iam.TrustedContextFrom(r.Context())
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "target id must be a UUID")
		return
	}
	if !s.hasAuthorizationCandidate(w, r, id) {
		return
	}

	rt, err := s.store.GetRuntimeTarget(r.Context(), id, trusted.TenantID)
	if err != nil {
		if errors.Is(err, store.ErrTargetNotFound) {
			writeError(w, http.StatusNotFound, "target_not_found", "runtime target not found")
			return
		}
		log.Printf("platform-api: get runtime target: %v", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to get runtime target")
		return
	}
	if !s.authorize(w, r, id, "", "", "") {
		return
	}
	writeJSON(w, http.StatusOK, toRuntimeTargetResponse(rt))
}

func (s *Server) handleDeleteRuntimeTarget(w http.ResponseWriter, r *http.Request) {
	trusted, _ := iam.TrustedContextFrom(r.Context())
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "target id must be a UUID")
		return
	}
	if !s.hasAuthorizationCandidate(w, r, id) {
		return
	}
	if _, err := s.store.GetRuntimeTarget(r.Context(), id, trusted.TenantID); err != nil {
		writeError(w, http.StatusNotFound, "target_not_found", "runtime target not found")
		return
	}
	if !s.authorize(w, r, id, "", "", "") {
		return
	}

	if err := s.store.DeleteRuntimeTarget(r.Context(), id, trusted.TenantID); err != nil {
		if errors.Is(err, store.ErrTargetNotFound) {
			writeError(w, http.StatusNotFound, "target_not_found", "runtime target not found")
			return
		}
		log.Printf("platform-api: delete runtime target: %v", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to delete runtime target")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func pathUUID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "operation id must be a UUID")
		return "", false
	}
	return id, true
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBodyBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain a single JSON document")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	canonical := canonicalProblemCode(code, status)
	correlationID := w.Header().Get("X-Correlation-ID")
	traceID := canonicalTraceID(w.Header().Get("X-Trace-Id"), correlationID)
	w.Header().Set("Content-Type", "application/problem+json")
	writeJSON(w, status, problemDetails{
		Type:  "https://hnb.cloud/problems/" + strings.ToLower(strings.ReplaceAll(canonical, "_", "-")),
		Title: http.StatusText(status), Status: status, Detail: safeProblemDetail(canonical, message),
		Code: canonical, CorrelationID: correlationID, TraceID: traceID,
		Retryable: canonical == "UPSTREAM_UNAVAILABLE" || status == http.StatusServiceUnavailable,
	})
}

func writeValidationProblem(w http.ResponseWriter, r *http.Request, validation *engine.ValidationError) {
	correlationID := w.Header().Get("X-Correlation-ID")
	traceID := canonicalTraceID(w.Header().Get("X-Trace-Id"), correlationID)
	w.Header().Set("Content-Type", "application/problem+json")
	writeJSON(w, http.StatusBadRequest, problemDetails{
		Type: "https://hnb.cloud/problems/validation-failed", Title: "Bad Request", Status: http.StatusBadRequest,
		Detail: "One or more request fields are invalid.", Instance: r.URL.Path, Code: "VALIDATION_FAILED",
		CorrelationID: correlationID, TraceID: traceID,
		Violations: []problemViolation{{Field: validation.Field, Code: "INVALID_VALUE", Message: validation.Reason}},
	})
}

func canonicalProblemCode(code string, status int) string {
	aliases := map[string]string{
		"validation_error": "VALIDATION_FAILED", "invalid_request": "VALIDATION_FAILED", "invalid_intent": "VALIDATION_FAILED",
		"forbidden_field_injection": "VALIDATION_FAILED", "semantic-digest-mismatch": "VALIDATION_FAILED", "planning_failed": "VALIDATION_FAILED",
		"not_found": "NOT_FOUND", "target_not_found": "NOT_FOUND", "cluster_not_found": "NOT_FOUND",
		"forbidden": "FORBIDDEN", "permission_denied": "FORBIDDEN", "invalid_policy_snapshot": "FORBIDDEN",
		"idempotency-conflict": "IDEMPOTENCY_CONFLICT", "stale-confirmation-expired": "STALE_CONFIRMATION_EXPIRED",
		"stale-policy-denied": "STALE_POLICY_DENIED", "secret-reference-denied": "SECRET_REFERENCE_DENIED",
		"target-version-conflict": "TARGET_VERSION_CONFLICT", "invalid_state": "OPERATION_ACTION_NOT_ALLOWED",
		engine.CodeTargetActionUnsupported: engine.CodeTargetActionUnsupported,
		engine.CodeProviderRouteNotFound:   engine.CodeProviderRouteNotFound,
		engine.CodeProviderIncompatible:    engine.CodeProviderIncompatible,
	}
	if canonical, ok := aliases[code]; ok {
		return canonical
	}
	switch status {
	case http.StatusUnauthorized:
		return "UNAUTHORIZED"
	case http.StatusForbidden:
		return "FORBIDDEN"
	case http.StatusNotFound:
		return "NOT_FOUND"
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return "VALIDATION_FAILED"
	case http.StatusConflict:
		return "CONFLICT"
	case http.StatusServiceUnavailable, http.StatusBadGateway, http.StatusGatewayTimeout:
		return "SERVICE_UNAVAILABLE"
	default:
		return "INTERNAL_ERROR"
	}
}

func safeProblemDetail(code, _ string) string {
	details := map[string]string{
		"VALIDATION_FAILED": "One or more request fields are invalid.", "NOT_FOUND": "The requested resource was not found.",
		"FORBIDDEN": "Permission denied.", "UNAUTHORIZED": "Authentication is required.",
		"IDEMPOTENCY_CONFLICT":         "The idempotency key is committed to a different request.",
		"STALE_CONFIRMATION_EXPIRED":   "The stale confirmation is invalid or expired.",
		"STALE_POLICY_DENIED":          "The stale target policy denied the action.",
		"SECRET_REFERENCE_DENIED":      "The secret reference is not authorized for this action.",
		"TARGET_VERSION_CONFLICT":      "The runtime target version changed.",
		"TARGET_ACTION_UNSUPPORTED":    "The target does not support this action.",
		"PROVIDER_ROUTE_NOT_FOUND":     "The lifecycle provider route is unavailable.",
		"PROVIDER_INCOMPATIBLE":        "No compatible lifecycle provider is available.",
		"OPERATION_ACTION_NOT_ALLOWED": "The operation action is not allowed in its current state.",
		"SERVICE_UNAVAILABLE":          "The service is temporarily unavailable.", "INTERNAL_ERROR": "An internal error occurred.",
	}
	if detail, ok := details[code]; ok {
		return detail
	}
	return "The request could not be completed."
}

func canonicalTraceID(traceID, correlationID string) string {
	value := strings.ToLower(strings.ReplaceAll(traceID, "-", ""))
	if len(value) >= 16 {
		return value
	}
	digest := sha256.Sum256([]byte(correlationID))
	return fmt.Sprintf("%x", digest[:16])
}

func withTraceHeaders(w http.ResponseWriter, r *http.Request) *http.Request {
	correlationID := strings.TrimSpace(r.Header.Get("X-Correlation-ID"))
	if _, err := uuid.Parse(correlationID); err != nil {
		correlationID = uuid.NewString()
	}
	traceID := canonicalTraceID(strings.TrimSpace(r.Header.Get("X-Trace-Id")), correlationID)
	r.Header.Set("X-Trace-Id", traceID)
	r.Header.Set("X-Correlation-ID", correlationID)
	w.Header().Set("X-Trace-Id", traceID)
	w.Header().Set("X-Correlation-ID", correlationID)
	return r
}

func (s *Server) handleCreateCluster(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	// Validate authorization - CREATE permission on cluster
	if !s.authorize(w, r, "", "", "", "") {
		return
	}

	var req struct {
		Name          string            `json:"name"`
		ClusterType   string            `json:"clusterType"`
		APIEndpoint   string            `json:"apiEndpoint"`
		KubeconfigRef string            `json:"kubeconfigRef,omitempty"`
		Region        string            `json:"region,omitempty"`
		Zone          string            `json:"zone,omitempty"`
		Labels        map[string]string `json:"labels,omitempty"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	cluster, err := s.service.Register(ctx, &service.CreateClusterInput{
		Name:          req.Name,
		ClusterType:   req.ClusterType,
		APIEndpoint:   req.APIEndpoint,
		KubeconfigRef: req.KubeconfigRef,
		Region:        req.Region,
		Zone:          req.Zone,
		Labels:        req.Labels,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, "cluster_creation_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, cluster)
}

func (s *Server) handleGetClusters(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	// Validate authorization - LIST permission on cluster
	if !s.authorize(w, r, "", "", "", "") {
		return
	}

	clusterList, err := s.service.List(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "cluster_list_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, clusterList)
}

func (s *Server) handleGetCluster(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	// Validate authorization - READ permission on cluster
	if !s.hasAuthorizationCandidate(w, r, r.PathValue("id")) {
		return
	}
	id := r.PathValue("id")
	cluster, err := s.service.Get(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrClusterNotFound) {
			writeError(w, http.StatusNotFound, "cluster_not_found", "cluster not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "cluster_fetch_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, cluster)
}

func (s *Server) handleUpdateCluster(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	// Validate authorization - EXECUTE permission on cluster (UPDATE uses ActionExecute)
	if !s.hasAuthorizationCandidate(w, r, r.PathValue("id")) {
		return
	}
	id := r.PathValue("id")
	var req struct {
		Region *string            `json:"region,omitempty"`
		Zone   *string            `json:"zone,omitempty"`
		Labels *map[string]string `json:"labels,omitempty"`
		Status *string            `json:"status,omitempty"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	cluster, err := s.service.Update(ctx, id, &service.UpdateClusterInput{
		Region: req.Region,
		Zone:   req.Zone,
		Labels: req.Labels,
		Status: req.Status,
	})
	if err != nil {
		if errors.Is(err, store.ErrClusterNotFound) {
			writeError(w, http.StatusNotFound, "cluster_not_found", "cluster not found")
			return
		}
		writeError(w, http.StatusBadRequest, "cluster_update_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, cluster)
}

func (s *Server) handleDeleteCluster(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	// Validate authorization - DELETE permission on cluster
	if !s.hasAuthorizationCandidate(w, r, r.PathValue("id")) {
		return
	}
	id := r.PathValue("id")
	err := s.service.Delete(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrClusterNotFound) {
			writeError(w, http.StatusNotFound, "cluster_not_found", "cluster not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "cluster_delete_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

const maxClusterDescriptionLen = 2048

type updateClusterDescriptionRequest struct {
	Description string `json:"description"`
}

// handleUpdateClusterDescription is a BFF-forwarded, delegation-authenticated
// metadata write that updates the cluster's human-readable description on the
// runtime target read model.
func (s *Server) handleUpdateClusterDescription(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "trusted context is required")
		return
	}
	delegation, ok := iam.DelegationClaimsFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid_delegation", "trusted service delegation is required")
		return
	}
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "cluster id must be a UUID")
		return
	}
	if delegation.ActorSubject != trusted.SubjectID || delegation.MembershipID != trusted.MembershipID ||
		delegation.TenantID != trusted.TenantID ||
		delegation.Scope.ResourceKind != string(iam.ResourceClusterMetadata) || delegation.Scope.ResourceID != id ||
		delegation.Action != iam.ActionUpdate ||
		delegation.CorrelationID != trusted.CorrelationID {
		writeError(w, http.StatusUnauthorized, "delegation_evidence_mismatch", "delegation evidence does not match the request")
		return
	}
	trusted, allowed := s.authorizeClusterAction(w, r, trusted, id, iam.ActionUpdate)
	if !allowed {
		return
	}
	if _, err := s.store.GetRuntimeTarget(r.Context(), id, trusted.TenantID); err != nil {
		if errors.Is(err, store.ErrTargetNotFound) {
			writeError(w, http.StatusNotFound, "cluster_not_found", "cluster not found")
			return
		}
		log.Printf("platform-api: load cluster for description update: %v", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to load cluster")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBodyBytes))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "failed to read request body")
		return
	}
	var req updateClusterDescriptionRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "request body must be valid JSON")
		return
	}
	if len(req.Description) > maxClusterDescriptionLen {
		writeError(w, http.StatusBadRequest, "invalid_request", "description must be at most 2048 characters")
		return
	}
	if err := s.store.UpdateRuntimeTargetDescription(r.Context(), id, trusted.TenantID, req.Description); err != nil {
		if errors.Is(err, store.ErrTargetNotFound) {
			writeError(w, http.StatusNotFound, "cluster_not_found", "cluster not found")
			return
		}
		log.Printf("platform-api: update cluster description: %v", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to update description")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// handleIssueClusterKubeconfig returns the stored kubeconfig for a
// Kubernetes cluster so the console can download it. When the target carries a
// credential_ref (recorded at create/import), that exact secret reference is
// resolved and a missing secret fails closed (404); legacy targets without a
// reference fall back to the tenant's most recent kubeconfig secret.
func (s *Server) handleIssueClusterKubeconfig(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "trusted context is required")
		return
	}
	delegation, ok := iam.DelegationClaimsFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid_delegation", "trusted service delegation is required")
		return
	}
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "cluster id must be a UUID")
		return
	}
	if delegation.ActorSubject != trusted.SubjectID || delegation.MembershipID != trusted.MembershipID ||
		delegation.TenantID != trusted.TenantID ||
		delegation.Scope.ResourceKind != string(iam.ResourceClusterMetadata) || delegation.Scope.ResourceID != id ||
		delegation.Action != iam.ActionRead ||
		delegation.CorrelationID != trusted.CorrelationID {
		writeError(w, http.StatusUnauthorized, "delegation_evidence_mismatch", "delegation evidence does not match the request")
		return
	}
	trusted, allowed := s.authorizeClusterAction(w, r, trusted, id, iam.ActionRead)
	if !allowed {
		return
	}
	rt, err := s.store.GetRuntimeTarget(r.Context(), id, trusted.TenantID)
	if err != nil {
		if errors.Is(err, store.ErrTargetNotFound) {
			writeError(w, http.StatusNotFound, "cluster_not_found", "cluster not found")
			return
		}
		log.Printf("platform-api: load cluster for kubeconfig: %v", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to load cluster")
		return
	}
	if rt.TargetType != "kubernetes" {
		writeError(w, http.StatusConflict, "kubeconfig_unavailable", "kubeconfig download is only supported for Kubernetes clusters")
		return
	}
	if s.secretCipher == nil {
		writeError(w, http.StatusServiceUnavailable, "secret-store-unavailable", "secret storage is unavailable")
		return
	}
	var encrypted string
	if rt.CredentialRef != nil && strings.TrimSpace(rt.CredentialRef.Name) != "" {
		encrypted, err = s.store.KubeConfigEncryptedForRef(r.Context(), trusted.TenantID, rt.CredentialRef.Scope, rt.CredentialRef.Name)
	} else {
		encrypted, err = s.store.LatestKubeConfigEncrypted(r.Context(), trusted.TenantID)
	}
	if err != nil {
		if errors.Is(err, store.ErrSecretReferenceDenied) {
			writeError(w, http.StatusNotFound, "kubeconfig_not_found", "no downloadable kubeconfig is registered for this cluster")
			return
		}
		log.Printf("platform-api: load kubeconfig secret: %v", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to load kubeconfig")
		return
	}
	raw, err := s.secretCipher.Decrypt(encrypted)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "kubeconfig_decrypt_failed", "failed to decrypt kubeconfig")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"kubeconfig": string(raw),
		"filename":   id + ".kubeconfig",
	})
}

func (s *Server) authorizeClusterAction(w http.ResponseWriter, r *http.Request, trusted iam.TrustedContext, resourceID string, action iam.AuthorizationAction) (iam.TrustedContext, bool) {
	if s.permissionResolver != nil {
		policyVersion, permissions, err := s.permissionResolver.ResolvePermissions(r.Context(), trusted.SubjectID, trusted.MembershipID, trusted.TenantID)
		if err != nil {
			writeError(w, http.StatusForbidden, "permission_denied", "permission denied")
			return trusted, false
		}
		trusted.PolicyVersion, trusted.ScopedPermissions = policyVersion, permissions
	}
	decision := s.authz.Evaluate(trusted, iam.AuthorizationRequest{
		SubjectID: trusted.SubjectID, TenantID: trusted.TenantID, ResourceKind: string(iam.ResourceCluster),
		ResourceID: resourceID, Action: action,
		ProjectID: trusted.ProjectID, EnvironmentID: trusted.EnvironmentID, NamespaceID: trusted.NamespaceID,
	})
	if !decision.Allowed {
		writeError(w, http.StatusForbidden, decision.ReasonCode, "permission denied")
		return trusted, false
	}
	return trusted, true
}

func (s *Server) handleHeartbeatCluster(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	// Authorization check for heartbeat (use ActionExecute on cluster resource)
	if !s.hasAuthorizationCandidate(w, r, r.PathValue("id")) {
		return
	}
	id := r.PathValue("id")
	err := s.service.Heartbeat(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrClusterNotFound) {
			writeError(w, http.StatusNotFound, "cluster_not_found", "cluster not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "heartbeat_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleSaveManifest(w http.ResponseWriter, r *http.Request) {
	providerID := r.PathValue("id")
	if !s.authorize(w, r, providerID, "", "", "") {
		return
	}
	var req struct {
		Name                 string                     `json:"name"`
		Version              string                     `json:"version"`
		ProtocolVersion      string                     `json:"protocolVersion"`
		Capabilities         []string                   `json:"capabilities"`
		Actions              []string                   `json:"actions"`
		StorageDriverPackage *core.StorageDriverPackage `json:"storageDriverPackage,omitempty"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if req.StorageDriverPackage != nil {
		if err := req.StorageDriverPackage.Validate(req.Version, time.Now()); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_storage_driver_package", err.Error())
			return
		}
	}
	manifest := &store.ProviderManifest{
		ProviderID:           providerID,
		Name:                 req.Name,
		Version:              req.Version,
		ProtocolVersion:      req.ProtocolVersion,
		Capabilities:         req.Capabilities,
		Actions:              req.Actions,
		StorageDriverPackage: req.StorageDriverPackage,
		ConformanceLevel:     "none",
	}
	if err := s.store.SaveManifest(r.Context(), manifest); err != nil {
		writeError(w, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, manifest)
}

func (s *Server) handleGetManifest(w http.ResponseWriter, r *http.Request) {
	providerID := r.PathValue("id")
	if !s.authorize(w, r, providerID, "", "", "") {
		return
	}
	manifest, err := s.store.GetManifest(r.Context(), providerID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "manifest not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "fetch_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, manifest)
}

func (s *Server) handleDeleteManifest(w http.ResponseWriter, r *http.Request) {
	providerID := r.PathValue("id")
	if !s.authorize(w, r, providerID, "", "", "") {
		return
	}
	if err := s.store.DeleteManifest(r.Context(), providerID); err != nil {
		writeError(w, http.StatusInternalServerError, "delete_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleSaveCompatibility(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, "", "", "", "") {
		return
	}
	var req struct {
		CoreVersion       string `json:"coreVersion"`
		ProviderID        string `json:"providerId"`
		ProviderVersion   string `json:"providerVersion"`
		RuntimeTargetType string `json:"runtimeTargetType"`
		Compatible        bool   `json:"compatible"`
		ConstraintReason  string `json:"constraintReason,omitempty"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	entry := &store.CompatibilityEntry{
		CoreVersion:       req.CoreVersion,
		ProviderID:        req.ProviderID,
		ProviderVersion:   req.ProviderVersion,
		RuntimeTargetType: req.RuntimeTargetType,
		Compatible:        req.Compatible,
		ConstraintReason:  req.ConstraintReason,
	}
	if err := s.store.SaveCompatibility(r.Context(), entry); err != nil {
		writeError(w, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, entry)
}

func (s *Server) handleCheckCompatibility(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, "", "", "", "") {
		return
	}
	coreVersion := r.URL.Query().Get("coreVersion")
	providerID := r.URL.Query().Get("providerId")
	providerVersion := r.URL.Query().Get("providerVersion")
	targetType := r.URL.Query().Get("runtimeTargetType")
	if coreVersion == "" || providerID == "" || providerVersion == "" {
		writeError(w, http.StatusBadRequest, "missing_params", "coreVersion, providerId, providerVersion are required")
		return
	}
	entry, err := s.store.CheckCompatibility(r.Context(), coreVersion, providerID, providerVersion, targetType)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeJSON(w, http.StatusOK, map[string]any{"compatible": true, "reason": "no constraint recorded"})
			return
		}
		writeError(w, http.StatusInternalServerError, "query_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, entry)
}
