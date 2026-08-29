package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

const DefaultPackageMemoryCacheBytes = 64 * 1024 * 1024

type cachedPackageEntry struct {
	pkg  Package
	size int
}

// CachedPackageStore bounds repeated durable reads by package content hash.
// The cache is process-local, integrity-preserving and byte-bounded; oversized
// packages still pass through to the durable store without being retained.
type CachedPackageStore struct {
	upstream PackageStore
	maxBytes int

	mu      sync.Mutex
	entries map[string]cachedPackageEntry
	order   []string
	bytes   int
}

func NewCachedPackageStore(upstream PackageStore, maxBytes int) (*CachedPackageStore, error) {
	if upstream == nil {
		return nil, fmt.Errorf("upstream skill package store is required")
	}
	if maxBytes <= 0 {
		maxBytes = DefaultPackageMemoryCacheBytes
	}
	return &CachedPackageStore{
		upstream: upstream,
		maxBytes: maxBytes,
		entries:  map[string]cachedPackageEntry{},
	}, nil
}

func (c *CachedPackageStore) Put(ctx context.Context, pkg Package) error {
	if err := ValidatePackageIntegrity(pkg); err != nil {
		return err
	}
	if err := c.upstream.Put(ctx, pkg); err != nil {
		return err
	}
	c.remember(pkg)
	return nil
}

func (c *CachedPackageStore) Get(ctx context.Context, packageHash string) (Package, bool, error) {
	if err := ctx.Err(); err != nil {
		return Package{}, false, err
	}
	c.mu.Lock()
	if entry, ok := c.entries[packageHash]; ok {
		c.touchLocked(packageHash)
		pkg := entry.pkg
		c.mu.Unlock()
		if err := ValidatePackageIntegrity(pkg); err != nil {
			c.forget(packageHash)
			return Package{}, false, fmt.Errorf("validate cached skill package: %w", err)
		}
		return pkg, true, nil
	}
	c.mu.Unlock()

	pkg, ok, err := c.upstream.Get(ctx, packageHash)
	if err != nil || !ok {
		return pkg, ok, err
	}
	if err := ValidatePackageIntegrity(pkg); err != nil {
		return Package{}, false, err
	}
	c.remember(pkg)
	return pkg, true, nil
}

func (c *CachedPackageStore) remember(pkg Package) {
	payload, err := json.Marshal(pkg)
	if err != nil || len(payload) > c.maxBytes {
		return
	}
	entry := cachedPackageEntry{pkg: pkg, size: len(payload)}
	c.mu.Lock()
	defer c.mu.Unlock()
	if existing, ok := c.entries[pkg.PackageHash]; ok {
		c.bytes -= existing.size
		c.removeOrderLocked(pkg.PackageHash)
	}
	c.entries[pkg.PackageHash] = entry
	c.order = append(c.order, pkg.PackageHash)
	c.bytes += entry.size
	for c.bytes > c.maxBytes && len(c.order) > 0 {
		oldest := c.order[0]
		c.order = c.order[1:]
		if cached, ok := c.entries[oldest]; ok {
			c.bytes -= cached.size
			delete(c.entries, oldest)
		}
	}
}

func (c *CachedPackageStore) forget(packageHash string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if entry, ok := c.entries[packageHash]; ok {
		c.bytes -= entry.size
		delete(c.entries, packageHash)
	}
	c.removeOrderLocked(packageHash)
}

func (c *CachedPackageStore) touchLocked(packageHash string) {
	c.removeOrderLocked(packageHash)
	c.order = append(c.order, packageHash)
}

func (c *CachedPackageStore) removeOrderLocked(packageHash string) {
	for index, hash := range c.order {
		if hash == packageHash {
			copy(c.order[index:], c.order[index+1:])
			c.order = c.order[:len(c.order)-1]
			return
		}
	}
}
