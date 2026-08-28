package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/F31/hnb/pkg/iam"
)

func TestMarketProxyForwardsToAppMarket(t *testing.T) {
	var gotPath, gotQuery, gotAuth string
	market := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []string{"mysql"}, "total": 1})
	}))
	defer market.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/market/products?q=mysql&category=database", nil)
	req = req.WithContext(iam.WithRawAccessToken(req.Context(), "raw-token"))
	recorder := httptest.NewRecorder()

	NewMarketHandler(market.URL).Proxy(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if gotPath != "/api/v1/products" || gotQuery != "q=mysql&category=database" || gotAuth != "Bearer raw-token" {
		t.Fatalf("unexpected forward path=%q query=%q auth=%q", gotPath, gotQuery, gotAuth)
	}
}

func TestMarketProxyUnavailable(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/market/products", nil)
	req = req.WithContext(iam.WithRawAccessToken(req.Context(), "raw-token"))
	recorder := httptest.NewRecorder()

	NewMarketHandler("").Proxy(recorder, req)

	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func TestMarketProxyRequiresRawToken(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/market/products", nil)
	recorder := httptest.NewRecorder()

	NewMarketHandler("http://app-market").Proxy(recorder, req)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", recorder.Code)
	}
}
