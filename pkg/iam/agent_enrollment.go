package iam

import (
	"errors"
	"strings"
)

// NewEnrollmentSecret returns a high-entropy opaque bootstrap secret and the
// SHA-256 hash that is persisted. The plaintext is returned to the caller once
// and must never be stored, logged, or audited.
func NewEnrollmentSecret() (string, string, error) {
	value, err := randomOpaqueToken()
	if err != nil {
		return "", "", err
	}
	return value, hashRefreshToken(value), nil
}

// HashEnrollmentSecret hashes a presented bootstrap secret for the atomic
// single-use lookup. It is not a password KDF; the secret has 256 bits of
// randomness and is short-lived.
func HashEnrollmentSecret(value string) string {
	return hashRefreshToken(value)
}

// ReadEnrollmentSecretFile reads the single-use enrollment secret from a
// regular file that must be inaccessible to group/other users. It is consumed
// once and never written to disk by the Agent.
func ReadEnrollmentSecretFile(path string) (string, error) {
	data, err := readBoundedRegularFile(path, 1024, true)
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(string(data))
	if len(value) < 43 || len(value) > 512 || strings.ContainsAny(value, " \t\r\n") {
		return "", errors.New("enrollment secret file is invalid")
	}
	return value, nil
}
