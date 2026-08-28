package iam

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

const (
	maxKeyManifestSize = 64 << 10
	maxPEMKeySize      = 16 << 10
)

type KeyStatus string

const (
	KeyPending  KeyStatus = "pending"
	KeyActive   KeyStatus = "active"
	KeyRetiring KeyStatus = "retiring"
	KeyRevoked  KeyStatus = "revoked"
	KeyExpired  KeyStatus = "expired"
)

type KeyManifestEntry struct {
	PublicKeyPath string    `json:"publicKeyPath"`
	Status        KeyStatus `json:"status"`
	NotBefore     time.Time `json:"notBefore"`
	NotAfter      time.Time `json:"notAfter"`
}

// KeyManifest intentionally contains public lifecycle metadata only.
type KeyManifest struct {
	Issuer      string                      `json:"issuer"`
	Generation  uint64                      `json:"generation"`
	ActiveKeyID string                      `json:"activeKeyId"`
	Keys        map[string]KeyManifestEntry `json:"keys"`
}

type KeyManifestRecorder interface {
	RecordKeyManifest(context.Context, KeyManifest) error
}

type ReloadingKeySetConfig struct {
	ManifestPath         string
	ActivePrivateKeyPath string
	Issuer               string
	Now                  func() time.Time
	Recorder             KeyManifestRecorder
	OnSuccess            func(KeyReloadStats)
}

type KeyReloadStats struct {
	Generation uint64
	Successes  uint64
	Failures   uint64
}

type keySnapshot struct {
	manifest   KeyManifest
	canonical  []byte
	publicKeys map[string]*ecdsa.PublicKey
	privateKey *ecdsa.PrivateKey
}

type ReloadingKeySet struct {
	config    ReloadingKeySetConfig
	reloadMu  sync.Mutex
	snapshot  atomic.Pointer[keySnapshot]
	successes atomic.Uint64
	failures  atomic.Uint64
}

func LoadReloadingKeySet(ctx context.Context, config ReloadingKeySetConfig) (*ReloadingKeySet, error) {
	if config.Issuer == "" {
		return nil, errors.New("key manifest issuer is required")
	}
	if err := validateAbsolutePath(config.ManifestPath, "key manifest"); err != nil {
		return nil, err
	}
	if config.ActivePrivateKeyPath != "" {
		if err := validateAbsolutePath(config.ActivePrivateKeyPath, "active private key"); err != nil {
			return nil, err
		}
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	set := &ReloadingKeySet{config: config}
	if err := set.Reload(ctx); err != nil {
		return nil, err
	}
	return set, nil
}

func ValidateKeyReloadInterval(interval time.Duration) error {
	if interval < time.Second || interval > 60*time.Second {
		return errors.New("key reload interval must be between 1 and 60 seconds")
	}
	return nil
}

func ParseKeyReloadInterval(value string) (time.Duration, error) {
	if value == "" {
		return 0, errors.New("key reload interval is required")
	}
	interval, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("parse key reload interval %s: %w", strconv.Quote(value), err)
	}
	if err := ValidateKeyReloadInterval(interval); err != nil {
		return 0, err
	}
	return interval, nil
}

func (r *ReloadingKeySet) StartPolling(ctx context.Context, interval time.Duration, onError func(error)) error {
	if err := ValidateKeyReloadInterval(interval); err != nil {
		return err
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := r.Reload(ctx); err != nil && onError != nil {
					onError(err)
				}
			}
		}
	}()
	return nil
}

