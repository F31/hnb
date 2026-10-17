package api

import (
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/F31/hnb/cmd/platform-api/internal/store"
	"github.com/F31/hnb/pkg/iam"
)

var releaseDigestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

var releaseRoles = map[string]bool{
	"console": true, "control-plane": true, "worker": true,
}

// registerReleaseMaterialRequest is the owning-plane view of a validated
// offline bundle registered for one install target role. The install target
// resources are not node/container payloads; only content-addressed identity is
// recorded.
type registerReleaseMaterialRequest struct {
	ReleaseID      string `json:"releaseId"`
	ReleaseVersion string `json:"releaseVersion"`
	Role           string `json:"role"`
	BundleID       string `json:"bundleId"`
	LockDigest     string `json:"lockDigest"`
	ContentDigest  string `json:"contentDigest"`
}

func validateReleaseMaterial(req *registerReleaseMaterialRequest) (string, bool) {
	if strings.TrimSpace(req.ReleaseID) == "" || len(req.ReleaseID) > 128 {
		return "releaseId is required", true
	}
	if strings.TrimSpace(req.ReleaseVersion) == "" || len(req.ReleaseVersion) > 64 {
		return "releaseVersion is required", true
	}
	if !releaseRoles[req.Role] {
		return "role must be console, control-plane or worker", true
	}
	if strings.TrimSpace(req.BundleID) == "" || len(req.BundleID) > 256 {
		return "bundleId is required", true
	}
	if !releaseDigestPattern.MatchString(req.LockDigest) {
		return "lockDigest must be a sha256 content digest", true
	}
	if !releaseDigestPattern.MatchString(req.ContentDigest) {
		return "contentDigest must be a sha256 content digest", true
	}
	return "", false
}

func releaseMaterialResponse(e store.ReleaseCatalogEntry) map[string]string {
	return map[string]string{
		"id": e.ID, "releaseId": e.ReleaseID, "releaseVersion": e.ReleaseVersion,
		"role": e.Role, "bundleId": e.BundleID, "lockDigest": e.LockDigest,
		"contentDigest": e.ContentDigest, "importedBy": e.ImportedBy,
	}
}

func (s *Server) handleRegisterReleaseMaterial(w http.ResponseWriter, r *http.Request) {
	trusted, _ := iam.TrustedContextFrom(r.Context())
	if s.permissionResolver != nil {
		policy, permissions, err := s.permissionResolver.ResolvePermissions(r.Context(), trusted.SubjectID, trusted.MembershipID, trusted.TenantID)
		if err != nil {
			writeError(w, http.StatusForbidden, "permission_denied", "permission denied")
			return
		}
		trusted.PolicyVersion, trusted.ScopedPermissions = policy, permissions
		r = r.WithContext(iam.WithTrustedContext(r.Context(), trusted))
	}
	if !s.authorize(w, r, "", "", "", "") {
		return
	}
	var req registerReleaseMaterialRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_release_material", "invalid release material document")
		return
	}
	if reason, bad := validateReleaseMaterial(&req); bad {
		writeError(w, http.StatusBadRequest, "invalid_release_material", reason)
		return
	}
	entry, created, err := s.store.RegisterReleaseMaterial(r.Context(), store.ReleaseCatalogEntry{
		ReleaseID: req.ReleaseID, ReleaseVersion: req.ReleaseVersion, Role: req.Role,
		BundleID: req.BundleID, LockDigest: req.LockDigest, ContentDigest: req.ContentDigest,
		ImportedBy: trusted.SubjectID,
	})
	if err != nil {
		if errors.Is(err, store.ErrFixedReleaseConflict) {
			writeError(w, http.StatusConflict, "fixed-release-conflict", "fixed release already registered with a different content digest")
			return
		}
		writeError(w, http.StatusInternalServerError, "release_material_save_failed", "release material registration failed")
		return
	}
	status := http.StatusCreated
	if !created {
		status = http.StatusOK
	}
	writeJSON(w, status, releaseMaterialResponse(*entry))
}

type listReleaseMaterialsResponse struct {
	Releases    []map[string]string `json:"releases"`
	Total       int                 `json:"total"`
	RetrievedAt time.Time           `json:"retrievedAt"`
}

func (s *Server) handleListReleaseMaterials(w http.ResponseWriter, r *http.Request) {
	trusted, _ := iam.TrustedContextFrom(r.Context())
	if s.permissionResolver != nil {
		policy, permissions, err := s.permissionResolver.ResolvePermissions(r.Context(), trusted.SubjectID, trusted.MembershipID, trusted.TenantID)
		if err != nil {
			writeError(w, http.StatusForbidden, "permission_denied", "permission denied")
			return
		}
		trusted.PolicyVersion, trusted.ScopedPermissions = policy, permissions
		r = r.WithContext(iam.WithTrustedContext(r.Context(), trusted))
	}
	if !s.authorize(w, r, "", "", "", "") {
		return
	}
	entries, err := s.store.ListReleaseMaterials(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "release_material_list_failed", "release material list failed")
		return
	}
	resp := listReleaseMaterialsResponse{Releases: make([]map[string]string, 0, len(entries)), Total: len(entries), RetrievedAt: time.Now().UTC()}
	for _, e := range entries {
		resp.Releases = append(resp.Releases, releaseMaterialResponse(e))
	}
	writeJSON(w, http.StatusOK, resp)
}
