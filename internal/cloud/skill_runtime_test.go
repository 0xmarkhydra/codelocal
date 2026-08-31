package cloud

import (
	"context"
	"strings"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/skills"
)

type fakeSkillRuntimeCatalog struct {
	stable  []SkillVersionRecord
	states  []SkillUserState
	byKey   map[string]SkillVersionRecord
	quality map[string]SkillQualitySignal
}

func (f *fakeSkillRuntimeCatalog) StableSkillVersionRecords(context.Context, string) ([]SkillVersionRecord, error) {
	return append([]SkillVersionRecord(nil), f.stable...), nil
}

func (f *fakeSkillRuntimeCatalog) SkillVersionRecordByIdentity(_ context.Context, tenantUserID, skillID, version string) (SkillVersionRecord, bool, error) {
	record, ok := f.byKey[runtimeRecordKey(tenantUserID, skillID, version)]
	return record, ok, nil
}

func (f *fakeSkillRuntimeCatalog) ListSkillUserStates(context.Context, string) ([]SkillUserState, error) {
	return append([]SkillUserState(nil), f.states...), nil
}

func (f *fakeSkillRuntimeCatalog) SkillQualitySignals(_ context.Context, refs []SkillVersionRef) (map[string]SkillQualitySignal, error) {
	out := map[string]SkillQualitySignal{}
	for _, ref := range refs {
		if signal, ok := f.quality[skillQualityKey(ref.SkillID, ref.Version)]; ok {
			out[skillQualityKey(ref.SkillID, ref.Version)] = signal
		}
	}
	return out, nil
}

type fakeSkillRuntimePackageStore struct {
	packages map[string]skills.Package
}

func (f *fakeSkillRuntimePackageStore) Put(context.Context, skills.Package) error { return nil }

func (f *fakeSkillRuntimePackageStore) Get(_ context.Context, hash string) (skills.Package, bool, error) {
	pkg, ok := f.packages[hash]
	return pkg, ok, nil
}

func TestSkillRuntimeUsesPromotedFullSystemArtifact(t *testing.T) {
	manifest := skills.BuiltinManifests()[0]
	artifact, err := skills.BuildArtifact(manifest, []skills.KnowledgeChunk{{
		ID: "full:sentinel", SkillID: manifest.ID, SkillVersion: manifest.Version,
		Domain: "layout", Title: "Full artifact sentinel", Content: "FULL_PACKAGE_SENTINEL use the complete promoted UI knowledge package.",
		Tags: []string{"dashboard", "ui"}, Priority: 100,
		Source: manifest.SourceURL, SourceRef: manifest.SourceRef, SourceHash: manifest.SourceHash,
	}})
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := skills.BuildPackage(manifest, artifact)
	if err != nil {
		t.Fatal(err)
	}
	record, err := NewSkillVersionRecord("", "admin-user", pkg, "s3://skills/full", SkillVersionPromoted)
	if err != nil {
		t.Fatal(err)
	}
	catalog := &fakeSkillRuntimeCatalog{stable: []SkillVersionRecord{record}}
	packages := &fakeSkillRuntimePackageStore{packages: map[string]skills.Package{pkg.PackageHash: pkg}}
	runtime := NewSkillRuntime(catalog, packages)

	snapshot, err := runtime.Snapshot(context.Background(), "user-1")
	if err != nil {
		t.Fatal(err)
	}
	plan := snapshot.Engine.Plan(skills.TaskContext{Query: "redesign dashboard UI", Intents: []string{"design_ui"}})
	if len(plan.Selections) != 1 || plan.Selections[0].Skill.ID != manifest.ID {
		t.Fatalf("expected UI/UX Pro selection, got %#v", plan.Selections)
	}
	found := false
	for _, match := range plan.Knowledge {
		if strings.Contains(match.Chunk.Content, "FULL_PACKAGE_SENTINEL") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected promoted full artifact knowledge, got %#v", plan.Knowledge)
	}
}

func TestSkillRuntimeDisabledStateRemovesBuiltin(t *testing.T) {
	catalog := &fakeSkillRuntimeCatalog{states: []SkillUserState{{UserID: "user-1", SkillID: "ui-ux-pro", Mode: "disabled"}}}
	snapshot, err := NewSkillRuntime(catalog, nil).Snapshot(context.Background(), "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if got := snapshot.Engine.Catalog(); len(got) != 0 {
		t.Fatalf("disabled built-in must not be routable, got %#v", got)
	}
}

