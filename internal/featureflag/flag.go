package featureflag

import (
	"os"
	"strings"
)

// Known V3 rollout flags from the execution checklist (phase T0). Every flag
// defaults to disabled so unfinished runtime generations can never activate by
// accident; operators opt in explicitly per environment.
const (
	AgentRuntimeV2        = "agent_runtime_v2"
	ContextSurfaceV2      = "context_surface_v2"
	RuntimeEventsV2       = "runtime_events_v2"
	WorktreeV2            = "worktree_v2"
	PatchEngineV2         = "patch_engine_v2"
	PolicyKernelV2        = "policy_kernel_v2"
	ToolProgramRuntime    = "tool_program_runtime"
	FailureIntelligence   = "failure_intelligence"
	EcosystemAssimilation = "ecosystem_assimilation"
)

// Known returns every recognized flag in stable order.
func Known() []string {
	return []string{
		AgentRuntimeV2,
		ContextSurfaceV2,
		RuntimeEventsV2,
		WorktreeV2,
		PatchEngineV2,
		PolicyKernelV2,
		ToolProgramRuntime,
		FailureIntelligence,
		EcosystemAssimilation,
	}
}

func knownSet() map[string]struct{} {
	out := make(map[string]struct{}, 9)
	for _, name := range Known() {
		out[name] = struct{}{}
	}
	return out
}

func envName(name string) string {
	return "CODELOCAL_FEATURE_" + strings.ToUpper(strings.TrimSpace(name))
}

// Enabled reports whether a known flag is turned on in this environment.
// Unknown or empty names are always disabled (fail-closed). Accepted truthy
// values are 1, true, yes and on; everything else, including empty, is false.
func Enabled(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	if _, ok := knownSet()[name]; !ok {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv(envName(name)))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// Snapshot returns the current state of every known flag for observability.
func Snapshot() map[string]bool {
	out := make(map[string]bool, 9)
	for _, name := range Known() {
		out[name] = Enabled(name)
	}
	return out
}
