package orchestration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

type ProductSurface string

const (
	ProductBrowser  ProductSurface = "browser"
	ProductComputer ProductSurface = "computer"
	ProductMobile   ProductSurface = "mobile"
)

var ErrInvalidProductVerification = errors.New("invalid product verification")

type ProductCheck struct {
	CheckID    string         `json:"checkId"`
	Surface    ProductSurface `json:"surface"`
	Objective  string         `json:"objective"`
	AgentID    string         `json:"agentId,omitempty"`
	ArtifactHint string       `json:"artifactHint,omitempty"`
}

type ProductObservation struct {
	Passed      bool   `json:"passed"`
	ArtifactRef string `json:"artifactRef,omitempty"`
	FailureCode string `json:"failureCode,omitempty"`
	StartedAt   time.Time `json:"startedAt,omitempty"`
	FinishedAt  time.Time `json:"finishedAt,omitempty"`
}

type ProductVerifier interface {
	Surface() ProductSurface
	Verify(context.Context, ProductCheck) (ProductObservation, error)
}

// RunProductVerification converts browser/computer/mobile observations into the
// same durable VerificationEvidence used by code/test checks. Raw screenshots,
// DOM dumps and device logs stay behind ArtifactRef instead of entering runtime
// truth or model context by default.
func RunProductVerification(ctx context.Context, verifier ProductVerifier, input ProductCheck) (VerificationEvidence, error) {
	input.CheckID = strings.TrimSpace(input.CheckID)
	input.Objective = strings.Join(strings.Fields(input.Objective), " ")
	input.AgentID = strings.TrimSpace(input.AgentID)
	if verifier == nil || input.CheckID == "" || input.Objective == "" || !validProductSurface(input.Surface) || verifier.Surface() != input.Surface {
		return VerificationEvidence{}, ErrInvalidProductVerification
	}
	observation, err := verifier.Verify(ctx, input)
	if err != nil {
		return VerificationEvidence{CheckID: input.CheckID, AgentID: input.AgentID, Status: VerificationFailed, FailureSignature: productFailureSignature(input.Surface, "verifier_error"), StartedAt: observation.StartedAt, FinishedAt: observation.FinishedAt}, err
	}
	status := VerificationFailed
	failure := strings.TrimSpace(observation.FailureCode)
	if observation.Passed {
		status = VerificationPassed
		failure = ""
	} else if failure == "" {
		failure = "product_check_failed"
	}
	return VerificationEvidence{CheckID: input.CheckID, AgentID: input.AgentID, Status: status, ArtifactRef: strings.TrimSpace(observation.ArtifactRef), FailureSignature: productFailureSignature(input.Surface, failure), StartedAt: observation.StartedAt, FinishedAt: observation.FinishedAt}, nil
}

func validProductSurface(surface ProductSurface) bool { return surface == ProductBrowser || surface == ProductComputer || surface == ProductMobile }
func productFailureSignature(surface ProductSurface, code string) string {
	if strings.TrimSpace(code) == "" { return "" }
	sum := sha256.Sum256([]byte(string(surface)+"\x00"+strings.ToLower(strings.TrimSpace(code))))
	return "product_" + hex.EncodeToString(sum[:8])
}