func TestSkillRuntimePersonalOverridesCommunityAndPreferBoosts(t *testing.T) {
	community := runtimeTestPackage(t, skills.Manifest{
		ID: "review-pro", Name: "Community Review", Version: "1.0.0", Publisher: "community",
		Scope: skills.ScopeCommunity, Kind: skills.KindKnowledge, Intents: []string{"code_review"}, Quality: 0.7,
		SourceURL: "https://github.com/example/review", SourceRef: "abc", SourceHash: "tree", License: "MIT",
	})
	personal := runtimeTestPackage(t, skills.Manifest{
		ID: "review-pro", Name: "My Review", Version: "2.0.0", Publisher: "me",
		Scope: skills.ScopePersonal, Kind: skills.KindKnowledge, Intents: []string{"code_review"}, Quality: 0.6,
	})
	communityRecord, _ := NewSkillVersionRecord("", "creator", community, "s3://skills/community", SkillVersionPromoted)
	personalRecord, _ := NewSkillVersionRecord("user-1", "user-1", personal, "s3://skills/personal", SkillVersionActive)
	catalog := &fakeSkillRuntimeCatalog{
		stable: []SkillVersionRecord{communityRecord, personalRecord},
		states: []SkillUserState{{UserID: "user-1", SkillID: "review-pro", Mode: "prefer"}},
	}
	store := &fakeSkillRuntimePackageStore{packages: map[string]skills.Package{
		community.PackageHash: community,
		personal.PackageHash:  personal,
	}}
	snapshot, err := NewSkillRuntime(catalog, store).Snapshot(context.Background(), "user-1")
	if err != nil {
		t.Fatal(err)
	}
	var selected skills.Manifest
	for _, manifest := range snapshot.Engine.Catalog() {
		if manifest.ID == "review-pro" {
			selected = manifest
		}
	}
	if selected.Version != "2.0.0" || selected.Scope != skills.ScopePersonal {
		t.Fatalf("personal skill must override community, got %#v", selected)
	}
	if snapshot.PreferenceAffinity["review-pro"] != preferredSkillAffinityBoost {
		t.Fatalf("expected prefer affinity boost, got %#v", snapshot.PreferenceAffinity)
	}
}

func TestSkillRuntimeCommunityUsesTrustedQuality(t *testing.T) {
	pkg := runtimeTestPackage(t, skills.Manifest{
		ID: "community-only", Name: "Community Only", Version: "1.0.0", Publisher: "creator",
		Scope: skills.ScopeCommunity, Kind: skills.KindKnowledge, Intents: []string{"code_review"}, Quality: 1.0,
		SourceURL: "https://github.com/example/community", SourceRef: "abc", SourceHash: "tree", License: "MIT",
	})
	record, _ := NewSkillVersionRecord("", "creator", pkg, "s3://skills/community-only", SkillVersionPromoted)
	catalog := &fakeSkillRuntimeCatalog{
		stable: []SkillVersionRecord{record},
		quality: map[string]SkillQualitySignal{
			skillQualityKey("community-only", "1.0.0"): {SkillID: "community-only", Version: "1.0.0", EvaluationScore: 0.9, Quality: 0.82},
		},
	}
	store := &fakeSkillRuntimePackageStore{packages: map[string]skills.Package{pkg.PackageHash: pkg}}
	snapshot, err := NewSkillRuntime(catalog, store).Snapshot(context.Background(), "user-1")
	if err != nil {
		t.Fatal(err)
	}
	for _, manifest := range snapshot.Engine.Catalog() {
		if manifest.ID == "community-only" {
			if manifest.Quality != 0.82 {
				t.Fatalf("creator quality must be replaced by trusted quality, got %.2f", manifest.Quality)
			}
			return
		}
	}
	t.Fatal("trusted Community skill should be routable")
}

func TestSkillRuntimeCommunityRejectsTamperedPackageAfterTrustedQualityOverlay(t *testing.T) {
	pkg := runtimeTestPackage(t, skills.Manifest{
		ID: "tampered-community", Name: "Trusted Community", Version: "1.0.0", Publisher: "creator",
		Scope: skills.ScopeCommunity, Kind: skills.KindKnowledge, Intents: []string{"code_review"}, Quality: 1.0,
		SourceURL: "https://github.com/example/tampered", SourceRef: "abc", SourceHash: "tree", License: "MIT",
	})
	record, _ := NewSkillVersionRecord("", "creator", pkg, "s3://skills/tampered", SkillVersionPromoted)
	tampered := pkg
	tampered.Manifest.Name = "Tampered package"
	catalog := &fakeSkillRuntimeCatalog{
		stable: []SkillVersionRecord{record},
		quality: map[string]SkillQualitySignal{
			skillQualityKey("tampered-community", "1.0.0"): {SkillID: "tampered-community", Version: "1.0.0", EvaluationScore: 0.9, Quality: 0.82},
		},
	}
	store := &fakeSkillRuntimePackageStore{packages: map[string]skills.Package{pkg.PackageHash: tampered}}
	snapshot, err := NewSkillRuntime(catalog, store).Snapshot(context.Background(), "user-1")
	if err != nil {
		t.Fatal(err)
	}
	for _, manifest := range snapshot.Engine.Catalog() {
		if manifest.ID == "tampered-community" {
			t.Fatal("tampered Community package must not become routable")
		}
	}
	if len(snapshot.Warnings) == 0 || !strings.Contains(strings.Join(snapshot.Warnings, "\n"), "registry mismatch") {
		t.Fatalf("expected package integrity warning, got %#v", snapshot.Warnings)
	}
}

