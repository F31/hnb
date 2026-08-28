package observer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestDiscoverSnapshotInventoryMissingAndUnsupported(t *testing.T) {
	observedAt := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	t.Run("API group 404 is NotInstalled", func(t *testing.T) {
		server := httptest.NewServer(http.NotFoundHandler())
		defer server.Close()
		result, err := NewKubeDiscovery(server.URL, "").discoverSnapshotInventory(context.Background(), observedAt)
		if err != nil {
			t.Fatal(err)
		}
		if result.API.Status != "NotInstalled" || result.API.APIVersion != "" || result.Classes != nil || result.Snapshots != nil || result.Contents != nil {
			t.Fatalf("result = %+v", result)
		}
	})

	t.Run("v1 discovery 404 is NotInstalled", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == snapshotAPIGroupPath {
				writeJSON(t, w, snapshotGroup(true))
				return
			}
			http.NotFound(w, r)
		}))
		defer server.Close()
		result, err := NewKubeDiscovery(server.URL, "").discoverSnapshotInventory(context.Background(), observedAt)
		if err != nil {
			t.Fatal(err)
		}
		if result.API.Status != "NotInstalled" {
			t.Fatalf("status = %s", result.API.Status)
		}
	})

	t.Run("resource list 404 is NotInstalled", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case snapshotAPIGroupPath:
				writeJSON(t, w, snapshotGroup(true))
			case snapshotAPIGroupPath + "/v1":
				writeJSON(t, w, snapshotResources())
			default:
				http.NotFound(w, r)
			}
		}))
		defer server.Close()
		result, err := NewKubeDiscovery(server.URL, "").discoverSnapshotInventory(context.Background(), observedAt)
		if err != nil {
			t.Fatal(err)
		}
		if result.API.Status != "NotInstalled" {
			t.Fatalf("status = %s", result.API.Status)
		}
	})

	t.Run("group without v1 is Unsupported", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(t, w, snapshotGroup(false))
		}))
		defer server.Close()
		result, err := NewKubeDiscovery(server.URL, "").discoverSnapshotInventory(context.Background(), observedAt)
		if err != nil {
			t.Fatal(err)
		}
		if result.API.Status != "Unsupported" {
			t.Fatalf("status = %s", result.API.Status)
		}
	})

	t.Run("incomplete v1 resources are Unsupported", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case snapshotAPIGroupPath:
				writeJSON(t, w, snapshotGroup(true))
			case snapshotAPIGroupPath + "/v1":
				writeJSON(t, w, map[string]any{"groupVersion": snapshotAPIVersion, "resources": []any{map[string]any{"name": "volumesnapshots", "verbs": []string{"get"}}}})
			default:
				t.Errorf("unexpected list request %s", r.URL.Path)
			}
		}))
		defer server.Close()
		result, err := NewKubeDiscovery(server.URL, "").discoverSnapshotInventory(context.Background(), observedAt)
		if err != nil {
			t.Fatal(err)
		}
		if result.API.Status != "Unsupported" {
			t.Fatalf("status = %s", result.API.Status)
		}
	})
}

