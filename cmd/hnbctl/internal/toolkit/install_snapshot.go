package toolkit

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// SaveInstallSnapshot saves normalized, credential-reference-only input. The
// snapshot is bound to the run identity; it is not an assertion of installation.
func (s RunStore) SaveInstallSnapshot(cfg *DeploymentConfig, lockData []byte) error {
	run, err := LoadRunState(s.Dir)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(cfg)
	if err != nil || fmt.Sprintf("sha256:%x", sha256.Sum256(encoded)) != run.ConfigDigest {
		return fmt.Errorf("configuration does not match install run")
	}
	lock, err := DecodeReleaseLock(lockData)
	if err != nil || LockContentDigest(lock) != run.LockDigest {
		return fmt.Errorf("release lock does not match install run")
	}
	var document yaml.Node
	err = document.Encode(cfg)
	if err != nil {
		return fmt.Errorf("cannot encode install configuration")
	}
	// yaml.v3 otherwise serializes a nil node slice as [], changing the JSON
	// identity of existing-cluster configurations from null to an empty array.
	if cfg.Spec.Cluster.Nodes == nil {
		nodes := snapshotField(snapshotField(snapshotField(&document, "spec"), "cluster"), "nodes")
		if nodes != nil {
			*nodes = yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}
		}
	}
	data, err := yaml.Marshal(&document)
	if err != nil {
		return fmt.Errorf("cannot encode install configuration")
	}
	for _, item := range []struct {
		name string
		data []byte
	}{{"deployment.yaml", data}, {"release.lock.yaml", lockData}} {
		file, err := os.OpenFile(filepath.Join(s.Dir, item.name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return fmt.Errorf("cannot create private install snapshot")
		}
		_, writeErr := file.Write(item.data)
		syncErr := file.Sync()
		closeErr := file.Close()
		if writeErr != nil || syncErr != nil || closeErr != nil {
			return fmt.Errorf("cannot persist install snapshot")
		}
	}
	return nil
}

func snapshotField(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

// LoadInstallSnapshot rejects missing, changed, public or linked snapshots.
// Older runs can still be used by explicitly providing the original -f config.
func LoadInstallSnapshot(dir string) (*DeploymentConfig, error) {
	run, err := LoadRunState(dir)
	if err != nil {
		return nil, err
	}
	read := func(name string) ([]byte, error) {
		path := filepath.Join(dir, name)
		fi, err := os.Lstat(path)
		if err != nil || !fi.Mode().IsRegular() || fi.Mode().Perm() != 0600 {
			return nil, fmt.Errorf("private install snapshot unavailable; older runs require -f with the original configuration")
		}
		return os.ReadFile(path)
	}
	data, err := read("deployment.yaml")
	if err != nil {
		return nil, err
	}
	cfg, err := DecodeConfig(data)
	if err != nil {
		return nil, fmt.Errorf("invalid install configuration snapshot")
	}
	encoded, err := json.Marshal(cfg)
	if err != nil || fmt.Sprintf("sha256:%x", sha256.Sum256(encoded)) != run.ConfigDigest {
		return nil, fmt.Errorf("install configuration snapshot identity mismatch")
	}
	lockData, err := read("release.lock.yaml")
	if err != nil {
		return nil, err
	}
	lock, err := DecodeReleaseLock(lockData)
	if err != nil || LockContentDigest(lock) != run.LockDigest {
		return nil, fmt.Errorf("install release-lock snapshot identity mismatch")
	}
	return cfg, nil
}
