package mcpgateway

import (
	"context"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
)

const (
	semanticCanaryGateTTL            = 30 * time.Second
	semanticCanaryGateRetryTTL       = 5 * time.Second
	semanticCanaryGateProbeEvery     = 5 * time.Minute
	semanticCanaryGateMaxEntries     = 512
	semanticCanaryGateMinSamples     = 20
	semanticCanaryGateWindowSize     = 50
	semanticCanaryGateMaxErrorRate   = 0.10
	semanticCanaryGateMaxTimeout     = 0.05
	semanticCanaryGateMaxSlow        = 0.20
	semanticCanaryGateMaxBlocked     = 0.80
	semanticCanaryGateMaxNoValueRate = 0.90
)

var semanticCanaryGateRefreshSlots = make(chan struct{}, 4)

type semanticCanaryOutcome struct {
	Error            bool
	Timeout          bool
	Slow             bool
	ReadinessBlocked bool
	NoValue          bool
	Applied          bool
}

type semanticCanaryRollingWindow struct {
	Samples []semanticCanaryOutcome
}

func (w semanticCanaryRollingWindow) decision() semanticCanaryGateDecision {
	if len(w.Samples) < semanticCanaryGateMinSamples {
		return semanticCanaryGateDecision{Allow: true, Status: "collecting"}
	}
	var errorsTotal, timeoutTotal, slowTotal, readinessBlockedTotal, noValueTotal int
	for _, sample := range w.Samples {
		if sample.Error {
			errorsTotal++
		}
		if sample.Timeout {
			timeoutTotal++
		}
		if sample.Slow {
			slowTotal++
		}
		if sample.ReadinessBlocked {
			readinessBlockedTotal++
		}
		if sample.NoValue {
			noValueTotal++
		}
	}
	total := float64(len(w.Samples))
	if float64(errorsTotal)/total > semanticCanaryGateMaxErrorRate {
		return semanticCanaryGateDecision{Status: "blocked_error_rate"}
	}
	if float64(timeoutTotal)/total > semanticCanaryGateMaxTimeout {
		return semanticCanaryGateDecision{Status: "blocked_timeout_rate"}
	}
	if float64(slowTotal)/total > semanticCanaryGateMaxSlow {
		return semanticCanaryGateDecision{Status: "blocked_slow_rate"}
	}
	if float64(readinessBlockedTotal)/total > semanticCanaryGateMaxBlocked {
		return semanticCanaryGateDecision{Status: "blocked_readiness_rate"}
	}
	if float64(noValueTotal)/total > semanticCanaryGateMaxNoValueRate {
		return semanticCanaryGateDecision{Status: "blocked_no_value_rate"}
	}
	return semanticCanaryGateDecision{Allow: true, Status: "healthy"}
}

func (w semanticCanaryRollingWindow) append(sample semanticCanaryOutcome) semanticCanaryRollingWindow {
	samples := append(w.Samples, sample)
	if len(samples) > semanticCanaryGateWindowSize {
		samples = append([]semanticCanaryOutcome(nil), samples[len(samples)-semanticCanaryGateWindowSize:]...)
	}
	w.Samples = samples
	return w
}

type semanticCanaryGateEntry struct {
	Metrics       cloud.CanonicalSemanticCanaryMetrics
	Window        semanticCanaryRollingWindow
	ExpiresAt     time.Time
	RetryAfter    time.Time
	NextProbeAt   time.Time
	Refreshing    bool
	Available     bool
	ProbeInFlight bool
}

type semanticCanaryGateDecision struct {
	Allow  bool
	Status string
}

func semanticCanaryGateKey(userID, projectID string) string {
	return strings.TrimSpace(userID) + "\x00" + strings.TrimSpace(projectID)
}

func semanticCanaryOutcomeFrom(reason string, duration time.Duration, applied bool) semanticCanaryOutcome {
	outcome := semanticCanaryOutcome{Applied: applied, Slow: duration >= cloud.CanonicalEmbeddingShadowSlowDuration}
	switch strings.TrimSpace(reason) {
	case cloud.SemanticCanaryReasonReady:
		outcome.Applied = true
	case cloud.SemanticCanaryReasonNotReady:
		outcome.ReadinessBlocked = true
	case cloud.SemanticCanaryReasonReadinessError, cloud.SemanticCanaryReasonRecallError:
		outcome.Error = true
	case cloud.SemanticCanaryReasonTimeout:
		outcome.Timeout = true
	case cloud.SemanticCanaryReasonNoSimilarity, cloud.SemanticCanaryReasonNoUniqueClaims:
		outcome.NoValue = true
	}
	return outcome
}

