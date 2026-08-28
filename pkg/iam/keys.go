package iam

import (
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
)

type PEMKeySet struct {
	activeKeyID string
	privateKey  *ecdsa.PrivateKey
	publicKeys  map[string]*ecdsa.PublicKey
}

type PEMPublicKeySet struct {
	publicKeys map[string]*ecdsa.PublicKey
}

func LoadPEMPublicKeySet(verificationKeyPaths map[string]string) (*PEMPublicKeySet, error) {
	if len(verificationKeyPaths) == 0 {
		return nil, errors.New("verification key set is required")
	}
	publicKeys, err := loadPublicKeys(verificationKeyPaths)
	if err != nil {
		return nil, err
	}
	return &PEMPublicKeySet{publicKeys: publicKeys}, nil
}

func LoadPEMKeySet(activeKeyID, privateKeyPath string, verificationKeyPaths map[string]string) (*PEMKeySet, error) {
	if activeKeyID == "" || privateKeyPath == "" || len(verificationKeyPaths) == 0 {
		return nil, errors.New("active key ID, private key path, and verification key set are required")
	}
	privateKey, err := readECPrivateKey(privateKeyPath)
	if err != nil {
		return nil, err
	}
	publicKeys, err := loadPublicKeys(verificationKeyPaths)
	if err != nil {
		return nil, err
	}
	if _, ok := publicKeys[activeKeyID]; !ok {
		return nil, errors.New("active signing key is absent from verification key set")
	}
	if privateKey.Curve.Params().Name != "P-256" || publicKeys[activeKeyID].Curve.Params().Name != "P-256" ||
		privateKey.PublicKey.X.Cmp(publicKeys[activeKeyID].X) != 0 || privateKey.PublicKey.Y.Cmp(publicKeys[activeKeyID].Y) != 0 {
		return nil, errors.New("active ES256 private and public keys do not match")
	}
	return &PEMKeySet{activeKeyID: activeKeyID, privateKey: privateKey, publicKeys: publicKeys}, nil
}

func loadPublicKeys(verificationKeyPaths map[string]string) (map[string]*ecdsa.PublicKey, error) {
	publicKeys := make(map[string]*ecdsa.PublicKey, len(verificationKeyPaths))
	for kid, path := range verificationKeyPaths {
		if kid == "" || path == "" {
			return nil, errors.New("verification key ID and path are required")
		}
		key, err := readECPublicKey(path)
		if err != nil {
			return nil, fmt.Errorf("verification key %q: %w", kid, err)
		}
		if key.Curve == nil || key.Curve.Params().Name != "P-256" {
			return nil, fmt.Errorf("verification key %q is not a P-256 ES256 key", kid)
		}
		publicKeys[kid] = key
	}
	return publicKeys, nil
}

func (p *PEMKeySet) CurrentSigningKey(context.Context) (string, *ecdsa.PrivateKey, error) {
	return p.activeKeyID, p.privateKey, nil
}

func (p *PEMKeySet) VerificationKey(_ context.Context, kid string) (*ecdsa.PublicKey, error) {
	key, ok := p.publicKeys[kid]
	if !ok {
		return nil, errors.New("unknown key ID")
	}
	return key, nil
}

func (p *PEMPublicKeySet) VerificationKey(_ context.Context, kid string) (*ecdsa.PublicKey, error) {
	key, ok := p.publicKeys[kid]
	if !ok {
		return nil, errors.New("unknown key ID")
	}
	return key, nil
}

func readECPrivateKey(path string) (*ecdsa.PrivateKey, error) {
	data, err := readBoundedRegularFile(path, maxPEMKeySize, true)
	if err != nil {
		return nil, fmt.Errorf("read private key: %w", err)
	}
	block, rest := pem.Decode(data)
	if block == nil || len(rest) != 0 {
		return nil, errors.New("private key must contain exactly one PEM block")
	}
	if key, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.New("private key is not EC PKCS#8 or SEC1")
	}
	key, ok := parsed.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("private key is not ECDSA")
	}
	return key, nil
}

func readECPublicKey(path string) (*ecdsa.PublicKey, error) {
	data, err := readBoundedRegularFile(path, maxPEMKeySize, false)
	if err != nil {
		return nil, fmt.Errorf("read public key: %w", err)
	}
	block, rest := pem.Decode(data)
	if block == nil || len(rest) != 0 {
		return nil, errors.New("public key must contain exactly one PEM block")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, errors.New("public key is not PKIX")
	}
	key, ok := parsed.(*ecdsa.PublicKey)
	if !ok {
		return nil, errors.New("public key is not ECDSA")
	}
	return key, nil
}