func (r *ReloadingKeySet) Reload(ctx context.Context) error {
	r.reloadMu.Lock()
	defer r.reloadMu.Unlock()

	manifest, canonical, err := loadKeyManifest(r.config.ManifestPath, r.config.Issuer, r.config.Now().UTC())
	if err != nil {
		r.failures.Add(1)
		return err
	}
	current := r.snapshot.Load()
	if current != nil {
		if manifest.Generation < current.manifest.Generation {
			r.failures.Add(1)
			return errors.New("key manifest generation rollback is forbidden")
		}
		if manifest.Generation == current.manifest.Generation {
			if !bytes.Equal(canonical, current.canonical) {
				r.failures.Add(1)
				return errors.New("key manifest generation cannot be reused with different content")
			}
			return nil
		}
		if err := validateManifestTransition(current.manifest, manifest); err != nil {
			r.failures.Add(1)
			return err
		}
	}

	publicKeys := make(map[string]*ecdsa.PublicKey, len(manifest.Keys))
	for kid, entry := range manifest.Keys {
		key, err := readECPublicKey(entry.PublicKeyPath)
		if err != nil {
			r.failures.Add(1)
			return fmt.Errorf("verification key %q: %w", kid, err)
		}
		if key.Curve == nil || key.Curve.Params().Name != "P-256" {
			r.failures.Add(1)
			return fmt.Errorf("verification key %q is not a P-256 ES256 key", kid)
		}
		publicKeys[kid] = key
	}

	var privateKey *ecdsa.PrivateKey
	if r.config.ActivePrivateKeyPath != "" {
		privateKey, err = readECPrivateKey(r.config.ActivePrivateKeyPath)
		if err != nil {
			r.failures.Add(1)
			return err
		}
		activePublic := publicKeys[manifest.ActiveKeyID]
		if privateKey.Curve == nil || privateKey.Curve.Params().Name != "P-256" ||
			privateKey.PublicKey.X.Cmp(activePublic.X) != 0 || privateKey.PublicKey.Y.Cmp(activePublic.Y) != 0 {
			r.failures.Add(1)
			return errors.New("active ES256 private and public keys do not match")
		}
	}
	if r.config.Recorder != nil {
		if err := r.config.Recorder.RecordKeyManifest(ctx, manifest); err != nil {
			r.failures.Add(1)
			return fmt.Errorf("record key manifest: %w", err)
		}
	}
	r.snapshot.Store(&keySnapshot{manifest: manifest, canonical: canonical, publicKeys: publicKeys, privateKey: privateKey})
	r.successes.Add(1)
	if r.config.OnSuccess != nil {
		r.config.OnSuccess(r.Stats())
	}
	return nil
}

func validateManifestTransition(previous, next KeyManifest) error {
	for kid, oldEntry := range previous.Keys {
		newEntry, present := next.Keys[kid]
		if !present {
			if oldEntry.Status != KeyRevoked && oldEntry.Status != KeyExpired {
				return fmt.Errorf("key %q cannot be removed before revocation or expiry", kid)
			}
			continue
		}
		if !validKeyStatusTransition(oldEntry.Status, newEntry.Status) {
			return fmt.Errorf("key %q has forbidden status transition %s -> %s", kid, oldEntry.Status, newEntry.Status)
		}
		if !oldEntry.NotBefore.Equal(newEntry.NotBefore) || !oldEntry.NotAfter.Equal(newEntry.NotAfter) || oldEntry.PublicKeyPath != newEntry.PublicKeyPath {
			return fmt.Errorf("key %q immutable metadata changed", kid)
		}
	}
	return nil
}

func validKeyStatusTransition(oldStatus, newStatus KeyStatus) bool {
	if oldStatus == newStatus {
		return true
	}
	switch oldStatus {
	case KeyPending:
		return newStatus == KeyActive || newStatus == KeyRevoked || newStatus == KeyExpired
	case KeyActive:
		return newStatus == KeyRetiring || newStatus == KeyRevoked
	case KeyRetiring:
		return newStatus == KeyRevoked || newStatus == KeyExpired
	default:
		return false
	}
}

func (r *ReloadingKeySet) CurrentSigningKey(context.Context) (string, *ecdsa.PrivateKey, error) {
	snapshot := r.snapshot.Load()
	if snapshot == nil || snapshot.privateKey == nil {
		return "", nil, errors.New("active private signing key is unavailable")
	}
	entry := snapshot.manifest.Keys[snapshot.manifest.ActiveKeyID]
	now := r.config.Now().UTC()
	if entry.Status != KeyActive || now.Before(entry.NotBefore) || !now.Before(entry.NotAfter) {
		return "", nil, errors.New("active signing key is outside its valid window")
	}
	return snapshot.manifest.ActiveKeyID, snapshot.privateKey, nil
}

func (r *ReloadingKeySet) VerificationKey(_ context.Context, kid string) (*ecdsa.PublicKey, error) {
	snapshot := r.snapshot.Load()
	if snapshot == nil {
		return nil, errors.New("key manifest is unavailable")
	}
	entry, ok := snapshot.manifest.Keys[kid]
	now := r.config.Now().UTC()
	if !ok || (entry.Status != KeyActive && entry.Status != KeyRetiring) || now.Before(entry.NotBefore) || !now.Before(entry.NotAfter) {
		return nil, errors.New("signing key is not accepted")
	}
	return snapshot.publicKeys[kid], nil
}

func (r *ReloadingKeySet) Stats() KeyReloadStats {
	snapshot := r.snapshot.Load()
	var generation uint64
	if snapshot != nil {
		generation = snapshot.manifest.Generation
	}
	return KeyReloadStats{Generation: generation, Successes: r.successes.Load(), Failures: r.failures.Load()}
}

