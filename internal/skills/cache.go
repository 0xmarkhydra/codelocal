package skills

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type PackageCache interface {
	Put(pkg Package) error
	Get(packageHash string) (Package, bool, error)
}

// DirectoryPackageCache is a content-addressed cache suitable for CodeLocal
// desktop/local runtime. Cloud can implement the same interface over object
// storage. Files are addressed only by validated SHA-256 package hashes, never
// by user-controlled skill IDs or paths.
type DirectoryPackageCache struct {
	Root string
}

func (c DirectoryPackageCache) Put(pkg Package) error {
	if err := ValidatePackageIntegrity(pkg); err != nil {
		return err
	}
	path, err := c.packagePath(pkg.PackageHash)
	if err != nil {
		return err
	}
	if existing, ok, err := c.Get(pkg.PackageHash); err != nil {
		return err
	} else if ok {
		if existing.PackageHash != pkg.PackageHash {
			return fmt.Errorf("skill package cache hash collision")
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	payload, err := json.Marshal(pkg)
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".skill-package-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if _, err := temp.Write(payload); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Chmod(0o644); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		// Another process may have populated the same content-addressed entry.
		if _, ok, getErr := c.Get(pkg.PackageHash); getErr == nil && ok {
			return nil
		}
		return err
	}
	return nil
}

func (c DirectoryPackageCache) Get(packageHash string) (Package, bool, error) {
	path, err := c.packagePath(packageHash)
	if err != nil {
		return Package{}, false, err
	}
	payload, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Package{}, false, nil
	}
	if err != nil {
		return Package{}, false, err
	}
	var pkg Package
	if err := json.Unmarshal(payload, &pkg); err != nil {
		return Package{}, false, fmt.Errorf("decode cached skill package: %w", err)
	}
	if pkg.PackageHash != packageHash {
		return Package{}, false, fmt.Errorf("cached skill package address mismatch")
	}
	if err := ValidatePackageIntegrity(pkg); err != nil {
		return Package{}, false, fmt.Errorf("validate cached skill package: %w", err)
	}
	return pkg, true, nil
}

func (c DirectoryPackageCache) packagePath(packageHash string) (string, error) {
	root := strings.TrimSpace(c.Root)
	if root == "" {
		return "", fmt.Errorf("skill package cache root is required")
	}
	digest := strings.TrimPrefix(strings.TrimSpace(packageHash), "sha256:")
	if len(digest) != sha256HexLength {
		return "", fmt.Errorf("invalid skill package hash")
	}
	if _, err := hex.DecodeString(digest); err != nil {
		return "", fmt.Errorf("invalid skill package hash")
	}
	return filepath.Join(root, digest[:2], digest+".skill.json"), nil
}

const sha256HexLength = 64
