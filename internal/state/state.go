package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func Dir() string {
	if value := os.Getenv("CODELOCAL_STATE_DIR"); value != "" {
		return value
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".codelocal"
	}
	return filepath.Join(home, ".codelocal")
}

func EnsurePrivateDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil && !errors.Is(err, os.ErrPermission) {
		return err
	}
	return nil
}

func ReadJSON(path string, dst any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, dst); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

func WriteJSONAtomic(path string, value any) error {
	if err := EnsurePrivateDir(filepath.Dir(path)); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), ".codelocal-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(payload); err != nil {
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
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func AppendJSONL(path string, value any) error {
	if err := EnsurePrivateDir(filepath.Dir(path)); err != nil {
		return err
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(payload); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}
