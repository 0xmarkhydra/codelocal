package localclient

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/orchestration"
)

func taskExecutionArgs(taskID, owner string) map[string]any {
	return map[string]any{privateTaskExecutionID: taskID, privateTaskExecutionOwner: owner}
}

func TestTaskMutationUsesPrivateWorktreeAndReadOverlay(t *testing.T) {
	engine, _, auth := newMultiRepoEngine(t)
	args := taskExecutionArgs("task-login", "session-a")
	opts := HandleOptions{SessionID: "session-a"}

	result, err := engine.taskExactEdit(context.Background(), args, opts, "backend/auth/login.go", "initial\n", "task change\n", false, "")
	if err != nil {
		t.Fatal(err)
	}
	if result["path"] != "backend/auth/login.go" {
		t.Fatalf("logical path was not preserved: %#v", result)
	}
	mainData, err := os.ReadFile(filepath.Join(auth, "login.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(mainData) != "initial\n" {
		t.Fatalf("authoritative checkout was modified: %q", mainData)
	}

	taskRead, err := engine.taskReadFile(context.Background(), args, opts, "backend/auth/login.go", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(fmt.Sprint(taskRead["content"])) != "task change" {
		t.Fatalf("task read did not use execution overlay: %#v", taskRead)
	}
	mainRead, err := engine.taskReadFile(context.Background(), map[string]any{}, HandleOptions{}, "backend/auth/login.go", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(fmt.Sprint(mainRead["content"])) != "initial" {
		t.Fatalf("non-task read should remain authoritative: %#v", mainRead)
	}

	bundle, ok, err := engine.TaskExecutions.Store.Get(engine.WorkspaceKey, "task-login")
	if err != nil || !ok || len(bundle.RepositoryBindings) != 1 {
		t.Fatalf("execution bundle missing: %#v ok=%v err=%v", bundle, ok, err)
	}
	privatePath := bundle.RepositoryBindings[0].LocalPath
	if strings.Contains(fmt.Sprint(result), privatePath) || strings.Contains(fmt.Sprint(taskRead), privatePath) {
		t.Fatalf("private worktree path leaked in model-facing result: result=%#v read=%#v", result, taskRead)
	}
}

func TestTaskBundleExpandsAcrossRepositoriesWithoutTouchingMain(t *testing.T) {
	engine, web, auth := newMultiRepoEngine(t)
	args := taskExecutionArgs("task-cross-repo", "session-a")
	opts := HandleOptions{SessionID: "session-a"}

	if _, err := engine.taskExactEdit(context.Background(), args, opts, "web/app.ts", "initial\n", "web task\n", false, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.taskExactEdit(context.Background(), args, opts, "backend/auth/login.go", "initial\n", "auth task\n", false, ""); err != nil {
		t.Fatal(err)
	}
	bundle, ok, err := engine.TaskExecutions.Store.Get(engine.WorkspaceKey, "task-cross-repo")
	if err != nil || !ok || len(bundle.RepositoryBindings) != 2 {
		t.Fatalf("expected two execution bindings: %#v ok=%v err=%v", bundle, ok, err)
	}
	for path, want := range map[string]string{filepath.Join(web, "app.ts"): "initial\n", filepath.Join(auth, "login.go"): "initial\n"} {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if string(data) != want {
			t.Fatalf("authoritative repo changed at %s: %q", path, data)
		}
	}
}

func TestTaskFormatFilesFormatsExecutionWorktreeOnly(t *testing.T) {
	engine, _, auth := newMultiRepoEngine(t)
	args := taskExecutionArgs("task-format", "session-a")
	opts := HandleOptions{SessionID: "session-a"}
	if _, err := engine.taskExactEdit(context.Background(), args, opts, "backend/auth/login.go", "initial\n", "package auth\nfunc Login( ){ }\n", false, ""); err != nil {
		t.Fatal(err)
	}
	result, err := engine.taskFormatFiles(context.Background(), args, opts, []string{"backend/auth/login.go"})
	if err != nil {
		t.Fatal(err)
	}
	formatted, _ := result["formatted"].([]string)
	if len(formatted) != 1 || formatted[0] != "backend/auth/login.go" {
		t.Fatalf("task formatter did not report logical path: %#v", result)
	}
	taskRead, err := engine.taskReadFile(context.Background(), args, opts, "backend/auth/login.go", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(fmt.Sprint(taskRead["content"])) != "package auth\n\nfunc Login() {}" {
		t.Fatalf("task worktree was not formatted: %#v", taskRead)
	}
	mainData, err := os.ReadFile(filepath.Join(auth, "login.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(mainData) != "initial\n" {
		t.Fatalf("task formatting modified authoritative checkout: %q", mainData)
	}
}

func TestTaskVerifyChangesReadsExecutionWorktreeNotAuthoritativeCheckout(t *testing.T) {
	engine, _, auth := newMultiRepoEngine(t)
	args := taskExecutionArgs("task-verify", "session-a")
	opts := HandleOptions{SessionID: "session-a"}
	if _, err := engine.taskExactEdit(context.Background(), args, opts, "backend/auth/login.go", "initial\n", "task verify change\n", false, ""); err != nil {
		t.Fatal(err)
	}

	result, err := engine.taskVerifyChanges(context.Background(), args, opts, []string{"backend/auth/login.go"}, "")
	if err != nil {
		t.Fatal(err)
	}
	diff, _ := result["gitDiff"].(map[string]any)
	if !strings.Contains(fmt.Sprint(diff["diff"]), "task verify change") {
		t.Fatalf("task verify did not inspect worktree diff: %#v", diff)
	}
	mainDiff := gitTestRun(t, auth, "diff", "--", "login.go")
	if strings.TrimSpace(mainDiff) != "" {
		t.Fatalf("authoritative checkout unexpectedly changed: %s", mainDiff)
	}
	bundle, ok, err := engine.TaskExecutions.Store.Get(engine.WorkspaceKey, "task-verify")
	if err != nil || !ok {
		t.Fatalf("task bundle missing: %#v ok=%v err=%v", bundle, ok, err)
	}
	privatePath := bundle.RepositoryBindings[0].LocalPath
	if strings.Contains(fmt.Sprint(result), privatePath) || strings.Contains(fmt.Sprint(result), ".codelocal/worktrees") {
		t.Fatalf("task verification leaked private execution path: %#v", result)
	}
	plan, ok := result["verificationPlan"].(orchestration.VerificationPlan)
	if !ok {
		t.Fatalf("verification plan type missing: %#v", result["verificationPlan"])
	}
	foundLogicalCWD := false
	for _, check := range plan.Checks {
		if check.CWD == "backend/auth" {
			foundLogicalCWD = true
		}
	}
	if !foundLogicalCWD {
		t.Fatalf("task verification checks must keep logical repository cwd: %#v", plan.Checks)
	}
}

func TestTaskVerifyChangesAggregatesMultipleExecutionRepositories(t *testing.T) {
	engine, _, _ := newMultiRepoEngine(t)
	args := taskExecutionArgs("task-verify-multi", "session-a")
	opts := HandleOptions{SessionID: "session-a"}
	if _, err := engine.taskExactEdit(context.Background(), args, opts, "web/app.ts", "initial\n", "web task\n", false, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.taskExactEdit(context.Background(), args, opts, "backend/auth/login.go", "initial\n", "auth task\n", false, ""); err != nil {
		t.Fatal(err)
	}
	result, err := engine.taskVerifyChanges(context.Background(), args, opts, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	diff, _ := result["gitDiff"].(map[string]any)
	if diff["repositoryCount"] != 2 {
		t.Fatalf("expected two task repository diffs: %#v", diff)
	}
	text := fmt.Sprint(diff["diff"])
	if !strings.Contains(text, "web task") || !strings.Contains(text, "auth task") {
		t.Fatalf("multi-repo task diff incomplete: %s", text)
	}
}

func TestTaskTerminalRunsInWorktreeWithoutLeakingPrivatePath(t *testing.T) {
	engine, _, _ := newMultiRepoEngine(t)
	args := taskExecutionArgs("task-terminal", "session-a")
	args["command"] = "pwd"
	args["cwd"] = "backend/auth"
	args["yieldMs"] = 10000
	args["timeoutMs"] = 30000
	opts := HandleOptions{SessionID: "session-a", RequestID: "req-terminal"}

	result, err := engine.runCommand(context.Background(), args, opts)
	if err != nil {
		t.Fatal(err)
	}
	bundle, ok, err := engine.TaskExecutions.Store.Get(engine.WorkspaceKey, "task-terminal")
	if err != nil || !ok || len(bundle.RepositoryBindings) != 1 {
		t.Fatalf("terminal execution bundle missing: %#v ok=%v err=%v", bundle, ok, err)
	}
	privatePath := bundle.RepositoryBindings[0].LocalPath
	serialized := fmt.Sprint(result)
	if strings.Contains(serialized, privatePath) || strings.Contains(serialized, ".codelocal/worktrees") {
		t.Fatalf("terminal result leaked private worktree path: %#v", result)
	}
	if result["cwd"] != "backend/auth" {
		t.Fatalf("terminal result did not expose logical cwd: %#v", result)
	}
	stdout, _ := result["stdout"].(map[string]any)
	if text := fmt.Sprint(stdout["text"]); !strings.Contains(text, "backend/auth") || strings.Contains(text, privatePath) {
		t.Fatalf("terminal stdout was not sanitized to logical cwd: %#v", stdout)
	}
}

func TestTaskGitStatusAndDiffUseExecutionWorktree(t *testing.T) {
	engine, _, auth := newMultiRepoEngine(t)
	args := taskExecutionArgs("task-git", "session-a")
	opts := HandleOptions{SessionID: "session-a"}
	if _, err := engine.taskExactEdit(context.Background(), args, opts, "backend/auth/login.go", "initial\n", "task git change\n", false, ""); err != nil {
		t.Fatal(err)
	}

	status, err := engine.taskGitStatus(args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fmt.Sprint(status["output"]), "login.go") {
		t.Fatalf("task git status did not inspect execution worktree: %#v", status)
	}
	diff, err := engine.taskGitDiff(context.Background(), args, opts)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fmt.Sprint(diff["diff"]), "task git change") {
		t.Fatalf("task git diff did not inspect execution worktree: %#v", diff)
	}
	if mainDiff := gitTestRun(t, auth, "diff", "--", "login.go"); strings.TrimSpace(mainDiff) != "" {
		t.Fatalf("task git routing modified authoritative checkout: %s", mainDiff)
	}
	bundle, ok, err := engine.TaskExecutions.Store.Get(engine.WorkspaceKey, "task-git")
	if err != nil || !ok || len(bundle.RepositoryBindings) != 1 {
		t.Fatalf("task git bundle missing: %#v ok=%v err=%v", bundle, ok, err)
	}
	privatePath := bundle.RepositoryBindings[0].LocalPath
	if strings.Contains(fmt.Sprint(status), privatePath) || strings.Contains(fmt.Sprint(diff), privatePath) {
		t.Fatalf("task git response leaked private worktree path: status=%#v diff=%#v", status, diff)
	}
}

func TestTaskGitMutationTargetPreparesIsolatedBinding(t *testing.T) {
	engine, _, auth := newMultiRepoEngine(t)
	args := taskExecutionArgs("task-git-mutate", "session-a")
	opts := HandleOptions{SessionID: "session-a"}
	target, err := engine.taskGitTarget(context.Background(), args, opts, "backend/auth", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if !target.Active || target.Root == auth || target.Binding.LocalPath == "" {
		t.Fatalf("task git mutation did not prepare isolated binding: %#v", target)
	}
	if _, err := os.Stat(filepath.Join(target.Root, "login.go")); err != nil {
		t.Fatalf("isolated task git target missing repository checkout: %v", err)
	}
}
