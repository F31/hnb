package provision

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The bootstrap scripts run on nodes that may not ship an interpreter, so the
// containerd import merge must be verifiable as plain POSIX shell/awk.
func TestContainerdImportMergeFixtures(t *testing.T) {
	const entry = "/etc/containerd/hnb-bootstrap.toml"
	cases := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			// The shell handles this case by prepending the import line; the awk
			// merge is only reached when an imports key already exists, so a
			// document without one must fail closed rather than be guessed at.
			name:    "no imports key",
			input:   "version = 2\n[debug]\n  level = \"info\"\n",
			wantErr: true,
		},
		{
			name:  "single line imports",
			input: "imports = [\"/etc/other.toml\"]\nversion = 2\n",
			want:  "imports = [\"/etc/other.toml\", \"" + entry + "\"]\nversion = 2\n",
		},
		{
			name:  "trailing comma imports",
			input: "imports = [\"/etc/other.toml\",]\nversion = 2\n",
			want:  "imports = [\"/etc/other.toml\", \"" + entry + "\"]\nversion = 2\n",
		},
		{
			// The closing bracket sits on its own line, so the entry is appended
			// there. The result is equivalent TOML, only the formatting differs.
			name:  "multi line imports",
			input: "imports = [\n  \"/etc/a.toml\",\n  \"/etc/b.toml\"\n]\nversion = 2\n",
			want:  "imports = [\n  \"/etc/a.toml\",\n  \"/etc/b.toml\"\n, \"" + entry + "\"]\nversion = 2\n",
		},
		{
			name:  "indented imports assignment",
			input: "  imports = [\"/etc/a.toml\"]\nversion = 2\n",
			want:  "  imports = [\"/etc/a.toml\", \"" + entry + "\"]\nversion = 2\n",
		},
		{
			name:    "empty imports array",
			input:   "imports = []\nversion = 2\n",
			want:    "imports = [\"" + entry + "\"]\nversion = 2\n",
			wantErr: false,
		},
		{
			// An unterminated array is not something to guess about: the merge
			// must fail closed instead of rewriting administrator configuration.
			name:    "unterminated imports",
			input:   "imports = [\n  \"/etc/a.toml\"\n",
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command("awk", "-v", "entry="+entry, containerdImportMerge)
			cmd.Stdin = strings.NewReader(tc.input)
			out, err := cmd.Output()
			if tc.wantErr {
				if err == nil {
					t.Fatal("malformed configuration was rewritten instead of rejected")
				}
				return
			}
			if err != nil {
				t.Fatalf("merge failed: %v", err)
			}
			if string(out) != tc.want {
				t.Fatalf("unexpected merge result:\ngot:\n%s\nwant:\n%s", out, tc.want)
			}
		})
	}
}

// runRuntimeConfig executes the rendered bootstrap script against a fixture
// directory with stubbed service/runtime binaries, so the real script text is
// verified rather than a re-implementation of it.
func runRuntimeConfig(t *testing.T, seed string, times int) (string, error) {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"containerd", "systemctl"} {
		stub := "#!/bin/sh\nexit 0\n"
		if err := os.WriteFile(filepath.Join(bin, name), []byte(stub), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	containerdDir := filepath.Join(dir, "containerd")
	if err := os.MkdirAll(containerdDir, 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(containerdDir, "config.toml")
	if seed != "" {
		if err := os.WriteFile(configPath, []byte(seed), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	script := RuntimeConfigScript(ProvisionSpec{NetworkPlugin: "cilium", SandboxImage: "registry.example/pause:3.10"})
	cmd := exec.Command("sh", "-s")
	cmd.Stdin = strings.NewReader(script)
	cmd.Env = append(os.Environ(), "HNB_CONTAINERD_DIR="+containerdDir, "PATH="+bin+":"+os.Getenv("PATH"))
	var runErr error
	for i := 0; i < times; i++ {
		if err := cmd.Run(); err != nil {
			runErr = err
			break
		}
		cmd = exec.Command("sh", "-s")
		cmd.Stdin = strings.NewReader(script)
		cmd.Env = append(os.Environ(), "HNB_CONTAINERD_DIR="+containerdDir, "PATH="+bin+":"+os.Getenv("PATH"))
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	return string(data), runErr
}

func TestRuntimeConfigScriptMergesRealAdministratorConfig(t *testing.T) {
	seed := "version = 2\n[plugins.\"io.containerd.grpc.v1.cri\"]\n  sandbox_image = \"registry.administrator/pause:3.10\"\n[debug]\n  level = \"info\"\n"
	// The same fixture directory is used for both runs, so this covers the merge
	// and the idempotency of a repeated bootstrap stage.
	got, err := runRuntimeConfig(t, seed, 2)
	if err != nil {
		t.Fatalf("stage failed: %v", err)
	}
	if strings.Count(got, "hnb-bootstrap.toml") != 1 {
		t.Fatalf("import must be registered exactly once:\n%s", got)
	}
	if !strings.Contains(got, "registry.administrator/pause:3.10") || !strings.Contains(got, "[debug]") {
		t.Fatalf("administrator configuration was lost:\n%s", got)
	}
	if !strings.Contains(got, "imports = [") {
		t.Fatalf("no imports key produced:\n%s", got)
	}
}

func TestRuntimeConfigScriptCreatesAndSeedsConfig(t *testing.T) {
	got, err := runRuntimeConfig(t, "", 2)
	if err != nil {
		t.Fatalf("stage failed: %v", err)
	}
	if strings.Count(got, "hnb-bootstrap.toml") != 1 || !strings.Contains(got, "imports = [") {
		t.Fatalf("unexpected bootstrap config:\n%s", got)
	}
}

func TestRuntimeConfigScriptRejectsMalformedConfig(t *testing.T) {
	// An unterminated imports array must fail instead of rewriting it.
	got, err := runRuntimeConfig(t, "imports = [\n  \"/etc/a.toml\"\n", 1)
	if err == nil {
		t.Fatalf("malformed administrator config was rewritten:\n%s", got)
	}
	if !strings.Contains(got, "/etc/a.toml") {
		t.Fatal("malformed config was modified before failing")
	}
}

// Bootstrap must not require an interpreter that the OS image may not ship.
func TestRuntimeConfigScriptHasNoInterpreterDependency(t *testing.T) {
	script := RuntimeConfigScript(ProvisionSpec{NetworkPlugin: "cilium", SandboxImage: "registry.example/pause:3.10"})
	for _, forbidden := range []string{"python3", "python", "perl", "ruby", "yq"} {
		if strings.Contains(script, forbidden) {
			t.Fatalf("bootstrap still depends on %s", forbidden)
		}
	}
	for _, required := range []string{"command -v awk", "imports = [", "SystemdCgroup", "sandbox_image"} {
		if !strings.Contains(script, required) {
			t.Fatalf("runtime config script lost %q", required)
		}
	}
	if strings.Contains(script, "quay.io") || strings.Contains(script, "registry.k8s.io") {
		t.Fatal("runtime config script references an implicit upstream registry")
	}
}
