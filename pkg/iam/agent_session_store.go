package iam

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// AgentSessionStore persists the newest renewable session credential so an
// Agent restart can resume without reusing the single-use enrollment secret.
// Implementations must use a private (0600) location and never log the value.
type AgentSessionPersistenceStore interface {
	Load(path string) (AgentSessionCredential, bool, error)
	Save(path string, credential AgentSessionCredential) error
}

type FileAgentSessionStore struct{}

func (FileAgentSessionStore) Load(path string) (AgentSessionCredential, bool, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return AgentSessionCredential{}, false, nil
	}
	if err != nil {
		return AgentSessionCredential{}, false, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return AgentSessionCredential{}, false, errors.New("agent session file must be a private regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return AgentSessionCredential{}, false, err
	}
	var credential AgentSessionCredential
	if err := json.Unmarshal(data, &credential); err != nil {
		return AgentSessionCredential{}, false, err
	}
	if credential.Credential == "" || credential.SessionID == "" || credential.Generation < 1 {
		return AgentSessionCredential{}, false, errors.New("invalid agent session file")
	}
	return credential, true, nil
}

func (FileAgentSessionStore) Save(path string, credential AgentSessionCredential) error {
	if path == "" {
		return errors.New("agent session file path is required")
	}
	data, err := json.Marshal(credential)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".agent-session-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
