package config

import (
	"context"
	"testing"
)

func TestLayerPriority(t *testing.T) {
	if layerPriority[LayerDefault] != 1 {
		t.Errorf("default priority = %d, want 1", layerPriority[LayerDefault])
	}
	if layerPriority[LayerInstance] != 5 {
		t.Errorf("instance priority = %d, want 5", layerPriority[LayerInstance])
	}
}

func TestNewLayer(t *testing.T) {
	l := NewLayer(LayerEnvironment)
	if l.Name != LayerEnvironment {
		t.Errorf("name = %s, want environment", l.Name)
	}
	if l.Priority != 3 {
		t.Errorf("priority = %d, want 3", l.Priority)
	}
	if len(l.Values) != 0 {
		t.Error("new layer should have empty values")
	}
}

func TestConfigResolver_AddAndResolve(t *testing.T) {
	cr := NewConfigResolver()
	cr.AddEntry(LayerDefault, "replicas", "2", "number", "")
	cr.AddEntry(LayerEnvironment, "replicas", "3", "number", "")
	cr.AddEntry(LayerDefault, "log_level", "info", "string", "")
	cr.AddEntry(LayerTenant, "log_level", "debug", "string", "")

	result := cr.Resolve()

	if result["replicas"].Value != "3" {
		t.Errorf("replicas = %s, want 3 (env overrides default)", result["replicas"].Value)
	}
	if result["log_level"].Value != "debug" {
		t.Errorf("log_level = %s, want debug (tenant overrides default)", result["log_level"].Value)
	}
}

func TestConfigResolver_LayerOrder(t *testing.T) {
	cr := NewConfigResolver()
	cr.AddEntry(LayerInstance, "key", "instance-val", "string", "")
	cr.AddEntry(LayerDefault, "key", "default-val", "string", "")
	cr.AddEntry(LayerTenant, "key", "tenant-val", "string", "")

	result := cr.Resolve()
	if result["key"].Value != "instance-val" {
		t.Errorf("key = %s, want instance-val (highest priority)", result["key"].Value)
	}
}

func TestConfigResolver_MultipleKeys(t *testing.T) {
	cr := NewConfigResolver()
	cr.AddEntry(LayerDefault, "a", "1", "string", "")
	cr.AddEntry(LayerDefault, "b", "2", "string", "")
	cr.AddEntry(LayerEnvironment, "c", "3", "string", "")

	result := cr.Resolve()
	if len(result) != 3 {
		t.Errorf("got %d entries, want 3", len(result))
	}
}

func TestBuildSnapshot(t *testing.T) {
	config := map[string]ConfigEntry{
		"replicas":  {Key: "replicas", Value: "3", Layer: LayerTenant},
		"log_level": {Key: "log_level", Value: "debug", Layer: LayerEnvironment},
	}

	snapshot := BuildSnapshot("release", "rel-1", config)
	if snapshot.EntityType != "release" {
		t.Errorf("entity_type = %s, want release", snapshot.EntityType)
	}
	if snapshot.Digest == "" {
		t.Error("digest should not be empty")
	}
}

func TestComputeSnapshotDigest(t *testing.T) {
	config1 := map[string]ConfigEntry{
		"replicas": {Key: "replicas", Value: "3", ValueType: "number"},
	}
	config2 := map[string]ConfigEntry{
		"replicas": {Key: "replicas", Value: "3", ValueType: "number"},
	}
	config3 := map[string]ConfigEntry{
		"replicas": {Key: "replicas", Value: "4", ValueType: "number"},
	}

	d1 := ComputeSnapshotDigest(config1)
	d2 := ComputeSnapshotDigest(config2)
	d3 := ComputeSnapshotDigest(config3)

	if d1 != d2 {
		t.Error("identical configs should have same digest")
	}
	if d1 == d3 {
		t.Error("different configs should have different digests")
	}
}

func TestIsSensitiveKey(t *testing.T) {
	if !IsSensitiveKey("database_password") {
		t.Error("database_password should be sensitive")
	}
	if !IsSensitiveKey("api_key") {
		t.Error("api_key should be sensitive")
	}
	if !IsSensitiveKey("SecretToken") {
		t.Error("SecretToken should be sensitive (case insensitive)")
	}
	if IsSensitiveKey("replicas") {
		t.Error("replicas should not be sensitive")
	}
	if IsSensitiveKey("log_level") {
		t.Error("log_level should not be sensitive")
	}
}

func TestDesensitize(t *testing.T) {
	if Desensitize("my-password-123") != "***REDACTED***" {
		t.Error("should redact value")
	}
}

func TestDesensitizeMap(t *testing.T) {
	input := map[string]string{
		"database_password": "s3cret!",
		"replicas":          "3",
		"api_key":           "abc123",
	}
	result := DesensitizeMap(input)
	if result["database_password"] != "***REDACTED***" {
		t.Error("database_password should be redacted")
	}
	if result["replicas"] != "3" {
		t.Error("replicas should not be redacted")
	}
	if result["api_key"] != "***REDACTED***" {
		t.Error("api_key should be redacted")
	}
}

func TestIsSecretReference(t *testing.T) {
	if !IsSecretReference("ref://secrets/db-password") {
		t.Error("ref:// URI should be detected")
	}
	if IsSecretReference("plaintext-value") {
		t.Error("plaintext should not be detected as secret ref")
	}
	if IsSecretReference("") {
		t.Error("empty string should not be a secret ref")
	}
}

func TestParseSecretReference(t *testing.T) {
	ref := ParseSecretReference("ref://secrets/db-password")
	if ref == nil {
		t.Fatal("expected non-nil ref")
	}
	if ref.Name != "db-password" {
		t.Errorf("name = %s, want db-password", ref.Name)
	}
}

