package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"sync"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/skills"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

type fakeSkillS3 struct {
	mu      sync.Mutex
	objects map[string][]byte
}

func newFakeSkillS3() *fakeSkillS3 {
	return &fakeSkillS3{objects: map[string][]byte{}}
}

func (f *fakeSkillS3) PutObject(_ context.Context, input *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	payload, err := io.ReadAll(input.Body)
	if err != nil {
		return nil, err
	}
	f.mu.Lock()
	f.objects[aws.ToString(input.Key)] = payload
	f.mu.Unlock()
	return &s3.PutObjectOutput{}, nil
}

func (f *fakeSkillS3) GetObject(_ context.Context, input *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	f.mu.Lock()
	payload, ok := f.objects[aws.ToString(input.Key)]
	f.mu.Unlock()
	if !ok {
		return nil, fakeSkillS3Error{code: "NoSuchKey"}
	}
	copyPayload := append([]byte(nil), payload...)
	return &s3.GetObjectOutput{
		Body:          io.NopCloser(bytes.NewReader(copyPayload)),
		ContentLength: aws.Int64(int64(len(copyPayload))),
	}, nil
}

type fakeSkillS3Error struct{ code string }

func (e fakeSkillS3Error) Error() string              { return e.code }
func (e fakeSkillS3Error) ErrorCode() string          { return e.code }
func (e fakeSkillS3Error) ErrorMessage() string       { return e.code }
func (e fakeSkillS3Error) ErrorFault() smithy.ErrorFault { return smithy.FaultClient }

func TestS3SkillPackageStoreRoundTrip(t *testing.T) {
	client := newFakeSkillS3()
	store, err := newS3SkillPackageStore(client, "skills-bucket", "codelocal/packages")
	if err != nil {
		t.Fatal(err)
	}
	pkg := cloudTestSkillPackage(t)
	ctx := context.Background()
	if err := store.Put(ctx, pkg); err != nil {
		t.Fatal(err)
	}
	loaded, ok, err := store.Get(ctx, pkg.PackageHash)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || loaded.PackageHash != pkg.PackageHash || loaded.Manifest.ID != pkg.Manifest.ID {
		t.Fatalf("unexpected S3 package round trip: ok=%v package=%#v", ok, loaded)
	}
	uri, err := store.ObjectURI(pkg.PackageHash)
	if err != nil {
		t.Fatal(err)
	}
	if uri == "" || bytes.Contains([]byte(uri), []byte(pkg.Manifest.ID)) {
		t.Fatalf("content-addressed URI must not depend on skill id: %q", uri)
	}
}

func TestS3SkillPackageStoreReturnsMissing(t *testing.T) {
	store, _ := newS3SkillPackageStore(newFakeSkillS3(), "skills-bucket", "")
	_, ok, err := store.Get(context.Background(), "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("missing package should not be reported as present")
	}
}

func TestS3SkillPackageStoreRejectsTamperedObject(t *testing.T) {
	client := newFakeSkillS3()
	store, _ := newS3SkillPackageStore(client, "skills-bucket", "")
	pkg := cloudTestSkillPackage(t)
	ctx := context.Background()
	if err := store.Put(ctx, pkg); err != nil {
		t.Fatal(err)
	}
	key, _ := store.objectKey(pkg.PackageHash)
	var tampered skills.Package
	if err := json.Unmarshal(client.objects[key], &tampered); err != nil {
		t.Fatal(err)
	}
	tampered.Manifest.Intents = []string{"deploy_production"}
	payload, _ := json.Marshal(tampered)
	client.objects[key] = payload
	if _, _, err := store.Get(ctx, pkg.PackageHash); err == nil {
		t.Fatal("tampered object must fail package integrity validation")
	}
}

func cloudTestSkillPackage(t *testing.T) skills.Package {
	t.Helper()
	manifest := skills.Manifest{
		ID:        "cloud-review",
		Name:      "Cloud Review",
		Version:   "1.0.0",
		Publisher: "user",
		Scope:     skills.ScopePersonal,
		Kind:      skills.KindKnowledge,
		Quality:   0.5,
	}
	artifact, err := skills.BuildArtifact(manifest, []skills.KnowledgeChunk{{
		ID: "knowledge", SkillID: manifest.ID, SkillVersion: manifest.Version, Content: "Reusable cloud knowledge.",
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
