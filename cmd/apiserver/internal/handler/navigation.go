package handler

import (
	"encoding/json"
	"net/http"

	navapp "github.com/F31/hnb/cmd/apiserver/internal/application/navigation"
	"github.com/F31/hnb/cmd/apiserver/internal/capability"
	navinfra "github.com/F31/hnb/cmd/apiserver/internal/infrastructure/navigation"
	"github.com/F31/hnb/cmd/apiserver/internal/response"
	"github.com/F31/hnb/pkg/iam"
)

type NavigationHandler struct{ service *navapp.Service }

func NewNavigationHandler(repo navapp.MetadataRepository) *NavigationHandler {
	return &NavigationHandler{service: navapp.NewService(repo)}
}

func NewNavigationHandlerWithService(service *navapp.Service) *NavigationHandler {
	return &NavigationHandler{service: service}
}

// NewNavigationHandlerWithCapabilities additionally merges the staged cluster
// capability gates into the navigation snapshot so disabled stages hide their
// menus and routes (KERNEL-016).
func NewNavigationHandlerWithCapabilities(repo navapp.MetadataRepository, caps capability.Set) *NavigationHandler {
	return NewNavigationHandler(navinfra.NewCapabilityWrappingRepository(repo, caps.Snapshot()))
}

func (h *NavigationHandler) Menus(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Unauthorized(w, "trusted context required")
		return
	}
	tenantID := trusted.TenantID
	if tenantID == "" {
		tenantID = r.URL.Query().Get("tenant")
	}
	if tenantID == "" {
		tenantID = r.URL.Query().Get("tenantId")
	}
	if tenantID == "" {
		response.BadRequest(w, "tenant is required")
		return
	}
	nav, err := h.service.Build(navapp.Request{Context: r.Context(), Trusted: trusted, TenantID: tenantID, SpaceID: r.URL.Query().Get("spaceId"), Locale: r.URL.Query().Get("locale")})
	if err != nil {
		response.InternalError(w, err.Error())
		return
	}
	if r.Header.Get("If-None-Match") == nav.ETag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("ETag", nav.ETag)
	// no-cache 强制每次重验证（If-None-Match），防止启发式缓存复用旧导航。
	w.Header().Set("Cache-Control", "no-cache")
	_ = json.NewEncoder(w).Encode(nav)
}
