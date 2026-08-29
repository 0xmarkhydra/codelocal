package skills

import "testing"

func TestUserImportPolicyRejectsSystemAndVerified(t *testing.T) {
	manifest := BuiltinManifests()[0]
	artifact, err := BuildArtifact(manifest, BuiltinKnowledge())
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := BuildPackage(manifest, artifact)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidatePackageForImport(pkg, UserImportPolicy()); err == nil {
		t.Fatal("expected user import to reject system/verified skill")
	}
	if err := ValidatePackageForImport(pkg, AdminImportPolicy()); err != nil {
		t.Fatalf("admin import should accept built-in package: %v", err)
	}
}

func TestUserImportPolicyAllowsPersonalKnowledge(t *testing.T) {
	manifest := Manifest{
		ID:        "my-review-style",
		Name:      "My Review Style",
		Version:   "1.0.0",
		Publisher: "user",
		Scope:     ScopePersonal,
		Kind:      KindKnowledge,
		Quality:   0.5,
	}
	artifact, err := BuildArtifact(manifest, []KnowledgeChunk{{
		ID:           "review",
		SkillID:      manifest.ID,
		SkillVersion: manifest.Version,
		Content:      "Prefer concise review comments with concrete fixes.",
	}})
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := BuildPackage(manifest, artifact)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidatePackageForImport(pkg, UserImportPolicy()); err != nil {
		t.Fatalf("personal knowledge skill should be importable by user: %v", err)
	}
}

func TestKnowledgeManifestRejectsExecutionCapabilities(t *testing.T) {
	manifest := Manifest{
		ID:           "fake-knowledge",
		Name:         "Fake Knowledge",
		Version:      "1.0.0",
		Publisher:    "user",
		Scope:        ScopePersonal,
		Kind:         KindKnowledge,
		Capabilities: []Capability{CapabilityShell},
		Quality:      0.5,
	}
	if err := manifest.Validate(); err == nil {
		t.Fatal("knowledge skill must not declare runtime capabilities")
	}
}

func TestUserImportPolicyRejectsRuntimeSkill(t *testing.T) {
	manifest := Manifest{
		ID:           "deploy-helper",
		Name:         "Deploy Helper",
		Version:      "1.0.0",
		Publisher:    "user",
		Scope:        ScopePersonal,
		Kind:         KindRuntime,
		Capabilities: []Capability{CapabilityShell, CapabilityNetwork},
		Quality:      0.5,
	}
	artifact, err := BuildArtifact(manifest, []KnowledgeChunk{{
		ID:           "deploy",
		SkillID:      manifest.ID,
		SkillVersion: manifest.Version,
		Content:      "Deployment workflow metadata.",
	}})
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := BuildPackage(manifest, artifact)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidatePackageForImport(pkg, UserImportPolicy()); err == nil {
		t.Fatal("runtime skill must require an elevated/import review path")
	}
}

func TestUserImportPolicyRejectsWorkflowWithExecutionCapabilities(t *testing.T) {
	manifest := Manifest{
		ID:           "workflow-shell",
		Name:         "Workflow Shell",
		Version:      "1.0.0",
		Publisher:    "user",
		Scope:        ScopePersonal,
		Kind:         KindWorkflow,
		Capabilities: []Capability{CapabilityShell},
		Quality:      0.5,
	}
	artifact, err := BuildArtifact(manifest, []KnowledgeChunk{{
		ID:           "workflow",
		SkillID:      manifest.ID,
		SkillVersion: manifest.Version,
		Content:      "Workflow metadata that requires shell execution.",
	}})
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := BuildPackage(manifest, artifact)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidatePackageForImport(pkg, UserImportPolicy()); err == nil {
		t.Fatal("workflow with execution capabilities must require elevated review")
	}
	if err := ValidatePackageForImport(pkg, AdminImportPolicy()); err != nil {
		t.Fatalf("admin import should accept reviewed workflow package: %v", err)
	}
}

func TestPackageHashRejectsManifestTampering(t *testing.T) {
	manifest := Manifest{
		ID:        "safe-review",
		Name:      "Safe Review",
		Version:   "1.0.0",
		Publisher: "user",
		Scope:     ScopePersonal,
		Kind:      KindKnowledge,
		Intents:   []string{"code_review"},
		Quality:   0.5,
	}
	artifact, err := BuildArtifact(manifest, []KnowledgeChunk{{
		ID:           "review",
		SkillID:      manifest.ID,
		SkillVersion: manifest.Version,
		Content:      "Review code carefully.",
	}})
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := BuildPackage(manifest, artifact)
	if err != nil {
		t.Fatal(err)
	}
	pkg.Manifest.Intents = []string{"deploy_production"}
	pkg.Manifest.Capabilities = []Capability{CapabilityShell, CapabilityNetwork}
	if err := ValidatePackageForImport(pkg, AdminImportPolicy()); err == nil {
		t.Fatal("package metadata tampering must invalidate package hash")
	}
}
