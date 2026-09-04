package toolprogram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/contextsurface"
	"github.com/0xmarkhydra/codelocal/internal/security"
)

var (
	ErrInvalidProgram        = errors.New("invalid tool program")
	ErrCapabilityUnavailable = errors.New("tool program capability unavailable")
	ErrApprovalRequired      = errors.New("tool program capability requires approval")
	ErrProgramLimit          = errors.New("tool program resource limit exceeded")
	ErrProgramStep           = errors.New("tool program step failed")
)

type Step struct {
	ID           string         `json:"id"`
	CapabilityID string         `json:"capabilityId"`
	Args         map[string]any `json:"args,omitempty"`
}

type Program struct {
	ID    string `json:"id"`
	Steps []Step `json:"steps"`
}

type HandlerResult struct {
	Text        string                         `json:"-"`
	ArtifactRef string                         `json:"artifactRef,omitempty"`
	ExitCode    *int                           `json:"exitCode,omitempty"`
	Kind        contextsurface.ObservationKind `json:"kind,omitempty"`
	Source      string                         `json:"source,omitempty"`
}

type Handler func(context.Context, map[string]any) (HandlerResult, error)
type Authorizer func(context.Context, security.ProjectedCapability) error

type Binding struct {
	Capability security.ProjectedCapability
	Handler    Handler
}

type Limits struct {
	MaxOperations int           `json:"maxOperations"`
	MaxArgsBytes  int           `json:"maxArgsBytes"`
	MaxRawBytes   int           `json:"maxRawBytes"`
	MaxStepTokens int           `json:"maxStepTokens"`
	Timeout       time.Duration `json:"timeout"`
}

type StepReport struct {
	StepID         string                            `json:"stepId"`
	CapabilityID   string                            `json:"capabilityId"`
	Observation    contextsurface.ReducedObservation `json:"observation"`
	OriginalTokens int                               `json:"originalTokens"`
	ReducedTokens  int                               `json:"reducedTokens"`
}

type Report struct {
	ProgramID      string       `json:"programId"`
	Steps          []StepReport `json:"steps"`
	OriginalTokens int          `json:"originalTokens"`
	ReducedTokens  int          `json:"reducedTokens"`
	AvoidedTokens  int          `json:"avoidedTokens"`
	Completed      bool         `json:"completed"`
}

type Runtime struct {
	bindings   map[string]Binding
	limits     Limits
	authorizer Authorizer
}

func New(bindings []Binding, limits Limits, authorizer Authorizer) (*Runtime, error) {
	limits = normalizeLimits(limits)
	mapped := map[string]Binding{}
	for _, binding := range bindings {
		id := strings.TrimSpace(binding.Capability.ID)
		if id == "" || binding.Handler == nil {
			return nil, ErrInvalidProgram
		}
		if binding.Capability.Visibility != security.CapabilityAvailable && binding.Capability.Visibility != security.CapabilityPromptable {
			return nil, ErrInvalidProgram
		}
		if _, exists := mapped[id]; exists {
			return nil, ErrInvalidProgram
		}
		binding.Capability.ID = id
		mapped[id] = binding
	}
	return &Runtime{bindings: mapped, limits: limits, authorizer: authorizer}, nil
}

