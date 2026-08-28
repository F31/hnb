package capability

import "testing"

func TestFromCSVDefaultsToAllStages(t *testing.T) {
	if got := FromCSV(""); !got.Has(Contract) || !got.Has(Write) || !got.Has(Read) || !got.Has(StorageSupply) {
		t.Fatalf("empty CSV should enable all stages, got %v", got.EnabledStages())
	}
	if got := FromCSV("   "); !got.Has(Projector) {
		t.Fatalf("whitespace CSV should enable all stages")
	}
}

func TestFromCSVParsesExplicitSubset(t *testing.T) {
	got := FromCSV("cluster.read,cluster.schema")
	if !got.Has(Read) || !got.Has(Schema) {
		t.Fatalf("expected read+schema enabled, got %v", got.EnabledStages())
	}
	for _, stage := range []string{Write, Provider, Projector, Contract} {
		if got.Has(stage) {
			t.Fatalf("stage %s should be disabled", stage)
		}
	}
}

func TestStorageSupplyDoesNotGateContainerConsumption(t *testing.T) {
	got := FromCSV("cluster.read")
	if got.Has(StorageSupply) {
		t.Fatal("storage supply must remain disabled unless explicitly enabled")
	}
	if !got.Has(Read) {
		t.Fatal("independent cluster consumption capability was disabled")
	}
}

func TestUnknownNameFailsClosed(t *testing.T) {
	all := AllStages()
	if all.Has("cluster.nope") {
		t.Fatal("unknown capability must never be enabled")
	}
	if all.Has("") {
		t.Fatal("empty capability name must fail closed")
	}
	if FromCSV("cluster.nope,cluster.read").Has("cluster.nope") {
		t.Fatal("unknown name must not be implicitly enabled by CSV")
	}
}

func TestFromCSVIgnoresUnknownNames(t *testing.T) {
	got := FromCSV("cluster.read,cluster.does-not-exist")
	if !got.Has(Read) {
		t.Fatalf("read should be enabled, got %v", got.EnabledStages())
	}
	if got.Has("cluster.does-not-exist") {
		t.Fatal("unknown name leaked into enabled set")
	}
}

func TestSnapshot(t *testing.T) {
	got := FromCSV("cluster.read,cluster.write")
	snap := got.Snapshot()
	if !snap[Read] || !snap[Write] {
		t.Fatalf("snapshot missing stages: %v", snap)
	}
	if snap[Provider] {
		t.Fatal("snapshot must not contain disabled stage")
	}
}
