package cloudserver

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"net/netip"
	"strings"
	"testing"

	skillintel "github.com/0xmarkhydra/codelocal/internal/skills"
)

func TestBuildPersonalPackageFromTextIsKnowledgeOnly(t *testing.T) {
	pkg, sourceType, err := buildPersonalPackageFromIngest(context.Background(), skillIngestRequest{
		Name: "React Review",
		Text: "# React review\nAlways inspect component boundaries before changing UI state.",
	})
	if err != nil {
		t.Fatalf("build personal package: %v", err)
	}
	if sourceType != "text" {
		t.Fatalf("sourceType=%q want text", sourceType)
	}
	if pkg.Manifest.Scope != skillintel.ScopePersonal || pkg.Manifest.Kind != skillintel.KindKnowledge {
		t.Fatalf("unexpected manifest scope/kind: %#v", pkg.Manifest)
	}
	if pkg.Manifest.Verified || len(pkg.Manifest.Capabilities) != 0 {
		t.Fatalf("user ingestion must not grant verified/runtime authority: %#v", pkg.Manifest)
	}
	if pkg.Manifest.ID != "personal.react-review" {
		t.Fatalf("skill id=%q want personal.react-review", pkg.Manifest.ID)
	}
	if err := skillintel.ValidatePackageForImport(pkg, skillintel.UserImportPolicy()); err != nil {
		t.Fatalf("built package rejected by user policy: %v", err)
	}
}

func TestBuildPersonalPackageRejectsCommunityPassthrough(t *testing.T) {
	manifest := skillintel.Manifest{
		ID: "community.test", Name: "Community Test", Version: "1.0.0",
		Scope: skillintel.ScopeCommunity, Kind: skillintel.KindKnowledge,
		SourceURL: "https://example.com/skill", SourceRef: "main", SourceHash: "sha256:test", License: "MIT",
	}
	artifact, err := skillintel.BuildArtifactFromDocuments(manifest, []skillintel.SourceDocument{{Path: "SKILL.md", Content: "knowledge"}}, skillintel.DefaultKnowledgeIngestPolicy())
	if err != nil {
		t.Fatalf("artifact: %v", err)
	}
	pkg, err := skillintel.BuildPackage(manifest, artifact)
	if err != nil {
		t.Fatalf("package: %v", err)
	}
	_, _, err = buildPersonalPackageFromIngest(context.Background(), skillIngestRequest{Package: &pkg})
	if err == nil {
		t.Fatal("Community package passed through Personal Add Skill")
	}
}

func TestZIPIngestRejectsTraversal(t *testing.T) {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	entry, err := writer.Create("../escape.md")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("nope")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = sourceDocumentsFromZIPBase64(base64.StdEncoding.EncodeToString(buffer.Bytes()))
	if err == nil {
		t.Fatal("ZIP traversal entry accepted")
	}
}

func TestGitHubArchiveSubpathKeepsKnowledgeOnly(t *testing.T) {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, content := range map[string]string{
		"repo-main/skills/react/SKILL.md": "# React\nReview UI.",
		"repo-main/skills/react/data.json": `{"rule":"test"}`,
		"repo-main/skills/other/SKILL.md": "# Other",
		"repo-main/skills/react/run.sh": "echo unsafe",
	} {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	documents, err := sourceDocumentsFromZIP(buffer.Bytes(), "skills/react", true)
	if err != nil {
		t.Fatalf("extract source archive: %v", err)
	}
	artifact, err := skillintel.BuildArtifactFromDocuments(skillintel.Manifest{
		ID: "personal.react", Name: "React", Version: "1", Scope: skillintel.ScopePersonal, Kind: skillintel.KindKnowledge,
	}, documents, skillintel.DefaultKnowledgeIngestPolicy())
	if err != nil {
		t.Fatalf("build artifact: %v", err)
	}
	if len(artifact.Chunks) != 2 {
		t.Fatalf("chunks=%d want 2; unsupported shell/other subtree must be ignored", len(artifact.Chunks))
	}
}

func TestSkillSourceURLSecurity(t *testing.T) {
	for _, raw := range []string{
		"http://github.com/example/repo",
		"https://user:secret@example.com/file.md",
		"https://example.com:8443/file.md",
	} {
		if _, err := validateSkillSourceURL(raw); err == nil {
			t.Fatalf("unsafe URL accepted: %s", raw)
		}
	}
	if _, err := validateSkillSourceURL("https://github.com/example/repo"); err != nil {
		t.Fatalf("safe URL rejected: %v", err)
	}
	for _, raw := range []string{"127.0.0.1", "10.0.0.1", "169.254.1.1", "::1", "fc00::1"} {
		if publicSkillSourceIP(netip.MustParseAddr(raw)) {
			t.Fatalf("private/unsafe address accepted: %s", raw)
		}
	}
	if !publicSkillSourceIP(netip.MustParseAddr("1.1.1.1")) {
		t.Fatal("public address unexpectedly rejected")
	}
}

func TestHTMLKnowledgeTextDropsExecutableMarkup(t *testing.T) {
	text := htmlKnowledgeText([]byte(`<html><style>.x{display:none}</style><body><h1>Guide</h1><script>alert(1)</script><p>Use &amp; verify.</p></body></html>`))
	if strings.Contains(text, "alert") || strings.Contains(text, "display:none") {
		t.Fatalf("script/style leaked into knowledge: %q", text)
	}
	if !strings.Contains(text, "Guide") || !strings.Contains(text, "Use & verify.") {
		t.Fatalf("visible HTML text missing: %q", text)
	}
}

func TestSkillHTTPClientDoesNotUseEnvironmentProxy(t *testing.T) {
	client := safeSkillHTTPClient()
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("unexpected transport type %T", client.Transport)
	}
	if transport.Proxy != nil {
		t.Fatal("Skill source transport must not delegate requests through environment proxy")
	}
}
