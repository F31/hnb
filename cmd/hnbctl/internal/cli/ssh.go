package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/F31/hnb/cmd/hnbctl/internal/toolkit"
	"golang.org/x/term"
)

type sshAuthFlags struct {
	ask, askSudo           bool
	passwordFile, sudoFile string
}

func (a *sshAuthFlags) register(fs *flag.FlagSet) {
	fs.BoolVar(&a.ask, "ask-pass", false, "prompt for SSH password without echo")
	fs.StringVar(&a.passwordFile, "ssh-password-file", "", "private file containing SSH password")
	fs.BoolVar(&a.askSudo, "ask-sudo-pass", false, "prompt separately for sudo password")
	fs.StringVar(&a.sudoFile, "sudo-password-file", "", "private file containing sudo password")
}

func readSSHSecret(env *Env, ask bool, path, label string) (string, error) {
	if ask && path != "" {
		return "", fmt.Errorf("choose prompt or password file for %s", label)
	}
	var data []byte
	if ask {
		input, ok := env.Stdin.(*os.File)
		if !ok || !term.IsTerminal(int(input.Fd())) {
			return "", fmt.Errorf("%s prompt requires a terminal; use a private password file for automation", label)
		}
		fmt.Fprintf(env.Stderr, "%s: ", label)
		var err error
		data, err = term.ReadPassword(int(input.Fd()))
		fmt.Fprintln(env.Stderr)
		if err != nil {
			return "", fmt.Errorf("cannot read %s", label)
		}
	} else if path != "" {
		f, err := os.Open(path)
		if err != nil {
			return "", fmt.Errorf("cannot open %s file", label)
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
			return "", fmt.Errorf("%s file must be private (0600)", label)
		}
		data, err = io.ReadAll(io.LimitReader(f, 4098))
		if err != nil {
			return "", fmt.Errorf("cannot read %s file", label)
		}
		data = []byte(strings.TrimSuffix(strings.TrimSuffix(string(data), "\n"), "\r"))
	} else {
		return "", nil
	}
	if len(data) == 0 || len(data) > 4096 || strings.ContainsAny(string(data), "\r\n\x00") {
		return "", fmt.Errorf("%s must be a nonempty single line (maximum 4096 bytes)", label)
	}
	return string(data), nil
}

func (a sshAuthFlags) runner(env *Env, cfg *toolkit.DeploymentConfig) (toolkit.SSHNodeProbe, error) {
	var p toolkit.SSHNodeProbe
	var err error
	if len(cfg.Spec.Cluster.Nodes) == 0 && (a.ask || a.askSudo || a.passwordFile != "" || a.sudoFile != "") {
		return p, fmt.Errorf("SSH authentication options require declared host nodes; existing clusters use kubeconfig")
	}
	p.Password, err = readSSHSecret(env, a.ask, a.passwordFile, "SSH password")
	if err != nil {
		return p, err
	}
	if p.Password != "" {
		if _, err := exec.LookPath("sshpass"); err != nil {
			return p, fmt.Errorf("password authentication requires sshpass on the operator host")
		}
	}
	p.SudoPassword, err = readSSHSecret(env, a.askSudo, a.sudoFile, "sudo password")
	if err != nil {
		return p, err
	}
	for i := range cfg.Spec.Cluster.Nodes {
		n := &cfg.Spec.Cluster.Nodes[i]
		if p.Password != "" {
			n.SSHAuth = "password"
		}
		if n.SSHAuth == "password" && p.Password == "" {
			return p, fmt.Errorf("password nodes require --ask-pass or --ssh-password-file")
		}
	}
	return p, nil
}

