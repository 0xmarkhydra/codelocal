package orchestration

import (
	"context"
	"testing"
)

type fakeProductVerifier struct { surface ProductSurface; observation ProductObservation; err error }
func (f fakeProductVerifier) Surface() ProductSurface { return f.surface }
func (f fakeProductVerifier) Verify(context.Context, ProductCheck) (ProductObservation, error) { return f.observation, f.err }

func TestProductVerificationProducesStandardEvidence(t *testing.T) {
	verifier := fakeProductVerifier{surface: ProductBrowser, observation: ProductObservation{Passed:true, ArtifactRef:"artifact://browser/login"}}
	evidence, err := RunProductVerification(context.Background(), verifier, ProductCheck{CheckID:"browser-login", Surface:ProductBrowser, Objective:"login succeeds", AgentID:"tester"})
	if err != nil { t.Fatal(err) }
	if evidence.Status != VerificationPassed || evidence.ArtifactRef == "" || evidence.FailureSignature != "" {
		t.Fatalf("unexpected product evidence: %+v", evidence)
	}
}

func TestProductVerificationFailureStoresSignatureNotRawObservation(t *testing.T) {
	verifier := fakeProductVerifier{surface: ProductMobile, observation: ProductObservation{Passed:false, ArtifactRef:"artifact://mobile/run", FailureCode:"button_not_found"}}
	evidence, err := RunProductVerification(context.Background(), verifier, ProductCheck{CheckID:"mobile-flow", Surface:ProductMobile, Objective:"complete flow"})
	if err != nil { t.Fatal(err) }
	if evidence.Status != VerificationFailed || evidence.FailureSignature == "" || evidence.ArtifactRef == "" {
		t.Fatalf("failure evidence incomplete: %+v", evidence)
	}
}

func TestProductVerificationRejectsSurfaceMismatch(t *testing.T) {
	verifier := fakeProductVerifier{surface: ProductBrowser}
	if _, err := RunProductVerification(context.Background(), verifier, ProductCheck{CheckID:"check", Surface:ProductComputer, Objective:"open app"}); err == nil {
		t.Fatal("surface mismatch should fail closed")
	}
}