func loadKeyManifest(path, issuer string, now time.Time) (KeyManifest, []byte, error) {
	data, err := readBoundedRegularFile(path, maxKeyManifestSize, false)
	if err != nil {
		return KeyManifest{}, nil, fmt.Errorf("read key manifest: %w", err)
	}
	if err := rejectDuplicateJSONFields(data); err != nil {
		return KeyManifest{}, nil, fmt.Errorf("decode key manifest: %w", err)
	}
	var manifest KeyManifest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return KeyManifest{}, nil, fmt.Errorf("decode key manifest: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return KeyManifest{}, nil, errors.New("key manifest must contain exactly one JSON value")
	}
	if manifest.Issuer != issuer || !boundedClaim(manifest.Issuer, 2048) || manifest.Generation == 0 || manifest.Generation > math.MaxInt64 || !validManifestKeyID(manifest.ActiveKeyID) || len(manifest.Keys) == 0 || len(manifest.Keys) > 64 {
		return KeyManifest{}, nil, errors.New("key manifest header is invalid")
	}
	activeCount := 0
	for kid, entry := range manifest.Keys {
		if !validManifestKeyID(kid) || entry.NotBefore.IsZero() || entry.NotAfter.IsZero() || !entry.NotAfter.After(entry.NotBefore) {
			return KeyManifest{}, nil, fmt.Errorf("key manifest entry %q has an invalid identity or time window", kid)
		}
		if err := validateAbsolutePath(entry.PublicKeyPath, "public key"); err != nil {
			return KeyManifest{}, nil, fmt.Errorf("key manifest entry %q: %w", kid, err)
		}
		switch entry.Status {
		case KeyPending:
			if !now.Before(entry.NotAfter) {
				return KeyManifest{}, nil, fmt.Errorf("pending key %q has already expired", kid)
			}
		case KeyRevoked:
		case KeyActive:
			activeCount++
			if kid != manifest.ActiveKeyID || now.Before(entry.NotBefore) || !now.Before(entry.NotAfter) {
				return KeyManifest{}, nil, errors.New("active key is outside its valid window or does not match activeKeyId")
			}
		case KeyRetiring:
			if now.Before(entry.NotBefore) || !now.Before(entry.NotAfter) {
				return KeyManifest{}, nil, fmt.Errorf("retiring key %q is outside its valid window", kid)
			}
		case KeyExpired:
			if now.Before(entry.NotAfter) {
				return KeyManifest{}, nil, fmt.Errorf("expired key %q has not reached notAfter", kid)
			}
		default:
			return KeyManifest{}, nil, fmt.Errorf("key manifest entry %q has an invalid status", kid)
		}
	}
	if activeCount != 1 {
		return KeyManifest{}, nil, errors.New("key manifest must contain exactly one active key")
	}
	canonical, err := json.Marshal(manifest)
	return manifest, canonical, err
}

func validManifestKeyID(kid string) bool {
	if !boundedClaim(kid, 128) {
		return false
	}
	for _, character := range kid {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '.' || character == '_' || character == '-' {
			continue
		}
		return false
	}
	return true
}

func rejectDuplicateJSONFields(data []byte) error {
	type frame struct {
		object       bool
		expectingKey bool
		keys         map[string]struct{}
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	var stack []frame
	completeValue := func() {
		if len(stack) > 0 && stack[len(stack)-1].object {
			stack[len(stack)-1].expectingKey = true
		}
	}
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if delimiter, ok := token.(json.Delim); ok {
			switch delimiter {
			case '{':
				stack = append(stack, frame{object: true, expectingKey: true, keys: make(map[string]struct{})})
			case '[':
				stack = append(stack, frame{})
			case '}', ']':
				if len(stack) == 0 || (delimiter == '}' && (!stack[len(stack)-1].object || !stack[len(stack)-1].expectingKey)) || (delimiter == ']' && stack[len(stack)-1].object) {
					return errors.New("invalid JSON structure")
				}
				stack = stack[:len(stack)-1]
				completeValue()
			}
			continue
		}
		if len(stack) > 0 && stack[len(stack)-1].object {
			current := &stack[len(stack)-1]
			if current.expectingKey {
				key, ok := token.(string)
				if !ok {
					return errors.New("JSON object key must be a string")
				}
				if _, duplicate := current.keys[key]; duplicate {
					return fmt.Errorf("duplicate JSON field %q", key)
				}
				current.keys[key] = struct{}{}
				current.expectingKey = false
			} else {
				current.expectingKey = true
			}
		}
	}
}

func validateAbsolutePath(path, label string) error {
	if path == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return fmt.Errorf("%s path must be a clean absolute path", label)
	}
	return nil
}

func readBoundedRegularFile(path string, maxSize int64, private bool) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxSize {
		return nil, errors.New("file must be a non-empty bounded regular file")
	}
	if info.Mode().Perm()&0022 != 0 || (private && info.Mode().Perm()&0077 != 0) {
		return nil, errors.New("file permissions are too broad")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxSize+1))
	if err != nil || int64(len(data)) > maxSize {
		return nil, errors.New("file exceeds size limit")
	}
	return data, nil
}
