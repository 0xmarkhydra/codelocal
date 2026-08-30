package cloudserver

import (
	"context"
	"strings"
	"testing"
)

const testMediaHash = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func clearMediaEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"CODELOCAL_MEDIA_S3_ENDPOINT", "CODELOCAL_MEDIA_S3_BUCKET", "CODELOCAL_MEDIA_S3_ACCESS_KEY_ID", "CODELOCAL_MEDIA_S3_SECRET_ACCESS_KEY", "CODELOCAL_MEDIA_S3_REGION",
		"S3_ENDPOINT", "S3_BUCKET", "S3_ACCESS_KEY_ID", "S3_SECRET_ACCESS_KEY", "S3_REGION",
		"CODELOCAL_SKILL_STORAGE_ENDPOINT", "CODELOCAL_SKILL_STORAGE_BUCKET", "CODELOCAL_SKILL_STORAGE_ACCESS_KEY_ID", "CODELOCAL_SKILL_STORAGE_SECRET_ACCESS_KEY", "CODELOCAL_SKILL_STORAGE_REGION",
	} {
		t.Setenv(name, "")
	}
}

func TestMediaObjectKeyIsTenantScopedAndContentAddressed(t *testing.T) {
	first := mediaObjectKey("codelocal", "user-a", testMediaHash, "image/png")
	second := mediaObjectKey("codelocal", "user-b", testMediaHash, "image/png")
	if first == second {
		t.Fatal("visual objects must not deduplicate across users")
	}
	if !strings.Contains(first, "/sha256/01/"+testMediaHash+".png") {
		t.Fatalf("visual object is not content-addressed: %s", first)
	}
	if strings.Contains(first, "user-a") {
		t.Fatalf("raw user id must not appear in object key: %s", first)
	}
}

func TestValidateMediaPrepareRejectsUnsafeInput(t *testing.T) {
	max := int64(12 << 20)
	for _, input := range []mediaPrepareRequest{
		{SHA256: "bad", ContentType: "image/png", Size: 10},
		{SHA256: testMediaHash, ContentType: "text/plain", Size: 10},
		{SHA256: testMediaHash, ContentType: "image/png", Size: 0},
		{SHA256: testMediaHash, ContentType: "image/png", Size: max + 1},
	} {
		if err := validateMediaPrepare(input, max); err == nil {
			t.Fatalf("expected invalid media input to fail: %#v", input)
		}
	}
	if err := validateMediaPrepare(mediaPrepareRequest{SHA256: testMediaHash, ContentType: "image/webp", Size: 1024}, max); err != nil {
		t.Fatalf("valid image metadata rejected: %v", err)
	}
}

func TestMediaStoreIsOptionalButPartialConfigurationFails(t *testing.T) {
	clearMediaEnv(t)
	store, err := newS3MediaStoreFromEnvironment(context.Background())
	if err != nil || store != nil {
		t.Fatalf("empty media configuration should disable the feature: store=%v err=%v", store, err)
	}
	t.Setenv("CODELOCAL_MEDIA_S3_ENDPOINT", "https://objects.example.test")
	if _, err := newS3MediaStoreFromEnvironment(context.Background()); err == nil || !strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("partial media configuration must fail closed: %v", err)
	}
}

func TestMediaStoreFallsBackToSharedSkillStorage(t *testing.T) {
	clearMediaEnv(t)
	t.Setenv("CODELOCAL_SKILL_STORAGE_ENDPOINT", "https://objects.example.test")
	t.Setenv("CODELOCAL_SKILL_STORAGE_BUCKET", "shared-codelocal")
	t.Setenv("CODELOCAL_SKILL_STORAGE_ACCESS_KEY_ID", "test-access")
	t.Setenv("CODELOCAL_SKILL_STORAGE_SECRET_ACCESS_KEY", "test-secret")
	t.Setenv("CODELOCAL_SKILL_STORAGE_REGION", "ap-southeast-1")

	store, err := newS3MediaStoreFromEnvironment(context.Background())
	if err != nil {
		t.Fatalf("shared skill storage should initialize media store: %v", err)
	}
	if store == nil || store.bucket != "shared-codelocal" {
		t.Fatalf("expected shared S3 bucket fallback, got %#v", store)
	}
}

func TestNormalizeMediaPrefix(t *testing.T) {
	if got := normalizeMediaPrefix(" /code local/screens/ "); got != "code-local/screens" {
		t.Fatalf("unexpected normalized prefix: %q", got)
	}
}
