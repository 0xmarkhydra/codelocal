package agentruntime

import (
	"testing"
	"time"
)

func TestNextFallbackSwitchesAndStopsAtBudget(t *testing.T) {
	policy := FallbackPolicy{Primary: EngineTarget{EngineID: "codex"}, Chain: []EngineTarget{{EngineID: "claude"}, {EngineID: "gemini"}}, MaxSwitches: 1}
	state := FallbackState{Current: EngineTarget{EngineID: "codex"}}
	now := time.Now().UTC()
	decision := NextFallback(policy, state, FallbackRateLimit, now)
	if !decision.Switch || decision.Target.EngineID != "claude" || !decision.PersistEvent {
		t.Fatalf("unexpected decision: %+v", decision)
	}
	state = ApplyFallbackDecision(state, decision, time.Minute, now)
	if next := NextFallback(policy, state, FallbackTransient, now); !next.Exhausted {
		t.Fatalf("switch budget must stop fallback: %+v", next)
	}
}

func TestNextFallbackSkipsCooldownTarget(t *testing.T) {
	now := time.Now().UTC()
	policy := FallbackPolicy{Primary: EngineTarget{EngineID: "codex"}, Chain: []EngineTarget{{EngineID: "claude"}, {EngineID: "gemini"}}, MaxSwitches: 3}
	state := FallbackState{Current: EngineTarget{EngineID: "codex"}, CooldownUntil: map[string]time.Time{EngineTarget{EngineID: "claude"}.key(): now.Add(time.Minute)}}
	decision := NextFallback(policy, state, FallbackQuota, now)
	if !decision.Switch || decision.Target.EngineID != "gemini" {
		t.Fatalf("cooldown target was not skipped: %+v", decision)
	}
}
