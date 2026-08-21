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
	ModeFull     Mode = "full"
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
	case string(ModeAgent), "smart":
		return ModeAgent
	case string(ModeFull):
		return ModeFull
	default:
		return ModePrompt
	}
}

func UserMode(mode Mode) string {
	switch NormalizeMode(string(mode)) {
	case ModeAgent:
		return "smart"
	case ModeFull:
		return "full"
	default:
		return "prompt"
	}
}

func UserModeLabel(mode Mode) string {
	switch UserMode(mode) {
	case "smart":
		return "Phê duyệt giúp tôi"
	case "full":
		return "Toàn quyền truy cập"
	default:
		return "Yêu cầu phê duyệt"
	}
}

func ParseUserMode(value string) (Mode, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "prompt":
		return ModePrompt, true
	case "smart":
		return ModeAgent, true
	case "full":
		return ModeFull, true
	default:
		return ModePrompt, false
	}
}

type UserModeChoice struct {
	Mode        string `json:"mode"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

func UserModeChoices() []UserModeChoice {
	return []UserModeChoice{
		{Mode: "prompt", Label: "Yêu cầu phê duyệt", Description: "Hỏi trước các thao tác cần phê duyệt."},
		{Mode: "smart", Label: "Phê duyệt giúp tôi", Description: "Tự phê duyệt thao tác thông thường; vẫn hỏi khi rủi ro cao."},
		{Mode: "full", Label: "Toàn quyền truy cập", Description: "Tự phê duyệt mọi thao tác không bị hard-block trong workspace."},
	}
}

func UserModePrompt(mode Mode) map[string]any {
	return map[string]any{
		"message":     "Bạn muốn CodeLocal xử lý phê duyệt thế nào?",
		"currentMode": UserMode(mode),
		"choices":     UserModeChoices(),
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

func workspaceMode(workspaceID string) (Mode, bool, error) {
	data, err := readModes()
	if err != nil {
		return ModePrompt, false, err
	}
	value, configured := data.Workspaces[strings.TrimSpace(workspaceID)]
	if !configured {
		return ModePrompt, false, nil
	}
	return NormalizeMode(string(value)), true, nil
}

func WorkspaceMode(workspaceID string) (Mode, error) {
	mode, _, err := workspaceMode(workspaceID)
	return mode, err
}

func ResolveMode(workspaceID string) Mode {
	raw := strings.TrimSpace(os.Getenv("CODELOCAL_APPROVAL_MODE"))
	if raw != "" {
		envMode := NormalizeMode(raw)
		if envMode == ModeDeny || envMode == ModeAutoSafe {
			return envMode
		}
	}
	if mode, configured, err := workspaceMode(workspaceID); err == nil && configured {
		return mode
	}
	if raw != "" {
		return NormalizeMode(raw)
	}
	return ModePrompt
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

// FullAllows grants every approval-requiring action that the deterministic
// policy has not hard blocked. Full access is workspace-scoped and durable;
// credential access, workspace escapes, privilege escalation, disk/system
// administration and other blocked policy decisions remain non-bypassable.
func FullAllows(mode Mode, decision security.Decision) bool {
	return mode == ModeFull && !decision.Blocked && decision.RequiresApproval
}

func DeniesApproval(mode Mode, decision security.Decision) bool {
	if decision.Blocked || !decision.RequiresApproval {
		return false
	}
	return mode == ModeDeny || mode == ModeAutoSafe
}