// Execute interprets a bounded declarative program. The program has no ambient
// filesystem, environment, process or network access: all effects must cross a
// projected capability binding. Raw tool output is reduced before it enters the
// report/model surface; large raw data should remain behind ArtifactRef.
func (r *Runtime) Execute(parent context.Context, input Program) (Report, error) {
	program, err := normalizeProgram(input, r.limits)
	if err != nil {
		return Report{}, err
	}
	ctx := parent
	cancel := func() {}
	if r.limits.Timeout > 0 {
		ctx, cancel = context.WithTimeout(parent, r.limits.Timeout)
	}
	defer cancel()
	report := Report{ProgramID: program.ID, Steps: make([]StepReport, 0, len(program.Steps))}
	for _, step := range program.Steps {
		if err := ctx.Err(); err != nil {
			return report, fmt.Errorf("%w: context deadline", ErrProgramLimit)
		}
		binding, ok := r.bindings[step.CapabilityID]
		if !ok {
			return report, ErrCapabilityUnavailable
		}
		if binding.Capability.Visibility == security.CapabilityPromptable {
			if r.authorizer == nil {
				return report, ErrApprovalRequired
			}
			if err := r.authorizer(ctx, binding.Capability); err != nil {
				return report, ErrApprovalRequired
			}
		}
		result, err := binding.Handler(ctx, cloneArgs(step.Args))
		if err != nil {
			return report, fmt.Errorf("%w: step=%s", ErrProgramStep, step.ID)
		}
		result.Text = boundRaw(result.Text, r.limits.MaxRawBytes)
		reduced := contextsurface.ReduceObservation(contextsurface.ToolObservation{
			ID:   "toolprogram:" + program.ID + ":" + step.ID,
			Kind: result.Kind, Tool: binding.Capability.Tool, Operation: binding.Capability.Action,
			Text: result.Text, Source: result.Source, RawArtifactRef: result.ArtifactRef, ExitCode: result.ExitCode,
		}, r.limits.MaxStepTokens)
		stepReport := StepReport{StepID: step.ID, CapabilityID: step.CapabilityID, Observation: reduced, OriginalTokens: reduced.OriginalTokens, ReducedTokens: reduced.ReducedTokens}
		report.Steps = append(report.Steps, stepReport)
		report.OriginalTokens += reduced.OriginalTokens
		report.ReducedTokens += reduced.ReducedTokens
	}
	report.AvoidedTokens = report.OriginalTokens - report.ReducedTokens
	if report.AvoidedTokens < 0 {
		report.AvoidedTokens = 0
	}
	report.Completed = true
	return report, nil
}

func normalizeProgram(program Program, limits Limits) (Program, error) {
	program.ID = strings.TrimSpace(program.ID)
	if program.ID == "" || len(program.Steps) == 0 || len(program.Steps) > limits.MaxOperations {
		return Program{}, ErrInvalidProgram
	}
	seen := map[string]struct{}{}
	for index := range program.Steps {
		step := &program.Steps[index]
		step.ID = strings.TrimSpace(step.ID)
		step.CapabilityID = strings.TrimSpace(step.CapabilityID)
		if step.ID == "" || step.CapabilityID == "" {
			return Program{}, ErrInvalidProgram
		}
		if _, exists := seen[step.ID]; exists {
			return Program{}, ErrInvalidProgram
		}
		seen[step.ID] = struct{}{}
		raw, err := json.Marshal(step.Args)
		if err != nil || len(raw) > limits.MaxArgsBytes {
			return Program{}, ErrProgramLimit
		}
	}
	return program, nil
}

func normalizeLimits(limits Limits) Limits {
	if limits.MaxOperations <= 0 {
		limits.MaxOperations = 32
	}
	if limits.MaxOperations > 256 {
		limits.MaxOperations = 256
	}
	if limits.MaxArgsBytes <= 0 {
		limits.MaxArgsBytes = 64 * 1024
	}
	if limits.MaxArgsBytes > 1024*1024 {
		limits.MaxArgsBytes = 1024 * 1024
	}
	if limits.MaxRawBytes <= 0 {
		limits.MaxRawBytes = 1024 * 1024
	}
	if limits.MaxRawBytes > 8*1024*1024 {
		limits.MaxRawBytes = 8 * 1024 * 1024
	}
	if limits.MaxStepTokens <= 0 {
		limits.MaxStepTokens = 512
	}
	if limits.MaxStepTokens > 4096 {
		limits.MaxStepTokens = 4096
	}
	if limits.Timeout <= 0 {
		limits.Timeout = 30 * time.Second
	}
	if limits.Timeout > 10*time.Minute {
		limits.Timeout = 10 * time.Minute
	}
	return limits
}

func boundRaw(value string, maxBytes int) string {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value
	}
	if maxBytes <= 64 {
		return value[:maxBytes]
	}
	head := maxBytes * 3 / 4
	tail := maxBytes - head - len("\n… raw output bounded …\n")
	if tail < 0 {
		tail = 0
	}
	return value[:head] + "\n… raw output bounded …\n" + value[len(value)-tail:]
}

func cloneArgs(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	raw, _ := json.Marshal(input)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return out
}
