package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	schemaapp "github.com/F31/hnb/cmd/apiserver/internal/application/schema"
	"github.com/F31/hnb/cmd/apiserver/internal/response"
	"github.com/F31/hnb/pkg/iam"
)

type SchemaHandler struct{ service *schemaapp.Service }

func NewSchemaHandler() *SchemaHandler {
	return &SchemaHandler{service: schemaapp.NewService(schemaapp.NewStaticRepository())}
}

func NewSchemaHandlerWithService(service *schemaapp.Service) *SchemaHandler {
	return &SchemaHandler{service: service}
}

// Page 返回 PageSchema 信封，支持 If-None-Match / ETag 条件请求
// （V2.6 §15.3 前端 ETag / revision 比对，TTL 仅兜底）。
func (h *SchemaHandler) Page(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Unauthorized(w, "trusted context required")
		return
	}
	id := r.PathValue("id")

	// 先按 revision 派生 ETag：命中则 304，避免全量读取。
	if revision, found := h.service.ActiveRevision(r.Context(), id); found {
		etag := schemaEtag(id, revision)
		if r.Header.Get("If-None-Match") == etag {
			w.Header().Set("ETag", etag)
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}

	page, err := h.service.Get(r.Context(), id, trusted)
	if err != nil {
		switch {
		case errors.Is(err, schemaapp.ErrNotFound):
			response.NotFound(w, "schema page not found")
		case errors.Is(err, schemaapp.ErrForbidden):
			response.Forbidden(w, "forbidden")
		default:
			response.InternalError(w, err.Error())
		}
		return
	}
	if page.Metadata.Etag != "" {
		w.Header().Set("ETag", page.Metadata.Etag)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(page)
}

// Publish 将请求体中的 PageSchema 发布为新 revision（V2.6 §20.3）：
// 同一事务写入不可变历史 + Outbox 事件，前端经 revision 提升感知变更。
func (h *SchemaHandler) Publish(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Unauthorized(w, "trusted context required")
		return
	}
	id := r.PathValue("id")

	var page schemaapp.Page
	if err := json.NewDecoder(r.Body).Decode(&page); err != nil {
		response.BadRequest(w, "invalid PageSchema body")
		return
	}
	if page.Metadata.ID == "" {
		page.Metadata.ID = id
	}
	if page.Metadata.ID != id {
		response.BadRequest(w, "page id in body does not match path")
		return
	}

	published, err := h.service.Publish(r.Context(), page, trusted)
	if err != nil {
		switch {
		case errors.Is(err, schemaapp.ErrInvalid):
			response.BadRequest(w, "invalid PageSchema payload")
		case errors.Is(err, schemaapp.ErrForbidden):
			response.Forbidden(w, "forbidden")
		default:
			response.InternalError(w, err.Error())
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if published.Metadata.Etag != "" {
		w.Header().Set("ETag", published.Metadata.Etag)
	}
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(published)
}

// RollbackRequest 回滚目标 revision（V2.6 §20.4）。
type RollbackRequest struct {
	Revision int `json:"revision"`
}

// Rollback 将页面切换回历史 revision：只切换 active_revision，不覆盖历史，
// 同事务写 Outbox 回滚事件，前端经 revision 变化感知并刷新缓存。
func (h *SchemaHandler) Rollback(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Unauthorized(w, "trusted context required")
		return
	}
	id := r.PathValue("id")

	var body RollbackRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.BadRequest(w, "invalid rollback body")
		return
	}
	if body.Revision < 1 {
		response.BadRequest(w, "revision must be >= 1")
		return
	}

	page, err := h.service.Rollback(r.Context(), id, body.Revision, trusted)
	if err != nil {
		switch {
		case errors.Is(err, schemaapp.ErrNotFound):
			response.NotFound(w, "schema page not found")
		case errors.Is(err, schemaapp.ErrRevisionNotFound):
			response.NotFound(w, "schema page revision not found")
		case errors.Is(err, schemaapp.ErrInvalid):
			response.BadRequest(w, "invalid rollback target")
		case errors.Is(err, schemaapp.ErrForbidden):
			response.Forbidden(w, "forbidden")
		default:
			response.InternalError(w, err.Error())
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if page.Metadata.Etag != "" {
		w.Header().Set("ETag", page.Metadata.Etag)
	}
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(page)
}

func schemaEtag(id string, revision int) string {
	return "page-" + id + "-r" + strconv.Itoa(revision)
}
