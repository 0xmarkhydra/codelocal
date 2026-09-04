package security

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	codelocalstate "github.com/0xmarkhydra/codelocal/internal/state"
)

type LeaseStatus string

const (
	LeaseActive   LeaseStatus = "active"
	LeaseConsumed LeaseStatus = "consumed"
	LeaseRevoked  LeaseStatus = "revoked"
	LeaseExpired  LeaseStatus = "expired"
)

var (
	ErrInvalidCapabilityLease = errors.New("invalid capability lease")
	ErrCapabilityLeaseUsed    = errors.New("capability lease is no longer active")
	ErrCapabilityLeaseScope   = errors.New("capability lease scope mismatch")
	ErrCapabilityLeaseExpired = errors.New("capability lease expired")
)

type CapabilityLease struct {
	ID              string      `json:"id"`
	WorkspaceKey    string      `json:"workspaceKey"`
	ActorID         string      `json:"actorId"`
	AllowedAgentIDs []string    `json:"allowedAgentIds,omitempty"`
	ApprovalKey     string      `json:"approvalKey"`
	ParentID        string      "parentId,omitempty"
	DelegatedBy     string      "delegatedBy,omitempty"
	Status          LeaseStatus `json:"status"`
	IssuedAt        time.Time   `json:"issuedAt"`
	ExpiresAt       time.Time   `json:"expiresAt"`
	ConsumedAt      time.Time   `json:"consumedAt,omitempty"`
	RevokedAt       time.Time   `json:"revokedAt,omitempty"`
}

type CapabilityLeaseIssue struct {
	ID              string
	WorkspaceKey    string
	ActorID         string
	AllowedAgentIDs []string
	ApprovalKey     string
	TTL             time.Duration
}

type AuthorizationResult struct {
	Allowed  bool           `json:"allowed"`
	Decision PolicyDecision `json:"decision"`
	LeaseID  string         `json:"leaseId,omitempty"`
	Reason   string         `json:"reason"`
}

type CapabilityLeaseStore struct {
	root string
	now  func() time.Time
}

func NewCapabilityLeaseStore(root string) (*CapabilityLeaseStore, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		root = filepath.Join(codelocalstate.Dir(), "capability-leases")
	}
	if err := codelocalstate.EnsurePrivateDir(root); err != nil {
		return nil, err
	}
	return &CapabilityLeaseStore{root: root, now: func() time.Time { return time.Now().UTC() }}, nil
}

func (s *CapabilityLeaseStore) Issue(input CapabilityLeaseIssue) (CapabilityLease, error) {
	input.WorkspaceKey = strings.TrimSpace(input.WorkspaceKey)
	input.ActorID = strings.TrimSpace(input.ActorID)
	input.ApprovalKey = strings.TrimSpace(input.ApprovalKey)
	input.ID = strings.TrimSpace(input.ID)
	if input.WorkspaceKey == "" || input.ActorID == "" || input.ApprovalKey == "" {
		return CapabilityLease{}, ErrInvalidCapabilityLease
	}
	if input.ID == "" {
		input.ID = "lease_" + randomLeaseID()
	}
	agents := normalizeLeaseAgents(input.AllowedAgentIDs)
	ttl := input.TTL
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	if ttl > time.Hour {
		ttl = time.Hour
	}
	now := s.now().UTC()
	lease := CapabilityLease{
		ID: input.ID, WorkspaceKey: input.WorkspaceKey, ActorID: input.ActorID,
		AllowedAgentIDs: agents, ApprovalKey: input.ApprovalKey, Status: LeaseActive,
		IssuedAt: now, ExpiresAt: now.Add(ttl),
	}
	if err := s.ensureLeaseIDUnused(lease.WorkspaceKey, lease.ID); err != nil {
		return CapabilityLease{}, err
	}
	if err := writeLeaseExclusive(s.path(lease.WorkspaceKey, LeaseActive, lease.ID), lease); err != nil {
		if errors.Is(err, os.ErrExist) {
			return CapabilityLease{}, ErrInvalidCapabilityLease
		}
		return CapabilityLease{}, err
	}
	return lease, nil
}

// Authorize never turns a DENY into success. ALLOW needs no lease. PROMPT may
// be satisfied only by atomically consuming a matching active one-shot lease.
func (s *CapabilityLeaseStore) Authorize(workspaceKey string, req PolicyRequest, decision PolicyDecision, leaseID string) (AuthorizationResult, error) {
	if decision.Effect == EffectDeny {
		return AuthorizationResult{Allowed: false, Decision: decision, Reason: "policy_denied"}, nil
	}
	if decision.Effect == EffectAllow {
		return AuthorizationResult{Allowed: true, Decision: decision, Reason: "policy_allowed"}, nil
	}
	if decision.Effect != EffectPrompt || decision.ApprovalKey == "" {
		return AuthorizationResult{}, ErrInvalidCapabilityLease
	}
	normalized := normalizePolicyRequest(req)
	expectedKey := policyApprovalKey(normalized, effectiveSecretMode(normalized))
	if expectedKey != decision.ApprovalKey {
		return AuthorizationResult{}, ErrCapabilityLeaseScope
	}
	lease, err := s.consume(workspaceKey, leaseID, normalized.ActorID, normalized.AgentID, decision.ApprovalKey)
	if err != nil {
		return AuthorizationResult{Allowed: false, Decision: decision, Reason: "lease_rejected"}, err
	}
	return AuthorizationResult{Allowed: true, Decision: decision, LeaseID: lease.ID, Reason: "lease_consumed"}, nil
}

