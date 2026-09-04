package agentruntime

import (
	"strings"
	"time"
)

type FallbackCause string

const (
	FallbackRateLimit       FallbackCause = "rate_limit"
	FallbackQuota           FallbackCause = "quota"
	FallbackAuth            FallbackCause = "auth"
	FallbackTransient       FallbackCause = "transient"
	FallbackContextOverflow FallbackCause = "context_overflow"
	FallbackUnavailable     FallbackCause = "unavailable"
)

type EngineTarget struct { EngineID string `json:"engineId"`; ModelID string `json:"modelId,omitempty"` }
func (target EngineTarget) key() string { return strings.ToLower(strings.TrimSpace(target.EngineID)) + "\x00" + strings.ToLower(strings.TrimSpace(target.ModelID)) }

type FallbackPolicy struct { Primary EngineTarget `json:"primary"`; Chain []EngineTarget `json:"chain,omitempty"`; MaxSwitches int `json:"maxSwitches,omitempty"`; Cooldown time.Duration `json:"cooldown,omitempty"` }
type FallbackState struct { Current EngineTarget `json:"current"`; Switches int `json:"switches"`; CooldownUntil map[string]time.Time `json:"cooldownUntil,omitempty"` }
type FallbackDecision struct { Switch bool `json:"switch"`; Target EngineTarget `json:"target"`; Reason FallbackCause `json:"reason"`; PersistEvent bool `json:"persistEvent"`; Exhausted bool `json:"exhausted"` }

func fallbackEligible(cause FallbackCause) bool { switch cause { case FallbackRateLimit, FallbackQuota, FallbackAuth, FallbackTransient, FallbackContextOverflow, FallbackUnavailable: return true; default: return false } }
func normalizeTarget(target EngineTarget) EngineTarget { target.EngineID = strings.ToLower(strings.TrimSpace(target.EngineID)); target.ModelID = strings.TrimSpace(target.ModelID); return target }

// NextFallback never retries by itself; callers persist the decision and start
// the selected engine through normal policy/isolation gates.
func NextFallback(policy FallbackPolicy, state FallbackState, cause FallbackCause, now time.Time) FallbackDecision {
	if !fallbackEligible(cause) { return FallbackDecision{Reason: cause} }
	if policy.MaxSwitches <= 0 { policy.MaxSwitches = 2 }
	if state.Switches >= policy.MaxSwitches { return FallbackDecision{Reason: cause, Exhausted: true} }
	if policy.Cooldown <= 0 { policy.Cooldown = 30 * time.Second }
	current := normalizeTarget(state.Current); if current.EngineID == "" { current = normalizeTarget(policy.Primary) }
	candidates := make([]EngineTarget, 0, 1+len(policy.Chain)); candidates = append(candidates, normalizeTarget(policy.Primary)); for _, candidate := range policy.Chain { candidates = append(candidates, normalizeTarget(candidate)) }
	cooldowns := state.CooldownUntil; if cooldowns == nil { cooldowns = map[string]time.Time{} }
	currentIndex := -1; for i, candidate := range candidates { if candidate.key() == current.key() { currentIndex = i; break } }
	for offset := 1; offset <= len(candidates); offset++ { index := (currentIndex + offset) % len(candidates); candidate := candidates[index]; if candidate.EngineID == "" || candidate.key() == current.key() { continue }; if until := cooldowns[candidate.key()]; until.After(now) { continue }; return FallbackDecision{Switch: true, Target: candidate, Reason: cause, PersistEvent: true} }
	return FallbackDecision{Reason: cause, Exhausted: true}
}

func ApplyFallbackDecision(state FallbackState, decision FallbackDecision, cooldown time.Duration, now time.Time) FallbackState { if !decision.Switch { return state }; if cooldown <= 0 { cooldown = 30 * time.Second }; if state.CooldownUntil == nil { state.CooldownUntil = map[string]time.Time{} }; if state.Current.EngineID != "" { state.CooldownUntil[normalizeTarget(state.Current).key()] = now.Add(cooldown) }; state.Current = normalizeTarget(decision.Target); state.Switches++; return state }
