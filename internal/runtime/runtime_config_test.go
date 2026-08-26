package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
)

func TestRuntimeConfigEnvironmentExportsOnlyNonSecretEnvKeys(t *testing.T) {
	snapshot := cloud.RuntimeConfigSnapshot{Values: map[string]string{"FFMPEG_PATH": "ffmpeg", "video.aspect": "9:16"}}
	env := runtimeConfigEnvironment(snapshot)
	if env["FFMPEG_PATH"] != "ffmpeg" {
		t.Fatalf("expected runtime config value missing: %#v", env)
	}
	if _, ok := env["video.aspect"]; ok {
		t.Fatalf("preference key leaked into environment: %#v", env)
	}
	if _, ok := env["VBEE_API_KEY"]; ok {
		t.Fatal("secret unexpectedly merged into ordinary runtime config")
	}
}

func TestResolveRuntimeSnapshotUsesSharedOpenMontagePath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	snapshot := resolveRuntimeSnapshot(cloud.RuntimeConfigSnapshot{SystemProjects: []cloud.RuntimeSystemProject{{ID: "openmontage", Enabled: true}}})
	if len(snapshot.SystemProjects) != 1 {
		t.Fatalf("projects=%#v", snapshot.SystemProjects)
	}
	project := snapshot.SystemProjects[0]
	want := filepath.Join(home, ".codelocal", "system-projects", "openmontage")
	if project.Path != want || !project.Managed || !project.Hidden || !strings.Contains(project.Source, "OpenMontage") {
		t.Fatalf("project=%#v", project)
	}
}

func TestManagedRuntimeSystemProjectsFiltersDisabledProjects(t *testing.T) {
	settings := map[string]cloud.RuntimeMaterializedConfig{
		"one": {Snapshot: resolveRuntimeSnapshot(cloud.RuntimeConfigSnapshot{SystemProjects: []cloud.RuntimeSystemProject{{ID: "openmontage", Enabled: true}}})},
		"two": {Snapshot: cloud.RuntimeConfigSnapshot{SystemProjects: []cloud.RuntimeSystemProject{{ID: "disabled", Managed: true, Enabled: false}}}},
	}
	projects := managedRuntimeSystemProjects(settings)
	if len(projects) != 1 || projects[0].ID != "openmontage" {
		t.Fatalf("projects=%#v", projects)
	}
}

func TestValidateManagedSystemProjectRejectsUnexpectedSource(t *testing.T) {
	project := resolveRuntimeSnapshot(cloud.RuntimeConfigSnapshot{SystemProjects: []cloud.RuntimeSystemProject{{ID: "openmontage", Enabled: true}}}).SystemProjects[0]
	project.Source = "https://example.com/not-openmontage.git"
	if err := validateManagedSystemProject(project); err == nil {
		t.Fatal("expected unexpected managed source to be rejected")
	}
}
