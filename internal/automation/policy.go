package automation

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/approval"
	"github.com/0xmarkhydra/codelocal/internal/security"
)

type Action struct {
	Domain    string
	Operation string
	Origin    string
	Target    string
	Text      string
}

type Authorizer struct {
	WorkspaceKey string
	Broker       *approval.Broker
	Memory       *approval.Memory
}

func NewAuthorizer(workspaceKey string) *Authorizer {
	return &Authorizer{WorkspaceKey: workspaceKey, Broker: approval.NewBroker(), Memory: approval.New()}
}

func automationCommand(action Action) string {
	parts := []string{"automation", strings.TrimSpace(action.Domain), strings.TrimSpace(action.Operation)}
	if action.Origin != "" {
		parts = append(parts, action.Origin)
	}
	if action.Target != "" {
		parts = append(parts, action.Target)
	}
	return strings.Join(parts, " ")
}

func sensitiveAutomationText(value string) bool {
	value = strings.ToLower(value)
	for _, marker := range []string{"password", "passcode", "otp", "2fa", "private key", "seed phrase", "secret key", "credential", "api key"} {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}

func criticalAutomationText(value string) bool {
	value = strings.ToLower(value)
	for _, marker := range []string{"confirm payment", "pay now", "send money", "place order", "delete account", "delete permanently", "publish", "send message", "change permission", "grant access"} {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}

func browserOriginKey(origin string) string {
	if origin == "" {
		return "unknown"
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return strings.ToLower(origin)
	}
	return strings.ToLower(u.Scheme + "://" + u.Host)
}

func ClassifyAutomation(action Action) security.Decision {
	domain := strings.ToLower(strings.TrimSpace(action.Domain))
	op := strings.ToLower(strings.TrimSpace(action.Operation))
	command := automationCommand(action)
	decision := security.Decision{
		RiskLevel:       security.RiskSafe,
		MatchedRules:    []string{"automation:" + domain + ":" + op},
		RedactedCommand: command,
		Reason:          "read-only automation observation",
		ApprovalPolicy:  security.ApprovalNone,
	}

	if sensitiveAutomationText(action.Target + " " + action.Text) && (op == "read" || strings.Contains(op, "extract")) {
		decision.RiskLevel = security.RiskBlocked
		decision.Blocked = true
		decision.RequiresApproval = false
		decision.ApprovalPolicy = security.ApprovalBlocked
		decision.Reason = "credential and secret extraction through automation is blocked"
		return decision
	}

	switch domain {
	case "browser":
		switch op {
		case "status", "snapshot", "find", "console", "requests":
			return decision
		case "screenshot":
			decision.RiskLevel = security.RiskReview
			decision.RequiresApproval = true
			decision.ApprovalPolicy = security.ApprovalRememberable
			decision.ApprovalKey = "browser:screenshot:" + browserOriginKey(action.Origin)
			decision.ApprovalLabel = "Allow browser screenshots for " + browserOriginKey(action.Origin)
			decision.Reason = "browser screenshot may contain private on-screen information"
		case "open", "navigate":
			if BrowserLocalURL(action.Origin) || BrowserLocalURL(action.Target) {
				return decision
			}
			decision.RiskLevel = security.RiskReview
			decision.RequiresApproval = true
			decision.ApprovalPolicy = security.ApprovalRememberable
			decision.ApprovalKey = "browser:navigate:" + browserOriginKey(firstNonEmpty(action.Origin, action.Target))
			decision.ApprovalLabel = "Allow browser navigation to " + browserOriginKey(firstNonEmpty(action.Origin, action.Target))
			decision.Reason = "external browser navigation can contact a remote site"
		case "click", "fill", "press":
			decision.RiskLevel = security.RiskHigh
			decision.RequiresApproval = true
			decision.ApprovalPolicy = security.ApprovalRememberable
			decision.ApprovalKey = "browser:interact:" + browserOriginKey(action.Origin)
			decision.ApprovalLabel = "Allow browser interaction on " + browserOriginKey(action.Origin)
			decision.Reason = "browser interaction can change remote or local application state"
			if criticalAutomationText(action.Target + " " + action.Text) {
				decision.RiskLevel = security.RiskCritical
				decision.ApprovalPolicy = security.ApprovalAlways
				decision.ApprovalKey = ""
				decision.ApprovalLabel = ""
				decision.Reason = "sensitive browser action requires fresh confirmation"
			}
		case "close":
			decision.RiskLevel = security.RiskReview
			decision.RequiresApproval = true
			decision.ApprovalPolicy = security.ApprovalRememberable
			decision.ApprovalKey = "browser:close"
			decision.ApprovalLabel = "Allow CodeLocal to close its browser session"
			decision.Reason = "closing a browser session can discard unsaved page state"
		default:
			decision.RiskLevel = security.RiskBlocked
			decision.Blocked = true
			decision.ApprovalPolicy = security.ApprovalBlocked
			decision.Reason = "unknown browser automation operation"
		}
	case "computer":
		switch op {
		case "status", "list_windows", "ui_tree":
			return decision
		case "screenshot":
			decision.RiskLevel = security.RiskReview
			decision.RequiresApproval = true
			decision.ApprovalPolicy = security.ApprovalRememberable
			decision.ApprovalKey = "computer:screenshot"
			decision.ApprovalLabel = "Allow CodeLocal screen capture"
			decision.Reason = "screen capture may reveal private application content"
		case "focus", "click", "type", "key", "scroll", "drag":
			decision.RiskLevel = security.RiskHigh
			decision.RequiresApproval = true
			decision.ApprovalPolicy = security.ApprovalRememberable
			decision.ApprovalKey = "computer:input"
			decision.ApprovalLabel = "Allow desktop mouse and keyboard control"
			decision.Reason = "desktop input can affect applications outside the code workspace"
			if criticalAutomationText(action.Target + " " + action.Text) {
				decision.RiskLevel = security.RiskCritical
				decision.ApprovalPolicy = security.ApprovalAlways
				decision.ApprovalKey = ""
				decision.ApprovalLabel = ""
				decision.Reason = "sensitive desktop action requires fresh confirmation"
			}
		default:
			decision.RiskLevel = security.RiskBlocked
			decision.Blocked = true
			decision.ApprovalPolicy = security.ApprovalBlocked
			decision.Reason = "unknown computer automation operation"
		}
	default:
		decision.RiskLevel = security.RiskBlocked
		decision.Blocked = true
		decision.ApprovalPolicy = security.ApprovalBlocked
		decision.Reason = "unknown automation domain"
	}
	return decision
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (a *Authorizer) Authorize(action Action, providedToken string) (bool, map[string]any, error) {
	if a == nil {
		return false, nil, errors.New("automation authorizer unavailable")
	}
	decision := ClassifyAutomation(action)
	command := automationCommand(action)
	if decision.Blocked {
		return false, map[string]any{"status": "blocked", "riskLevel": decision.RiskLevel, "reason": decision.Reason, "matchedRules": decision.MatchedRules, "approvalPolicy": decision.ApprovalPolicy}, nil
	}
	if !decision.RequiresApproval {
		return true, nil, nil
	}
	if decision.ApprovalPolicy == security.ApprovalRememberable && decision.ApprovalKey != "" {
		remembered, err := a.Memory.Find(a.WorkspaceKey, decision.ApprovalKey)
		if err != nil {
			return false, nil, err
		}
		if remembered != nil {
			_, _ = a.Memory.Touch(a.WorkspaceKey, decision.ApprovalKey)
			return true, map[string]any{"remembered": true, "approvalId": remembered.ID}, nil
		}
	}
	if providedToken != "" && a.Broker.Consume(providedToken, command, a.WorkspaceKey, decision) {
		state := map[string]any{"approved": true}
		if decision.ApprovalPolicy == security.ApprovalRememberable {
			entry, err := a.Memory.Remember(a.WorkspaceKey, decision)
			if err != nil {
				return false, nil, err
			}
			if entry != nil {
				state["remembered"] = true
				state["approvalId"] = entry.ID
			}
		}
		return true, state, nil
	}
	preflight := a.Broker.Preflight(command, a.WorkspaceKey, decision)
	return false, map[string]any{
		"status":         preflight.Status,
		"riskLevel":      preflight.RiskLevel,
		"reason":         preflight.Reason,
		"matchedRules":   preflight.MatchedRules,
		"approvalPolicy": preflight.ApprovalPolicy,
		"approvalKey":    preflight.ApprovalKey,
		"approvalLabel":  preflight.ApprovalLabel,
		"approvalToken":  preflight.ApprovalToken,
		"expiresAt":      preflight.ExpiresAt,
	}, nil
}

func (a Action) String() string {
	return fmt.Sprintf("%s:%s", a.Domain, a.Operation)
}
