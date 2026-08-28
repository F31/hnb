package messaging

import (
	"errors"
	"fmt"
	"os"
	"strconv"

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
	if (c.TLSCert == "") != (c.TLSKey == "") {
		return errors.New("NATS TLS certificate and key must be configured together")
	}
	if c.Credentials == "" && c.NKeySeed == "" && c.TLSCert == "" && !c.AllowInsecure {
		return errors.New("NATS credentials, NKey seed, or mTLS client certificate are required; NATS_INSECURE defaults to false")
	}
	return nil
}

func ConnectNATS(config NATSConfig, options ...nats.Option) (*nats.Conn, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if config.Credentials != "" {
		options = append(options, nats.UserCredentials(config.Credentials))
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
	return ConnectNATS(config, options...)
}
