package messaging

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
)

func TestNATSConfigFailsClosedWithoutCredentials(t *testing.T) {
	for _, key := range []string{"NATS_CREDS_FILE", "NATS_NKEY_SEED_FILE", "NATS_TLS_CERT_FILE", "NATS_TLS_KEY_FILE", "NATS_TLS_CA_FILE", "NATS_INSECURE"} {
		t.Setenv(key, "")
	}
	_, err := NATSConfigFromEnv("nats://localhost:4222")
	if err == nil || !strings.Contains(err.Error(), "credentials") {
		t.Fatalf("error = %v", err)
	}
}

func TestNATSStartupRetriesUntilServerStarts(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()
	started := make(chan net.Listener, 1)
	serverError := make(chan error, 1)
	// The first connection is refused; the next must complete a real handshake.
	go func() {
		time.Sleep(60 * time.Millisecond)
		listener, err := net.Listen("tcp", address)
		if err != nil {
			serverError <- err
			return
		}
		started <- listener
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				fmt.Fprint(conn, "INFO {\"server_id\":\"test\",\"version\":\"2.11.1\",\"proto\":1,\"max_payload\":1048576}\r\n")
				scanner := bufio.NewScanner(conn)
				for scanner.Scan() {
					if scanner.Text() == "PING" {
						fmt.Fprint(conn, "PONG\r\n")
					}
				}
			}()
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	conn, err := ConnectNATSWithRetry(ctx, NATSConfig{
		URL: "nats://" + address, AllowInsecure: true,
	}, nats.NoReconnect())
	select {
	case listener := <-started:
		defer listener.Close()
	case serverErr := <-serverError:
		t.Fatal(serverErr)
	case <-time.After(time.Second):
		t.Fatal("test server did not start")
	}
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if !conn.IsConnected() {
		t.Fatal("returned a pending connection")
	}
}

func TestNATSStartupHonorsDeadlineAndRejectsInvalidConfig(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err = ConnectNATSWithRetry(ctx, NATSConfig{URL: "nats://" + address, AllowInsecure: true})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err=%v", err)
	}
	_, err = ConnectNATSWithRetry(context.Background(), NATSConfig{URL: "nats://localhost:4222"})
	if err == nil || !strings.Contains(err.Error(), "credentials") {
		t.Fatalf("err=%v", err)
	}
}

func TestNATSConfigAllowsExplicitDevelopmentInsecureMode(t *testing.T) {
	t.Setenv("NATS_INSECURE", "true")
	config, err := NATSConfigFromEnv("nats://localhost:4222")
	if err != nil || !config.AllowInsecure {
		t.Fatalf("config = %#v, err = %v", config, err)
	}
}

func TestNATSConfigAcceptsIndependentCredentialPaths(t *testing.T) {
	for name, config := range map[string]NATSConfig{
		"creds":   {URL: "nats://nats:4222", Credentials: "/run/nats/client.creds"},
		"nkey":    {URL: "nats://nats:4222", NKeySeed: "/run/nats/client.nk"},
		"mtls":    {URL: "tls://nats:4222", TLSCert: "/run/nats/tls.crt", TLSKey: "/run/nats/tls.key", TLSCA: "/run/nats/ca.crt"},
		"account": {URL: "nats://nats:4222", UserFile: "/run/nats/user", PasswordFile: "/run/nats/password"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := config.Validate(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// The bundled NATS server declares one username/password user per plane account,
// so a client certificate alone cannot authenticate. Account credentials must be
// accepted as a first-class option and only from private files.
func TestNATSConfigRequiresAccountCredentialsTogether(t *testing.T) {
	for name, config := range map[string]NATSConfig{
		"user only":     {URL: "nats://nats:4222", UserFile: "/run/nats/user"},
		"password only": {URL: "nats://nats:4222", PasswordFile: "/run/nats/password"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := config.Validate(); err == nil {
				t.Fatal("a half-configured account credential was accepted")
			}
		})
	}
}

func TestNATSConfigFromEnvReadsAccountCredentialPaths(t *testing.T) {
	t.Setenv("NATS_USER_FILE", "/run/nats/user")
	t.Setenv("NATS_PASSWORD_FILE", "/run/nats/password")
	config, err := NATSConfigFromEnv("nats://nats:4222")
	if err != nil {
		t.Fatal(err)
	}
	if config.UserFile != "/run/nats/user" || config.PasswordFile != "/run/nats/password" {
		t.Fatalf("account credentials not read from the environment: %#v", config)
	}
}

func TestReadSecretLineRejectsWorldReadableFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "password")
	if err := os.WriteFile(path, []byte("secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readSecretLine(path, "password"); err == nil {
		t.Fatal("a group/world-readable secret file was accepted")
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	value, err := readSecretLine(path, "password")
	if err != nil || value != "secret" {
		t.Fatalf("value = %q, err = %v", value, err)
	}
}

func TestReadSecretLineRejectsEmptyAndMissingFiles(t *testing.T) {
	dir := t.TempDir()
	empty := filepath.Join(dir, "user")
	if err := os.WriteFile(empty, []byte("  \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readSecretLine(empty, "user"); err == nil {
		t.Fatal("an empty secret file was accepted")
	}
	if _, err := readSecretLine(filepath.Join(dir, "absent"), "password"); err == nil {
		t.Fatal("a missing secret file was accepted")
	}
	if _, err := readSecretLine(dir, "password"); err == nil {
		t.Fatal("a directory was accepted as a secret file")
	}
}
