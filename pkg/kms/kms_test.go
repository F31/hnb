package kms

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestMasterKeyFromHex(t *testing.T) {
	if _, err := MasterKeyFromHex(""); err == nil {
		t.Fatal("expected error for empty key")
	}
	if _, err := MasterKeyFromHex(strings.Repeat("ab", 32)); err != nil {
		t.Fatalf("valid key rejected: %v", err)
	}
	if _, err := MasterKeyFromHex(strings.Repeat("ab", 31)); err == nil {
		t.Fatal("expected error for short key")
	}
}

func TestAESGCMRoundtrip(t *testing.T) {
	cipher, err := NewAESGCMFromHex(strings.Repeat("ab", 32))
	if err != nil {
		t.Fatalf("NewAESGCM: %v", err)
	}
	secret := []byte("a kubeconfig value")
	sealed, err := cipher.Encrypt(secret)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if sealed == string(secret) {
		t.Fatal("encrypted output leaked plaintext")
	}
	got, err := cipher.Decrypt(sealed)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if string(got) != string(secret) {
		t.Fatalf("roundtrip mismatch: %q", got)
	}

	if _, err := base64.StdEncoding.DecodeString(sealed); err != nil {
		t.Fatalf("encoded output is not base64: %v", err)
	}
}

func TestAESGCMWrongKey(t *testing.T) {
	cipher, err := NewAESGCMFromHex(strings.Repeat("ab", 32))
	if err != nil {
		t.Fatalf("NewAESGCM: %v", err)
	}
	sealed, err := cipher.Encrypt([]byte("value"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	other, err := NewAESGCMFromHex(strings.Repeat("cd", 32))
	if err != nil {
		t.Fatalf("NewAESGCM other: %v", err)
	}
	if _, err := other.Decrypt(sealed); err == nil {
		t.Fatal("expected decrypt failure with wrong key")
	}
}

func TestAESGCMTamper(t *testing.T) {
	cipher, err := NewAESGCMFromHex(strings.Repeat("ab", 32))
	if err != nil {
		t.Fatalf("NewAESGCM: %v", err)
	}
	sealed, err := cipher.Encrypt([]byte("value"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	raw, _ := base64.StdEncoding.DecodeString(sealed)
	raw[len(raw)-1] ^= 0x01
	tampered := base64.StdEncoding.EncodeToString(raw)
	if _, err := cipher.Decrypt(tampered); err == nil {
		t.Fatal("expected decrypt failure on tampered ciphertext")
	}
}
