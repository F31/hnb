package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/F31/hnb/cmd/apiserver/internal/response"
)

type SettingsHandler struct {
	db *sql.DB
}

func NewSettingsHandler(db *sql.DB) *SettingsHandler {
	return &SettingsHandler{db: db}
}

func (h *SettingsHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(`SELECT key, value FROM platform_settings ORDER BY key`)
	if err != nil {
		response.InternalError(w, err.Error())
		return
	}
	defer rows.Close()
	settings := make(map[string]any)
	for rows.Next() {
		var key string
		var valueJSON []byte
		if err := rows.Scan(&key, &valueJSON); err != nil {
			continue
		}
		var val any
		if len(valueJSON) > 0 {
			json.Unmarshal(valueJSON, &val)
		}
		settings[key] = val
	}
	response.Success(w, settings)
}

func (h *SettingsHandler) Update(w http.ResponseWriter, r *http.Request) {
	var req map[string]any
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "invalid body")
		return
	}
	tx, err := h.db.Begin()
	if err != nil {
		response.InternalError(w, err.Error())
		return
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	for key, val := range req {
		valueJSON, err := json.Marshal(val)
		if err != nil {
			response.BadRequest(w, "invalid value for key "+key)
			return
		}
		_, err = tx.Exec(`INSERT INTO platform_settings (key, value, updated_at) VALUES ($1, $2, $3) ON CONFLICT (key) DO UPDATE SET value = $2, updated_at = $3`, key, valueJSON, now)
		if err != nil {
			response.InternalError(w, err.Error())
			return
		}
	}
	if err := tx.Commit(); err != nil {
		response.InternalError(w, err.Error())
		return
	}
	response.Success(w, req)
}