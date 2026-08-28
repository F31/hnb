package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/F31/hnb/cmd/apiserver/internal/response"
	"github.com/F31/hnb/pkg/core"
	"github.com/F31/hnb/pkg/iam"
	"github.com/google/uuid"
)

type ExtensionHandler struct{ db *sql.DB }

func NewExtensionHandler(db *sql.DB) *ExtensionHandler { return &ExtensionHandler{db: db} }

func (h *ExtensionHandler) List(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, 40100, "trusted context required")
		return
	}
	rows, err := h.db.Query(`SELECT e.id, e.name, e.version, e.provider_type, e.workspace_id, e.target_id, e.phase, e.manifest, e.health_failures, e.last_error, e.created_at, e.updated_at
		FROM extensions e
		LEFT JOIN workspaces ws ON ws.id = e.workspace_id
		LEFT JOIN runtime_targets rt ON rt.id::text = COALESCE(e.runtime_target_id::text, e.target_id)
		WHERE ws.tenant_id = $1 OR rt.tenant_id = $1
		ORDER BY e.name`, trusted.TenantID)
	if err != nil {
		response.InternalError(w, err.Error())
		return
	}
	defer rows.Close()
	var extensions []core.Extension
	for rows.Next() {
		var ext core.Extension
		var mJSON []byte
		var wsID, tID, lErr sql.NullString
		if err := rows.Scan(&ext.ID, &ext.Name, &ext.Version, &ext.ProviderType, &wsID, &tID, &ext.Phase, &mJSON, &ext.HealthFailures, &lErr, &ext.CreatedAt, &ext.UpdatedAt); err != nil {
			continue
		}
		if wsID.Valid {
			ext.WorkspaceID = wsID.String
		}
		if tID.Valid {
			ext.TargetID = tID.String
		}
		if lErr.Valid {
			ext.LastError = lErr.String
		}
		if mJSON != nil {
			json.Unmarshal(mJSON, &ext.Manifest)
		}
		extensions = append(extensions, ext)
	}
	if extensions == nil {
		extensions = []core.Extension{}
	}
	response.Success(w, extensions)
}

func (h *ExtensionHandler) Install(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, 40100, "trusted context required")
		return
	}
	var req struct {
		Name        string                 `json:"name"`
		Version     string                 `json:"version"`
		WorkspaceID string                 `json:"workspace_id,omitempty"`
		TargetID    string                 `json:"target_id,omitempty"`
		Manifest    core.ExtensionManifest `json:"manifest"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "invalid body")
		return
	}
	if req.Name == "" {
		response.BadRequest(w, "name is required")
		return
	}
	if req.WorkspaceID == "" && req.TargetID == "" {
		response.BadRequest(w, "workspace_id or target_id is required")
		return
	}
	ext := core.Extension{
		ID: uuid.NewString(), Name: req.Name, Version: req.Version, Phase: core.ExtPending,
		Manifest: req.Manifest, WorkspaceID: req.WorkspaceID, TargetID: req.TargetID,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	mJSON, _ := json.Marshal(ext.Manifest)
	result, err := h.db.Exec(`INSERT INTO extensions (id, name, version, provider_type, workspace_id, target_id, phase, manifest, created_at, updated_at)
		SELECT $1,$2,$3,$4,NULLIF($5,''),NULLIF($6,''),$7,$8,$9,$10
		WHERE ($5 = '' OR EXISTS (SELECT 1 FROM workspaces WHERE id::text = $5 AND tenant_id = $11))
		  AND ($6 = '' OR EXISTS (SELECT 1 FROM runtime_targets WHERE id::text = $6 AND tenant_id = $11))`,
		ext.ID, ext.Name, ext.Version, string(ext.Manifest.Provider),
		ext.WorkspaceID, ext.TargetID, string(ext.Phase), string(mJSON), ext.CreatedAt, ext.UpdatedAt, trusted.TenantID)
	if err != nil {
		response.InternalError(w, err.Error())
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		response.NotFound(w, "workspace or target not found")
		return
	}
	response.Created(w, ext)
}

func (h *ExtensionHandler) Delete(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, 40100, "trusted context required")
		return
	}
	id := r.PathValue("id")
	result, err := h.db.Exec(`DELETE FROM extensions e WHERE e.id=$1 AND (
		EXISTS (SELECT 1 FROM workspaces ws WHERE ws.id=e.workspace_id AND ws.tenant_id=$2)
		OR EXISTS (SELECT 1 FROM runtime_targets rt WHERE rt.id::text=COALESCE(e.runtime_target_id::text,e.target_id) AND rt.tenant_id=$2)
	)`, id, trusted.TenantID)
	if err != nil {
		response.InternalError(w, err.Error())
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		response.NotFound(w, "extension not found")
		return
	}
	response.Success(w, map[string]string{"status": "deleted"})
}
