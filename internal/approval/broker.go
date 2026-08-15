package approval

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/security"
)

type Preflight struct {
	Status         string                  `json:"status"`
	RiskLevel      security.RiskLevel      `json:"riskLevel"`
	Reason         string                  `json:"reason"`
	MatchedRules   []string                `json:"matchedRules"`
	Command        string                  `json:"command"`
	ApprovalPolicy security.ApprovalPolicy `json:"approvalPolicy"`
	ApprovalKey    string                  `json:"approvalKey,omitempty"`
	ApprovalLabel  string                  `json:"approvalLabel,omitempty"`
	Remembered     bool                    `json:"remembered,omitempty"`
	ApprovalToken  string                  `json:"approvalToken,omitempty"`
	ExpiresAt      int64                   `json:"expiresAt,omitempty"`
}

type pendingApproval struct {
	Token       string
	TokenHash   string
	Fingerprint string
	ExpiresAt   int64
}

type Broker struct {
	mu                   sync.Mutex
	pending              map[string]pendingApproval
	pendingByFingerprint map[string]string
	ttl                  time.Duration
}

func NewBroker() *Broker {
	// Chat approval is a human round trip. The token is still one-time and
	// exact-fingerprint-bound; a longer pending window does not broaden scope.
	ttl := 30 * time.Minute
	if raw := strings.TrimSpace(os.Getenv("CODELOCAL_CHAT_APPROVAL_TTL_MS")); raw != "" {
		if parsed, err := time.ParseDuration(raw + "ms"); err == nil && parsed > 0 {
			ttl = parsed
		}
	}
	return &Broker{
		pending:              map[string]pendingApproval{},
		pendingByFingerprint: map[string]string{},
		ttl:                  ttl,
	}
}

func sha(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func randomSecret(bytes int) string {
	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err == nil {
		return hex.EncodeToString(buf)
	}
	return sha(time.Now().String())
}

func fingerprint(sessionID, command, cwd string, decision security.Decision) string {
	rules := append([]string(nil), decision.MatchedRules...)
	sort.Strings(rules)
	payload, _ := json.Marshal(map[string]any{
		"sessionId":       strings.TrimSpace(sessionID),
		"rawCommandHash":  sha(command),
		"redactedCommand": decision.RedactedCommand,
		"cwd":             cwd,
		"rules":           rules,
		"risk":            decision.RiskLevel,
		"approvalPolicy":  decision.ApprovalPolicy,
		"approvalKey":     decision.ApprovalKey,
	})
	return sha(string(payload))
}

func (b *Broker) deletePendingLocked(id string) {
	entry, ok := b.pending[id]
	if !ok {
		return
	}
	delete(b.pending, id)
	if current, exists := b.pendingByFingerprint[entry.Fingerprint]; exists && current == id {
		delete(b.pendingByFingerprint, entry.Fingerprint)
	}
}

func (b *Broker) pruneLocked(now int64) {
	for id, entry := range b.pending {
		if entry.ExpiresAt <= now {
			b.deletePendingLocked(id)
		}
	}
}

func preflightBase(status string, decision security.Decision) Preflight {
	return Preflight{Status: status, RiskLevel: decision.RiskLevel, Reason: decision.Reason, MatchedRules: append([]string(nil), decision.MatchedRules...), Command: decision.RedactedCommand, ApprovalPolicy: decision.ApprovalPolicy, ApprovalKey: decision.ApprovalKey, ApprovalLabel: decision.ApprovalLabel}
}

// PreflightScoped returns one stable pending token for the exact
// session+command+cwd+policy fingerprint. Repeated retries while a user is
// deciding do not rotate the nonce and invalidate the approval being reviewed.
func (b *Broker) PreflightScoped(sessionID, command, cwd string, decision security.Decision) Preflight {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now().UnixMilli()
	b.pruneLocked(now)
	if decision.Blocked {
		return preflightBase("blocked", decision)
	}
	if !decision.RequiresApproval {
		return preflightBase("safe", decision)
	}

	fp := fingerprint(sessionID, command, cwd, decision)
	if id := b.pendingByFingerprint[fp]; id != "" {
		if existing, ok := b.pending[id]; ok && existing.ExpiresAt > now && existing.Token != "" {
			out := preflightBase("approval_required", decision)
			out.ApprovalToken = existing.Token
			out.ExpiresAt = existing.ExpiresAt
			return out
		}
		delete(b.pendingByFingerprint, fp)
	}

	id := randomSecret(16)
	secret := randomSecret(32)
	token := id + "." + secret
	expires := time.Now().Add(b.ttl).UnixMilli()
	b.pending[id] = pendingApproval{Token: token, TokenHash: sha(secret), Fingerprint: fp, ExpiresAt: expires}
	b.pendingByFingerprint[fp] = id
	out := preflightBase("approval_required", decision)
	out.ApprovalToken = token
	out.ExpiresAt = expires
	return out
}

func (b *Broker) Preflight(command, cwd string, decision security.Decision) Preflight {
	return b.PreflightScoped("", command, cwd, decision)
}

func sameHex(a, b string) bool {
	aa, errA := hex.DecodeString(a)
	bb, errB := hex.DecodeString(b)
	if errA != nil || errB != nil || len(aa) != len(bb) {
		return false
	}
	return subtle.ConstantTimeCompare(aa, bb) == 1
}

// ConsumeScoped validates and consumes exactly one pending capability. Invalid
// tokens, a different session, or a changed command/policy never erase the
// legitimate pending approval; only successful consumption or expiry does.
func (b *Broker) ConsumeScoped(sessionID, token, command, cwd string, decision security.Decision) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now().UnixMilli()
	b.pruneLocked(now)
	if !decision.RequiresApproval || decision.Blocked {
		return !decision.Blocked
	}
	if token == "" {
		return false
	}
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return false
	}
	entry, ok := b.pending[parts[0]]
	if !ok || entry.ExpiresAt <= now {
		return false
	}
	if !sameHex(entry.TokenHash, sha(parts[1])) {
		return false
	}
	if !sameHex(entry.Fingerprint, fingerprint(sessionID, command, cwd, decision)) {
		return false
	}
	b.deletePendingLocked(parts[0])
	return true
}

func (b *Broker) Consume(token, command, cwd string, decision security.Decision) bool {
	return b.ConsumeScoped("", token, command, cwd, decision)
}
