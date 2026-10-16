package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestOSCatalogJSONAndScope(t *testing.T) {
	for _, args := range [][]string{{"os", "--format", "json"}, {"os", "list", "--format=json"}} {
		code, out, errOut := runCLI(t, args...)
		if code != ExitSuccess || errOut != "" {
			t.Fatalf("catalog failed: %s", errOut)
		}
		var result struct {
			SchemaVersion string          `json:"schemaVersion"`
			Items         []osCatalogItem `json:"items"`
		}
		if err := json.Unmarshal([]byte(out), &result); err != nil || result.SchemaVersion != "os-catalog/v1" {
			t.Fatalf("stdout is not the versioned JSON contract: %s (%v)", out, err)
		}
		found := false
		for _, item := range result.Items {
			if item.OS == "ubuntu-26.04" {
				found = true
				if item.BootstrapCertified || item.PackageManager != "apt" || item.DefaultSSHUser != "ubuntu" || len(item.Evidence) != 1 || item.Evidence[0].Route != "existing" {
					t.Fatalf("wrong compatibility scope: %+v", item)
				}
			}
		}
		if !found {
			t.Fatal("Ubuntu 26.04 missing from catalog")
		}
	}
}

func TestSimpleHelpAndVersionAliases(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"-h"}, {"init", "--help"}, {"os", "list", "--help"}, {"recipe", "show", "-h"}} {
		code, out, errOut := runCLI(t, args...)
		if code != ExitSuccess || !strings.Contains(out, "Usage:") || errOut != "" {
			t.Fatalf("help %v failed: %s %s", args, out, errOut)
		}
	}
	code, out, errOut := runCLI(t, "--version")
	if code != ExitSuccess || strings.TrimSpace(out) != VersionString || errOut != "" {
		t.Fatal("version alias differs from version command")
	}
	for _, args := range [][]string{{"os", "install"}, {"os", "--format", "xml"}, {"os", "--unknown"}} {
		code, _, _ := runCLI(t, args...)
		if code == ExitSuccess {
			t.Fatalf("invalid OS command accepted: %v", args)
		}
	}
}
