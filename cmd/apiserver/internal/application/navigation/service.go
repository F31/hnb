package navigation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/F31/hnb/pkg/iam"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	GenerationSeconds = promauto.NewHistogram(prometheus.HistogramOpts{Name: "hnb_navigation_generation_seconds", Help: "Navigation response generation latency."})
	CacheEvents       = promauto.NewCounterVec(prometheus.CounterOpts{Name: "hnb_navigation_cache_events_total", Help: "Navigation cache events."}, []string{"result"})
	FilteredItems     = promauto.NewCounterVec(prometheus.CounterOpts{Name: "hnb_navigation_filtered_items_total", Help: "Navigation items filtered by reason."}, []string{"reason"})
)

type Service struct {
	repo  MetadataRepository
	cache map[string]Response
	mu    sync.Mutex
}

type MetadataRepository interface {
	Snapshot(ctx context.Context, tenantID string, locale string) (Snapshot, error)
}

type Snapshot struct {
	Versions     map[string]string
	Capabilities map[string]bool
	Plugins      []PluginManifestRef
	Menus        []Menu
	Routes       []Route
}

type Request struct {
	Trusted  iam.TrustedContext
	Context  context.Context
	TenantID string
	SpaceID  string
	Locale   string
}

type Response struct {
	APIVersion  string              `json:"apiVersion"`
	ETag        string              `json:"etag"`
	GeneratedAt string              `json:"generatedAt"`
	Context     Context             `json:"context"`
	Versions    map[string]string   `json:"versions"`
	Plugins     []PluginManifestRef `json:"plugins"`
	Menus       []Menu              `json:"menus"`
	Routes      []Route             `json:"routes"`
}

type Context struct {
	TenantID string `json:"tenantId"`
	SpaceID  string `json:"spaceId,omitempty"`
}

type PluginManifestRef struct {
	ID           string              `json:"id,omitempty"`
	Name         string              `json:"name"`
	Version      string              `json:"version"`
	DisplayName  string              `json:"displayName"`
	Tier         string              `json:"tier"`
	Enabled      bool                `json:"enabled"`
	Mode         string              `json:"mode"`
	Permissions  PluginPermissions   `json:"permissions"`
	Capabilities PluginCapabilities  `json:"capabilities"`
	Dependencies PluginDependencies  `json:"dependencies"`
	Menu         PluginManifestMenu  `json:"menu"`
	Routes       []PluginRouteConfig `json:"routes,omitempty"`
}

type PluginPermissions struct {
	Required []string `json:"required"`
	Optional []string `json:"optional,omitempty"`
}

type PluginCapabilities struct {
	Required []string `json:"required"`
	Optional []string `json:"optional,omitempty"`
}

type PluginDependencies struct {
	Backend []string `json:"backend"`
	Plugins []string `json:"plugins,omitempty"`
}

type PluginManifestMenu struct {
	Group string `json:"group"`
	Items []Item `json:"items"`
}

type PluginRouteConfig struct {
	Path         string `json:"path"`
	ComponentKey string `json:"componentKey"`
	PluginID     string `json:"pluginId,omitempty"`
	Permission   string `json:"permission,omitempty"`
	Capability   string `json:"capability,omitempty"`
}

type Menu struct {
	Group string `json:"group"`
	Items []Item `json:"items"`
}

type Item struct {
	Title      string `json:"title"`
	Path       string `json:"path"`
	Icon       string `json:"icon,omitempty"`
	Permission string `json:"permission,omitempty"`
	Capability string `json:"capability,omitempty"`
	Children   []Item `json:"children,omitempty"`
}

type Route struct {
	Name         string `json:"name"`
	Path         string `json:"path"`
	PluginID     string `json:"pluginId"`
	ComponentKey string `json:"componentKey"`
	SchemaID     string `json:"schemaId,omitempty"`
	Redirect     string `json:"redirect,omitempty"`
	Permission   string `json:"permission,omitempty"`
	Capability   string `json:"capability,omitempty"`
	KeepAlive    bool   `json:"keepAlive,omitempty"`
}

func NewService(repo MetadataRepository) *Service {
	return &Service{repo: repo, cache: make(map[string]Response)}
}

