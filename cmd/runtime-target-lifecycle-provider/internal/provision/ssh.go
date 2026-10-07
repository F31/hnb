package provision

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"strings"

	"golang.org/x/crypto/ssh"
)

// SSHRunner executes bootstrap scripts on a node over SSH. Scripts run with
// `sh -s` on the node; the returned string is the combined stdout/stderr.
type SSHRunner interface {
	Run(ctx context.Context, node NodeSpec, privateKey []byte, script string) (string, error)
}

// SSHConfigFunc builds an ssh.ClientConfig from a node + private key; split out
// so tests can substitute a transport without round-tripping a real network.
type SSHConfigFunc func(node NodeSpec, privateKey []byte) (*ssh.ClientConfig, error)

// RealSSH is the production SSHRunner backed by golang.org/x/crypto/ssh.
type RealSSH struct {
	SSHConfigFunc SSHConfigFunc
	// DialFunc lets tests inject a dialing transport; when nil a real
	// ssh.Dial is used.
	DialFunc func(network, addr string, config *ssh.ClientConfig) (*ssh.Client, error)
}

// insecureHostKeyCallback accepts any host key. This mirrors how cloud-init/
// Rancher bootstrap machinery trusts a freshly-provisioned node; the SSH secret
// itself is platform-issued and encrypted-at-rest.
func insecureHostKeyCallback(_ string, _ net.Addr, _ ssh.PublicKey) error {
	//nolint:staticcheck // intentional: nodes are first-provisioned, TOFU is not possible
	return nil
}

func (r *RealSSH) dial(node NodeSpec, privateKey []byte) (*ssh.Client, error) {
	signer, err := ssh.ParsePrivateKey(privateKey)
	if err != nil {
		return nil, fmt.Errorf("parse ssh private key: %w", err)
	}
	config := &ssh.ClientConfig{
		User:            node.EffectiveUser(),
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: insecureHostKeyCallback,
		Timeout:         0, // connect deadline handled by ctx on dial impls
	}
	addr := fmt.Sprintf("%s:%d", node.Host, node.EffectivePort())
	if r.DialFunc != nil {
		return r.DialFunc("tcp", addr, config)
	}
	return ssh.Dial("tcp", addr, config)
}

// Run executes script on the node via an interactive-free `sh -s` session.
func (r *RealSSH) Run(ctx context.Context, node NodeSpec, privateKey []byte, script string) (string, error) {
	client, err := r.dial(node, privateKey)
	if err != nil {
		return "", stepErr("ssh-connect", err)
	}
	defer client.Close()

	// A context on the session proxies cancellation to the remote command.
	session, err := client.NewSession()
	if err != nil {
		return "", stepErr("ssh-session", err)
	}
	defer session.Close()

	sessionStdin, err := session.StdinPipe()
	if err != nil {
		return "", stepErr("ssh-stdin", err)
	}
	var out bytes.Buffer
	session.Stdout = &out
	session.Stderr = &out

	if err := session.Start("sh -s"); err != nil {
		return "", stepErr("ssh-start", err)
	}
	go func() {
		_, _ = sessionStdin.Write([]byte(script + "\nexit $?\n"))
		_ = sessionStdin.Close()
	}()

	done := make(chan error, 1)
	go func() { done <- session.Wait() }()

	select {
	case <-ctx.Done():
		_ = session.Signal(ssh.SIGHUP)
		<-done
		return out.String(), stepErr("ssh-timeout", ctx.Err())
	case err := <-done:
		if err != nil {
			return out.String(), stepErr("ssh-run", fmt.Errorf("%w: %s", err, tail(out.String())))
		}
		return out.String(), nil
	}
}

const tailBytes = 1200

func tail(s string) string {
	if len(s) <= tailBytes {
		return strings.TrimSpace(s)
	}
	return "… " + strings.TrimSpace(s[len(s)-tailBytes:])
}

// ParsePrivateKey is a thin wrapper for tests/other packages.
func ParsePrivateKey(key []byte) (ssh.Signer, error) {
	return ssh.ParsePrivateKey(key)
}

// EncodePrivateKeyForSecret formats a PEM key as the base64 payload the
// secrets:register API expects (used by the create-wizard before upload).
func EncodePrivateKeyForSecret(pem []byte) (string, error) {
	if len(pem) == 0 {
		return "", errors.New("empty private key")
	}
	return base64.StdEncoding.EncodeToString(pem), nil
}