func (s *Service) semanticCanaryGate(input cloud.CanonicalKnowledgeRecallInput) semanticCanaryGateDecision {
	if s == nil || s.Store == nil || strings.TrimSpace(input.UserID) == "" || strings.TrimSpace(input.ProjectID) == "" {
		return semanticCanaryGateDecision{Status: "scope_unavailable"}
	}
	key := semanticCanaryGateKey(input.UserID, input.ProjectID)
	now := time.Now()
	s.semanticCanaryMu.Lock()
	if s.semanticCanaryGates == nil {
		s.semanticCanaryGates = map[string]semanticCanaryGateEntry{}
	}
	entry, exists := s.semanticCanaryGates[key]
	if exists && entry.Available && now.Before(entry.ExpiresAt) {
		decision := entry.Window.decision()
		if !decision.Allow && !entry.ProbeInFlight && !entry.NextProbeAt.IsZero() && !now.Before(entry.NextProbeAt) {
			entry.NextProbeAt = now.Add(semanticCanaryGateProbeEvery)
			entry.ProbeInFlight = true
			s.semanticCanaryGates[key] = entry
			s.semanticCanaryMu.Unlock()
			return semanticCanaryGateDecision{Allow: true, Status: "half_open_probe"}
		}
		s.semanticCanaryMu.Unlock()
		return decision
	}
	if exists && !entry.Available && now.Before(entry.RetryAfter) {
		s.semanticCanaryMu.Unlock()
		return semanticCanaryGateDecision{Status: "cache_retry_wait"}
	}
	if !exists || !entry.Refreshing {
		entry.Refreshing = true
		s.semanticCanaryGates[key] = entry
		s.trimSemanticCanaryGatesLocked(now, key)
		s.semanticCanaryMu.Unlock()
		s.refreshSemanticCanaryGateAsync(input.UserID, input.ProjectID, key)
		return semanticCanaryGateDecision{Status: "cache_refreshing"}
	}
	s.semanticCanaryMu.Unlock()
	return semanticCanaryGateDecision{Status: "cache_refreshing"}
}

func (s *Service) observeSemanticCanaryOutcome(input cloud.CanonicalKnowledgeRecallInput, reason string, duration time.Duration, applied bool) {
	if s == nil || strings.TrimSpace(input.UserID) == "" || strings.TrimSpace(input.ProjectID) == "" {
		return
	}
	key := semanticCanaryGateKey(input.UserID, input.ProjectID)
	now := time.Now()
	outcome := semanticCanaryOutcomeFrom(reason, duration, applied)
	s.semanticCanaryMu.Lock()
	if s.semanticCanaryGates == nil {
		s.semanticCanaryGates = map[string]semanticCanaryGateEntry{}
	}
	entry := s.semanticCanaryGates[key]
	before := entry.Window.decision()
	probeSucceeded := entry.ProbeInFlight && outcome.Applied && !outcome.Error && !outcome.Timeout && !outcome.Slow && !outcome.ReadinessBlocked && !outcome.NoValue
	if probeSucceeded {
		entry.Window = semanticCanaryRollingWindow{}.append(outcome)
	} else {
		entry.Window = entry.Window.append(outcome)
	}
	after := entry.Window.decision()
	entry.Available = true
	entry.Refreshing = false
	entry.ProbeInFlight = false
	entry.RetryAfter = time.Time{}
	entry.ExpiresAt = now.Add(semanticCanaryGateTTL)
	if probeSucceeded || after.Allow {
		entry.NextProbeAt = time.Time{}
	} else if before.Allow || entry.NextProbeAt.IsZero() {
		entry.NextProbeAt = now.Add(semanticCanaryGateProbeEvery)
	}
	s.semanticCanaryGates[key] = entry
	s.trimSemanticCanaryGatesLocked(now, key)
	s.semanticCanaryMu.Unlock()
}

func (s *Service) trimSemanticCanaryGatesLocked(now time.Time, keep string) {
	for key, entry := range s.semanticCanaryGates {
		if key != keep && !entry.Refreshing && !now.Before(entry.ExpiresAt) && !now.Before(entry.RetryAfter) {
			delete(s.semanticCanaryGates, key)
		}
	}
	for len(s.semanticCanaryGates) > semanticCanaryGateMaxEntries {
		var oldestKey string
		var oldest time.Time
		for key, entry := range s.semanticCanaryGates {
			if key == keep || entry.Refreshing {
				continue
			}
			candidate := entry.ExpiresAt
			if candidate.IsZero() || (!entry.RetryAfter.IsZero() && entry.RetryAfter.Before(candidate)) {
				candidate = entry.RetryAfter
			}
			if oldestKey == "" || candidate.Before(oldest) {
				oldestKey, oldest = key, candidate
			}
		}
		if oldestKey == "" {
			return
		}
		delete(s.semanticCanaryGates, oldestKey)
	}
}

func (s *Service) refreshSemanticCanaryGateAsync(userID, projectID, key string) {
	select {
	case semanticCanaryGateRefreshSlots <- struct{}{}:
	default:
		s.finishSemanticCanaryGateRefresh(key, cloud.CanonicalSemanticCanaryMetrics{}, false)
		return
	}
	go func() {
		defer func() { <-semanticCanaryGateRefreshSlots }()
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		defer cancel()
		metrics, err := s.Store.CanonicalSemanticCanaryProjectMetrics(ctx, userID, projectID)
		s.finishSemanticCanaryGateRefresh(key, metrics, err == nil)
	}()
}

func (s *Service) finishSemanticCanaryGateRefresh(key string, metrics cloud.CanonicalSemanticCanaryMetrics, available bool) {
	if s == nil {
		return
	}
	now := time.Now()
	s.semanticCanaryMu.Lock()
	previous := s.semanticCanaryGates[key]
	previous.Refreshing = false
	previous.Metrics = metrics
	if available {
		previous.Available = true
		previous.RetryAfter = time.Time{}
		previous.ExpiresAt = now.Add(semanticCanaryGateTTL)
	} else {
		previous.Available = false
		previous.RetryAfter = now.Add(semanticCanaryGateRetryTTL)
		previous.ExpiresAt = time.Time{}
	}
	s.semanticCanaryGates[key] = previous
	s.semanticCanaryMu.Unlock()
}
