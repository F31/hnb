package kms

import (
	"errors"
	"fmt"
	"os"
)

// MasterKeyFromHex loads a 32-byte AES-256 master key encoded as 64 hex
// characters (same format as HNB_MASTER_KEY used elsewhere in the platform).
func MasterKeyFromHex(keyHex string) ([]byte, error) {
	if keyHex == "" {
		return nil, errors.New("kms: master key not set")
	}
	key, err := decodeHex(keyHex)
	if err != nil {
		return nil, fmt.Errorf("kms: invalid master key: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("kms: master key must be 32 bytes (64 hex chars), got %d", len(key))
	}
	return key, nil
}

// MasterKeyFromEnv loads the AES-256 master key from the HNB_MASTER_KEY
// environment variable.
func MasterKeyFromEnv() ([]byte, error) {
	return MasterKeyFromHex(os.Getenv("HNB_MASTER_KEY"))
}

// Decrypter opens AES-GCM sealed payloads (base64 nonce||ciphertext) sealed by
// the platform's master-key cipher. It is the narrow surface service workers
// need to read stored kubeconfig secrets.
type Decrypter interface {
	Decrypt(sealed string) ([]byte, error)
}

// Decrypt opens a sealed payload through the given decrypter.
func Decrypt(d Decrypter, sealed string) ([]byte, error) {
	return d.Decrypt(sealed)
}
