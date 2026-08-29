package skills

import (
	"context"
	"testing"
)

type countingPackageStore struct {
	packages map[string]Package
	gets     int
	puts     int
}

func (s *countingPackageStore) Put(_ context.Context, pkg Package) error {
	s.puts++
	if s.packages == nil {
		s.packages = map[string]Package{}
	}
	s.packages[pkg.PackageHash] = pkg
	return nil
}

func (s *countingPackageStore) Get(_ context.Context, hash string) (Package, bool, error) {
	s.gets++
	pkg, ok := s.packages[hash]
	return pkg, ok, nil
}

func TestCachedPackageStoreAvoidsRepeatedUpstreamReads(t *testing.T) {
	pkg := cacheTestPackage(t, "cache-one", "1.0.0", "small reusable knowledge")
	upstream := &countingPackageStore{packages: map[string]Package{pkg.PackageHash: pkg}}
	cache, err := NewCachedPackageStore(upstream, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		got, ok, err := cache.Get(context.Background(), pkg.PackageHash)
		if err != nil || !ok || got.PackageHash != pkg.PackageHash {
			t.Fatalf("unexpected cached read: ok=%v err=%v", ok, err)
		}
	}
	if upstream.gets != 1 {
		t.Fatalf("expected one durable read, got %d", upstream.gets)
	}
}

func TestCachedPackageStoreEvictsByByteBudget(t *testing.T) {
	first := cacheTestPackage(t, "cache-first", "1.0.0", "first payload with enough bytes to matter")
	second := cacheTestPackage(t, "cache-second", "1.0.0", "second payload with enough bytes to matter")
	upstream := &countingPackageStore{packages: map[string]Package{first.PackageHash: first, second.PackageHash: second}}
	cache, err := NewCachedPackageStore(upstream, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := cache.Get(context.Background(), first.PackageHash); err != nil || !ok {
		t.Fatalf("first read failed: ok=%v err=%v", ok, err)
	}
	if _, ok, err := cache.Get(context.Background(), first.PackageHash); err != nil || !ok {
		t.Fatalf("second read failed: ok=%v err=%v", ok, err)
	}
	if upstream.gets != 2 {
		t.Fatalf("oversized package should not be retained, upstream gets=%d", upstream.gets)
	}
}

func TestCachedPackageStorePutWarmsCache(t *testing.T) {
	pkg := cacheTestPackage(t, "cache-put", "1.0.0", "warm after put")
	upstream := &countingPackageStore{}
	cache, err := NewCachedPackageStore(upstream, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if err := cache.Put(context.Background(), pkg); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := cache.Get(context.Background(), pkg.PackageHash); err != nil || !ok {
		t.Fatalf("cache read after put failed: ok=%v err=%v", ok, err)
	}
	if upstream.puts != 1 || upstream.gets != 0 {
		t.Fatalf("expected warm cache after put, puts=%d gets=%d", upstream.puts, upstream.gets)
	}
}

func cacheTestPackage(t *testing.T, id, version, content string) Package {
	t.Helper()
	manifest := Manifest{ID: id, Name: id, Version: version, Publisher: "user", Scope: ScopePersonal, Kind: KindKnowledge, Quality: 0.5}
	artifact, err := BuildArtifact(manifest, []KnowledgeChunk{{ID: "knowledge", SkillID: id, SkillVersion: version, Content: content}})
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := BuildPackage(manifest, artifact)
	if err != nil {
		t.Fatal(err)
	}
	return pkg
}
