package cloudserver

import (
	"strings"
	"testing"
)

func TestValidateVideoArtifactPrepareAcceptsOnlyMP4(t *testing.T) {
	validHash := strings.Repeat("a", 64)
	if err := validateVideoArtifactPrepare(videoArtifactPrepareRequest{SHA256: validHash, ContentType: "video/mp4", Size: 1024}, 2048); err != nil {
		t.Fatal(err)
	}
	for _, input := range []videoArtifactPrepareRequest{
		{SHA256: "bad", ContentType: "video/mp4", Size: 1024},
		{SHA256: validHash, ContentType: "image/png", Size: 1024},
		{SHA256: validHash, ContentType: "video/mp4", Size: 0},
		{SHA256: validHash, ContentType: "video/mp4", Size: 4096},
	} {
		if err := validateVideoArtifactPrepare(input, 2048); err == nil {
			t.Fatalf("expected invalid artifact to fail: %#v", input)
		}
	}
}

func TestVideoArtifactNamespaceIsDurableAndSeparateFromVisualCleanup(t *testing.T) {
	hash := strings.Repeat("b", 64)
	key := videoArtifactObjectKey("codelocal", "0123456789abcdef", hash)
	if !strings.HasPrefix(key, "codelocal/artifacts/users/0123456789abcdef/") || !strings.HasSuffix(key, ".mp4") {
		t.Fatalf("unexpected artifact key: %s", key)
	}
	if strings.HasPrefix(key, "codelocal/users/") {
		t.Fatalf("video artifact overlaps temporary visual namespace: %s", key)
	}
}

func TestVideoArtifactPublicURLSupportsCDNAndStableGatewayFallback(t *testing.T) {
	hash := strings.Repeat("c", 64)
	key := videoArtifactObjectKey("codelocal", "0123456789abcdef", hash)
	t.Setenv("CODELOCAL_ARTIFACT_PUBLIC_BASE_URL", "https://media.example.test")
	if got := videoArtifactPublicURL("0123456789abcdef", hash, key); got != "https://media.example.test/"+key {
		t.Fatalf("cdn public URL=%s", got)
	}
	t.Setenv("CODELOCAL_ARTIFACT_PUBLIC_BASE_URL", "")
	t.Setenv("PUBLIC_BASE_URL", "https://codelocal.example.test")
	want := "https://codelocal.example.test/api/public/artifacts/0123456789abcdef/" + hash + ".mp4"
	if got := videoArtifactPublicURL("0123456789abcdef", hash, key); got != want {
		t.Fatalf("gateway public URL=%s want=%s", got, want)
	}
}