func TestSkillRuntimeCommunityWithoutEvaluationIsSuppressed(t *testing.T) {
	pkg := runtimeTestPackage(t, skills.Manifest{
		ID: "untrusted-community", Name: "Untrusted", Version: "1.0.0", Publisher: "creator",
		Scope: skills.ScopeCommunity, Kind: skills.KindKnowledge, Quality: 1.0,
		SourceURL: "https://github.com/example/untrusted", SourceRef: "abc", SourceHash: "tree", License: "MIT",
	})
	record, _ := NewSkillVersionRecord("", "creator", pkg, "s3://skills/untrusted", SkillVersionPromoted)
	snapshot, err := NewSkillRuntime(&fakeSkillRuntimeCatalog{stable: []SkillVersionRecord{record}}, &fakeSkillRuntimePackageStore{packages: map[string]skills.Package{pkg.PackageHash: pkg}}).Snapshot(context.Background(), "user-1")
	if err != nil {
		t.Fatal(err)
	}
	for _, manifest := range snapshot.Engine.Catalog() {
		if manifest.ID == "untrusted-community" {
			t.Fatal("Community skill without trusted evaluation must not be routable")
		}
	}
}

func TestSkillRuntimePinSelectsRequestedImmutableVersion(t *testing.T) {
	v1 := runtimeTestPackage(t, skills.Manifest{
		ID: "my-style", Name: "My Style", Version: "1.0.0", Publisher: "me",
		Scope: skills.ScopePersonal, Kind: skills.KindKnowledge, Quality: 0.5,
	})
	v2 := runtimeTestPackage(t, skills.Manifest{
		ID: "my-style", Name: "My Style", Version: "2.0.0", Publisher: "me",
		Scope: skills.ScopePersonal, Kind: skills.KindKnowledge, Quality: 0.6,
	})
	r1, _ := NewSkillVersionRecord("user-1", "user-1", v1, "s3://skills/v1", SkillVersionActive)
	r2, _ := NewSkillVersionRecord("user-1", "user-1", v2, "s3://skills/v2", SkillVersionActive)
	catalog := &fakeSkillRuntimeCatalog{
		stable: []SkillVersionRecord{r2},
		states: []SkillUserState{{UserID: "user-1", SkillID: "my-style", Mode: "auto", PinnedVersion: "1.0.0"}},
		byKey:  map[string]SkillVersionRecord{runtimeRecordKey("user-1", "my-style", "1.0.0"): r1},
	}
	store := &fakeSkillRuntimePackageStore{packages: map[string]skills.Package{v1.PackageHash: v1, v2.PackageHash: v2}}
	snapshot, err := NewSkillRuntime(catalog, store).Snapshot(context.Background(), "user-1")
	if err != nil {
		t.Fatal(err)
	}
	for _, manifest := range snapshot.Engine.Catalog() {
		if manifest.ID == "my-style" && manifest.Version == "1.0.0" {
			return
		}
	}
	t.Fatalf("pinned version was not selected: %#v", snapshot.Engine.Catalog())
}

func TestSkillRuntimeMissingFullSystemPackageFallsBackToBuiltin(t *testing.T) {
	manifest := skills.BuiltinManifests()[0]
	pkg := runtimeTestPackage(t, manifest)
	record, _ := NewSkillVersionRecord("", "admin", pkg, "s3://skills/missing", SkillVersionPromoted)
	snapshot, err := NewSkillRuntime(&fakeSkillRuntimeCatalog{stable: []SkillVersionRecord{record}}, &fakeSkillRuntimePackageStore{packages: map[string]skills.Package{}}).Snapshot(context.Background(), "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Engine.Catalog()) != 1 || snapshot.Engine.Catalog()[0].ID != "ui-ux-pro" {
		t.Fatalf("expected bounded built-in fallback, got %#v", snapshot.Engine.Catalog())
	}
	if len(snapshot.Warnings) == 0 {
		t.Fatal("expected degraded package warning")
	}
}

func runtimeTestPackage(t *testing.T, manifest skills.Manifest) skills.Package {
	t.Helper()
	chunk := skills.KnowledgeChunk{
		ID: "knowledge:" + manifest.Version, SkillID: manifest.ID, SkillVersion: manifest.Version,
		Content: "Reusable runtime knowledge for " + manifest.ID + " " + manifest.Version,
		Source:  manifest.SourceURL, SourceRef: manifest.SourceRef, SourceHash: manifest.SourceHash,
	}
	artifact, err := skills.BuildArtifact(manifest, []skills.KnowledgeChunk{chunk})
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := skills.BuildPackage(manifest, artifact)
	if err != nil {
		t.Fatal(err)
	}
	return pkg
}

func runtimeRecordKey(tenantUserID, skillID, version string) string {
	return tenantUserID + "\x00" + skillID + "\x00" + version
}
