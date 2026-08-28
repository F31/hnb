package observer

import (
	"testing"
	"time"
)

func TestStorageProjectionRecordsKeepDriverEvidenceIndependent(t *testing.T) {
	observedAt := time.Now().UTC().Add(-time.Minute)
	inventory := &StorageInventory{
		StorageClasses: []StorageClassFact{{
			KubernetesResourceIdentity: KubernetesResourceIdentity{
				UID: "sc-uid", ResourceVersion: "1", Name: "fast",
				Source: "kubernetes.storage.k8s.io/v1", ObservedAt: observedAt,
			},
			Provisioner: "example.csi.io",
		}},
		CSIDrivers: []CSIDriverFact{{
			KubernetesResourceIdentity: KubernetesResourceIdentity{
				UID: "driver-uid", ResourceVersion: "2", Name: "example.csi.io",
				Source: "kubernetes.storage.k8s.io/v1", ObservedAt: observedAt,
			},
		}},
		CSINodes: []CSINodeFact{{
			KubernetesResourceIdentity: KubernetesResourceIdentity{
				UID: "node-uid", ResourceVersion: "3", Name: "worker-1",
				Source: "kubernetes.storage.k8s.io/v1", ObservedAt: observedAt,
			},
			Drivers: []CSINodeDriverFact{{Name: "example.csi.io", NodeID: "node-1"}},
		}},
	}

	records, err := storageProjectionRecords(inventory)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 {
		t.Fatalf("records=%d want 3", len(records))
	}
	wantEvidence := []string{"StorageClassReference", "CSIDriverRegistration", "CSINodeRegistration"}
	for i, record := range records {
		if len(record.Evidence) != 1 {
			t.Fatalf("record %s evidence=%d want 1", record.Kind, len(record.Evidence))
		}
		if record.Evidence[0].Kind != wantEvidence[i] || record.Evidence[0].DriverName != "example.csi.io" {
			t.Fatalf("record %s evidence=%+v", record.Kind, record.Evidence[0])
		}
	}
}

func TestStorageProjectionRecordsDoNotInventMissingDriverEvidence(t *testing.T) {
	records, err := storageProjectionRecords(&StorageInventory{
		StorageClasses: []StorageClassFact{{
			KubernetesResourceIdentity: KubernetesResourceIdentity{
				UID: "sc-uid", ResourceVersion: "1", Name: "static",
				Source: "kubernetes.storage.k8s.io/v1", ObservedAt: time.Now().UTC(),
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || len(records[0].Evidence) != 0 {
		t.Fatalf("unexpected inferred evidence: %+v", records)
	}
}
