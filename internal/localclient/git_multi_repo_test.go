package localclient

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/orchestration"
)

func gitTestRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=CodeLocal Test", "GIT_AUTHOR_EMAIL=test@codelocal.invalid",
		"GIT_COMMITTER_NAME=CodeLocal Test", "GIT_COMMITTER_EMAIL=test@codelocal.invalid",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
	return string(out)
}

func makeNestedGitRepo(t *testing.T, root, relative, filename string) string {
	t.Helper()
	dir := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	gitTestRun(t, dir, "init", "-q")
	if err := os.WriteFile(filepath.Join(dir, filename), []byte("initial\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitTestRun(t, dir, "add", filename)
	gitTestRun(t, dir, "commit", "-qm", "initial")
	return dir
}

func newMultiRepoEngine(t *testing.T) (*Engine, string, string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "BIDDI")
	web := makeNestedGitRepo(t, root, "web", "app.ts")
	auth := makeNestedGitRepo(t, root, "backend/auth", "login.go")
	t.Setenv("CODELOCAL_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	engine, err := New(root, "biddi-test", "BIDDI", "device::biddi-test", "device")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(engine.Close)
	return engine, web, auth
}

func TestGitStatusAggregatesLogicalMultiRepoWorkspace(t *testing.T) {
	engine, web, _ := newMultiRepoEngine(t)
	if err := os.WriteFile(filepath.Join(web, "app.ts"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := engine.gitStatus("")
	if err != nil {
		t.Fatal(err)
	}
	if result["repositoryCount"] != 2 {
		t.Fatalf("expected two repositories, got %#v", result)
	}
	items, ok := result["repositories"].([]map[string]any)
	if !ok || len(items) != 2 {
		t.Fatalf("unexpected repository status payload: %#v", result)
	}
	dirty := 0
	for _, item := range items {
		if item["dirty"] == true {
			dirty++
		}
	}
	if dirty != 1 {
		t.Fatalf("expected exactly one dirty repository: %#v", items)
	}
}

func TestGitDiffRoutesWorkspacePathToOwningRepository(t *testing.T) {
	engine, _, auth := newMultiRepoEngine(t)
	if err := os.WriteFile(filepath.Join(auth, "login.go"), []byte("changed auth\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := engine.gitDiff("", "backend/auth/login.go", false)
	if err != nil {
		t.Fatal(err)
	}
	if result["repositoryPath"] != "backend/auth" {
		t.Fatalf("wrong repository routing: %#v", result)
	}
	if !strings.Contains(asString(result["diff"]), "changed auth") {
		t.Fatalf("expected auth diff, got %#v", result)
	}
}

func TestGitRepositorySelectorRequiredForAmbiguousRepoWideRead(t *testing.T) {
	engine, _, _ := newMultiRepoEngine(t)
	if _, _, err := engine.gitTarget("", ""); err == nil {
		t.Fatal("repo-wide operation in multi-repo workspace must require repository selector")
	}
	if repo, _, err := engine.gitTarget("web", ""); err != nil || repo.RelativePath != "web" {
		t.Fatalf("repository selector did not resolve: repo=%#v err=%v", repo, err)
	}
}

func commitRepoFile(t *testing.T, repo, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repo, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	gitTestRun(t, repo, "add", name)
	gitTestRun(t, repo, "commit", "-qm", "add "+name)
}

func TestVerifyChangesBuildsRepositoryScopedPlanAndDiff(t *testing.T) {
	engine, web, auth := newMultiRepoEngine(t)
	commitRepoFile(t, web, "package.json", `{"scripts":{"typecheck":"tsc --noEmit","test":"node --test"}}`)
	commitRepoFile(t, auth, "go.mod", "module example.com/auth\n\ngo 1.22\n")
	if err := os.WriteFile(filepath.Join(web, "app.ts"), []byte("export const changed = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(auth, "login.go"), []byte("package auth\n\nfunc Login() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	engine.Project.Invalidate()

	result, err := engine.verifyChanges(context.Background(), []string{"web/app.ts", "backend/auth/login.go"}, "")
	if err != nil {
		t.Fatal(err)
	}
	plan, ok := result["verificationPlan"].(orchestration.VerificationPlan)
	if !ok || plan.Mode != "multi-repository" {
		t.Fatalf("unexpected multi-repo verification plan: %#v", result["verificationPlan"])
	}
	seenCWD := map[string]bool{}
	for _, check := range plan.Checks {
		if check.CWD != "" {
			seenCWD[check.CWD] = true
		}
		if check.CWD == "web" && check.Key != orchestration.ScopedCheckID(check.Command, "web") {
			t.Fatalf("web check evidence is not cwd-scoped: %#v", check)
		}
		if check.CWD == "backend/auth" && check.Key != orchestration.ScopedCheckID(check.Command, "backend/auth") {
			t.Fatalf("auth check evidence is not cwd-scoped: %#v", check)
		}
	}
	if !seenCWD["web"] || !seenCWD["backend/auth"] {
		t.Fatalf("verification checks were not split by repository: %#v", plan.Checks)
	}
	diff, _ := result["gitDiff"].(map[string]any)
	repositories, _ := diff["repositories"].([]map[string]any)
	if len(repositories) != 2 {
		t.Fatalf("expected two repository diffs, got %#v", diff)
	}
	runs, _ := result["recommendedCheckRuns"].([]map[string]any)
	if len(runs) == 0 {
		t.Fatalf("expected structured cwd-aware check runs: %#v", result)
	}
}

func TestChangedPathsFromGitStatusAggregatesRepositories(t *testing.T) {
	engine, web, auth := newMultiRepoEngine(t)
	if err := os.WriteFile(filepath.Join(web, "app.ts"), []byte("changed web\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(auth, "login.go"), []byte("changed auth\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	paths := engine.changedPathsFromGitStatus()
	joined := strings.Join(paths, "\n")
	if !strings.Contains(joined, "web/app.ts") || !strings.Contains(joined, "backend/auth/login.go") {
		t.Fatalf("multi-repo changed paths missing: %#v", paths)
	}
}

func TestFormatFilesUsesOwningRepositoryRoot(t *testing.T) {
	engine, _, auth := newMultiRepoEngine(t)
	if err := os.WriteFile(filepath.Join(auth, "login.go"), []byte("package auth\nfunc Login( ){ }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := engine.formatFiles(context.Background(), []string{"backend/auth/login.go"})
	if err != nil {
		t.Fatal(err)
	}
	formatted, _ := result["formatted"].([]string)
	if len(formatted) != 1 || formatted[0] != "backend/auth/login.go" {
		t.Fatalf("nested repository file was not formatted: %#v", result)
	}
	data, err := os.ReadFile(filepath.Join(auth, "login.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "package auth\n\nfunc Login() {}\n" {
		t.Fatalf("unexpected gofmt output: %q", data)
	}
}

func TestRepositoryBranchStateReportsMixedBranches(t *testing.T) {
	engine, web, _ := newMultiRepoEngine(t)
	gitTestRun(t, web, "checkout", "-qb", "dev")
	display, fingerprint, branches := engine.repositoryBranchState()
	if display != "multiple" || len(branches) != 2 {
		t.Fatalf("expected mixed branch state: display=%q branches=%#v", display, branches)
	}
	if !strings.Contains(fingerprint, "web=dev") || !strings.Contains(fingerprint, "backend/auth=") {
		t.Fatalf("branch fingerprint missing per-repo state: %q", fingerprint)
	}
}

func TestVerifyChangesAllowsLogicalWorkspaceFileOutsideGitRepositories(t *testing.T) {
	engine, _, _ := newMultiRepoEngine(t)
	rootDoc := filepath.Join(engine.Root, "PROJECT_NOTES.md")
	if err := os.WriteFile(rootDoc, []byte("workspace-level notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := engine.verifyChanges(context.Background(), []string{"PROJECT_NOTES.md"}, "")
	if err != nil {
		t.Fatalf("logical workspace file outside nested Git repos must remain verifiable: %v", err)
	}
	diff, _ := result["gitDiff"].(map[string]any)
	if count, _ := diff["repositoryCount"].(int); count != 0 {
		t.Fatalf("unowned workspace file must not be attributed to a Git repository: %#v", diff)
	}
}

func TestVerifyChangesAcceptsDeletedMultiRepoPath(t *testing.T) {
	engine, _, auth := newMultiRepoEngine(t)
	if err := os.Remove(filepath.Join(auth, "login.go")); err != nil {
		t.Fatal(err)
	}
	engine.Project.Invalidate()
	result, err := engine.verifyChanges(context.Background(), []string{"backend/auth/login.go"}, "")
	if err != nil {
		t.Fatalf("deleted path must remain verifiable: %v", err)
	}
	diff, _ := result["gitDiff"].(map[string]any)
	if !strings.Contains(asString(diff["diff"]), "deleted file mode") && !strings.Contains(asString(diff["diff"]), "-initial") {
		t.Fatalf("deleted-file diff missing: %#v", diff)
	}
}
