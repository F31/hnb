package observer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestDiscoverStorageInventoryPaginatesAndMapsCoreResources(t *testing.T) {
	var mu sync.Mutex
	requests := make(map[string]int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer kube-token" {
			t.Errorf("authorization = %q", r.Header.Get("Authorization"))
		}
		if r.URL.Path != snapshotAPIGroupPath && (r.URL.Query().Get("limit") == "" || r.URL.Query().Get("timeoutSeconds") == "") {
			t.Errorf("missing bounded list query: %s", r.URL.RawQuery)
		}
		mu.Lock()
		requests[r.URL.Path]++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/apis/storage.k8s.io/v1/storageclasses":
			if r.URL.Query().Get("continue") == "" {
				writeJSON(t, w, map[string]any{
					"metadata": map[string]any{"continue": "next-sc"},
					"items": []any{map[string]any{
						"metadata":    map[string]any{"name": "fast", "uid": "sc-uid-1", "resourceVersion": "11", "annotations": map[string]string{"storageclass.kubernetes.io/is-default-class": "true"}},
						"provisioner": "example.csi.io", "parameters": map[string]string{"tier": "fast"}, "reclaimPolicy": "Delete",
						"volumeBindingMode": "WaitForFirstConsumer", "allowVolumeExpansion": true,
						"allowedTopologies": []any{map[string]any{"matchLabelExpressions": []any{map[string]any{"key": "topology.kubernetes.io/zone", "values": []string{"a", "b"}}}}},
					}},
				})
				return
			}
			if r.URL.Query().Get("continue") != "next-sc" {
				t.Errorf("continue = %q", r.URL.Query().Get("continue"))
			}
			writeJSON(t, w, map[string]any{"metadata": map[string]any{}, "items": []any{map[string]any{
				"metadata":    map[string]any{"name": "archive", "uid": "sc-uid-2", "resourceVersion": "12"},
				"provisioner": "archive.csi.io", "reclaimPolicy": "Retain", "volumeBindingMode": "Immediate",
			}}})
		case "/apis/storage.k8s.io/v1/csidrivers":
			writeJSON(t, w, listResponse(map[string]any{
				"metadata": map[string]any{"name": "example.csi.io", "uid": "driver-uid-1", "resourceVersion": "21"},
				"spec":     map[string]any{"attachRequired": true, "podInfoOnMount": false, "storageCapacity": true, "fsGroupPolicy": "File", "requiresRepublish": true, "seLinuxMount": false, "volumeLifecycleModes": []string{"Persistent"}},
			}))
		case "/apis/storage.k8s.io/v1/csinodes":
			writeJSON(t, w, listResponse(map[string]any{
				"metadata": map[string]any{"name": "worker-1", "uid": "node-uid-1", "resourceVersion": "31"},
				"spec":     map[string]any{"drivers": []any{map[string]any{"name": "example.csi.io", "nodeID": "provider-node-1", "allocatable": map[string]any{"count": 16}, "topologyKeys": []string{"topology.kubernetes.io/zone"}}}},
			}))
		case "/apis/storage.k8s.io/v1/csistoragecapacities":
			writeJSON(t, w, listResponse(map[string]any{
				"metadata":         map[string]any{"name": "capacity-a", "namespace": "storage-system", "uid": "capacity-uid-1", "resourceVersion": "41"},
				"storageClassName": "fast", "capacity": "2Gi", "maximumVolumeSize": "512Mi",
				"nodeTopology": map[string]any{"matchLabels": map[string]string{"topology.kubernetes.io/zone": "a"}},
			}))
		case "/apis/storage.k8s.io/v1/volumeattachments":
			writeJSON(t, w, listResponse(map[string]any{
				"metadata": map[string]any{"name": "attachment-a", "uid": "attachment-uid-1", "resourceVersion": "51"},
				"spec":     map[string]any{"attacher": "example.csi.io", "nodeName": "worker-1", "source": map[string]any{"persistentVolumeName": "pv-a"}},
				"status":   map[string]any{"attached": false, "attachError": map[string]any{"message": "pending"}},
			}))
		case snapshotAPIGroupPath:
			http.NotFound(w, r)
		default:
			t.Errorf("unexpected discovery path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	discovery := NewKubeDiscovery(server.URL, "kube-token")
	discovery.pageLimit = 1
	observedAt := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	inventory, err := discovery.DiscoverStorageInventory(context.Background(), observedAt)
	if err != nil {
		t.Fatalf("discover storage inventory: %v", err)
	}
	if len(inventory.StorageClasses) != 2 || inventory.StorageClasses[0].UID != "sc-uid-1" || inventory.StorageClasses[0].ResourceVersion != "11" {
		t.Fatalf("storage classes = %+v", inventory.StorageClasses)
	}
	if inventory.StorageClasses[0].IsDefault == nil || !*inventory.StorageClasses[0].IsDefault || inventory.StorageClasses[0].AllowedTopologies[0]["topology.kubernetes.io/zone"] != "a,b" {
		t.Fatalf("storage class mapping = %+v", inventory.StorageClasses[0])
	}
	if got := inventory.CSIDrivers[0]; got.StorageCapacity == nil || !*got.StorageCapacity || got.FSGroupPolicy != "File" {
		t.Fatalf("CSI driver mapping = %+v", got)
	}
	if got := inventory.CSINodes[0].Drivers[0]; got.AllocatableCount == nil || *got.AllocatableCount != 16 || got.NodeID != "provider-node-1" {
		t.Fatalf("CSI node mapping = %+v", got)
	}
	if got := inventory.CSIStorageCapacities[0]; got.CapacityBytes == nil || *got.CapacityBytes != 2<<30 || got.MaximumVolumeSizeBytes == nil || *got.MaximumVolumeSizeBytes != 512<<20 {
		t.Fatalf("capacity mapping = %+v", got)
	}
	if got := inventory.VolumeAttachments[0]; got.Attached == nil || *got.Attached || got.PersistentVolumeName != "pv-a" || got.AttachError != "pending" {
		t.Fatalf("attachment mapping = %+v", got)
	}
	if inventory.StorageClasses[0].ObservedAt != observedAt || inventory.StorageClasses[0].Source != storageAPISource {
		t.Fatalf("source identity = %+v", inventory.StorageClasses[0].KubernetesResourceIdentity)
	}
	if inventory.SnapshotAPI == nil || inventory.SnapshotAPI.Status != "NotInstalled" {
		t.Fatalf("snapshot API = %+v", inventory.SnapshotAPI)
	}
	if requests["/apis/storage.k8s.io/v1/storageclasses"] != 2 {
		t.Fatalf("StorageClass requests = %d want 2", requests["/apis/storage.k8s.io/v1/storageclasses"])
	}
}

func TestStorageDiscoveryPropagatesFailuresAndBounds(t *testing.T) {
	t.Run("HTTP failure", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "forbidden", http.StatusForbidden)
		}))
		defer server.Close()
		_, err := NewKubeDiscovery(server.URL, "").DiscoverStorageInventory(context.Background(), time.Now().UTC())
		if err == nil || !strings.Contains(err.Error(), "StorageClass") || !strings.Contains(err.Error(), "403") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("item limit", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(t, w, map[string]any{"metadata": map[string]any{}, "items": []any{map[string]any{}, map[string]any{}}})
		}))
		defer server.Close()
		discovery := NewKubeDiscovery(server.URL, "")
		_, err := listPaginated[storageClassAPI](context.Background(), discovery, "/items", 1)
		if err == nil || !strings.Contains(err.Error(), "item limit 1 exceeded") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("page limit", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(t, w, map[string]any{"metadata": map[string]any{"continue": "next"}, "items": []any{map[string]any{}}})
		}))
		defer server.Close()
		discovery := NewKubeDiscovery(server.URL, "")
		discovery.maxPages = 1
		_, err := listPaginated[storageClassAPI](context.Background(), discovery, "/items", 10)
		if err == nil || !strings.Contains(err.Error(), "page limit 1 exceeded") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("response body limit", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = fmt.Fprint(w, `{"metadata":{},"items":[],"padding":"`+strings.Repeat("x", 128)+`"}`)
		}))
		defer server.Close()
		discovery := NewKubeDiscovery(server.URL, "")
		discovery.maxResponseBytes = 64
		_, err := listPaginated[storageClassAPI](context.Background(), discovery, "/items", 10)
		if err == nil || !strings.Contains(err.Error(), "response exceeds 64 bytes") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("request timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			<-r.Context().Done()
		}))
		defer server.Close()
		discovery := NewKubeDiscovery(server.URL, "")
		discovery.requestTimeout = 20 * time.Millisecond
		_, err := listPaginated[storageClassAPI](context.Background(), discovery, "/items", 10)
		if err == nil || !strings.Contains(err.Error(), "context deadline exceeded") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("parent cancellation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(t, w, map[string]any{"metadata": map[string]any{}, "items": []any{}})
		}))
		defer server.Close()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := listPaginated[storageClassAPI](ctx, NewKubeDiscovery(server.URL, ""), "/items", 10)
		if err == nil || !strings.Contains(err.Error(), "context canceled") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestStorageDiscoveryRejectsMissingStableIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/apis/storage.k8s.io/v1/storageclasses" {
			writeJSON(t, w, listResponse(map[string]any{"metadata": map[string]any{"name": "fast", "resourceVersion": "1"}}))
			return
		}
		writeJSON(t, w, map[string]any{"metadata": map[string]any{}, "items": []any{}})
	}))
	defer server.Close()
	_, err := NewKubeDiscovery(server.URL, "").DiscoverStorageInventory(context.Background(), time.Now().UTC())
	if err == nil || !strings.Contains(err.Error(), "missing uid or resourceVersion") {
		t.Fatalf("error = %v", err)
	}
}

func listResponse(item map[string]any) map[string]any {
	return map[string]any{"metadata": map[string]any{}, "items": []any{item}}
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Errorf("encode response: %v", err)
	}
}
