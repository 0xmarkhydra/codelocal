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
	TokenHash   string
	Fingerprint string
	ExpiresAt   int64
}

type Broker struct {
	mu      sync.Mutex
	pending map[string]pendingApproval
	ttl     time.Duration
}

func NewBroker() *Broker {
	ttl := 5 * time.Minute
	if raw := strings.TrimSpace(os.Getenv("CODELOCAL_CHAT_APPROVAL_TTL_MS")); raw != "" {
		if parsed, err := time.ParseDuration(raw + "ms"); err == nil && parsed > 0 {
			ttl = parsed
		}
	}
	return &Broker{pending: map[string]pendingApproval{}, ttl: ttl}
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

func fingerprint(command, cwd string, decision security.Decision) string {
	rules := append([]string(nil), decision.MatchedRules...)
	sort.Strings(rules)
	payload, _ := json.Marshal(map[string]any{
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

func (b *Broker) pruneLocked(now int64) {
	for id, entry := range b.pending {
		if entry.ExpiresAt <= now {
			delete(b.pending, id)
		}
	}
}

func preflightBase(status string, decision security.Decision) Preflight {
	return Preflight{Status: status, RiskLevel: decision.RiskLevel, Reason: decision.Reason, MatchedRules: append([]string(nil), decision.MatchedRules...), Command: decision.RedactedCommand, ApprovalPolicy: decision.ApprovalPolicy, ApprovalKey: decision.ApprovalKey, ApprovalLabel: decision.ApprovalLabel}
}

func (b *Broker) Preflight(command, cwd string, decision security.Decision) Preflight {
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
	id := randomSecret(16)
	secret := randomSecret(32)
	expires := time.Now().Add(b.ttl).UnixMilli()
	b.pending[id] = pendingApproval{TokenHash: sha(secret), Fingerprint: fingerprint(command, cwd, decision), ExpiresAt: expires}
	out := preflightBase("approval_required", decision)
	out.ApprovalToken = id + "." + secret
	out.ExpiresAt = expires
	return out
}

func sameHex(a, b string) bool {
	aa, errA := hex.DecodeString(a)
	bb, errB := hex.DecodeString(b)
	if errA != nil || errB != nil || len(aa) != len(bb) {
		return false
	}
	return subtle.ConstantTimeCompare(aa, bb) == 1
}

func (b *Broker) Consume(token, command, cwd string, decision security.Decision) bool {
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
	if !ok {
		return false
	}
	delete(b.pending, parts[0])
	if entry.ExpiresAt <= now || !sameHex(entry.TokenHash, sha(parts[1])) {
		return false
	}
	return sameHex(entry.Fingerprint, fingerprint(command, cwd, decision))
}
