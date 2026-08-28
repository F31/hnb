package router

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStorageRoutesUseDedicatedMethods(t *testing.T) {
	mux := http.NewServeMux()
	called := ""
	handlers := map[string]http.HandlerFunc{}
	for _, name := range []string{
		"storage.overview", "storage.backends", "storage.backends.create", "storage.backends.get", "storage.backends.update", "storage.backends.delete",
		"storage.offerings", "storage.offerings.create", "storage.offerings.get", "storage.offerings.update", "storage.offerings.delete",
		"storage.driver-installations", "storage.target-inventory", "storage.target-metrics", "storage.offering-bindings",
		"storage.drivers.install", "storage.drivers.upgrade", "storage.drivers.uninstall",
		"storage.bindings.create", "storage.bindings.get", "storage.bindings.update", "storage.bindings.delete",
		"storage.bindings.import", "storage.bindings.reconcile",
		"storage.provider-schemas",
		"storage.retained-volumes.release", "storage.retained-volumes.sanitize",
		"storage.alert-rules.list", "storage.alert-rules.create",
	} {
		name := name
		handlers[name] = func(w http.ResponseWriter, _ *http.Request) { called = name; w.WriteHeader(http.StatusNoContent) }
	}
	registerStorageRoutes(mux, handlers)
	for _, test := range []struct{ method, path, want string }{
		{http.MethodGet, "/api/v1/storage/overview", "storage.overview"},
		{http.MethodGet, "/api/v1/storage/backends", "storage.backends"},
		{http.MethodGet, "/api/v1/storage/provider-schemas", "storage.provider-schemas"},
		{http.MethodPost, "/api/v1/storage/backends", "storage.backends.create"},
		{http.MethodPut, "/api/v1/storage/backends/32684d2c-fca8-4f28-a946-fb267363fd6c", "storage.backends.update"},
		{http.MethodDelete, "/api/v1/storage/backends/32684d2c-fca8-4f28-a946-fb267363fd6c", "storage.backends.delete"},
		{http.MethodGet, "/api/v1/storage/offerings", "storage.offerings"},
		{http.MethodPost, "/api/v1/storage/offerings", "storage.offerings.create"},
		{http.MethodGet, "/api/v1/storage/driver-installations", "storage.driver-installations"},
		{http.MethodPost, "/api/v1/storage/driver-installations/32684d2c-fca8-4f28-a946-fb267363fd6c/intents/install", "storage.drivers.install"},
		{http.MethodPost, "/api/v1/storage/driver-installations/32684d2c-fca8-4f28-a946-fb267363fd6c/intents/upgrade", "storage.drivers.upgrade"},
		{http.MethodPost, "/api/v1/storage/driver-installations/32684d2c-fca8-4f28-a946-fb267363fd6c/intents/uninstall", "storage.drivers.uninstall"},
		{http.MethodGet, "/api/v1/storage/targets/32684d2c-fca8-4f28-a946-fb267363fd6c/inventory", "storage.target-inventory"},
		{http.MethodGet, "/api/v1/storage/targets/32684d2c-fca8-4f28-a946-fb267363fd6c/metrics", "storage.target-metrics"},
		{http.MethodGet, "/api/v1/storage/offerings/32684d2c-fca8-4f28-a946-fb267363fd6c/bindings", "storage.offering-bindings"},
		{http.MethodPost, "/api/v1/storage/offerings/32684d2c-fca8-4f28-a946-fb267363fd6c/bindings", "storage.bindings.create"},
		{http.MethodPut, "/api/v1/storage/bindings/32684d2c-fca8-4f28-a946-fb267363fd6c", "storage.bindings.update"},
		{http.MethodPost, "/api/v1/storage/offerings/32684d2c-fca8-4f28-a946-fb267363fd6c/bindings/intents/import", "storage.bindings.import"},
		{http.MethodPost, "/api/v1/storage/bindings/32684d2c-fca8-4f28-a946-fb267363fd6c/intents/reconcile", "storage.bindings.reconcile"},
		{http.MethodPost, "/api/v1/storage/retained-volumes/volume-a/intents/release", "storage.retained-volumes.release"},
		{http.MethodPost, "/api/v1/storage/retained-volumes/volume-a/intents/sanitize", "storage.retained-volumes.sanitize"},
		{http.MethodGet, "/api/v1/storage/alert-rules", "storage.alert-rules.list"},
		{http.MethodPost, "/api/v1/storage/alert-rules", "storage.alert-rules.create"},
	} {
		called = ""
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, httptest.NewRequest(test.method, test.path, nil))
		if recorder.Code != http.StatusNoContent || called != test.want {
			t.Fatalf("%s status=%d called=%q", test.path, recorder.Code, called)
		}
	}
}
