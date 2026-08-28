package skills

import (
	"context"
	"encoding/json"
	"os"
	"testing"
)

func TestDirectoryPackageCacheRoundTrip(t *testing.T) {
	manifest := Manifest{
		ID:        "cached-review",
		Name:      "Cached Review",
		Version:   "1.0.0",
		Publisher: "user",
		Scope:     ScopePersonal,
		Kind:      KindKnowledge,
		Quality:   0.5,
	}
	pkg := testCachePackage(t, manifest)
	cache := DirectoryPackageCache{Root: t.TempDir()}
	ctx := context.Background()
	if err := cache.Put(ctx, pkg); err != nil {
		t.Fatal(err)
	}
	loaded, ok, err := cache.Get(ctx, pkg.PackageHash)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || loaded.PackageHash != pkg.PackageHash || loaded.Manifest.ID != manifest.ID {
		t.Fatalf("unexpected cache round trip: ok=%v package=%#v", ok, loaded)
	}
}

func TestDirectoryPackageCacheRejectsTamperedFile(t *testing.T) {
	manifest := Manifest{
		ID:        "tamper-cache",
		Name:      "Tamper Cache",
		Version:   "1.0.0",
		Publisher: "user",
		Scope:     ScopePersonal,
		Kind:      KindKnowledge,
		Quality:   0.5,
	}
	pkg := testCachePackage(t, manifest)
	cache := DirectoryPackageCache{Root: t.TempDir()}
	ctx := context.Background()
	if err := cache.Put(ctx, pkg); err != nil {
		t.Fatal(err)
	}
	path, err := cache.packagePath(pkg.PackageHash)
	if err != nil {
		t.Fatal(err)
	}
	pkg.Manifest.Intents = []string{"deploy_production"}
	payload, err := json.Marshal(pkg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := cache.Get(ctx, pkg.PackageHash); err == nil {
		t.Fatal("tampered cached package must be rejected")
	}
}

func TestDirectoryPackageCacheRejectsUntrustedAddress(t *testing.T) {
	cache := DirectoryPackageCache{Root: t.TempDir()}
	if _, _, err := cache.Get(context.Background(), "../../outside"); err == nil {
		t.Fatal("untrusted package address must be rejected")
	}
}

func TestDirectoryPackageCacheHonorsCancelledContext(t *testing.T) {
	cache := DirectoryPackageCache{Root: t.TempDir()}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := cache.Get(ctx, "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"); err == nil {
		t.Fatal("cancelled cache read must fail")
	}
}

func testCachePackage(t *testing.T, manifest Manifest) Package {
	t.Helper()
	artifact, err := BuildArtifact(manifest, []KnowledgeChunk{{
		ID:           "knowledge",
		SkillID:      manifest.ID,
		SkillVersion: manifest.Version,
		Content:      "Reusable cached knowledge.",
	}})
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := BuildPackage(manifest, artifact)
	if err != nil {
		t.Fatal(err)
	}
	return pkg
}