func (s *Service) Build(req Request) (Response, error) {
	start := time.Now()
	ctx := req.Context
	if ctx == nil {
		ctx = context.Background()
	}
	snapshot, err := s.repo.Snapshot(ctx, req.TenantID, req.Locale)
	if err != nil {
		return Response{}, err
	}
	if snapshot.Versions == nil {
		snapshot.Versions = make(map[string]string)
	}
	if req.Trusted.PolicyVersion != "" {
		snapshot.Versions["permission"] = req.Trusted.PolicyVersion
	} else if snapshot.Versions["permission"] == "" {
		snapshot.Versions["permission"] = "unknown"
	}
	key := cacheKey(req, snapshot.Versions)
	s.mu.Lock()
	if cached, ok := s.cache[key]; ok {
		s.mu.Unlock()
		CacheEvents.WithLabelValues("hit").Inc()
		return cached, nil
	}
	s.mu.Unlock()
	CacheEvents.WithLabelValues("miss").Inc()

	menus, routes := filter(snapshot, req.Trusted)
	resp := Response{APIVersion: "navigation.hnb.io/v1", GeneratedAt: time.Now().UTC().Format(time.RFC3339), Context: Context{TenantID: req.TenantID, SpaceID: req.SpaceID}, Versions: snapshot.Versions, Plugins: snapshot.Plugins, Menus: menus, Routes: routes}
	resp.ETag = navigationETag(req, snapshot.Versions)
	s.mu.Lock()
	s.cache[key] = resp
	s.mu.Unlock()
	GenerationSeconds.Observe(time.Since(start).Seconds())
	subjectHash := sha256.Sum256([]byte(req.Trusted.SubjectID))
	log.Printf("navigation generated tenant=%s subject=%x menus=%d routes=%d", req.TenantID, subjectHash[:4], len(resp.Menus), len(resp.Routes))
	return resp, nil
}

func (s *Service) Invalidate() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache = make(map[string]Response)
	CacheEvents.WithLabelValues("invalidate").Inc()
}

func filter(snapshot Snapshot, trusted iam.TrustedContext) ([]Menu, []Route) {
	allowedRoutes := make(map[string]struct{})
	routes := make([]Route, 0, len(snapshot.Routes))
	for _, route := range snapshot.Routes {
		if !canAccessRoute(route, snapshot.Capabilities, trusted) {
			continue
		}
		routes = append(routes, route)
		allowedRoutes[route.Path] = struct{}{}
	}
	filteredRoutes := routes[:0]
	for _, route := range routes {
		if route.Redirect != "" {
			if _, targetAllowed := allowedRoutes[route.Redirect]; !targetAllowed {
				FilteredItems.WithLabelValues("route").Inc()
				delete(allowedRoutes, route.Path)
				continue
			}
		}
		filteredRoutes = append(filteredRoutes, route)
	}
	routes = filteredRoutes
	menus := make([]Menu, 0, len(snapshot.Menus))
	for _, menu := range snapshot.Menus {
		items := filterItems(menu.Items, allowedRoutes, snapshot.Capabilities, trusted)
		if len(items) > 0 {
			menus = append(menus, Menu{Group: menu.Group, Items: items})
		}
	}
	return menus, routes
}

func filterItems(items []Item, allowedRoutes map[string]struct{}, capabilities map[string]bool, trusted iam.TrustedContext) []Item {
	filtered := make([]Item, 0, len(items))
	for _, item := range items {
		children := filterItems(item.Children, allowedRoutes, capabilities, trusted)
		_, routeAllowed := allowedRoutes[item.Path]
		if !canAccessItemMetadata(item, capabilities, trusted) {
			continue
		}
		if item.Path == "" && len(children) == 0 {
			FilteredItems.WithLabelValues("route").Inc()
			continue
		}
		if item.Path != "" && !routeAllowed && len(children) == 0 {
			FilteredItems.WithLabelValues("route").Inc()
			continue
		}
		item.Children = children
		filtered = append(filtered, item)
	}
	return filtered
}

func canAccessRoute(route Route, capabilities map[string]bool, trusted iam.TrustedContext) bool {
	if route.PluginID != "shell" && !capabilities[route.PluginID] {
		FilteredItems.WithLabelValues("capability").Inc()
		return false
	}
	if route.Capability != "" && !capabilities[route.Capability] {
		FilteredItems.WithLabelValues("capability").Inc()
		return false
	}
	if route.Permission != "" && !hasPermission(trusted, route.Permission) {
		FilteredItems.WithLabelValues("permission").Inc()
		return false
	}
	return true
}

func canAccessItemMetadata(item Item, capabilities map[string]bool, trusted iam.TrustedContext) bool {
	if item.Capability != "" && !capabilities[item.Capability] {
		FilteredItems.WithLabelValues("capability").Inc()
		return false
	}
	if item.Permission != "" && !hasPermission(trusted, item.Permission) {
		FilteredItems.WithLabelValues("permission").Inc()
		return false
	}
	return true
}

func hasPermission(trusted iam.TrustedContext, permission string) bool {
	parts := strings.Split(permission, ":")
	if len(parts) < 2 {
		return false
	}
	resource, action := parts[0], parts[len(parts)-1]
	for _, scoped := range trusted.ScopedPermissions {
		if scoped.TenantID == trusted.TenantID && (scoped.ResourceKind == resource || scoped.ResourceKind == "*") && (string(scoped.Action) == action || string(scoped.Action) == "*") {
			return true
		}
	}
	return false
}

func cacheKey(req Request, versions map[string]string) string {
	return req.Trusted.SubjectID + ":" + req.TenantID + ":" + req.SpaceID + ":" + req.Locale + ":" + versions["permission"] + ":" + versions["pluginCatalog"] + ":" + versions["navigation"] + ":" + versions["license"]
}

func navigationETag(req Request, versions map[string]string) string {
	h := sha256.Sum256([]byte(cacheKey(req, versions)))
	return `"` + hex.EncodeToString(h[:]) + `"`
}
