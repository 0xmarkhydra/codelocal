package cloud

import (
	"context"
	"fmt"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/skills"
)

type fakeSkillPackageObjectStore struct {
	packages map[string]skills.Package
}

func (f *fakeSkillPackageObjectStore) Put(_ context.Context, pkg skills.Package) error {
	if f.packages == nil {
		f.packages = map[string]skills.Package{}
	}
	f.packages[pkg.PackageHash] = pkg
	return nil
}

func (f *fakeSkillPackageObjectStore) Get(_ context.Context, packageHash string) (skills.Package, bool, error) {
	pkg, ok := f.packages[packageHash]
	return pkg, ok, nil
}

func (f *fakeSkillPackageObjectStore) ObjectURI(packageHash string) (string, error) {
	if _, ok := f.packages[packageHash]; !ok {
		return "", fmt.Errorf("package not stored")
	}
	return "s3://skills/" + packageHash, nil
}

type fakeSkillRegistryPersistence struct {
	records  []SkillVersionRecord
	channels []string
}

func (f *fakeSkillRegistryPersistence) CreateSkillVersion(_ context.Context, record SkillVersionRecord) (bool, error) {
	f.records = append(f.records, record)
	return true, nil
}

func (f *fakeSkillRegistryPersistence) SetSkillChannel(_ context.Context, tenantUserID, skillID, channel, version string) error {
	f.channels = append(f.channels, tenantUserID+"|"+skillID+"|"+channel+"|"+version)
	return nil
}

func TestSkillImportServiceActivatesPersonalSkillPrivately(t *testing.T) {
	pkg := registryTestPackage(t, skills.Manifest{
		ID: "my-review", Name: "My Review", Version: "1.0.0", Publisher: "claimed-name",
		Scope: skills.ScopePersonal, Kind: skills.KindKnowledge, Quality: 0.5,
	})
	objects := &fakeSkillPackageObjectStore{}
	registry := &fakeSkillRegistryPersistence{}
	service, err := NewSkillImportService(objects, registry)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.ImportPersonal(context.Background(), "user-1", pkg)
	if err != nil {
		t.Fatal(err)
	}
	if result.Disposition != "active" || result.Record.TenantUserID != "user-1" {
		t.Fatalf("unexpected personal import result: %#v", result)
	}
	if result.Record.Publisher != "user-1" {
		t.Fatalf("personal publisher authority must come from authenticated user, got %q", result.Record.Publisher)
	}
	if len(registry.channels) != 1 || registry.channels[0] != "user-1|my-review|stable|1.0.0" {
		t.Fatalf("personal skill must get private stable channel: %#v", registry.channels)
	}
}

func TestSkillImportServiceStagesCommunityWithoutGlobalChannel(t *testing.T) {
	pkg := registryTestPackage(t, skills.Manifest{
		ID: "community-review", Name: "Community Review", Version: "1.0.0", Publisher: "CodeLocal",
		Scope: skills.ScopeCommunity, Kind: skills.KindKnowledge, Quality: 0.5,
		SourceURL: "https://github.com/example/review", SourceRef: "abc123", SourceHash: "tree123", License: "MIT",
	})
	objects := &fakeSkillPackageObjectStore{}
	registry := &fakeSkillRegistryPersistence{}
	service, _ := NewSkillImportService(objects, registry)
	result, err := service.PublishCommunity(context.Background(), "user-42", pkg)
	if err != nil {
		t.Fatal(err)
	}
	if result.Disposition != "candidate" || result.Record.TenantUserID != "" || result.Record.CreatorUserID != "user-42" {
		t.Fatalf("unexpected community import result: %#v", result)
	}
	if result.Record.Publisher != "user-42" {
		t.Fatalf("community publisher must ignore spoofed manifest publisher, got %q", result.Record.Publisher)
	}
	if len(registry.channels) != 0 {
		t.Fatalf("community candidate must not become globally routable: %#v", registry.channels)
	}
}

func TestSkillImportServiceRejectsRuntimeFromUser(t *testing.T) {
	pkg := registryTestPackage(t, skills.Manifest{
		ID: "deploy-helper", Name: "Deploy Helper", Version: "1.0.0", Publisher: "user",
		Scope: skills.ScopePersonal, Kind: skills.KindRuntime,
		Capabilities: []skills.Capability{skills.CapabilityShell, skills.CapabilityNetwork}, Quality: 0.5,
	})
	service, _ := NewSkillImportService(&fakeSkillPackageObjectStore{}, &fakeSkillRegistryPersistence{})
	if _, err := service.ImportPersonal(context.Background(), "user-1", pkg); err == nil {
		t.Fatal("user runtime import must stay on elevated review path")
	}
}

func TestSkillImportServiceStagesAdminSystemSkill(t *testing.T) {
	manifest := skills.BuiltinManifests()[0]
	pkg := registryTestPackage(t, manifest)
	objects := &fakeSkillPackageObjectStore{}
	registry := &fakeSkillRegistryPersistence{}
	service, _ := NewSkillImportService(objects, registry)
	result, err := service.ImportAdminCandidate(context.Background(), "admin-1", pkg)
	if err != nil {
		t.Fatal(err)
	}
	if result.Record.State != SkillVersionCandidate || result.Record.Publisher != manifest.Publisher || len(registry.channels) != 0 {
		t.Fatalf("admin system import must stage before promotion: %#v", result)
	}
}
