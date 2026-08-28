package skills

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const MaxStoredSkillPackageBytes = 64 * 1024 * 1024
const sha256HexLength = 64

type PackageStore interface {
	Put(ctx context.Context, pkg Package) error
	Get(ctx context.Context, packageHash string) (Package, bool, error)
}

// DirectoryPackageCache is a content-addressed PackageStore suitable for
// CodeLocal desktop/local runtime. Cloud implements the same interface over
// durable object storage. Files are addressed only by validated SHA-256 package
// hashes, never by user-controlled skill IDs or paths.
type DirectoryPackageCache struct {
	Root string
}

func (c DirectoryPackageCache) Put(ctx context.Context, pkg Package) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ValidatePackageIntegrity(pkg); err != nil {
		return err
	}
	path, err := c.packagePath(pkg.PackageHash)
	if err != nil {
		return err
	}
	if existing, ok, err := c.Get(ctx, pkg.PackageHash); err != nil {
		return err
	} else if ok {
		if existing.PackageHash != pkg.PackageHash {
			return fmt.Errorf("skill package cache hash collision")
		}
		return nil
	}
	payload, err := json.Marshal(pkg)
	if err != nil {
		return err
	}
	if len(payload) > MaxStoredSkillPackageBytes {
		return fmt.Errorf("skill package exceeds %d stored bytes", MaxStoredSkillPackageBytes)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
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
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		// Another process may have populated the same content-addressed entry.
		if _, ok, getErr := c.Get(ctx, pkg.PackageHash); getErr == nil && ok {
			return nil
		}
		return err
	}
	return nil
}

func (c DirectoryPackageCache) Get(ctx context.Context, packageHash string) (Package, bool, error) {
	if err := ctx.Err(); err != nil {
		return Package{}, false, err
	}
	path, err := c.packagePath(packageHash)
	if err != nil {
		return Package{}, false, err
	}
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return Package{}, false, nil
	}
	if err != nil {
		return Package{}, false, err
	}
	if info.Size() > MaxStoredSkillPackageBytes {
		return Package{}, false, fmt.Errorf("cached skill package exceeds %d stored bytes", MaxStoredSkillPackageBytes)
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return Package{}, false, err
	}
	if err := ctx.Err(); err != nil {
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
	digest, err := PackageHashDigest(packageHash)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, digest[:2], digest+".skill.json"), nil
}

// PackageHashDigest validates a content-addressed package hash and returns its
// lowercase hexadecimal digest for storage-key construction.
func PackageHashDigest(packageHash string) (string, error) {
	packageHash = strings.TrimSpace(packageHash)
	if !strings.HasPrefix(packageHash, "sha256:") {
		return "", fmt.Errorf("invalid skill package hash")
	}
	digest := strings.TrimPrefix(packageHash, "sha256:")
	if len(digest) != sha256HexLength {
		return "", fmt.Errorf("invalid skill package hash")
	}
	if _, err := hex.DecodeString(digest); err != nil {
		return "", fmt.Errorf("invalid skill package hash")
	}
	return strings.ToLower(digest), nil
}
