package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
)

func TestRuntimeConfigEnvironmentExportsOnlyEnvKeys(t *testing.T) {
	snapshot := cloud.RuntimeConfigSnapshot{Values: map[string]string{"FFMPEG_PATH": "ffmpeg", "video.aspect": "9:16"}}
	env := runtimeConfigEnvironment(snapshot, map[string]string{"VBEE_API_KEY": "secret", "bad.key": "ignored"})
	if env["FFMPEG_PATH"] != "ffmpeg" || env["VBEE_API_KEY"] != "secret" {
		t.Fatalf("expected environment values missing: %#v", env)
	}
	if _, ok := env["video.aspect"]; ok {
		t.Fatalf("preference key leaked into environment: %#v", env)
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
