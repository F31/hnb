package handler

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/F31/hnb/cmd/apiserver/internal/response"
	"github.com/F31/hnb/pkg/iam"
)

type MarketHandler struct {
	marketURL string
	client    *http.Client
}

func NewMarketHandler(marketURL string) *MarketHandler {
	return &MarketHandler{marketURL: strings.TrimRight(marketURL, "/"), client: newInternalHTTPClient(30 * time.Second)}
}

func (h *MarketHandler) Proxy(w http.ResponseWriter, r *http.Request) {
	if h.marketURL == "" {
		response.Error(w, http.StatusBadGateway, response.CodeServiceUnavailable, "app-market unavailable")
		return
	}
	token, ok := iam.RawAccessTokenFrom(r.Context())
	if !ok {
		response.Unauthorized(w, "access token required")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/market")
	if path == "" || path == "/" {
		path = "/products"
	}
	if strings.Contains(path, "..") {
		response.BadRequest(w, "invalid market path")
		return
	}
	target := h.marketURL + "/api/v1" + path
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}

	var body io.Reader
	if r.Body != nil {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			response.BadRequest(w, "invalid body")
			return
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(r.Context(), r.Method, target, body)
	if err != nil {
		response.InternalError(w, err.Error())
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)
	copyHeader(req.Header, r.Header, "Content-Type")
	copyHeader(req.Header, r.Header, "Accept")
	copyHeader(req.Header, r.Header, "X-Tenant-ID")
	copyHeader(req.Header, r.Header, "X-Space-ID")
	copyHeader(req.Header, r.Header, "X-Trace-Id")
	copyHeader(req.Header, r.Header, "X-Correlation-ID")
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/json")
	}
	if req.Header.Get("Content-Type") == "" && body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := h.client.Do(req)
	if err != nil {
		response.Error(w, http.StatusBadGateway, response.CodeServiceUnavailable, "app-market unavailable")
		return
	}
	defer resp.Body.Close()
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}
