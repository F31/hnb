package main

import (
	"strings"
	"testing"

	"github.com/F31/hnb/cmd/rbac-syncer/internal/config"
)

func TestOpenDBRejectsInvalidChartDSNWithoutFallback(t *testing.T) {
	_, err := openDB(&config.Config{DBDSN: "mysql://user:private-pass@host/db"})
	if err == nil || strings.Contains(err.Error(), "private-pass") {
		t.Fatal("invalid URL must fail without exposing credentials or using localhost")
	}
}
