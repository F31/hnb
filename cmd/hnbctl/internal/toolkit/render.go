package toolkit

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// RenderLockedChart produces a local candidate manifest. It never contacts a
// cluster or authorizes installation. The package, overlay and image identities
// all come from the same lock; operator bindings contain only Secret references.
func RenderLockedChart(ctx context.Context, cfg *DeploymentConfig, lock *ReleaseLock, chartPath string, bindings []byte, stage string) ([]byte, error) {
	if cfg == nil || lock == nil || cfg.Spec.Mode != ModeExisting {
		return nil, fmt.Errorf("render requires an existing-cluster configuration and lock")
	}
	if stage != "startup" && stage != "full" {
		return nil, fmt.Errorf("stage must be startup or full")
	}
	if cfg.Spec.Profile != ProfileMinimal {
		return nil, fmt.Errorf("HA profiles have no installation certification")
	}
	packageBytes, err := os.ReadFile(chartPath)
	if err != nil {
		return nil, fmt.Errorf("chart package unavailable")
	}
	name, version, err := chartMetadata(chartPath)
	if err != nil {
		return nil, fmt.Errorf("invalid chart package")
	}
	matched := false
	for _, chart := range lock.Charts {
		if chart.Name == name && chart.Version == version && chart.Digest == "sha256:"+sha256Hex(packageBytes) {
			matched = true
		}
	}
	if !matched {
		return nil, fmt.Errorf("chart package does not match release lock")
	}
	values := map[string]any{}
	if err := yaml.Unmarshal(bindings, &values); err != nil {
		return nil, fmt.Errorf("invalid binding values")
	}
	if global, ok := values["global"].(map[string]any); ok {
		if backing, _ := global["backings"].(string); backing != "" {
			if !knownBacking(backing) {
				return nil, fmt.Errorf("unknown global.backings %q", backing)
			}
			if backing == BackingInClusterHA {
				return nil, fmt.Errorf("in-cluster-ha backing requires a certified HA profile")
			}
		}
	}
	// Reject inline connection credentials even if a later override would hide
	// them: they must never be copied to Helm values or retained output.
	for _, entry := range values {
		if m, ok := entry.(map[string]any); ok {
			if db, ok := m["db"].(map[string]any); ok && db["dsn"] != nil && db["dsn"] != "" {
				return nil, fmt.Errorf("bindings must use db.existingSecret instead of db.dsn")
			}
		}
	}
	entry := func(key string) map[string]any {
		m, ok := values[key].(map[string]any)
		if !ok {
			m = map[string]any{}
			values[key] = m
		}
		return m
	}
	global := entry("global")
	global["imageRegistry"] = ""
	if pg, ok := global["postgresql"].(map[string]any); ok && pg["dsn"] != nil && pg["dsn"] != "" {
		return nil, fmt.Errorf("bindings must not use a global database DSN")
	}
	secretName := func(ref string) (string, error) {
		parts := strings.Split(ref, "/")
		if !validSecretRef(ref) || len(parts) == 2 && parts[0] != cfg.Spec.Platform.Namespace {
			return "", fmt.Errorf("Secret binding is outside the installation namespace")
		}
		return parts[len(parts)-1], nil
	}
	for _, plane := range cfg.Spec.Platform.Infrastructure.Planes {
		dbSecret, err := secretName(plane.PostgreSQLSecretRef)
		if err != nil {
			return nil, err
		}
		natsSecret, err := secretName(plane.NATSSecretRef)
		if err != nil {
			return nil, err
		}
		for name := range values {
			if name == "global" {
				continue
			}
			m, ok := values[name].(map[string]any)
			if !ok {
				continue
			}
			owner := "platform"
			if name == "app-market" {
				owner = "market"
			}
			if owner != plane.Name {
				continue
			}
			if db, ok := m["db"].(map[string]any); ok {
				db["existingSecret"], db["secretKey"] = dbSecret, "dsn"
			}
			if nc, ok := m["nats"].(map[string]any); ok {
				nc["existingAccountSecret"] = natsSecret
			}
		}
	}
	allowedImages := map[string]bool{}
	for _, image := range lock.Images {
		if image.ManifestDigest == "" || image.Platform != "linux/amd64" {
			return nil, fmt.Errorf("lock must identify each image's amd64 manifest")
		}
		repository := strings.Split(image.Ref, "@")[0]
		if colon := strings.LastIndex(repository, ":"); colon > strings.LastIndex(repository, "/") {
			repository = repository[:colon]
		}
		imageValues := map[string]any{"repository": repository, "digest": image.ManifestDigest}
		if image.Name == "sandbox" {
			entry("apiserver")["sandbox"] = imageValues
		} else {
			entry(image.Name)["image"] = imageValues
		}
		allowedImages[repository+"@"+image.ManifestDigest] = true
		if image.Name == "cluster-agent" {
			entry("apiserver")["agentOnboarding"] = map[string]any{"image": repository + "@" + image.ManifestDigest}
		}
	}
	// Read the selection overlay from the verified package, not an unrelated
	// working-tree file. No extraction or remote Helm dependency update is needed.
	gz, err := gzip.NewReader(bytes.NewReader(packageBytes))
	if err != nil {
		return nil, fmt.Errorf("invalid chart archive")
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	overlayName := "minimal"
	if stage == "startup" {
		overlayName = "startup"
	}
	var overlay []byte
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("invalid chart archive")
		}
		if strings.TrimPrefix(header.Name, "./") == name+"/overlays/"+overlayName+".yaml" {
			if header.Typeflag != tar.TypeReg || header.Size > 1024*1024 {
				return nil, fmt.Errorf("invalid chart overlay")
			}
			overlay, err = io.ReadAll(reader)
			if err != nil {
				return nil, fmt.Errorf("cannot read chart overlay")
			}
			break
		}
	}
	if len(overlay) == 0 {
		return nil, fmt.Errorf("verified chart lacks selection overlay")
	}
	dir, err := os.MkdirTemp("", "hnb-render-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	verifiedChart := filepath.Join(dir, "chart.tgz")
	if err := os.WriteFile(verifiedChart, packageBytes, 0o600); err != nil {
		return nil, err
	}
	encoded, err := yaml.Marshal(values)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, "bindings.yaml"), encoded, 0o600); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, "overlay.yaml"), overlay, 0o600); err != nil {
		return nil, err
	}
	command := exec.CommandContext(ctx, "helm", "template", cfg.Metadata.Name, verifiedChart, "--namespace", cfg.Spec.Platform.Namespace,
		"-f", filepath.Join(dir, "overlay.yaml"), "-f", filepath.Join(dir, "bindings.yaml"))
	output, err := command.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf("locked Chart rendering failed: %s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, fmt.Errorf("locked Chart rendering failed; check binding references and local Helm availability")
	}
	decoder := yaml.NewDecoder(bytes.NewReader(output))
	var result bytes.Buffer
	encoder := yaml.NewEncoder(&result)
	defer encoder.Close()
	clusterKinds := map[string]bool{"Namespace": true, "ClusterRole": true, "ClusterRoleBinding": true,
		"CustomResourceDefinition": true, "StorageClass": true, "PersistentVolume": true}
	for {
		var doc map[string]any
		if err := decoder.Decode(&doc); err == io.EOF {
			break
		} else if err != nil {
			return nil, fmt.Errorf("invalid rendered manifest")
		}
		if len(doc) == 0 {
			continue
		}
		kind, _ := doc["kind"].(string)
		if kind == "Secret" {
			return nil, fmt.Errorf("rendered chart must reference existing Secrets")
		}
		if !clusterKinds[kind] {
			metadata, ok := doc["metadata"].(map[string]any)
			if !ok {
				return nil, fmt.Errorf("rendered resource lacks metadata")
			}
			metadata["namespace"] = cfg.Spec.Platform.Namespace
		}
		if err := validateRenderValues(doc, allowedImages); err != nil {
			return nil, err
		}
		if err := encoder.Encode(doc); err != nil {
			return nil, err
		}
	}
	if result.Len() == 0 {
		return nil, fmt.Errorf("chart rendered no resources")
	}
	return result.Bytes(), nil
}

