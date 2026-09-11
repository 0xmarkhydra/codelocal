package mcpgateway

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	longmemory "github.com/0xmarkhydra/codelocal/internal/memory"
	"github.com/0xmarkhydra/codelocal/internal/orchestration"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// _ keeps the MCP request type linked for future session-scoped learning.
var _ = mcp.CallToolRequest{}

// subagent_experience.go — S5 learning seam.
//
// Every evidenced team member outcome is recorded as tenant-private Experience
// (subagent_id + verified outcome). Join-verified done members carry a positive
// signal; blocked/failed members with a concrete halt reason carry a negative
// signal. Unverified completions are skipped. History stays advisory:
// SubagentAffinity floors anecdotal samples (min 3) and caps boosts at ±0.10
// behind semantic+constraint scores.

// recordSubagentExperience persists one member outcome as verified Experience.
// It never fails dispatch: enqueue errors are logged, invalid inputs skipped.
func (s *Service) recordSubagentExperience(ctx context.Context, userID string, req *mcp.CallToolRequest, workspaceKey string, brief orchestration.ChildBrief, def orchestration.SubagentDefinition, reportStatus string, report agentTeamReport) {
	if s == nil || s.Store == nil {
		return
	}
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(brief.Objective) == "" {
		return
	}
	outcome, verified := subagentExperienceOutcome(reportStatus, report)
	if !verified {
		return
	}
	taskKind := semanticExperienceTaskKind(brief.Objective, "agent")
	files := append([]string(nil), report.FilesTouched...)
	checks := []string(nil)
	if verified {
		checks = []string{"subagent-verify"}
	}
	rootCause := strings.TrimSpace(report.HaltReason)
	verificationSummary := strings.TrimSpace(report.Verification)
	if verificationSummary == "" {
		verificationSummary = fmt.Sprintf("subagent %s finished with status %s", def.Name, reportStatus)
	}
	if verified && strings.TrimSpace(report.Summary) != "" {
		verificationSummary += "; " + truncateAgentReportText(strings.TrimSpace(report.Summary), 400)
	}
	// Bounded idempotency: same member + outcome never double-counts.
	idempotency := longmemory.IdempotencyKey(userID, brief.TaskID, brief.ID, string(def.Role), outcome, boolString(verified))
	input := cloud.ExperienceInput{
		UserID: userID, TaskID: brief.TaskID, TaskKind: taskKind, Objective: brief.Objective,
		Files: files, Checks: checks, Outcome: outcome, RootCause: rootCause,
		SubagentID: def.Name, SubagentRole: string(def.Role), SubagentScope: subagentExperienceScope(def),
		SubagentVerified: verified, VerificationSummary: verificationSummary, Verified: true,
		IdempotencyKey: idempotency,
		Metadata: map[string]any{
			"source": "subagent-team", "executionTool": "agent",
			"subagent": def.Name, "role": string(def.Role),
			"briefId": brief.ID, "reportStatus": reportStatus,
		},
	}
	if workspaceKey = strings.TrimSpace(workspaceKey); workspaceKey != "" && s.Workspaces != nil {
		if workspace, err := s.Workspaces.Activate(ctx, userID, workspaceKey); err == nil && workspace != nil {
			if id := strings.TrimSpace(workspace.WorkspaceID); id != "" {
				input.WorkspaceID = id
			}
			if id := strings.TrimSpace(workspace.DeviceID); id != "" {
				input.DeviceID = id
			}
		}
	}
	enqueueCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if _, err := s.Store.EnqueueExperience(enqueueCtx, input); err != nil {
		// Learning must never fail dispatch; the durable outbox retries.
		_ = err
	}
}

func subagentExperienceOutcome(reportStatus string, report agentTeamReport) (string, bool) {
	if reportStatus == "done" && strings.TrimSpace(report.Verification) != "" {
		return "succeeded", true
	}
	if reportStatus != "done" && strings.TrimSpace(report.HaltReason) != "" {
		return "failed", true
	}
	return "", false
}

func subagentExperienceScope(def orchestration.SubagentDefinition) string {
	switch strings.ToLower(strings.TrimSpace(def.Source)) {
	case orchestration.SubagentSourceProject:
		return "project"
	case orchestration.SubagentSourceGlobal:
		return "global"
	default:
		return "builtin"
	}
}

func boolString(value bool) string {
	if value {
		return "verified"
	}
	return "unverified"
}
