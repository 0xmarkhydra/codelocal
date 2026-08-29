package cloud

import (
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/skills"
)

func TestNewSkillVersionRecordRequiresPersonalTenant(t *testing.T) {
	pkg := registryTestPackage(t, skills.Manifest{
		ID: "my-style", Name: "My Style", Version: "1.0.0", Publisher: "user",
		Scope: skills.ScopePersonal, Kind: skills.KindKnowledge, Quality: 0.5,
	})
	if _, err := NewSkillVersionRecord("", "user-1", pkg, "s3://skills/hash", SkillVersionActive); err == nil {
		t.Fatal("personal skill must require tenant user")
	}
	record, err := NewSkillVersionRecord("user-1", "user-1", pkg, "s3://skills/hash", SkillVersionActive)
	if err != nil {
		t.Fatal(err)
	}
	if record.TenantUserID != "user-1" || record.CreatorUserID != "user-1" {
		t.Fatalf("unexpected personal registry record: %#v", record)
	}
}

func TestNewSkillVersionRecordRejectsTenantOnSharedSkill(t *testing.T) {
	manifest := skills.Manifest{
		ID: "community-review", Name: "Community Review", Version: "1.0.0", Publisher: "alice",
		Scope: skills.ScopeCommunity, Kind: skills.KindKnowledge, Quality: 0.5,
		SourceURL: "https://github.com/example/review", SourceRef: "abc123", SourceHash: "tree123", License: "MIT",
	}
	pkg := registryTestPackage(t, manifest)
	if _, err := NewSkillVersionRecord("user-1", "user-1", pkg, "s3://skills/hash", SkillVersionCandidate); err == nil {
		t.Fatal("community skill must use global registry namespace")
	}
	record, err := NewSkillVersionRecord("", "user-1", pkg, "s3://skills/hash", SkillVersionCandidate)
	if err != nil {
		t.Fatal(err)
	}
	if record.TenantUserID != "" || record.CreatorUserID != "user-1" {
		t.Fatalf("unexpected community registry record: %#v", record)
	}
}

func TestSkillRegistryRecordIDIsDeterministicAndTenantScoped(t *testing.T) {
	pkg := registryTestPackage(t, skills.Manifest{
		ID: "shared-id", Name: "Shared ID", Version: "1.0.0", Publisher: "user",
		Scope: skills.ScopePersonal, Kind: skills.KindKnowledge, Quality: 0.5,
	})
	a, err := NewSkillVersionRecord("user-a", "user-a", pkg, "s3://skills/hash", SkillVersionActive)
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewSkillVersionRecord("user-a", "user-a", pkg, "s3://skills/hash", SkillVersionActive)
	if err != nil {
		t.Fatal(err)
	}
	c, err := NewSkillVersionRecord("user-b", "user-b", pkg, "s3://skills/hash", SkillVersionActive)
	if err != nil {
		t.Fatal(err)
	}
	if a.RecordID == "" || a.RecordID != b.RecordID {
		t.Fatalf("same immutable identity must be deterministic: %q != %q", a.RecordID, b.RecordID)
	}
	if a.RecordID == c.RecordID {
		t.Fatal("personal registry identities must differ across tenants")
	}
}

func TestNewSkillVersionRecordRejectsInvalidStorageAndState(t *testing.T) {
	pkg := registryTestPackage(t, skills.Manifest{
		ID: "invalid-record", Name: "Invalid Record", Version: "1.0.0", Publisher: "user",
		Scope: skills.ScopePersonal, Kind: skills.KindKnowledge, Quality: 0.5,
	})
	if _, err := NewSkillVersionRecord("user-1", "user-1", pkg, "", SkillVersionActive); err == nil {
		t.Fatal("artifact URI is required")
	}
	if _, err := NewSkillVersionRecord("user-1", "user-1", pkg, "s3://skills/hash", SkillVersionState("mystery")); err == nil {
		t.Fatal("unknown registry state must be rejected")
	}
}

func TestSkillChannelLifecycleCannotBypassPromotion(t *testing.T) {
	for _, state := range []SkillVersionState{
		SkillVersionCandidate, SkillVersionEvaluating, SkillVersionCanary,
		SkillVersionRejected, SkillVersionRolledBack, SkillVersionDeprecated, SkillVersionBlocked,
	} {
		if skillChannelAllowsState("stable", state) {
			t.Fatalf("state %q must not be directly routable as stable", state)
		}
	}
	for _, state := range []SkillVersionState{SkillVersionActive, SkillVersionPromoted} {
		if !skillChannelAllowsState("stable", state) {
			t.Fatalf("state %q should be eligible for stable channel", state)
		}
	}
	if !skillChannelAllowsState("canary", SkillVersionCanary) {
		t.Fatal("canary state must be eligible for canary channel")
	}
	if skillChannelAllowsState("canary", SkillVersionCandidate) || skillChannelAllowsState("canary", SkillVersionPromoted) {
		t.Fatal("canary channel must require the explicit canary state")
	}
}

func registryTestPackage(t *testing.T, manifest skills.Manifest) skills.Package {
	t.Helper()
	artifact, err := skills.BuildArtifact(manifest, []skills.KnowledgeChunk{{
		ID: "knowledge", SkillID: manifest.ID, SkillVersion: manifest.Version,
		Content: "Reusable registry knowledge.", Source: manifest.SourceURL, SourceRef: manifest.SourceRef, SourceHash: manifest.SourceHash,
	}})
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := skills.BuildPackage(manifest, artifact)
	if err != nil {
		t.Fatal(err)
	}
	return pkg
}
