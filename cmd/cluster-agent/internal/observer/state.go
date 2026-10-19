package observer

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// State is the durable observer cursor. It is written only after a successful
// ingest or source-reset so a restart can resume without replaying or skipping.
type State struct {
	Generation   int64 `json:"generation"`
	NextSequence int64 `json:"nextSequence"`
}

func LoadState(path string) (State, bool, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return State{}, false, nil
	}
	if err != nil {
		return State{}, false, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o007 != 0 {
		return State{}, false, errors.New("observer state file must be a private regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return State{}, false, err
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil || state.Generation < 1 || state.NextSequence < 1 {
		return State{}, false, errors.New("invalid observer state file")
	}
	return state, true, nil
}

func SaveState(path string, state State) error {
	if path == "" || state.Generation < 1 || state.NextSequence < 1 {
		return errors.New("invalid observer state")
	}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".observer-state-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
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
	return os.Rename(name, path)
}
