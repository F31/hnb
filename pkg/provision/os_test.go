package provision

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestOSAdaptersAreIsolatedAndRejectUnknown(t *testing.T) {
	adapters := SupportedOSAdapters()
	adapters[0].OS = "changed"
	adapters[0].BootstrapPackages[0] = "changed"
	legacy, ok := LookupOSAdapter("ubuntu-24.04")
	if !ok || legacy.BootstrapPackages[0] != "apt-transport-https" {
		t.Fatal("caller mutated the adapter registry")
	}
	if _, ok := LookupOSAdapter("ubuntu-26.10"); ok {
		t.Fatal("unknown release was accepted")
	}
}

func TestAPTBootstrapPackagesResolvedOnTarget(t *testing.T) {
	for _, tc := range []struct{ release, want string }{
		{"ubuntu-24.04", "apt-transport-https ca-certificates curl gpg"},
		{"ubuntu-26.04", "ca-certificates curl gpg"},
		{"debian-12", "apt-transport-https ca-certificates curl gpg"},
	} {
		t.Run(tc.release, func(t *testing.T) {
			id, version, _ := strings.Cut(tc.release, "-")
			path := filepath.Join(t.TempDir(), "os-release")
			if err := os.WriteFile(path, []byte("ID="+id+"\nVERSION_ID=\""+version+"\"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			script := strings.ReplaceAll(aptBootstrapPackagesScript(), "/etc/os-release", ShQuote(path))
			cmd := exec.Command("sh", "-c", script+"printf '%s' \"$apt_bootstrap_packages\"")
			out, err := cmd.CombinedOutput()
			if err != nil || string(out) != tc.want {
				t.Fatalf("target package resolution: %s (%v)", out, err)
			}
		})
	}
	script := PrereqScript(ProvisionSpec{DesiredVersion: "1.36.5"})
	if !strings.Contains(script, aptBootstrapPackagesScript()) || !strings.Contains(script, "apt-get install -yq $apt_bootstrap_packages") || strings.Contains(script, "%!") {
		t.Fatal("the shipped prerequisite script does not use target OS adapters")
	}
}