func runSSH(ctx context.Context, env *Env, args []string) Result {
	if len(args) == 0 || args[0] != "enroll" {
		return fail(ReasonInvalidConfig, "use ssh enroll -f hnb.yaml --node NAME --key-file PATH --ask-pass")
	}
	fs := flag.NewFlagSet("ssh enroll", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var auth sshAuthFlags
	auth.register(fs)
	var configPath, name, keyPath string
	var yes bool
	fs.StringVar(&configPath, "f", "", "DeploymentConfig")
	fs.StringVar(&name, "node", "", "single node to enroll")
	fs.StringVar(&keyPath, "key-file", "", "local dedicated private key path (existing keys are reused)")
	fs.BoolVar(&yes, "yes", false, "confirm appending the public key to the selected account")
	if err := fs.Parse(args[1:]); err != nil || fs.NArg() != 0 || configPath == "" || name == "" || keyPath == "" {
		return fail(ReasonInvalidConfig, "ssh enroll requires -f, --node and --key-file")
	}
	cfg, err := readConfig(configPath)
	if err != nil {
		return fail(ReasonInvalidConfig, "cannot read DeploymentConfig")
	}
	// Enrollment selects exactly one declared host, not the entire cluster.
	var node *toolkit.NodeConfig
	for i := range cfg.Spec.Cluster.Nodes {
		if cfg.Spec.Cluster.Nodes[i].Name == name {
			if node != nil {
				return fail(ReasonInvalidConfig, "duplicate node name")
			}
			node = &cfg.Spec.Cluster.Nodes[i]
		}
	}
	if node == nil {
		return fail(ReasonInvalidConfig, "selected node does not exist")
	}
	selected := *node
	selected.SSHAuth = "password"
	cfg.Spec.Cluster.Nodes = []toolkit.NodeConfig{selected}
	p, err := auth.runner(env, cfg)
	if err != nil {
		return fail(ReasonInvalidConfig, err.Error())
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if _, err := p.UserRun(ctx, selected, "true", nil); err != nil {
		return fail(ReasonPreflightBlocked, err.Error())
	}
	fmt.Fprintf(env.Stderr, "Append dedicated public key to %s@%s ~/.ssh/authorized_keys; local key=%q.\n", selected.User, selected.SSHAddress, keyPath)
	if !yes {
		if env.Stdin == nil {
			return fail(ReasonInvalidConfig, "confirmation required; use --yes for automation")
		}
		fmt.Fprint(env.Stderr, "Type yes to continue: ")
		line, err := bufio.NewReader(env.Stdin).ReadString('\n')
		if err != nil || strings.TrimSpace(line) != "yes" {
			return fail(ReasonPreflightBlocked, "key enrollment canceled")
		}
	}
	keyPath, err = filepath.Abs(keyPath)
	if err != nil {
		return fail(ReasonInvalidConfig, "invalid key path")
	}
	if err := ensureEnrollmentKey(ctx, keyPath); err != nil {
		return fail(ReasonOutputFailed, err.Error())
	}
	pub, err := exec.CommandContext(ctx, "ssh-keygen", "-y", "-P", "", "-f", keyPath).Output()
	if err != nil {
		return fail(ReasonInvalidConfig, "cannot read dedicated public key (unencrypted key required)")
	}
	if !strings.HasPrefix(string(pub), "ssh-ed25519 ") {
		return fail(ReasonInvalidConfig, "dedicated enrollment key must be Ed25519")
	}
	if _, err := p.UserRun(ctx, selected, enrollPublicKeyScript, strings.NewReader(strings.TrimSpace(string(pub))+"\n")); err != nil {
		return fail(ReasonOutputFailed, err.Error())
	}
	selected.SSHAuth, selected.SSHKeyFile = "key", keyPath
	if _, err := (toolkit.SSHNodeProbe{}).UserRun(ctx, selected, "true", nil); err != nil {
		return fail(ReasonPreflightBlocked, "public key appended but key authentication failed; password login remains unchanged")
	}
	receipt, err := json.MarshalIndent(struct {
		SchemaVersion string             `json:"schemaVersion"`
		Node          toolkit.NodeConfig `json:"node"`
		VerifiedAt    time.Time          `json:"verifiedAt"`
	}{"ssh-enrollment/v1", selected, time.Now().UTC()}, "", "  ")
	if err != nil {
		return fail(ReasonOutputFailed, "cannot encode verified key receipt")
	}
	// The receipt is only published after successful key authentication.
	f, err := os.CreateTemp(filepath.Dir(keyPath), ".enrollment-*.tmp")
	if err != nil {
		return fail(ReasonOutputFailed, "cannot save verified key receipt")
	}
	defer os.Remove(f.Name())
	_, writeErr := f.Write(receipt)
	syncErr := f.Sync()
	closeErr := f.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil || os.Rename(f.Name(), keyPath+".enrollment.json") != nil {
		return fail(ReasonOutputFailed, "cannot save verified key receipt")
	}
	fmt.Fprintf(env.Stdout, "node=%s key authentication verified\nSet sshAuth: key and sshKeyFile: %q for this node in hnb.yaml.\n", selected.Name, keyPath)
	return success("")
}

func ensureEnrollmentKey(ctx context.Context, path string) error {
	if fi, err := os.Lstat(path); err == nil {
		if !fi.Mode().IsRegular() || fi.Mode().Perm()&0077 != 0 {
			return fmt.Errorf("existing key must be a private regular file (0600)")
		}
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("key path unavailable")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("cannot create key directory")
	}
	// Reserve the pathname exclusively before generation; ssh-keygen writes to a
	// private temporary directory so no existing user key is overwritten.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("cannot reserve key path")
	}
	defer f.Close()
	dir, err := os.MkdirTemp(filepath.Dir(path), ".hnb-enroll-")
	if err != nil {
		os.Remove(path)
		return fmt.Errorf("cannot create key workspace")
	}
	defer os.RemoveAll(dir)
	tmp := filepath.Join(dir, "key")
	if err := exec.CommandContext(ctx, "ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-C", "hnbctl-enroll", "-f", tmp).Run(); err != nil {
		os.Remove(path)
		return fmt.Errorf("dedicated key generation failed")
	}
	data, err := os.ReadFile(tmp)
	if err != nil {
		os.Remove(path)
		return fmt.Errorf("cannot read generated key")
	}
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("cannot save dedicated key")
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("cannot persist dedicated key")
	}
	return nil
}

const enrollPublicKeyScript = `set -eu
IFS= read -r key
case "$key" in 'ssh-ed25519 '*) ;; *) exit 1;; esac
umask 077
[ ! -L "$HOME/.ssh" ] || exit 1
mkdir -p "$HOME/.ssh"
chmod 700 "$HOME/.ssh"
file="$HOME/.ssh/authorized_keys"
[ ! -L "$file" ] || exit 1
# Serialize concurrent enrollments without replacing existing authorized keys.
[ ! -L "$HOME/.ssh/.hnb-enroll.lock" ] || exit 1
exec 9>"$HOME/.ssh/.hnb-enroll.lock"
flock -x 9
touch "$file"
chmod 600 "$file"
grep -qxF -- "$key" "$file" || printf '\n%s\n' "$key" >> "$file"
`
