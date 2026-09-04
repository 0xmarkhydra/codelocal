package taskexecution

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"strings"
)

var ErrInvalidRuntimeGeneration = errors.New("invalid runtime generation configuration")

type RuntimeRollout struct {
	Enabled bool `json:"enabled"`
	Percent int  `json:"percent"`
	ForceV1 bool `json:"forceV1,omitempty"`
	ForceV2 bool `json:"forceV2,omitempty"`
}

// SelectRuntimeGeneration pins a task deterministically. Once stored on Bundle,
// callers must keep using that generation for the task lifetime; reconnects and
// retries never silently move a running task between V1 and V2.
func SelectRuntimeGeneration(taskID string, rollout RuntimeRollout) (RuntimeGeneration, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" || rollout.Percent < 0 || rollout.Percent > 100 || (rollout.ForceV1 && rollout.ForceV2) {
		return "", ErrInvalidRuntimeGeneration
	}
	if rollout.ForceV1 || !rollout.Enabled {
		return RuntimeV1, nil
	}
	if rollout.ForceV2 {
		return RuntimeV2, nil
	}
	if rollout.Percent == 0 {
		return RuntimeV1, nil
	}
	if rollout.Percent == 100 {
		return RuntimeV2, nil
	}
	sum := sha256.Sum256([]byte(taskID))
	bucket := int(binary.BigEndian.Uint32(sum[:4]) % 100)
	if bucket < rollout.Percent {
		return RuntimeV2, nil
	}
	return RuntimeV1, nil
}

func PinRuntimeGeneration(bundle Bundle, generation RuntimeGeneration) (Bundle, error) {
	if generation != RuntimeV1 && generation != RuntimeV2 {
		return Bundle{}, ErrInvalidRuntimeGeneration
	}
	if bundle.RuntimeGeneration != "" && bundle.RuntimeGeneration != generation {
		return Bundle{}, ErrInvalidRuntimeGeneration
	}
	bundle.RuntimeGeneration = generation
	return bundle, nil
}
