package router

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	drapp "github.com/F31/hnb/cmd/apiserver/internal/application/dr"
	gslbapp "github.com/F31/hnb/cmd/apiserver/internal/application/gslb"
	schemaapp "github.com/F31/hnb/cmd/apiserver/internal/application/schema"
	"github.com/F31/hnb/cmd/apiserver/internal/capability"
	"github.com/F31/hnb/cmd/apiserver/internal/handler"
	drinfra "github.com/F31/hnb/cmd/apiserver/internal/infrastructure/dr"
	gslbinfra "github.com/F31/hnb/cmd/apiserver/internal/infrastructure/gslb"
	navinfra "github.com/F31/hnb/cmd/apiserver/internal/infrastructure/navigation"
	"github.com/F31/hnb/cmd/apiserver/internal/middleware"
	"github.com/F31/hnb/cmd/apiserver/internal/response"
	"github.com/F31/hnb/pkg/alert"
	"github.com/F31/hnb/pkg/audit"
	"github.com/F31/hnb/pkg/iam"
	"github.com/F31/hnb/pkg/tunnel"
)

var bypassPaths = []string{
	"/health", "/ready", "/openapi.json",
	"/api/v1/auth/login", "/api/v1/auth/refresh",
}

func New(db *sql.DB, ts *tunnel.TunnelServer, authMW *middleware.AuthMiddleware, tokenManager *iam.TokenManager, iamStore *iam.IAMDBStore, auditStore *audit.Store, rbac *iam.RBACEngine, reg *Registry, delegationSigner *iam.DelegationSigner, platformAPIURL, clusterProjectionMode, appMarketURL, harborURL, harborUser, harborPass string) http.Handler {
	return NewWithCapabilities(db, ts, authMW, tokenManager, iamStore, auditStore, rbac, reg, delegationSigner, nil, platformAPIURL, clusterProjectionMode, appMarketURL, harborURL, harborUser, harborPass, "", "", capability.AllStages())
}