func TestParseSecretReference_Plaintext(t *testing.T) {
	ref := ParseSecretReference("plaintext")
	if ref != nil {
		t.Error("plaintext should return nil ref")
	}
}

func TestLocalAESProvider_EncryptDecrypt(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	p := NewLocalAESProviderWithKey(key)
	ctx := context.Background()

	encrypted, err := p.Encrypt([]byte("my-secret-value"))
	if err != nil {
		t.Fatal(err)
	}

	ref := &SecretReference{EncryptedValue: encrypted}
	plaintext, err := p.Resolve(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if string(plaintext) != "my-secret-value" {
		t.Errorf("got %s, want my-secret-value", string(plaintext))
	}
}

func TestLocalAESProvider_Health(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	p := NewLocalAESProviderWithKey(key)
	if err := p.Health(context.Background()); err != nil {
		t.Errorf("health check failed: %v", err)
	}
}

func TestK8sSecretProvider_Resolve(t *testing.T) {
	p := NewK8sSecretProvider("k8s-main")
	ctx := context.Background()
	ref := &SecretReference{Name: "db-pass", SecretRef: "default/db-credentials"}
	val, err := p.Resolve(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if len(val) == 0 {
		t.Error("expected non-empty resolved value")
	}
}

func TestVaultProvider_Resolve(t *testing.T) {
	p := NewVaultProvider("vault-prod")
	ctx := context.Background()
	ref := &SecretReference{Name: "api-key", Version: 3}
	val, err := p.Resolve(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if len(val) == 0 {
		t.Error("expected non-empty resolved value")
	}
}

func TestRegistry(t *testing.T) {
	r := NewRegistry()
	p := NewLocalAESProviderWithKey(make([]byte, 32))
	r.Register(p)

	got, ok := r.Get("local-aes")
	if !ok {
		t.Fatal("local-aes not found")
	}
	if got.Name() != "local-aes" {
		t.Errorf("name = %s, want local-aes", got.Name())
	}

	_, ok = r.Get("nonexistent")
	if ok {
		t.Error("nonexistent provider should not be found")
	}
}

func TestRegistry_Health(t *testing.T) {
	r := NewRegistry()
	p := NewLocalAESProviderWithKey(make([]byte, 32))
	r.Register(p)

	results := r.Health(context.Background())
	if len(results) != 1 {
		t.Errorf("got %d results, want 1", len(results))
	}
}

func TestSecretResolver(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	localProvider := NewLocalAESProviderWithKey(key)
	registry := NewRegistry()
	registry.Register(localProvider)
	resolver := NewSecretResolver(registry)

	ctx := context.Background()
	encrypted, err := localProvider.Encrypt([]byte("secret-value"))
	if err != nil {
		t.Fatal(err)
	}

	ref := &SecretReference{
		Name:           "test-secret",
		EncryptedValue: encrypted,
	}
	val, err := resolver.Resolve(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if string(val) != "secret-value" {
		t.Errorf("got %s, want secret-value", string(val))
	}
}

func TestSecretResolver_WrongProvider(t *testing.T) {
	registry := NewRegistry()
	resolver := NewSecretResolver(registry)

	ctx := context.Background()
	ref := &SecretReference{
		Name:       "test",
		ProviderID: "nonexistent",
	}
	_, err := resolver.Resolve(ctx, ref)
	if err == nil {
		t.Error("expected error for nonexistent provider")
	}
}

func TestResolveStepInputs_Plaintext(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	p := NewLocalAESProviderWithKey(key)
	r := NewRegistry()
	r.Register(p)
	resolver := NewSecretResolver(r)

	ctx := context.Background()
	inputs := map[string]string{
		"replicas":  "3",
		"log_level": "info",
	}

	result, err := ResolveStepInputs(ctx, resolver, inputs)
	if err != nil {
		t.Fatal(err)
	}
	if result.Resolved["replicas"] != "3" {
		t.Errorf("replicas = %s, want 3", result.Resolved["replicas"])
	}
	if result.Audit["replicas"] != "3" {
		t.Errorf("audit replicas = %s, want 3", result.Audit["replicas"])
	}
}

func TestResolveStepInputs_SecretRef_FailsWithoutEncryptedValue(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	p := NewLocalAESProviderWithKey(key)
	r := NewRegistry()
	r.Register(p)
	resolver := NewSecretResolver(r)

	ctx := context.Background()
	inputs := map[string]string{
		"database_password": "ref://secrets/db-password",
	}

	_, err := ResolveStepInputs(ctx, resolver, inputs)
	if err == nil {
		t.Error("expected error when secret has no encrypted value")
	}
}

func TestResolveAndDesensitize(t *testing.T) {
	inputs := map[string]string{
		"database_password": "s3cret!",
		"replicas":          "3",
	}
	result := ResolveAndDesensitize(inputs, []string{"password"})
	if result["database_password"] != "***REDACTED***" {
		t.Error("password should be redacted")
	}
	if result["replicas"] != "3" {
		t.Error("replicas should not be redacted")
	}
}

func TestLocalAESProvider_InvalidKey(t *testing.T) {
	p := NewLocalAESProviderWithKey([]byte("short"))
	if err := p.Health(context.Background()); err == nil {
		t.Error("expected health error for short key")
	}
}

func TestLocalAESProvider_MissingEnvVar(t *testing.T) {
	_, err := NewLocalAESProvider()
	if err == nil {
		t.Error("expected error without HNB_MASTER_KEY set")
	}
}
