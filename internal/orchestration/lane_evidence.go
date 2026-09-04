package orchestration

import (
	"errors"
	"strings"
	"time"
)

var ErrInvalidLaneEvidence = errors.New("invalid lane evidence")

// LaneEvidence converts one lane check outcome (plan O adapters) into the
// canonical VerificationEvidence input. Check IDs are namespaced per lane so
// browser, computer, mobile and unit results can never collide; lane grouping
// on the graph side keys off the requirement category with the same name.
func LaneEvidence(lane Lane, name string, passed bool, exitCode *int, artifactRef string, at time.Time) (VerificationEvidence, error) {
	switch lane {
	case LaneBrowser, LaneComputer, LaneMobile, LaneUnit:
	default:
		return VerificationEvidence{}, ErrInvalidLaneEvidence
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return VerificationEvidence{}, ErrInvalidLaneEvidence
	}
	status := VerificationPassed
	if !passed {
		status = VerificationFailed
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	return VerificationEvidence{CheckID: string(lane) + ":" + name, Status: status, ExitCode: exitCode, ArtifactRef: strings.TrimSpace(artifactRef), FinishedAt: at}, nil
}
