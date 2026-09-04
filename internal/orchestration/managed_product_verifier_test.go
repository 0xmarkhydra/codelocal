package orchestration

import (
	"context"
	"testing"
)

type fakeProductDriver struct{ passed bool }

func (f fakeProductDriver) Launch(context.Context, ProductCheck) (string, error) {
	return "session-1", nil
}
func (f fakeProductDriver) Interact(context.Context, string, ProductCheck) error { return nil }
func (f fakeProductDriver) Observe(context.Context, string, ProductCheck) (string, error) {
	return "artifact://shot-1", nil
}
func (f fakeProductDriver) Assert(context.Context, string, ProductCheck) (bool, string, error) {
	if f.passed {
		return true, "", nil
	}
	return false, "ui_mismatch", nil
}
func (f fakeProductDriver) Close(context.Context, string) error { return nil }

func TestManagedProductVerifierPassesThroughCanonicalEvidence(t *testing.T) {
	verifier, err := NewManagedProductVerifier(ProductMobile, fakeProductDriver{passed: true}, 0)
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := RunProductVerification(context.Background(), verifier, ProductCheck{CheckID: "mobile-login", Surface: ProductMobile, Objective: "open app and verify login"})
	if err != nil {
		t.Fatal(err)
	}
	if evidence.Status != VerificationPassed || evidence.ArtifactRef != "artifact://shot-1" || evidence.FailureSignature != "" {
		t.Fatalf("unexpected evidence: %+v", evidence)
	}
}

func TestManagedProductVerifierFailureIsNormalized(t *testing.T) {
	verifier, err := NewManagedProductVerifier(ProductBrowser, fakeProductDriver{passed: false}, 0)
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := RunProductVerification(context.Background(), verifier, ProductCheck{CheckID: "browser-ui", Surface: ProductBrowser, Objective: "verify page"})
	if err != nil {
		t.Fatal(err)
	}
	if evidence.Status != VerificationFailed || evidence.FailureSignature == "" {
		t.Fatalf("expected normalized failure evidence: %+v", evidence)
	}
}
