package mcpgateway

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/orchestration"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type boundedStepPolicy func(operationInvocation, map[string]any, orchestration.AgentPlan) autonomousStepDecision

type boundedStepsOutcome struct {
	summary       string
	filesTouched  []string
	verification  string
	haltReason    string
	dirty         bool
	verified      bool
	ops           int
}

// executeBoundedSteps is the shared step-loop extracted for lead + subagent
// reuse. The lead path keeps its full loop in agent_tool.go; this helper runs
// a small bounded program (context grounding + caller steps + verify) with a
// caller-supplied policy projection. It never consumes approval tokens and
// halts on the first policy/error/approval signal.
func (s *Service) executeBoundedSteps(ctx context.Context, userID string, req *mcp.CallToolRequest, workspaceKey, objective string, steps []boundedAgentStep, policy boundedStepPolicy) boundedStepsOutcome {
	out := boundedStepsOutcome{}
	if policy == nil {
		out.haltReason = "missing step policy"
		return out
	}
	if s.Workspaces == nil {
		out.haltReason = "workspace service unavailable"
		return out
	}
	workspace, err := s.Workspaces.Activate(ctx, userID, workspaceKey)
	if err != nil {
		out.haltReason = err.Error()
		return out
	}
	caps := executionCapabilities(workspace)
	definitions := compactDefinitionsByName()

	project := orchestration.ProjectProfile{}
	plan := s.tenantAgentPlanFromState(ctx, userID, currentAgentState(userID, sessionID(req), workspaceKey), caps, project)

	summaries := []string{}
	touched := map[string]struct{}{}
	ops := 0
	for _, step := range steps {
		if ops >= maxBoundedAgentOps {
			out.haltReason = "bounded agent operation budget exhausted"
			break
		}
		definition, exists := definitions[step.Tool]
		if !exists || definition.Execute != nil || definition.Resolve == nil {
			out.haltReason = "unsupported bounded agent tool: " + step.Tool
			break
		}
		forwardInput := cloneArgs(step.Args)
		forwardInput["workspaceKey"] = workspaceKey
		operation, forward, resolveErr := definition.Resolve(forwardInput)
		if resolveErr != nil {
			out.haltReason = resolveErr.Error()
			break
		}
		decision := policy(operation, forward, plan)
		if !decision.Allowed {
			out.haltReason = decision.Reason
			break
		}
		started := time.Now()
		_ = started
		result, _ := s.callOperationRemembering(ctx, userID, step.Tool, operation, forward, req)
		ops++
		if operation.OperationID == "terminal.run" {
			result = s.awaitBoundedVerificationProcess(ctx, userID, result, req, workspaceKey)
		}
		if halt, reason := resultNeedsAgentHalt(result); halt {
			out.haltReason = reason
			break
		}
		if strings.HasPrefix(operation.OperationID, "edit.") {
			out.dirty = true
		}
		if operation.OperationID == "verify.changes" {
			out.verified = true
			out.verification = summarizeVerificationResult(result)
		}
		state := currentAgentState(userID, sessionID(req), workspaceKey)
		for _, file := range state.TouchedFiles {
			touched[strings.TrimSpace(file)] = struct{}{}
		}
		plan = s.tenantAgentPlanFromState(ctx, userID, state, caps, project)
		summaries = append(summaries, step.Tool+":"+operation.OperationID+" ok")
	}
	out.ops = ops
	for file := range touched {
		if file != "" {
			out.filesTouched = append(out.filesTouched, file)
		}
	}
	if len(out.filesTouched) > 20 {
		out.filesTouched = out.filesTouched[:20]
	}
	if len(summaries) > 0 {
		out.summary = truncateAgentReportText(strings.Join(summaries, "; "), 2000)
	}
	return out
}

func summarizeVerificationResult(result *mcp.CallToolResult) string {
	if result == nil {
		return ""
	}
	root := resultRoot(result)
	if root == nil {
		return ""
	}
	status := strings.TrimSpace(fmt.Sprint(root["status"]))
	if status == "" {
		if result.IsError {
			status = "failed"
		} else {
			status = "completed"
		}
	}
	return "verify.changes: " + status
}
