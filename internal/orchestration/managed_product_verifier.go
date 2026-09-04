package orchestration

import (
	"context"
	"errors"
	"strings"
	"time"
)

var ErrManagedProductDriver = errors.New("managed product driver failed")

// ProductDriver is the narrow adapter implemented by the existing Browser,
// Computer and managed Mobile backends. Agent OS owns verification semantics;
// each backend only owns how to launch/interact/observe/assert on its surface.
type ProductDriver interface {
	Launch(context.Context, ProductCheck) (sessionRef string, err error)
	Interact(context.Context, string, ProductCheck) error
	Observe(context.Context, string, ProductCheck) (artifactRef string, err error)
	Assert(context.Context, string, ProductCheck) (passed bool, failureCode string, err error)
	Close(context.Context, string) error
}

type ManagedProductVerifier struct {
	surface ProductSurface
	driver  ProductDriver
	timeout time.Duration
}

func NewManagedProductVerifier(surface ProductSurface, driver ProductDriver, timeout time.Duration) (*ManagedProductVerifier, error) {
	if !validProductSurface(surface) || driver == nil {
		return nil, ErrInvalidProductVerification
	}
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	if timeout > 15*time.Minute {
		timeout = 15 * time.Minute
	}
	return &ManagedProductVerifier{surface: surface, driver: driver, timeout: timeout}, nil
}

func (v *ManagedProductVerifier) Surface() ProductSurface {
	if v == nil {
		return ""
	}
	return v.surface
}

func (v *ManagedProductVerifier) Verify(parent context.Context, check ProductCheck) (ProductObservation, error) {
	started := time.Now().UTC()
	observation := ProductObservation{StartedAt: started}
	if v == nil || v.driver == nil || check.Surface != v.surface {
		observation.FinishedAt = time.Now().UTC()
		return observation, ErrInvalidProductVerification
	}
	ctx, cancel := context.WithTimeout(parent, v.timeout)
	defer cancel()

	sessionRef, err := v.driver.Launch(ctx, check)
	if err != nil || strings.TrimSpace(sessionRef) == "" {
		observation.FailureCode = "launch_failed"
		observation.FinishedAt = time.Now().UTC()
		if err == nil {
			err = ErrManagedProductDriver
		}
		return observation, err
	}
	closed := false
	closeDriver := func() error {
		if closed {
			return nil
		}
		closed = true
		return v.driver.Close(ctx, sessionRef)
	}
	defer func() { _ = closeDriver() }()

	if err := v.driver.Interact(ctx, sessionRef, check); err != nil {
		observation.FailureCode = "interaction_failed"
		observation.FinishedAt = time.Now().UTC()
		return observation, err
	}
	artifactRef, err := v.driver.Observe(ctx, sessionRef, check)
	observation.ArtifactRef = boundArtifactRef(artifactRef)
	if err != nil {
		observation.FailureCode = "observation_failed"
		observation.FinishedAt = time.Now().UTC()
		return observation, err
	}
	passed, failureCode, err := v.driver.Assert(ctx, sessionRef, check)
	if err != nil {
		observation.FailureCode = "assertion_error"
		observation.FinishedAt = time.Now().UTC()
		return observation, err
	}
	observation.Passed = passed
	observation.FailureCode = strings.ToLower(strings.TrimSpace(failureCode))
	if !passed && observation.FailureCode == "" {
		observation.FailureCode = "assertion_failed"
	}
	if err := closeDriver(); err != nil {
		observation.Passed = false
		observation.FailureCode = "cleanup_failed"
		observation.FinishedAt = time.Now().UTC()
		return observation, err
	}
	observation.FinishedAt = time.Now().UTC()
	return observation, nil
}

func boundArtifactRef(ref string) string {
	ref = strings.TrimSpace(ref)
	if len(ref) > 1024 {
		ref = ref[:1024]
	}
	return ref
}
