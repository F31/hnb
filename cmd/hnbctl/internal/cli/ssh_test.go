package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSSHPasswordFileAndPromptValidation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "password")
	if err := os.WriteFile(path, []byte(" secret with spaces \n"), 0600); err != nil {
		t.Fatal(err)
	}
	env := &Env{Stdin: strings.NewReader("secret"), Stderr: &bytes.Buffer{}}
	secret, err := readSSHSecret(env, false, path, "SSH password")
	if err != nil || secret != " secret with spaces " {
		t.Fatalf("password changed: %q %v", secret, err)
	}
	if _, err := readSSHSecret(env, true, "", "SSH password"); err == nil {
		t.Fatal("nonterminal prompt accepted")
	}
	if _, err := readSSHSecret(env, true, path, "SSH password"); err == nil {
		t.Fatal("ambiguous credential sources accepted")
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := readSSHSecret(env, false, path, "SSH password"); err == nil {
		t.Fatal("public password file accepted")
	}
}

// Local transport fixture executes the real authorized_keys script against an
// isolated HOME, without contacting or modifying any external machine.
func TestSSHEnrollConfirmationIdempotencyAndKeyVerification(t *testing.T) {
	dir := t.TempDir()
	config := writeBootstrapDraft(t, dir)
	data, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.Replace(string(data), `sshAddress: ""`, "sshAddress: 192.0.2.10", 1))
	if err := os.WriteFile(config, data, 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := readConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	name := cfg.Spec.Cluster.Nodes[0].Name
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	for name, script := range map[string]string{
		"sshpass": "#!/bin/sh\nIFS= read -r password <&3\n[ \"$password\" = login-secret ] || exit 1\nshift 2\nexec \"$@\"\n",
		"ssh":     "#!/bin/sh\nkey=no\nfor last do [ \"$last\" != -i ] || key=yes; done\n[ \"${SSH_TEST_FAIL_KEY:-}:$key\" != yes:yes ] || exit 1\nexec /bin/sh -c \"$last\"\n",
	} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	home := filepath.Join(dir, "remote-home")
	if err := os.Mkdir(home, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(home, ".ssh"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".ssh", "authorized_keys"), []byte("ssh-rsa existing-other-key comment\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	password := filepath.Join(dir, "password")
	if err := os.WriteFile(password, []byte("login-secret\n"), 0600); err != nil {
		t.Fatal(err)
	}
	key := filepath.Join(dir, "dedicated-key")
	args := []string{"ssh", "enroll", "-f", config, "--node", name, "--key-file", key, "--ssh-password-file", password}
	var out, stderr bytes.Buffer
	code := ExecuteWithInput(context.Background(), args, strings.NewReader("no\n"), &out, &stderr)
	if code == ExitSuccess {
		t.Fatal("enrolled without confirmation")
	}
	if _, err := os.Stat(key); !os.IsNotExist(err) {
		t.Fatal("key created before confirmation")
	}
	for i := 0; i < 2; i++ {
		code, out, errOut := runCLI(t, append(args, "--yes")...)
		if code != ExitSuccess {
			t.Fatalf("enroll failed: %d %s %s", code, out, errOut)
		}
		if strings.Contains(out+errOut, "login-secret") {
			t.Fatal("password leaked")
		}
	}
	authorized, err := os.ReadFile(filepath.Join(home, ".ssh", "authorized_keys"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(authorized), "ssh-ed25519 ") != 1 {
		t.Fatal("duplicate public key appended")
	}
	if !strings.Contains(string(authorized), "ssh-rsa existing-other-key comment") {
		t.Fatal("existing authorized key removed")
	}
	for _, file := range []string{key, key + ".enrollment.json", filepath.Join(home, ".ssh", "authorized_keys")} {
		fi, err := os.Stat(file)
		if err != nil || fi.Mode().Perm() != 0600 {
			t.Fatalf("private file permissions: %s %v", file, err)
		}
	}
	// Failed key login must not publish a success receipt.
	t.Setenv("SSH_TEST_FAIL_KEY", "yes")
	failedKey := filepath.Join(dir, "failed-key")
	failedArgs := append([]string(nil), args...)
	for i := range failedArgs {
		if failedArgs[i] == key {
			failedArgs[i] = failedKey
		}
	}
	code, _, errOut := runCLI(t, append(failedArgs, "--yes")...)
	if code == ExitSuccess || !strings.Contains(errOut, "key authentication failed") {
		t.Fatalf("failed verification reported success: %d %s", code, errOut)
	}
	if _, err := os.Stat(failedKey + ".enrollment.json"); !os.IsNotExist(err) {
		t.Fatal("success receipt published before key verification")
	}
}
