package toolprogram

import (
	"context"
	"errors"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/security"
)

var ErrGuardedTokenBudget=errors.New("tool program reduced-token budget exceeded")

type RetryDecider func(security.ProjectedCapability,error)bool

type Guardrails struct{
	PerStepTimeout time.Duration
	MaxRetries int
	MaxReducedTokens int
	Retryable RetryDecider
}

// ExecuteGuarded reuses the canonical single-step executor so authorization,
// capability projection and semantic reduction stay identical while adding
// per-step timeout, bounded retry and a whole-program model-visible token cap.
// Retries are disabled unless Retryable explicitly marks the failed capability
// safe to repeat; mutating side effects are therefore never retried by default.
func (r *Runtime) ExecuteGuarded(parent context.Context,input Program,guard Guardrails)(Report,error){
	if r==nil{return Report{},ErrInvalidProgram}
	program,err:=normalizeProgram(input,r.limits);if err!=nil{return Report{},err}
	if guard.PerStepTimeout<=0{guard.PerStepTimeout=r.limits.Timeout};if guard.PerStepTimeout>5*time.Minute{guard.PerStepTimeout=5*time.Minute}
	if guard.MaxRetries<0{guard.MaxRetries=0};if guard.MaxRetries>3{guard.MaxRetries=3}
	if guard.MaxReducedTokens<=0{guard.MaxReducedTokens=r.limits.MaxStepTokens*len(program.Steps)}
	if guard.MaxReducedTokens>64_000{guard.MaxReducedTokens=64_000}
	report:=Report{ProgramID:program.ID,Steps:[]StepReport{}}
	for _,step:=range program.Steps{
		binding,ok:=r.bindings[step.CapabilityID];if !ok{return report,ErrCapabilityUnavailable}
		attempts:=0
		for{
			attempts++
			child:=*r;child.limits=r.limits;child.limits.Timeout=guard.PerStepTimeout;child.limits.MaxOperations=1
			partial,runErr:=child.Execute(parent,Program{ID:program.ID+":"+step.ID,Steps:[]Step{step}})
			if runErr==nil{
				if len(partial.Steps)!=1{return report,ErrProgramStep}
				report.Steps=append(report.Steps,partial.Steps[0]);report.OriginalTokens+=partial.OriginalTokens;report.ReducedTokens+=partial.ReducedTokens
				if report.ReducedTokens>guard.MaxReducedTokens{return report,ErrGuardedTokenBudget}
				break
			}
			if errors.Is(runErr,context.Canceled)||errors.Is(runErr,context.DeadlineExceeded){return report,runErr}
			if attempts>guard.MaxRetries || guard.Retryable==nil || !guard.Retryable(binding.Capability,runErr){return report,runErr}
		}
	}
	report.AvoidedTokens=report.OriginalTokens-report.ReducedTokens;if report.AvoidedTokens<0{report.AvoidedTokens=0};report.Completed=true;return report,nil
}
