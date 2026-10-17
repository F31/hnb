package messaging

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
)

type NATSConfig struct {
	URL           string
	Credentials   string
	NKeySeed      string
	TLSCert       string
	TLSKey        string
	TLSCA         string
	AllowInsecure bool
	// User and Password authenticate a server account. The bundled NATS server
	// declares one username/password user per plane account, so a client
	// certificate alone cannot satisfy it. Values come from files so a secret
	// never has to appear in the process environment or on the command line.
	UserFile     string
	PasswordFile string
}

func NATSConfigFromEnv(url string) (NATSConfig, error) {
	insecure := false
	if value := os.Getenv("NATS_INSECURE"); value != "" {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return NATSConfig{}, errors.New("NATS_INSECURE must be true or false")
		}
		insecure = parsed
	}
	config := NATSConfig{
		URL: url, Credentials: os.Getenv("NATS_CREDS_FILE"), NKeySeed: os.Getenv("NATS_NKEY_SEED_FILE"),
		TLSCert: os.Getenv("NATS_TLS_CERT_FILE"), TLSKey: os.Getenv("NATS_TLS_KEY_FILE"),
		TLSCA: os.Getenv("NATS_TLS_CA_FILE"), AllowInsecure: insecure,
		UserFile: os.Getenv("NATS_USER_FILE"), PasswordFile: os.Getenv("NATS_PASSWORD_FILE"),
	}
	if err := config.Validate(); err != nil {
		return NATSConfig{}, err
	}
	return config, nil
}

func (c NATSConfig) Validate() error {
	if c.URL == "" {
		return errors.New("NATS URL is required")
	}
	if c.Credentials != "" && c.NKeySeed != "" {
		return errors.New("configure only one of NATS credentials or NKey seed")
	}
	if (c.UserFile == "") != (c.PasswordFile == "") {
		return errors.New("NATS user and password files must be configured together")
	}
	if (c.TLSCert == "") != (c.TLSKey == "") {
		return errors.New("NATS TLS certificate and key must be configured together")
	}
	if c.Credentials == "" && c.NKeySeed == "" && c.TLSCert == "" && c.UserFile == "" && !c.AllowInsecure {
		return errors.New("NATS credentials, NKey seed, mTLS client certificate, or account user/password are required; NATS_INSECURE defaults to false")
	}
	return nil
}

// readSecretLine loads one secret value. A file readable only by its owner is
// required: anything readable by others is refused outright. A group-readable
// file is accepted because a container mounts a Secret owned by root with the
// group set to the runtime UID, so group access is exactly the non-root owner
// case rather than a world-readable secret.
func readSecretLine(path, kind string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("read NATS %s file: %w", kind, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("NATS %s file is not a regular file", kind)
	}
	if info.Mode().Perm()&0o007 != 0 {
		return "", fmt.Errorf("NATS %s file must not be readable by others", kind)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read NATS %s file: %w", kind, err)
	}
	value := strings.TrimSpace(string(data))
	if value == "" {
		return "", fmt.Errorf("NATS %s file is empty", kind)
	}
	return value, nil
}

func ConnectNATS(config NATSConfig, options ...nats.Option) (*nats.Conn, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if config.Credentials != "" {
		options = append(options, nats.UserCredentials(config.Credentials))
	}
	if config.UserFile != "" {
		user, err := readSecretLine(config.UserFile, "user")
		if err != nil {
			return nil, err
		}
		password, err := readSecretLine(config.PasswordFile, "password")
		if err != nil {
			return nil, err
		}
		options = append(options, nats.UserInfo(user, password))
	}
	if config.NKeySeed != "" {
		option, err := nats.NkeyOptionFromSeed(config.NKeySeed)
		if err != nil {
			return nil, fmt.Errorf("load NATS NKey seed: %w", err)
		}
		options = append(options, option)
	}
	if config.TLSCert != "" {
		options = append(options, nats.ClientCert(config.TLSCert, config.TLSKey))
	}
	if config.TLSCA != "" {
		options = append(options, nats.RootCAs(config.TLSCA))
	}
	return nats.Connect(config.URL, options...)
}

func ConnectNATSFromEnv(url string, options ...nats.Option) (*nats.Conn, error) {
	config, err := NATSConfigFromEnv(url)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	return ConnectNATSWithRetry(ctx, config, options...)
}

// ConnectNATSWithRetry waits for a server at startup, but returns a connected
// client, never a pending connection. Credential/config errors are not retried.
func ConnectNATSWithRetry(ctx context.Context, config NATSConfig, options ...nats.Option) (*nats.Conn, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	delay := 100 * time.Millisecond
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		timeout := 3 * time.Second
		if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < timeout {
			timeout = time.Until(deadline)
		}
		attemptOptions := append(append([]nats.Option{}, options...), nats.Timeout(timeout))
		conn, err := ConnectNATS(config, attemptOptions...)
		if err == nil {
			return conn, nil
		}
		if !errors.Is(err, nats.ErrNoServers) && !errors.Is(err, nats.ErrTimeout) {
			return nil, err
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
		if delay < 2*time.Second {
			delay *= 2
			if delay > 2*time.Second {
				delay = 2 * time.Second
			}
		}
	}
}
