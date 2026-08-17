package approval

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/security"
	"github.com/0xmarkhydra/codelocal/internal/state"
)

type Mode string

const (
	ModePrompt   Mode = "prompt"
	ModeDeny     Mode = "deny"
	ModeAutoSafe Mode = "auto-safe"
	ModeAgent    Mode = "agent"
)

type modeFile struct {
	Version    int             `json:"version"`
	Workspaces map[string]Mode `json:"workspaces"`
}

func NormalizeMode(value string) Mode {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(ModeDeny):
		return ModeDeny
	case string(ModeAutoSafe):
		return ModeAutoSafe
	case string(ModeAgent):
		return ModeAgent
	default:
		return ModePrompt
	}
}

func modeFilePath() string {
	return filepath.Join(state.Dir(), "approval-mode.json")
}

func readModes() (modeFile, error) {
	data := modeFile{Version: 1, Workspaces: map[string]Mode{}}
	if err := state.ReadJSON(modeFilePath(), &data); err != nil {
		if os.IsNotExist(err) {
			return data, nil
		}
		return modeFile{}, err
	}
	if data.Workspaces == nil {
		data.Workspaces = map[string]Mode{}
	}
	data.Version = 1
	return data, nil
}

func WorkspaceMode(workspaceID string) (Mode, error) {
	data, err := readModes()
	if err != nil {
		return ModePrompt, err
	}
	return NormalizeMode(string(data.Workspaces[strings.TrimSpace(workspaceID)])), nil
}

func ResolveMode(workspaceID string) Mode {
	if raw := strings.TrimSpace(os.Getenv("CODELOCAL_APPROVAL_MODE")); raw != "" {
		return NormalizeMode(raw)
	}
	mode, err := WorkspaceMode(workspaceID)
	if err != nil {
		return ModePrompt
	}
	return mode
}

func SetWorkspaceMode(workspaceID string, mode Mode) error {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return os.ErrInvalid
	}
	data, err := readModes()
	if err != nil {
		return err
	}
	data.Workspaces[workspaceID] = NormalizeMode(string(mode))
	return state.WriteJSONAtomic(modeFilePath(), data)
}

// AgentAllows deliberately grants only actions that the deterministic policy
// already considers safely rememberable. Critical/always-confirm and blocked
// actions remain outside Agent Mode and must still stop for fresh confirmation
// or fail closed.
func AgentAllows(mode Mode, decision security.Decision) bool {
	if mode != ModeAgent || decision.Blocked || !decision.RequiresApproval {
		return false
	}
	if decision.ApprovalPolicy != security.ApprovalRememberable {
		return false
	}
	return decision.RiskLevel == security.RiskReview || decision.RiskLevel == security.RiskHigh
}

func DeniesApproval(mode Mode, decision security.Decision) bool {
	if decision.Blocked || !decision.RequiresApproval {
		return false
	}
	return mode == ModeDeny || mode == ModeAutoSafe
}