func (s *CapabilityLeaseStore) Revoke(workspaceKey, leaseID string) (CapabilityLease, error) {
	lease, err := s.loadActive(workspaceKey, leaseID)
	if err != nil {
		return CapabilityLease{}, err
	}
	now := s.now().UTC()
	lease.Status = LeaseRevoked
	lease.RevokedAt = now
	if err := s.claim(workspaceKey, leaseID, LeaseRevoked); err != nil {
		return CapabilityLease{}, err
	}
	if err := codelocalstate.WriteJSONAtomic(s.path(workspaceKey, LeaseRevoked, leaseID), lease); err != nil {
		return CapabilityLease{}, err
	}
	return lease, nil
}

func (s *CapabilityLeaseStore) consume(workspaceKey, leaseID, actorID, agentID, approvalKey string) (CapabilityLease, error) {
	lease, err := s.loadActive(workspaceKey, leaseID)
	if err != nil {
		return CapabilityLease{}, err
	}
	now := s.now().UTC()
	if !now.Before(lease.ExpiresAt) {
		_ = s.claim(workspaceKey, leaseID, LeaseExpired)
		lease.Status = LeaseExpired
		_ = codelocalstate.WriteJSONAtomic(s.path(workspaceKey, LeaseExpired, leaseID), lease)
		return CapabilityLease{}, ErrCapabilityLeaseExpired
	}
	if lease.ActorID != actorID || lease.ApprovalKey != approvalKey || !leaseAllowsAgent(lease, agentID) {
		return CapabilityLease{}, ErrCapabilityLeaseScope
	}
	if err := s.claim(workspaceKey, leaseID, LeaseConsumed); err != nil {
		return CapabilityLease{}, err
	}
	lease.Status = LeaseConsumed
	lease.ConsumedAt = now
	if err := codelocalstate.WriteJSONAtomic(s.path(workspaceKey, LeaseConsumed, leaseID), lease); err != nil {
		// Active token has already been atomically removed: fail closed rather than
		// resurrecting a capability after an audit-write failure.
		return CapabilityLease{}, err
	}
	return lease, nil
}

func (s *CapabilityLeaseStore) loadActive(workspaceKey, leaseID string) (CapabilityLease, error) {
	workspaceKey = strings.TrimSpace(workspaceKey)
	leaseID = strings.TrimSpace(leaseID)
	if workspaceKey == "" || leaseID == "" {
		return CapabilityLease{}, ErrInvalidCapabilityLease
	}
	var lease CapabilityLease
	if err := codelocalstate.ReadJSON(s.path(workspaceKey, LeaseActive, leaseID), &lease); err != nil {
		if os.IsNotExist(err) {
			return CapabilityLease{}, ErrCapabilityLeaseUsed
		}
		return CapabilityLease{}, err
	}
	if lease.ID != leaseID || lease.WorkspaceKey != workspaceKey || lease.Status != LeaseActive || lease.ActorID == "" || lease.ApprovalKey == "" || lease.IssuedAt.IsZero() || lease.ExpiresAt.IsZero() {
		return CapabilityLease{}, ErrInvalidCapabilityLease
	}
	return lease, nil
}

func (s *CapabilityLeaseStore) claim(workspaceKey, leaseID string, status LeaseStatus) error {
	from := s.path(workspaceKey, LeaseActive, leaseID)
	to := s.path(workspaceKey, status, leaseID)
	if err := codelocalstate.EnsurePrivateDir(filepath.Dir(to)); err != nil {
		return err
	}
	if err := os.Rename(from, to); err != nil {
		if os.IsNotExist(err) {
			return ErrCapabilityLeaseUsed
		}
		return err
	}
	return nil
}

func (s *CapabilityLeaseStore) ensureLeaseIDUnused(workspaceKey, leaseID string) error {
	for _, status := range []LeaseStatus{LeaseActive, LeaseConsumed, LeaseRevoked, LeaseExpired} {
		if _, err := os.Stat(s.path(workspaceKey, status, leaseID)); err == nil {
			return ErrInvalidCapabilityLease
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func (s *CapabilityLeaseStore) path(workspaceKey string, status LeaseStatus, leaseID string) string {
	return filepath.Join(s.root, "workspace_"+leaseDigest(workspaceKey), string(status), "lease_"+leaseDigest(leaseID)+".json")
}

func writeLeaseExclusive(path string, lease CapabilityLease) error {
	if err := codelocalstate.EnsurePrivateDir(filepath.Dir(path)); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(lease, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	remove := true
	defer func() {
		_ = file.Close()
		if remove {
			_ = os.Remove(path)
		}
	}()
	if _, err := file.Write(payload); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	remove = false
	return nil
}

func leaseAllowsAgent(lease CapabilityLease, agentID string) bool {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return len(lease.AllowedAgentIDs) == 0
	}
	for _, allowed := range lease.AllowedAgentIDs {
		if allowed == agentID {
			return true
		}
	}
	return false
}

func normalizeLeaseAgents(values []string) []string {
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return uniquePolicyStrings(out)
}

func leaseDigest(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:])[:24]
}

func randomLeaseID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		return leaseDigest(now)
	}
	return hex.EncodeToString(buf)
}
