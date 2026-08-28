package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/F31/hnb/cmd/apiserver/internal/response"
	"github.com/F31/hnb/pkg/tunnel"
)

type ProxyHandler struct {
	tunnelServer *tunnel.TunnelServer
	db           *sql.DB
}

func NewProxyHandler(db *sql.DB, ts *tunnel.TunnelServer) *ProxyHandler {
	return &ProxyHandler{db: db, tunnelServer: ts}
}

func (h *ProxyHandler) ProxyRequest(w http.ResponseWriter, r *http.Request) {
	clusterID := r.PathValue("cluster_id")
	path := r.PathValue("path")
	if clusterID == "" || path == "" {
		response.BadRequest(w, "cluster_id and path are required")
		return
	}
	if !requireClusterAccess(h.db, w, r, clusterID, "") {
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		response.BadRequest(w, "read body")
		return
	}
	headers := proxyHeaders(r.Header)
	req := proxyRequestPayload(r, path, headers, body)
	if req.RequestID == "" {
		req.RequestID = fmt.Sprintf("proxy-%s-%d", clusterID, len(body))
	}
	resp, err := h.tunnelServer.ProxyRequest(clusterID, req)
	if err != nil {
		response.Error(w, http.StatusBadGateway, 50200, err.Error())
		return
	}
	responseHeaders := make(http.Header)
	for k, v := range resp.Headers {
		responseHeaders.Set(k, v)
	}
	for k, v := range proxyHeaders(responseHeaders) {
		w.Header().Set(k, v)
	}
	w.WriteHeader(resp.StatusCode)
	if resp.Body != nil {
		w.Write(resp.Body)
	}
}

func proxyRequestPayload(r *http.Request, path string, headers map[string]string, body []byte) *tunnel.RequestPayload {
	return &tunnel.RequestPayload{
		RequestID: r.Header.Get("X-Correlation-ID"),
		Method:    r.Method,
		Path:      path,
		RawQuery:  r.URL.RawQuery,
		Headers:   headers,
		Body:      body,
	}
}

func proxyHeaders(source http.Header) map[string]string {
	headers := make(map[string]string)
	for _, name := range []string{"Content-Type", "Accept", "If-Match", "Idempotency-Key", "X-Correlation-ID", "traceparent"} {
		if value := source.Get(name); value != "" {
			headers[name] = value
		}
	}
	return headers
}

func (h *ProxyHandler) ListAgents(w http.ResponseWriter, r *http.Request) {
	agents := h.tunnelServer.Registry().List()
	filtered := agents[:0]
	for _, agent := range agents {
		if clusterAccessibleTo(h.db, agent.ClusterID, trustedTenantID(r), "") {
			filtered = append(filtered, agent)
		}
	}
	data, _ := json.Marshal(filtered)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

func (h *ProxyHandler) GetAgent(w http.ResponseWriter, r *http.Request) {
	clusterID := r.PathValue("cluster_id")
	if !requireClusterAccess(h.db, w, r, clusterID, "") {
		return
	}
	agent, ok := h.tunnelServer.Registry().Get(clusterID)
	if !ok {
		response.NotFound(w, "agent not connected")
		return
	}
	data, _ := json.Marshal(agent.Info)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}
