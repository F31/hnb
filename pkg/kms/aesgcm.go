package kms

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

const nonceSize = 12

func decodeHex(s string) ([]byte, error) {
	return hex.DecodeString(s)
}

type aesGCM struct {
	gcm cipher.AEAD
}

// NewAESGCM builds an AES-256-GCM cipher from a raw 32-byte master key.
// The returned value offers Encrypt/Decrypt with a random 12-byte nonce
// prepended to the ciphertext (compatible with the AES-GCM sealed format used
// elsewhere in the platform).
func NewAESGCM(masterKey []byte) (*aesGCM, error) {
	if len(masterKey) != 32 {
		return nil, fmt.Errorf("kms: aes key must be 32 bytes, got %d", len(masterKey))
	}
	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return nil, fmt.Errorf("kms: aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("kms: gcm: %w", err)
	}
	return &aesGCM{gcm: gcm}, nil
}

// NewAESGCMFromHex builds the cipher from a 64-char hex-encoded master key.
func NewAESGCMFromHex(keyHex string) (*aesGCM, error) {
	key, err := MasterKeyFromHex(keyHex)
	if err != nil {
		return nil, err
	}
	return NewAESGCM(key)
}

// Encrypt seals plaintext and returns nonce||ciphertext encoded in base64.
func (c *aesGCM) Encrypt(plaintext []byte) (string, error) {
	nonce := make([]byte, nonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := c.gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt opens a base64 nonce||ciphertext sealed by Encrypt.
func (c *aesGCM) Decrypt(sealed string) ([]byte, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(sealed)
	if err != nil {
		return nil, fmt.Errorf("kms: base64 decode: %w", err)
	}
	if len(ciphertext) < nonceSize {
		return nil, errors.New("kms: ciphertext too short")
	}
	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := c.gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("kms: decrypt: %w", err)
	}
	return plaintext, nil
}