func TestDiscoverSnapshotInventoryInstalledEmpty(t *testing.T) {
	var mu sync.Mutex
	listed := make(map[string]int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case snapshotAPIGroupPath:
			writeJSON(t, w, snapshotGroup(true))
		case snapshotAPIGroupPath + "/v1":
			writeJSON(t, w, snapshotResources())
		case snapshotAPIGroupPath + "/v1/volumesnapshotclasses", snapshotAPIGroupPath + "/v1/volumesnapshots", snapshotAPIGroupPath + "/v1/volumesnapshotcontents":
			if r.URL.Query().Get("limit") == "" || r.URL.Query().Get("timeoutSeconds") == "" {
				t.Errorf("unbounded list query %s", r.URL.RawQuery)
			}
			mu.Lock()
			listed[r.URL.Path]++
			mu.Unlock()
			writeJSON(t, w, map[string]any{"metadata": map[string]any{}, "items": []any{}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	result, err := NewKubeDiscovery(server.URL, "").discoverSnapshotInventory(context.Background(), time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if result.API.Status != "Installed" || result.API.APIVersion != snapshotAPIVersion {
		t.Fatalf("API = %+v", result.API)
	}
	if result.Classes == nil || result.Snapshots == nil || result.Contents == nil || len(result.Classes)+len(result.Snapshots)+len(result.Contents) != 0 {
		t.Fatalf("installed empty lists = classes:%#v snapshots:%#v contents:%#v", result.Classes, result.Snapshots, result.Contents)
	}
	if len(listed) != 3 {
		t.Fatalf("listed endpoints = %v", listed)
	}
}

func TestDiscoverSnapshotInventoryPaginationAndMapping(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case snapshotAPIGroupPath:
			writeJSON(t, w, snapshotGroup(true))
		case snapshotAPIGroupPath + "/v1":
			writeJSON(t, w, snapshotResources())
		case snapshotAPIGroupPath + "/v1/volumesnapshotclasses":
			if r.URL.Query().Get("continue") == "" {
				writeJSON(t, w, map[string]any{"metadata": map[string]any{"continue": "class-next"}, "items": []any{snapshotClass("class-a", "class-uid-a", "1")}})
				return
			}
			if r.URL.Query().Get("continue") != "class-next" {
				t.Errorf("continue = %q", r.URL.Query().Get("continue"))
			}
			writeJSON(t, w, map[string]any{"metadata": map[string]any{}, "items": []any{snapshotClass("class-b", "class-uid-b", "2")}})
		case snapshotAPIGroupPath + "/v1/volumesnapshots":
			writeJSON(t, w, listResponse(map[string]any{
				"metadata": map[string]any{"name": "snapshot-a", "namespace": "tenant-a", "uid": "snapshot-uid-a", "resourceVersion": "3"},
				"spec":     map[string]any{"volumeSnapshotClassName": "class-a", "source": map[string]any{"persistentVolumeClaimName": "claim-a"}},
				"status":   map[string]any{"boundVolumeSnapshotContentName": "content-a", "readyToUse": true, "restoreSize": "4Gi"},
			}))
		case snapshotAPIGroupPath + "/v1/volumesnapshotcontents":
			writeJSON(t, w, listResponse(map[string]any{
				"metadata": map[string]any{"name": "content-a", "uid": "content-uid-a", "resourceVersion": "4"},
				"spec":     map[string]any{"driver": "example.csi.io", "deletionPolicy": "Delete", "source": map[string]any{}, "volumeSnapshotRef": map[string]any{"name": "snapshot-a", "namespace": "tenant-a"}},
				"status":   map[string]any{"snapshotHandle": "provider-snapshot-a", "readyToUse": true, "restoreSize": "4Gi"},
			}))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	discovery := NewKubeDiscovery(server.URL, "")
	discovery.pageLimit = 1
	observedAt := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	result, err := discovery.discoverSnapshotInventory(context.Background(), observedAt)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Classes) != 2 || result.Classes[0].Driver != "example.csi.io" || result.Classes[0].ResourceVersion != "1" {
		t.Fatalf("classes = %+v", result.Classes)
	}
	if got := result.Snapshots[0]; got.Namespace != "tenant-a" || got.SourceKind != "PersistentVolumeClaim" || got.SourceName != "claim-a" || got.RestoreSizeBytes == nil || *got.RestoreSizeBytes != 4<<30 {
		t.Fatalf("snapshot = %+v", got)
	}
	if got := result.Contents[0]; got.SnapshotHandle != "provider-snapshot-a" || got.VolumeSnapshotName != "snapshot-a" || got.UID != "content-uid-a" || got.Source != snapshotObjectSource {
		t.Fatalf("content = %+v", got)
	}
}

func TestDiscoverSnapshotInventoryPropagatesNon404Failure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case snapshotAPIGroupPath:
			writeJSON(t, w, snapshotGroup(true))
		case snapshotAPIGroupPath + "/v1":
			writeJSON(t, w, snapshotResources())
		default:
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
		}
	}))
	defer server.Close()
	_, err := NewKubeDiscovery(server.URL, "").discoverSnapshotInventory(context.Background(), time.Now().UTC())
	if err == nil || !strings.Contains(err.Error(), "503") || !strings.Contains(err.Error(), "list VolumeSnapshotClass") {
		t.Fatalf("error = %v", err)
	}
}

func snapshotGroup(v1 bool) map[string]any {
	version := "v1beta1"
	if v1 {
		version = "v1"
	}
	return map[string]any{"versions": []any{map[string]any{"groupVersion": "snapshot.storage.k8s.io/" + version, "version": version}}}
}

func snapshotResources() map[string]any {
	return map[string]any{"groupVersion": snapshotAPIVersion, "resources": []any{
		map[string]any{"name": "volumesnapshotclasses", "verbs": []string{"get", "list"}},
		map[string]any{"name": "volumesnapshots", "verbs": []string{"get", "list"}},
		map[string]any{"name": "volumesnapshotcontents", "verbs": []string{"get", "list"}},
	}}
}

func snapshotClass(name, uid, resourceVersion string) map[string]any {
	return map[string]any{
		"metadata": map[string]any{"name": name, "uid": uid, "resourceVersion": resourceVersion},
		"driver":   "example.csi.io", "deletionPolicy": "Delete", "parameters": map[string]string{"tier": "fast"},
	}
}
