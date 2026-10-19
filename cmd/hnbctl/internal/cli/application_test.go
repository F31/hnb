package cli

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplicationRequiresSubcommandAndFile(t *testing.T) {
	code, _, errOut := runCLI(t, "application")
	if code == ExitSuccess || !strings.Contains(errOut, "application subcommand required") {
		t.Fatalf("missing subcommand: %d %s", code, errOut)
	}
	code, _, errOut = runCLI(t, "application", "plan")
	if code == ExitSuccess || !strings.Contains(errOut, "--composition-file") {
		t.Fatalf("missing composition file: %d %s", code, errOut)
	}
}

func TestApplicationRejectsTenantScopeMismatch(t *testing.T) {
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"tenantId":"tenant-a"}`))
	token := "header." + payload + ".signature"
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte(token), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, errOut := runCLI(t, "application", "status", "--delivery-id", "11111111-1111-4111-8111-111111111111",
		"--platform-url", "https://api.invalid", "--token-file", path, "--tenant-scope", "tenant-b")
	if code == ExitSuccess || !strings.Contains(errOut, "tenant scope does not match") {
		t.Fatalf("tenant mismatch accepted: %d %s", code, errOut)
	}
}

func TestApplicationActionsRequireDeliveryAndIdempotency(t *testing.T) {
	for _, action := range []string{"deploy", "upgrade", "rollback", "delete"} {
		code, _, errOut := runCLI(t, "application", action, "--delivery-id", "11111111-1111-4111-8111-111111111111")
		if code == ExitSuccess || !strings.Contains(errOut, "--idempotency-key") {
			t.Fatalf("%s missing idempotency key: %d %s", action, code, errOut)
		}
	}
}
