package provider

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestPGObserverRegistryRegisterKubernetesTarget(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	registry := NewPGObserverRegistry(db)
	mock.ExpectExec(regexp.QuoteMeta("UPDATE runtime_targets")).
		WithArgs("515eba09-0a41-5b92-b972-69af1f0f655c", "tenant-a", "kubernetes", "agent", "Agent:515eba09-0a41-5b92-b972-69af1f0f655c").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := registry.Register(context.Background(), "tenant-a", "515eba09-0a41-5b92-b972-69af1f0f655c", "KubernetesTarget", "Agent"); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPGObserverRegistryRegisterEdgeTarget(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	registry := NewPGObserverRegistry(db)
	mock.ExpectExec(regexp.QuoteMeta("UPDATE runtime_targets")).
		WithArgs("6d384d43-243b-5e14-b7e4-c03be376cb7c", "tenant-a", "edge_runtime", "cloudcore", "CloudCore:6d384d43-243b-5e14-b7e4-c03be376cb7c").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := registry.Register(context.Background(), "tenant-a", "6d384d43-243b-5e14-b7e4-c03be376cb7c", "EdgeRuntimeTarget", "CloudCore"); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPGObserverRegistryUnregister(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	registry := NewPGObserverRegistry(db)
	mock.ExpectExec(regexp.QuoteMeta("UPDATE runtime_targets")).
		WithArgs("515eba09-0a41-5b92-b972-69af1f0f655c", "tenant-a").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := registry.Unregister(context.Background(), "tenant-a", "515eba09-0a41-5b92-b972-69af1f0f655c"); err != nil {
		t.Fatalf("unregister: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPGObserverRegistryReportsMissingTarget(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	registry := NewPGObserverRegistry(db)
	mock.ExpectExec(regexp.QuoteMeta("UPDATE runtime_targets")).
		WithArgs("515eba09-0a41-5b92-b972-69af1f0f655c", "tenant-a", "kubernetes", "agent", "Agent:515eba09-0a41-5b92-b972-69af1f0f655c").
		WillReturnResult(sqlmock.NewResult(0, 0))

	err = registry.Register(context.Background(), "tenant-a", "515eba09-0a41-5b92-b972-69af1f0f655c", "KubernetesTarget", "Agent")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not found error, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
