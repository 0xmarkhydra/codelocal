package mcpgateway

import (
	"context"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
)

const (
	semanticCanaryGateTTL          = 30 * time.Second
	semanticCanaryGateRetryTTL     = 5 * time.Second
	semanticCanaryGateProbeEvery   = 5 * time.Minute
	semanticCanaryGateMaxEntries   = 512
	semanticCanaryGateMinSamples   = int64(20)
	semanticCanaryGateMaxErrorRate = 0.10
	semanticCanaryGateMaxTimeout   = 0.05
	semanticCanaryGateMaxSlow      = 0.20
	semanticCanaryGateMaxBlocked   = 0.80
)

var semanticCanaryGateRefreshSlots = make(chan struct{}, 4)

type semanticCanaryGateEntry struct {
	Metrics     cloud.CanonicalSemanticCanaryMetrics
	ExpiresAt   time.Time
	NextProbeAt time.Time
	Refreshing  bool
	Available   bool
}

type semanticCanaryGateDecision struct {
	Allow  bool
	Status string
}

func semanticCanaryRate(value, total int64) float64 {
	if total <= 0 || value <= 0 {
		return 0
	}
	return float64(value) / float64(total)
}

func semanticCanaryDecisionFromMetrics(metrics cloud.CanonicalSemanticCanaryMetrics) semanticCanaryGateDecision {
	if metrics.AttemptsTotal < semanticCanaryGateMinSamples {
		return semanticCanaryGateDecision{Allow: true, Status: "collecting"}
	}
	errorsTotal := metrics.ReadinessErrorCount + metrics.RecallErrorCount
	if semanticCanaryRate(errorsTotal, metrics.AttemptsTotal) > semanticCanaryGateMaxErrorRate {
		return semanticCanaryGateDecision{Status: "blocked_error_rate"}
	}
	if semanticCanaryRate(metrics.TimeoutCount, metrics.AttemptsTotal) > semanticCanaryGateMaxTimeout {
		return semanticCanaryGateDecision{Status: "blocked_timeout_rate"}
	}
	if semanticCanaryRate(metrics.SlowCount, metrics.AttemptsTotal) > semanticCanaryGateMaxSlow {
		return semanticCanaryGateDecision{Status: "blocked_slow_rate"}
	}
	if semanticCanaryRate(metrics.ReadinessBlockedCount, metrics.AttemptsTotal) > semanticCanaryGateMaxBlocked {
		return semanticCanaryGateDecision{Status: "blocked_readiness_rate"}
	}
	return semanticCanaryGateDecision{Allow: true, Status: "healthy"}
}

func semanticCanaryGateKey(userID, projectID string) string {
	return strings.TrimSpace(userID) + "\x00" + strings.TrimSpace(projectID)
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
		decision := semanticCanaryDecisionFromMetrics(entry.Metrics)
		if !decision.Allow && !entry.NextProbeAt.IsZero() && !now.Before(entry.NextProbeAt) {
			entry.NextProbeAt = now.Add(semanticCanaryGateProbeEvery)
			s.semanticCanaryGates[key] = entry
			s.semanticCanaryMu.Unlock()
			return semanticCanaryGateDecision{Allow: true, Status: "half_open_probe"}
		}
		s.semanticCanaryMu.Unlock()
		return decision
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

func (s *Service) trimSemanticCanaryGatesLocked(now time.Time, keep string) {
	for key, entry := range s.semanticCanaryGates {
		if key != keep && !entry.Refreshing && !now.Before(entry.ExpiresAt) {
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
			if oldestKey == "" || entry.ExpiresAt.Before(oldest) {
				oldestKey, oldest = key, entry.ExpiresAt
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
	ttl := semanticCanaryGateTTL
	if !available {
		ttl = semanticCanaryGateRetryTTL
	}
	s.semanticCanaryMu.Lock()
	previous := s.semanticCanaryGates[key]
	nextProbeAt := previous.NextProbeAt
	if available {
		decision := semanticCanaryDecisionFromMetrics(metrics)
		if decision.Allow {
			nextProbeAt = time.Time{}
		} else if nextProbeAt.IsZero() {
			nextProbeAt = now.Add(semanticCanaryGateProbeEvery)
		}
	}
	s.semanticCanaryGates[key] = semanticCanaryGateEntry{
		Metrics: metrics, ExpiresAt: now.Add(ttl), NextProbeAt: nextProbeAt, Available: available,
	}
	s.semanticCanaryMu.Unlock()
}
