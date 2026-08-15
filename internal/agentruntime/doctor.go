package agentruntime

import (
	"context"
	"os/exec"
	"sort"
	"strings"
)

type ExecutableSpec struct {
	EngineID    string `json:"engineId"`
	DisplayName string `json:"displayName"`
	Binary      string `json:"binary"`
}

type ExecutableStatus struct {
	EngineID    string `json:"engineId"`
	DisplayName string `json:"displayName"`
	Binary      string `json:"binary"`
	Installed   bool   `json:"installed"`
	Path        string `json:"path,omitempty"`
	Note        string `json:"note"`
}

type DoctorReport struct {
	Executables []ExecutableStatus `json:"executables"`
	Registered  []ProbeResult      `json:"registered"`
	SafetyNote  string             `json:"safetyNote"`
}

func DefaultExecutableSpecs() []ExecutableSpec {
	return []ExecutableSpec{
		{EngineID: "claude", DisplayName: "Claude Code", Binary: "claude"},
		{EngineID: "codex", DisplayName: "OpenAI Codex", Binary: "codex"},
		{EngineID: "cosine", DisplayName: "Cosine", Binary: "cos"},
	}
}

func DiscoverExecutables(specs []ExecutableSpec, lookup func(string) (string, error)) []ExecutableStatus {
	if lookup == nil {
		lookup = exec.LookPath
	}
	out := make([]ExecutableStatus, 0, len(specs))
	seen := map[string]struct{}{}
	for _, spec := range specs {
		spec.EngineID = strings.ToLower(strings.TrimSpace(spec.EngineID))
		spec.Binary = strings.TrimSpace(spec.Binary)
		if spec.EngineID == "" || spec.Binary == "" {
			continue
		}
		if _, exists := seen[spec.EngineID]; exists {
			continue
		}
		seen[spec.EngineID] = struct{}{}
		path, err := lookup(spec.Binary)
		status := ExecutableStatus{EngineID: spec.EngineID, DisplayName: strings.TrimSpace(spec.DisplayName), Binary: spec.Binary, Note: "binary lookup only; authentication, credentials and provider processes were not inspected"}
		if err == nil && strings.TrimSpace(path) != "" {
			status.Installed = true
			status.Path = path
		}
		out = append(out, status)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].EngineID < out[j].EngineID })
	return out
}

func Doctor(registry *Registry, lookup func(string) (string, error)) DoctorReport {
	if registry == nil {
		registry = NewRegistry()
	}
	return DoctorReport{
		Executables: DiscoverExecutables(DefaultExecutableSpecs(), lookup),
		Registered:  registry.ProbeAll(context.Background()),
		SafetyNote:  "Discovery does not execute provider binaries or read provider credentials. Mutation remains disabled unless an adapter declares mediated tools or validated sandbox isolation.",
	}
}
