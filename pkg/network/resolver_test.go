package network

import (
	"context"
	"strings"
	"testing"
)

// With no master key configured the resolver must fail closed before touching
// the database, so a misconfigured worker can never fall back to its local
// kube context.
func TestPGCredentialResolverFailsClosedWithoutMasterKey(t *testing.T) {
	r := NewPGCredentialResolver(nil, nil, t.TempDir())
	path, err := r.KubeconfigFor(context.Background(), "target-1")
	if err == nil {
		t.Fatalf("expected error, got path %q", path)
	}
	if !strings.Contains(err.Error(), "master key not configured") {
		t.Fatalf("error = %q, want master key failure", err)
	}
}