// NewWithCapabilities wires the cluster-management routes behind the staged
// server capability gates. Routes gated by a disabled stage fail closed before
// any application state is touched.
func NewWithCapabilities(db *sql.DB, ts *tunnel.TunnelServer, authMW *middleware.AuthMiddleware, tokenManager *iam.TokenManager, iamStore *iam.IAMDBStore, auditStore *audit.Store, rbac *iam.RBACEngine, reg *Registry, delegationSigner *iam.DelegationSigner, agentTunnelSigner *iam.AgentTunnelTokenSigner, platformAPIURL, clusterProjectionMode, appMarketURL, harborURL, harborUser, harborPass, publicBaseURL, agentImage string, caps capability.Set) http.Handler {
	authH := handler.NewIAMHandler(
		iam.NewAuthenticator(iamStore),
		tokenManager,
		rbac,
		iamStore,
	)
	tenantH := handler.NewTenantHandler(db)
	tenantAdminH := handler.NewTenantAdminHandler(db, harborURL, harborUser, harborPass)
	clusterH := handler.NewClusterHandler(db)
	if platformAPIURL != "" {
		clusterH = handler.NewPlatformClusterHandler(platformAPIURL)
	}
	resourceClusterH := handler.NewResourceClusterHandler(db, platformAPIURL, clusterProjectionMode)
	resourceClusterH.ConfigureDelegation(delegationSigner)
	agentOnboardingH := handler.NewAgentOnboardingHandler(db, agentTunnelSigner, publicBaseURL, agentImage)
	monitoringH := handler.NewClusterMonitoringHandler(db, os.Getenv("HNB_PROMETHEUS_URL"))
	extensionH := handler.NewExtensionHandler(db)
	navigationH := handler.NewNavigationHandler(navinfra.NewCapabilityWrappingRepository(navinfra.NewPostgresRepository(db), caps.Snapshot()))
	schemaH := handler.NewSchemaHandlerWithService(schemaapp.NewService(schemaapp.NewPostgresRepository(db)))
	gslbApp := gslbapp.NewService(gslbinfra.NewPostgresStore(db))
	gslbH := handler.NewGSLBHandler(gslbApp)
	drH := handler.NewDRHandler(drapp.NewService(drinfra.NewPostgresStore(db), gslbApp))
	proxyH := handler.NewProxyHandler(db, ts)
	auditH := handler.NewAuditHandler(auditStore)
	sessionH := handler.NewSessionHandlerWithCapabilities(platformAPIURL, caps)
	settingsH := handler.NewSettingsHandler(db)
	storageH := handler.NewStorageHandler(handler.NewPostgresStorageStore(db))
	storageDesiredH := handler.NewStorageDesiredHandler(handler.NewPostgresStorageDesiredStore(db))
	storageAlertH := handler.NewStorageAlertHandler(alert.NewAlertDBStore(db))
	storageDesiredH.ConfigureOperations(platformAPIURL, delegationSigner)
	marketURL := ""
	if appMarketURL != "" {
		marketURL = appMarketURL
	}
	marketH := handler.NewMarketHandler(marketURL)
	pluginCatalogH := handler.NewPluginCatalogHandler(db, marketURL)
	capabilityH := handler.NewCapabilityHandler(caps)

	// Staged cluster capability gates (fail-closed before application state).
	readGate := gate(caps, capability.Read)
	writeGate := gate(caps, capability.Write)
	schemaGate := gate(caps, capability.Schema)
	providerGate := gate(caps, capability.Provider)
	contractGate := gate(caps, capability.Contract)

	// Core route handler map (code-registered)
	coreHandlers := map[string]http.HandlerFunc{
		"health": func(w http.ResponseWriter, r *http.Request) {
			response.Success(w, map[string]string{"status": "ok"})
		},
		"ready": func(w http.ResponseWriter, r *http.Request) {
			response.Success(w, map[string]string{"status": "ready"})
		},
		"openapi": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"openapi": "3.0.0", "info": "HNB Platform API"})
		},
		"auth.login":                            authH.Login,
		"auth.refresh":                          authH.RefreshToken,
		"auth.logout":                           authH.Logout,
		"session.bootstrap":                     sessionH.Bootstrap,
		"users.list":                            authH.ListUsers,
		"users.create":                          authH.CreateUser,
		"users.get":                             authH.GetUser,
		"users.update":                          authH.UpdateUser,
		"users.delete":                          authH.DeleteUser,
		"users.reset-password":                  authH.ResetPassword,
		"roles.list":                            authH.ListRoles,
		"roles.get":                             authH.GetRole,
		"roles.create":                          authH.CreateRole,
		"roles.delete":                          authH.DeleteRole,
		"role-bindings.create":                  authH.BindRole,
		"role-bindings.delete":                  authH.UnbindRole,
		"role-bindings.list":                    authH.ListRoleBindings,
		"check-permission":                      authH.CheckPermission,
		"tenants.list":                          tenantAdminH.ListTenants,
		"tenants.create":                        tenantAdminH.CreateTenant,
		"tenants.get":                           tenantAdminH.GetTenant,
		"tenants.update":                        tenantAdminH.UpdateTenant,
		"tenants.delete":                        tenantAdminH.DeleteTenant,
		"tenants.workspaces.list":               tenantAdminH.ListTenantWorkspaces,
		"tenants.workspaces.create":             tenantAdminH.CreateTenantWorkspace,
		"tenants.quota.get":                     tenantAdminH.GetTenantQuota,
		"tenants.quota.update":                  tenantAdminH.UpdateTenantQuota,
		"tenants.clusterAllocations.list":       tenantAdminH.ListTenantClusterAllocations,
		"tenants.clusterAllocations.upsert":     tenantAdminH.UpsertTenantClusterAllocation,
		"tenants.clusterAllocations.delete":     tenantAdminH.DeleteTenantClusterAllocation,
		"workspaces.quota.get":                  tenantAdminH.GetWorkspaceQuota,
		"workspaces.quota.update":               tenantAdminH.UpdateWorkspaceQuota,
		"workspaces.bind-cluster":               tenantAdminH.BindWorkspaceCluster,
		"workspaces.unbind-cluster":             tenantAdminH.UnbindWorkspaceCluster,
		"workspaces.clusters.list":              tenantAdminH.ListWorkspaceClusters,
		"workspaces.list":                       tenantH.ListWorkspaces,
		"workspaces.create":                     tenantH.CreateWorkspace,
		"tenant.namespaces.list":                tenantH.ListTenantNamespaces,
		"tenant.namespaces.create":              tenantH.CreateTenantNamespace,
		"namespaces.list":                       tenantH.ListNamespaces,
		"namespaces.get":                        tenantH.GetNamespace,
		"namespaces.create":                     tenantH.CreateNamespace,
		"namespaces.update":                     tenantH.UpdateNamespace,
		"namespaces.delete":                     tenantH.DeleteNamespace,
		"namespaces.quota-remaining":            tenantH.GetNamespaceQuotaRemaining,
		"namespaces.members.list":               tenantH.ListNamespaceMembers,
		"namespaces.members.add":                tenantH.AddNamespaceMember,
		"namespaces.members.remove":             tenantH.RemoveNamespaceMember,
		"tenant-users.list":                     tenantH.ListTenantUsers,
		"clusters.list":                         clusterH.List,
		"clusters.create":                       clusterH.Register,
		"clusters.get":                          clusterH.Get,
		"clusters.delete":                       clusterH.Delete,
		"resources.clusters.list":               readGate(resourceClusterH.ListClusters),
		"resources.clusters.get":                readGate(resourceClusterH.GetCluster),
		"resources.clusters.nodes":              readGate(resourceClusterH.ListClusterNodes),
		"resources.clusters.plugins":            readGate(resourceClusterH.ListClusterPluginStatuses),
		"resources.clusters.dictionary":         readGate(resourceClusterH.StatusDictionary),
		"resources.clusters.updateDescription":  writeGate(resourceClusterH.UpdateClusterDescription),
		"resources.clusters.downloadKubeconfig": readGate(resourceClusterH.DownloadKubeConfig),
		"resources.clusters.agentOnboarding":    readGate(agentOnboardingH.AgentOnboarding),
		"resources.clusters.monitoring.summary": readGate(monitoringH.Summary),
		"resources.clusters.monitoring.metrics": readGate(monitoringH.Metrics),
		"runtime-intents.submit":                writeGate(providerGate(resourceClusterH.SubmitRuntimeIntent)),
		"runtime-intents.batchDelete":           writeGate(providerGate(resourceClusterH.SubmitRuntimeIntentBatch)),
		"secrets.register":                      writeGate(providerGate(resourceClusterH.RegisterSecret)),
		"resources.operations.list":             readGate(resourceClusterH.ListOperations),
		"resources.operations.get":              readGate(resourceClusterH.GetOperation),
		"resources.operations.approve":          writeGate(resourceClusterH.OperationApprove),
		"resources.operations.reject":           writeGate(resourceClusterH.OperationReject),
		"resources.operations.cancel":           writeGate(resourceClusterH.OperationCancel),
		"extensions.list":                       extensionH.List,
		"extensions.install":                    extensionH.Install,
		"extensions.delete":                     extensionH.Delete,
		"navigation.menus":                      navigationH.Menus,
		"schema.page":                           schemaGate(contractGate(schemaH.Page)),
		"schema.publish":                        writeGate(schemaGate(contractGate(schemaH.Publish))),
		"schema.rollback":                       writeGate(schemaGate(contractGate(schemaH.Rollback))),
		"gslb.services.list":                    readGate(gslbH.ListServices),
		"gslb.services.get":                     readGate(gslbH.GetService),
		"gslb.services.drills":                  readGate(gslbH.ListDrills),
		"gslb.intents.submit":                   writeGate(gslbH.SubmitIntent),
		"gslb.switch-requests.approve":          writeGate(gslbH.Approve),
		"gslb.switch-requests.reject":           writeGate(gslbH.Reject),
		"dr.groups.list":                        readGate(drH.ListGroups),
		"dr.groups.create":                      writeGate(drH.CreateGroup),
		"dr.groups.get":                         readGate(drH.GetGroup),
		"dr.groups.members.add":                 writeGate(drH.AddMember),
		"dr.groups.runs.list":                   readGate(drH.ListRuns),
		"dr.groups.switch":                      writeGate(drH.InitiateSwitch),
		"dr.runs.confirm-data-layer":            writeGate(drH.ConfirmDataLayer),
		"proxy":                                 proxyH.ProxyRequest,
		"agents.list":                           proxyH.ListAgents,
		"agents.get":                            proxyH.GetAgent,
		"audit.list":                            auditH.List,
		"audit.get":                             auditH.Get,
		"market.proxy":                          marketH.Proxy,
		"plugin-catalog.list":                   readGate(pluginCatalogH.List),
		"plugin-catalog.install":                writeGate(pluginCatalogH.Install),
		"plugin-catalog.uninstall":              writeGate(pluginCatalogH.Uninstall),
		"settings.list":                         settingsH.List,
		"settings.update":                       settingsH.Update,
		"storage.overview":                      storageH.Overview,
		"storage.backends":                      storageDesiredH.Backends,
		"storage.backends.create":               storageDesiredH.CreateBackend,
		"storage.backends.get":                  storageDesiredH.GetBackend,
		"storage.backends.update":               storageDesiredH.UpdateBackend,
		"storage.backends.delete":               storageDesiredH.DeleteBackend,
		"storage.provider-schemas":              storageDesiredH.ProviderSchemas,
		"storage.offerings":                     storageDesiredH.Offerings,
		"storage.offerings.create":              storageDesiredH.CreateOffering,
		"storage.offerings.get":                 storageDesiredH.GetOffering,
		"storage.offerings.update":              storageDesiredH.UpdateOffering,
		"storage.offerings.delete":              storageDesiredH.DeleteOffering,
		"storage.driver-installations":          storageH.DriverInstallations,
		"storage.drivers.install":               writeGate(providerGate(storageDesiredH.InstallDriverIntent)),
		"storage.drivers.upgrade":               writeGate(providerGate(storageDesiredH.UpgradeDriverIntent)),
		"storage.drivers.uninstall":             writeGate(providerGate(storageDesiredH.UninstallDriverIntent)),
		"storage.target-inventory":              storageH.TargetInventory,
		"storage.target-metrics":                storageH.TargetMetrics,
		"storage.offering-bindings":             storageDesiredH.Bindings,
		"storage.bindings.create":               storageDesiredH.CreateBinding,
		"storage.bindings.get":                  storageDesiredH.GetBinding,
		"storage.bindings.update":               storageDesiredH.UpdateBinding,
		"storage.bindings.delete":               storageDesiredH.DeleteBinding,
		"storage.bindings.import":               writeGate(providerGate(storageDesiredH.ImportBindingIntent)),
		"storage.bindings.reconcile":            writeGate(providerGate(storageDesiredH.ReconcileBindingIntent)),
		"storage.retained-volumes.release":      writeGate(providerGate(storageDesiredH.ReleaseRetainedVolumeIntent)),
		"storage.retained-volumes.sanitize":     writeGate(providerGate(storageDesiredH.SanitizeRetainedVolumeIntent)),
		"storage.alert-rules.list":              storageAlertH.ListRules,
		"storage.alert-rules.create":            storageAlertH.CreateRule,
		"capabilities.list":                     capabilityH.List,
		"capabilities.get":                      capabilityH.Get,
	}

	// Core routes (code-registered, compile-time safety)
	coreMux := http.NewServeMux()
	coreMux.HandleFunc("GET /health", coreHandlers["health"])
	coreMux.HandleFunc("GET /ready", coreHandlers["ready"])
	coreMux.HandleFunc("GET /openapi.json", coreHandlers["openapi"])
	coreMux.HandleFunc("POST /api/v1/auth/login", coreHandlers["auth.login"])
	coreMux.HandleFunc("POST /api/v1/auth/refresh", coreHandlers["auth.refresh"])
	coreMux.HandleFunc("POST /api/v1/auth/logout", coreHandlers["auth.logout"])
	coreMux.HandleFunc("GET /api/v1/session/bootstrap", coreHandlers["session.bootstrap"])
	coreMux.HandleFunc("GET /api/v1/users", coreHandlers["users.list"])
	coreMux.HandleFunc("POST /api/v1/users", coreHandlers["users.create"])
	coreMux.HandleFunc("GET /api/v1/users/{id}", coreHandlers["users.get"])
	coreMux.HandleFunc("PATCH /api/v1/users/{id}", coreHandlers["users.update"])
	coreMux.HandleFunc("DELETE /api/v1/users/{id}", coreHandlers["users.delete"])
	coreMux.HandleFunc("POST /api/v1/users/{id}/reset-password", coreHandlers["users.reset-password"])
	coreMux.HandleFunc("GET /api/v1/roles", coreHandlers["roles.list"])
	coreMux.HandleFunc("GET /api/v1/roles/{id}", coreHandlers["roles.get"])
	coreMux.HandleFunc("POST /api/v1/roles", coreHandlers["roles.create"])
	coreMux.HandleFunc("DELETE /api/v1/roles/{id}", coreHandlers["roles.delete"])
	coreMux.HandleFunc("POST /api/v1/role-bindings", coreHandlers["role-bindings.create"])
	coreMux.HandleFunc("DELETE /api/v1/role-bindings/{user_id}/{scope}/{scope_id}", coreHandlers["role-bindings.delete"])
	coreMux.HandleFunc("GET /api/v1/role-bindings", coreHandlers["role-bindings.list"])
	coreMux.HandleFunc("GET /api/v1/check-permission", coreHandlers["check-permission"])
	coreMux.HandleFunc("GET /api/v1/tenants", coreHandlers["tenants.list"])
	coreMux.HandleFunc("POST /api/v1/tenants", coreHandlers["tenants.create"])
	coreMux.HandleFunc("GET /api/v1/tenants/{id}", coreHandlers["tenants.get"])
	coreMux.HandleFunc("PATCH /api/v1/tenants/{id}", coreHandlers["tenants.update"])
	coreMux.HandleFunc("DELETE /api/v1/tenants/{id}", coreHandlers["tenants.delete"])
	coreMux.HandleFunc("GET /api/v1/tenants/{id}/workspaces", coreHandlers["tenants.workspaces.list"])
	coreMux.HandleFunc("POST /api/v1/tenants/{id}/workspaces", coreHandlers["tenants.workspaces.create"])
	coreMux.HandleFunc("GET /api/v1/tenants/{id}/quota", coreHandlers["tenants.quota.get"])
	coreMux.HandleFunc("PUT /api/v1/tenants/{id}/quota", coreHandlers["tenants.quota.update"])
	coreMux.HandleFunc("GET /api/v1/tenants/{id}/cluster-allocations", coreHandlers["tenants.clusterAllocations.list"])
	coreMux.HandleFunc("PUT /api/v1/tenants/{id}/cluster-allocations/{cluster_id}", coreHandlers["tenants.clusterAllocations.upsert"])
	coreMux.HandleFunc("DELETE /api/v1/tenants/{id}/cluster-allocations/{cluster_id}", coreHandlers["tenants.clusterAllocations.delete"])
	coreMux.HandleFunc("GET /api/v1/workspaces/{workspace_id}/quota", coreHandlers["workspaces.quota.get"])
	coreMux.HandleFunc("PUT /api/v1/workspaces/{workspace_id}/quota", coreHandlers["workspaces.quota.update"])
	coreMux.HandleFunc("GET /api/v1/workspaces", coreHandlers["workspaces.list"])
	coreMux.HandleFunc("POST /api/v1/workspaces", coreHandlers["workspaces.create"])
	coreMux.HandleFunc("GET /api/v1/namespaces", coreHandlers["tenant.namespaces.list"])
	coreMux.HandleFunc("POST /api/v1/namespaces", coreHandlers["tenant.namespaces.create"])
	coreMux.HandleFunc("POST /api/v1/workspaces/{workspace_id}/bind-cluster", coreHandlers["workspaces.bind-cluster"])
	coreMux.HandleFunc("DELETE /api/v1/workspaces/{workspace_id}/clusters/{cluster_id}", coreHandlers["workspaces.unbind-cluster"])
	coreMux.HandleFunc("GET /api/v1/workspaces/{workspace_id}/clusters", coreHandlers["workspaces.clusters.list"])
	coreMux.HandleFunc("GET /api/v1/workspaces/{workspace_id}/namespaces", coreHandlers["namespaces.list"])
	coreMux.HandleFunc("GET /api/v1/workspaces/{workspace_id}/namespaces/{namespace_id}", coreHandlers["namespaces.get"])
	coreMux.HandleFunc("POST /api/v1/workspaces/{workspace_id}/namespaces", coreHandlers["namespaces.create"])
	coreMux.HandleFunc("PUT /api/v1/workspaces/{workspace_id}/namespaces/{namespace_id}", coreHandlers["namespaces.update"])
	coreMux.HandleFunc("DELETE /api/v1/workspaces/{workspace_id}/namespaces/{namespace_id}", coreHandlers["namespaces.delete"])
	coreMux.HandleFunc("GET /api/v1/workspaces/{workspace_id}/namespaces/quota-remaining", coreHandlers["namespaces.quota-remaining"])
	coreMux.HandleFunc("GET /api/v1/workspaces/{workspace_id}/namespaces/{namespace_id}/members", coreHandlers["namespaces.members.list"])
	coreMux.HandleFunc("POST /api/v1/workspaces/{workspace_id}/namespaces/{namespace_id}/members", coreHandlers["namespaces.members.add"])
	coreMux.HandleFunc("DELETE /api/v1/workspaces/{workspace_id}/namespaces/{namespace_id}/members/{subject_id}", coreHandlers["namespaces.members.remove"])
	coreMux.HandleFunc("GET /api/v1/workspaces/{workspace_id}/users", coreHandlers["tenant-users.list"])
	coreMux.HandleFunc("GET /api/v1/clusters", coreHandlers["clusters.list"])
	coreMux.HandleFunc("POST /api/v1/clusters", coreHandlers["clusters.create"])
	coreMux.HandleFunc("GET /api/v1/clusters/{id}", coreHandlers["clusters.get"])
	coreMux.HandleFunc("DELETE /api/v1/clusters/{id}", coreHandlers["clusters.delete"])
	coreMux.HandleFunc("GET /api/v1/resources/clusters", coreHandlers["resources.clusters.list"])
	coreMux.HandleFunc("GET /api/v1/resources/clusters/{id}", coreHandlers["resources.clusters.get"])
	coreMux.HandleFunc("GET /api/v1/resources/clusters/{id}/nodes", coreHandlers["resources.clusters.nodes"])
	coreMux.HandleFunc("GET /api/v1/resources/clusters/{id}/plugins", coreHandlers["resources.clusters.plugins"])
	coreMux.HandleFunc("GET /api/v1/resources/clusters/{id}/monitoring/summary", coreHandlers["resources.clusters.monitoring.summary"])
	coreMux.HandleFunc("GET /api/v1/resources/clusters/{id}/monitoring/metrics", coreHandlers["resources.clusters.monitoring.metrics"])
	coreMux.HandleFunc("PATCH /api/v1/resources/clusters/{id}/description", coreHandlers["resources.clusters.updateDescription"])
	coreMux.HandleFunc("POST /api/v1/resources/clusters/{id}/kubeconfig:download", coreHandlers["resources.clusters.downloadKubeconfig"])
	coreMux.HandleFunc("POST /api/v1/resources/clusters/{id}/agent-onboarding", coreHandlers["resources.clusters.agentOnboarding"])
	coreMux.HandleFunc("GET /api/v1/dictionaries/cluster.status", coreHandlers["resources.clusters.dictionary"])
	coreMux.HandleFunc("POST /api/v1/runtime-intents", coreHandlers["runtime-intents.submit"])
	coreMux.HandleFunc("POST /api/v1/runtime-intent-batches", coreHandlers["runtime-intents.batchDelete"])
	coreMux.HandleFunc("POST /api/v1/secrets:register", coreHandlers["secrets.register"])
	coreMux.HandleFunc("GET /api/v1/operations", coreHandlers["resources.operations.list"])
	coreMux.HandleFunc("GET /api/v1/operations/{id}", coreHandlers["resources.operations.get"])
	coreMux.HandleFunc("POST /api/v1/operations/{id}/actions/approve", coreHandlers["resources.operations.approve"])
	coreMux.HandleFunc("POST /api/v1/operations/{id}/actions/reject", coreHandlers["resources.operations.reject"])
	coreMux.HandleFunc("POST /api/v1/operations/{id}/actions/cancel", coreHandlers["resources.operations.cancel"])
	coreMux.HandleFunc("GET /api/v1/extensions", coreHandlers["extensions.list"])
	coreMux.HandleFunc("POST /api/v1/extensions", coreHandlers["extensions.install"])
	coreMux.HandleFunc("DELETE /api/v1/extensions/{id}", coreHandlers["extensions.delete"])
	coreMux.HandleFunc("GET /api/v1/navigation/menus", coreHandlers["navigation.menus"])
	coreMux.HandleFunc("GET /api/v1/schema/page/{id}", coreHandlers["schema.page"])
	coreMux.HandleFunc("POST /api/v1/ui/pages/{id}/publish", coreHandlers["schema.publish"])
	coreMux.HandleFunc("POST /api/v1/ui/pages/{id}/rollback", coreHandlers["schema.rollback"])
	coreMux.HandleFunc("GET /api/v1/gslb/services", coreHandlers["gslb.services.list"])
	coreMux.HandleFunc("GET /api/v1/gslb/services/{id}", coreHandlers["gslb.services.get"])
	coreMux.HandleFunc("GET /api/v1/gslb/services/{id}/drills", coreHandlers["gslb.services.drills"])
	coreMux.HandleFunc("POST /api/v1/gslb/services/{id}/intents", coreHandlers["gslb.intents.submit"])
	coreMux.HandleFunc("POST /api/v1/gslb/switch-requests/{id}/approve", coreHandlers["gslb.switch-requests.approve"])
	coreMux.HandleFunc("POST /api/v1/gslb/switch-requests/{id}/reject", coreHandlers["gslb.switch-requests.reject"])
	coreMux.HandleFunc("GET /api/v1/dr/groups", coreHandlers["dr.groups.list"])
	coreMux.HandleFunc("POST /api/v1/dr/groups", coreHandlers["dr.groups.create"])
	coreMux.HandleFunc("GET /api/v1/dr/groups/{id}", coreHandlers["dr.groups.get"])
	coreMux.HandleFunc("POST /api/v1/dr/groups/{id}/members", coreHandlers["dr.groups.members.add"])
	coreMux.HandleFunc("GET /api/v1/dr/groups/{id}/runs", coreHandlers["dr.groups.runs.list"])
	coreMux.HandleFunc("POST /api/v1/dr/groups/{id}/switch", coreHandlers["dr.groups.switch"])
	coreMux.HandleFunc("POST /api/v1/dr/runs/{id}/confirm-data-layer", coreHandlers["dr.runs.confirm-data-layer"])
	coreMux.HandleFunc("GET /api/v1/proxy/{cluster_id}/{path...}", coreHandlers["proxy"])
	coreMux.HandleFunc("POST /api/v1/proxy/{cluster_id}/{path...}", coreHandlers["proxy"])
	coreMux.HandleFunc("PUT /api/v1/proxy/{cluster_id}/{path...}", coreHandlers["proxy"])
	coreMux.HandleFunc("DELETE /api/v1/proxy/{cluster_id}/{path...}", coreHandlers["proxy"])
	coreMux.HandleFunc("PATCH /api/v1/proxy/{cluster_id}/{path...}", coreHandlers["proxy"])
	coreMux.HandleFunc("GET /api/v1/agents", coreHandlers["agents.list"])
	coreMux.HandleFunc("GET /api/v1/agents/{cluster_id}", coreHandlers["agents.get"])
	coreMux.HandleFunc("GET /api/v1/audit-logs", coreHandlers["audit.list"])
	coreMux.HandleFunc("GET /api/v1/audit-logs/{id}", coreHandlers["audit.get"])
	coreMux.HandleFunc("GET /api/v1/settings", coreHandlers["settings.list"])
	coreMux.HandleFunc("PUT /api/v1/settings", coreHandlers["settings.update"])
	registerStorageRoutes(coreMux, coreHandlers)
	coreMux.HandleFunc("GET /api/v1/capabilities", coreHandlers["capabilities.list"])
	coreMux.HandleFunc("GET /api/v1/capabilities/{name}", coreHandlers["capabilities.get"])
	coreMux.HandleFunc("GET /api/v1/market", coreHandlers["market.proxy"])
	coreMux.HandleFunc("GET /api/v1/market/{path...}", coreHandlers["market.proxy"])
	coreMux.HandleFunc("POST /api/v1/market/{path...}", coreHandlers["market.proxy"])
	coreMux.HandleFunc("PUT /api/v1/market/{path...}", coreHandlers["market.proxy"])
	coreMux.HandleFunc("PATCH /api/v1/market/{path...}", coreHandlers["market.proxy"])
	coreMux.HandleFunc("DELETE /api/v1/market/{path...}", coreHandlers["market.proxy"])
	coreMux.HandleFunc("GET /api/v1/plugin-catalog", coreHandlers["plugin-catalog.list"])
	coreMux.HandleFunc("POST /api/v1/plugin-catalog/installs", coreHandlers["plugin-catalog.install"])
	coreMux.HandleFunc("DELETE /api/v1/plugin-catalog/installs/{name}", coreHandlers["plugin-catalog.uninstall"])

	// Authorization runs before tenant enrichment so denied routes cannot query application state.
	chain := middleware.NewChain()
	chain.Add(middleware.NewRecovery())
	chain.Add(middleware.NewRequestID())
	chain.Add(middleware.NewCORS())
	chain.Add(authMW)
	chain.Add(middleware.NewAuthz())
	chain.Add(middleware.NewTenantContext(db, bypassPaths))
	chain.Add(middleware.NewRateLimiter("100/s"))
	chain.Add(middleware.NewAuditMW(auditStore))
	chain.Add(middleware.NewTracing())

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/tunnel" {
			ts.ServeHTTP(w, r)
			return
		}
		ctx := &middleware.Context{
			Request:  r,
			Response: w,
			Params:   make(map[string]string),
		}

		handler := chain.Then(func(ctx *middleware.Context) {
			// Try core routes first
			coreMux.ServeHTTP(ctx.Response, ctx.Request)
		})

		handler(ctx)
	})
}