func validateRenderValues(value any, allowedImages map[string]bool) error {
	switch v := value.(type) {
	case map[string]any:
		if v["name"] == "DB_DSN" && v["value"] != nil {
			return fmt.Errorf("rendered database credential is inline")
		}
		if image, ok := v["image"].(string); ok && !allowedImages[image] {
			return fmt.Errorf("rendered image is outside the release lock")
		}
		for _, child := range v {
			if err := validateRenderValues(child, allowedImages); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range v {
			if err := validateRenderValues(child, allowedImages); err != nil {
				return err
			}
		}
	}
	return nil
}

// WriteRenderedManifest is replay-safe: identical bytes are accepted, different
// bytes never replace an existing output. The artifact remains permission-restricted.
func WriteRenderedManifest(path string, data []byte) error {
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
			return fmt.Errorf("render output must be a private regular file")
		}
		old, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(old, data) {
			return fmt.Errorf("render output differs; choose a new output path")
		}
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("render output inaccessible")
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	n, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil || n != len(data) {
		os.Remove(path)
		return fmt.Errorf("render output incomplete")
	}
	return nil
}

// ApplyBackingValue injects the selected data-plane backing into a bindings
// values document before rendering. External disables the bundled subcharts;
// bundled re-enables them; in-cluster-ha leaves both enabled (the render-time
// profile gate still refuses an HA backing for a non-HA profile).
func ApplyBackingValue(bindings []byte, backing string) ([]byte, error) {
	if !knownBacking(backing) {
		return nil, fmt.Errorf("unknown backing %q", backing)
	}
	values := map[string]any{}
	if len(bindings) > 0 {
		if err := yaml.Unmarshal(bindings, &values); err != nil {
			return nil, fmt.Errorf("invalid binding values")
		}
	}
	global, _ := values["global"].(map[string]any)
	if global == nil {
		global = map[string]any{}
	}
	global["backings"] = backing
	values["global"] = global
	setEnabled := func(key string, enabled bool) {
		m, _ := values[key].(map[string]any)
		if m == nil {
			m = map[string]any{}
		}
		m["enabled"] = enabled
		values[key] = m
	}
	if backing == BackingExternal {
		setEnabled("postgresql", false)
		setEnabled("nats", false)
	} else {
		setEnabled("postgresql", true)
		setEnabled("nats", true)
	}
	out, err := yaml.Marshal(values)
	if err != nil {
		return nil, fmt.Errorf("cannot encode binding values")
	}
	return out, nil
}
