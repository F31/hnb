package config

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
)

type SecretReference struct {
	ID             string `json:"id"`
	TenantID       string `json:"tenant_id"`
	Name           string `json:"name"`
	SecretRef      string `json:"secret_ref"`
	Version        int    `json:"version"`
	Algorithm      string `json:"algorithm"`
	ProviderID     string `json:"provider_id"`
	EncryptedValue string `json:"-"`
}

type KMSProvider interface {
	Name() string
	Type() string
	Resolve(ctx context.Context, ref *SecretReference) ([]byte, error)
	Health(ctx context.Context) error
}

type LocalAESProvider struct {
	masterKey []byte
}

func NewLocalAESProvider() (*LocalAESProvider, error) {
	keyHex := os.Getenv("HNB_MASTER_KEY")
	if keyHex == "" {
		return nil, errors.New("HNB_MASTER_KEY not set")
	}
	key, err := hex.DecodeString(keyHex)
	if err != nil {
		return nil, fmt.Errorf("invalid HNB_MASTER_KEY: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("HNB_MASTER_KEY must be 32 bytes (64 hex chars), got %d", len(key))
	}
	return &LocalAESProvider{masterKey: key}, nil
}

func NewLocalAESProviderWithKey(key []byte) *LocalAESProvider {
	return &LocalAESProvider{masterKey: key}
}

func (p *LocalAESProvider) Name() string { return "local-aes" }
func (p *LocalAESProvider) Type() string { return "local_aes" }

func (p *LocalAESProvider) Resolve(ctx context.Context, ref *SecretReference) ([]byte, error) {
	if ref.EncryptedValue == "" {
		return nil, errors.New("empty encrypted value")
	}
	ciphertext, err := base64.StdEncoding.DecodeString(ref.EncryptedValue)
	if err != nil {
		return nil, fmt.Errorf("base64 decode: %w", err)
	}
	block, err := aes.NewCipher(p.masterKey)
	if err != nil {
		return nil, fmt.Errorf("aes: %w", err)
	}
	if len(ciphertext) < 12 {
		return nil, errors.New("ciphertext too short")
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("gcm: %w", err)
	}
	nonce, ciphertext := ciphertext[:12], ciphertext[12:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}
	return plaintext, nil
}

func (p *LocalAESProvider) Encrypt(plaintext []byte) (string, error) {
	block, err := aes.NewCipher(p.masterKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, 12)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func (p *LocalAESProvider) Health(ctx context.Context) error {
	if len(p.masterKey) != 32 {
		return errors.New("invalid key length")
	}
	return nil
}

type K8sSecretProvider struct {
	name string
}

func NewK8sSecretProvider(name string) *K8sSecretProvider {
	return &K8sSecretProvider{name: name}
}

func (p *K8sSecretProvider) Name() string { return p.name }
func (p *K8sSecretProvider) Type() string { return "k8s_secret" }

func (p *K8sSecretProvider) Resolve(ctx context.Context, ref *SecretReference) ([]byte, error) {
	return []byte(fmt.Sprintf("k8s-resolved-%s-%s", ref.Name, ref.SecretRef)), nil
}

func (p *K8sSecretProvider) Health(ctx context.Context) error { return nil }

type VaultProvider struct {
	name string
}

func NewVaultProvider(name string) *VaultProvider {
	return &VaultProvider{name: name}
}

func (p *VaultProvider) Name() string { return p.name }
func (p *VaultProvider) Type() string { return "vault" }

func (p *VaultProvider) Resolve(ctx context.Context, ref *SecretReference) ([]byte, error) {
	return []byte(fmt.Sprintf("vault-resolved-%s-v%d", ref.Name, ref.Version)), nil
}

func (p *VaultProvider) Health(ctx context.Context) error { return nil }

type Registry struct {
	providers map[string]KMSProvider
}

func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]KMSProvider),
	}
}

func (r *Registry) Register(p KMSProvider) {
	r.providers[p.Name()] = p
}

func (r *Registry) Get(name string) (KMSProvider, bool) {
	p, ok := r.providers[name]
	return p, ok
}

func (r *Registry) Health(ctx context.Context) map[string]error {
	results := make(map[string]error, len(r.providers))
	for name, p := range r.providers {
		results[name] = p.Health(ctx)
	}
	return results
}

type SecretResolver struct {
	registry *Registry
}

func NewSecretResolver(registry *Registry) *SecretResolver {
	return &SecretResolver{registry: registry}
}

func (sr *SecretResolver) Resolve(ctx context.Context, ref *SecretReference) ([]byte, error) {
	providerName := ref.ProviderID
	if providerName == "" {
		providerName = "local-aes"
	}
	provider, ok := sr.registry.Get(providerName)
	if !ok {
		return nil, fmt.Errorf("kms provider %q not registered", providerName)
	}
	return provider.Resolve(ctx, ref)
}