func registerStorageRoutes(mux *http.ServeMux, handlers map[string]http.HandlerFunc) {
	mux.HandleFunc("GET /api/v1/storage/overview", handlers["storage.overview"])
	mux.HandleFunc("GET /api/v1/storage/backends", handlers["storage.backends"])
	mux.HandleFunc("POST /api/v1/storage/backends", handlers["storage.backends.create"])
	mux.HandleFunc("GET /api/v1/storage/backends/{backendId}", handlers["storage.backends.get"])
	mux.HandleFunc("PUT /api/v1/storage/backends/{backendId}", handlers["storage.backends.update"])
	mux.HandleFunc("DELETE /api/v1/storage/backends/{backendId}", handlers["storage.backends.delete"])
	mux.HandleFunc("GET /api/v1/storage/provider-schemas", handlers["storage.provider-schemas"])
	mux.HandleFunc("GET /api/v1/storage/offerings", handlers["storage.offerings"])
	mux.HandleFunc("POST /api/v1/storage/offerings", handlers["storage.offerings.create"])
	mux.HandleFunc("GET /api/v1/storage/offerings/{offeringId}", handlers["storage.offerings.get"])
	mux.HandleFunc("PUT /api/v1/storage/offerings/{offeringId}", handlers["storage.offerings.update"])
	mux.HandleFunc("DELETE /api/v1/storage/offerings/{offeringId}", handlers["storage.offerings.delete"])
	mux.HandleFunc("GET /api/v1/storage/driver-installations", handlers["storage.driver-installations"])
	mux.HandleFunc("POST /api/v1/storage/driver-installations/{installationId}/intents/install", handlers["storage.drivers.install"])
	mux.HandleFunc("POST /api/v1/storage/driver-installations/{installationId}/intents/upgrade", handlers["storage.drivers.upgrade"])
	mux.HandleFunc("POST /api/v1/storage/driver-installations/{installationId}/intents/uninstall", handlers["storage.drivers.uninstall"])
	mux.HandleFunc("GET /api/v1/storage/targets/{targetId}/inventory", handlers["storage.target-inventory"])
	mux.HandleFunc("GET /api/v1/storage/targets/{targetId}/metrics", handlers["storage.target-metrics"])
	mux.HandleFunc("GET /api/v1/storage/offerings/{offeringId}/bindings", handlers["storage.offering-bindings"])
	mux.HandleFunc("POST /api/v1/storage/offerings/{offeringId}/bindings", handlers["storage.bindings.create"])
	mux.HandleFunc("GET /api/v1/storage/bindings/{bindingId}", handlers["storage.bindings.get"])
	mux.HandleFunc("PUT /api/v1/storage/bindings/{bindingId}", handlers["storage.bindings.update"])
	mux.HandleFunc("DELETE /api/v1/storage/bindings/{bindingId}", handlers["storage.bindings.delete"])
	mux.HandleFunc("POST /api/v1/storage/offerings/{offeringId}/bindings/intents/import", handlers["storage.bindings.import"])
	mux.HandleFunc("POST /api/v1/storage/bindings/{bindingId}/intents/reconcile", handlers["storage.bindings.reconcile"])
	mux.HandleFunc("POST /api/v1/storage/retained-volumes/{volumeId}/intents/release", handlers["storage.retained-volumes.release"])
	mux.HandleFunc("POST /api/v1/storage/retained-volumes/{volumeId}/intents/sanitize", handlers["storage.retained-volumes.sanitize"])
	mux.HandleFunc("GET /api/v1/storage/alert-rules", handlers["storage.alert-rules.list"])
	mux.HandleFunc("POST /api/v1/storage/alert-rules", handlers["storage.alert-rules.create"])
}

// gate returns a handler wrapper that fails closed with 503 when the given
// capability stage is disabled. Unknown stages are treated as disabled.
func gate(caps capability.Set, name string) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if !caps.Has(name) {
				response.ServiceUnavailable(w, "capability_disabled: "+name+" is not enabled for this deployment")
				return
			}
			next(w, r)
		}
	}
}

func init() {
	_ = fmt.Sprintf
}
