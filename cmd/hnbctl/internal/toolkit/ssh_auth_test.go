package toolkit

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Exercise the actual process boundary: credentials use FD 3, while stdin
// remains available for scripts/data and the separate sudo credential.
func TestPasswordTransportKeepsCredentialsOutOfArgvAndPreservesStreams(t *testing.T) {
	dir := t.TempDir()
	trace := filepath.Join(dir, "argv")
	t.Setenv("SSH_TEST_TRACE", trace)
	t.Setenv("SSH_TEST_PASSWORD", "login-secret")
	t.Setenv("SSH_TEST_SUDO", "sudo-secret")
	scripts := map[string]string{
		"sshpass": `#!/bin/sh
[ "$1" = -d ] && [ "$2" = 3 ] || exit 1
IFS= read -r password <&3
[ "$password" = "$SSH_TEST_PASSWORD" ] || exit 1
shift 2
exec "$@"
`,
		"ssh": `#!/bin/sh
printf '%s\n' "$@" >> "$SSH_TEST_TRACE"
for last do :; done
exec /bin/sh -c "$last"
`,
		"id": "#!/bin/sh\nprintf '1000\\n'\n",
		"sudo": `#!/bin/sh
if [ "$1" = -k ]; then
  IFS= read -r password
  [ "$password" = "$SSH_TEST_SUDO" ] || exit 1
  exit 0
fi
while [ "$1" != -- ]; do shift; done
shift
exec "$@"
`,
	}
	for name, script := range scripts {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	p := SSHNodeProbe{Password: "login-secret", SudoPassword: "sudo-secret"}
	n := NodeConfig{User: "ubuntu", SSHAddress: "192.0.2.10", SSHAuth: "password"}
	want := "payload\nwith spaces\n"
	out, err := p.Stream(context.Background(), n, "cat", strings.NewReader(want))
	if err != nil || out != want {
		t.Fatalf("stream changed: %q %v", out, err)
	}
	out, err = p.Run(context.Background(), n, "printf 'probe-ok'")
	if err != nil || out != "probe-ok" {
		t.Fatalf("probe failed: %q %v", out, err)
	}
	args, err := os.ReadFile(trace)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{p.Password, p.SudoPassword} {
		if strings.Contains(string(args), secret) {
			t.Fatal("credential leaked to argv")
		}
	}
	if !strings.Contains(string(args), "StrictHostKeyChecking=yes") {
		t.Fatal("host verification disabled")
	}
}

func TestPasswordTransportRefusesMissingCredentials(t *testing.T) {
	n := NodeConfig{User: "ubuntu", SSHAddress: "192.0.2.10", SSHAuth: "password"}
	if _, err := (SSHNodeProbe{}).Run(context.Background(), n, "true"); err == nil {
		t.Fatal("missing password accepted")
	}
	if _, err := (SSHNodeProbe{Password: "secret\nextra"}).Run(context.Background(), n, "true"); err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatal("invalid password accepted or leaked")
	}
}

func TestPasswordNodeConfigurationDoesNotRequireAKeyFile(t *testing.T) {
	cfg, err := DecodeConfig([]byte(validConfig))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Spec.Cluster.Nodes[0].SSHAuth = "password"
	cfg.Spec.Cluster.Nodes[0].SSHKeyFile = ""
	if problems := cfg.Validate(CurrentEvidence); len(problems) != 0 {
		t.Fatalf("password configuration rejected: %v", problems)
	}
	if problems := cfg.ValidateCredentialFiles(); len(problems) != 0 {
		t.Fatalf("password node requires local key: %v", problems)
	}
	cfg.Spec.Cluster.Nodes[0].SSHAuth = "invalid"
	if problems := cfg.Validate(CurrentEvidence); !strings.Contains(strings.Join(problems, ","), "sshAuth") {
		t.Fatal("unknown authentication accepted")
	}
}
