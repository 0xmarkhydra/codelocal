package cloud

import (
	"strings"
	"testing"
)

func TestProjectGoalTransitionsAreExplicit(t *testing.T) {
	valid := [][2]string{
		{"DRAFT", "PLAN_PROPOSED"},
		{"PLAN_PROPOSED", "WAITING_APPROVAL"},
		{"WAITING_APPROVAL", "APPROVED"},
		{"APPROVED", "EXECUTING"},
		{"EXECUTING", "VERIFYING"},
		{"VERIFYING", "DONE"},
		{"EXECUTING", "BLOCKED"},
		{"BLOCKED", "EXECUTING"},
	}
	for _, tc := range valid {
		if !validProjectGoalTransition(tc[0], tc[1]) {
			t.Fatalf("goal transition %s->%s should be valid", tc[0], tc[1])
		}
	}
	invalid := [][2]string{
		{"DRAFT", "EXECUTING"},
		{"DRAFT", "DONE"},
		{"PLAN_PROPOSED", "APPROVED"},
		{"WAITING_APPROVAL", "EXECUTING"},
		{"DONE", "EXECUTING"},
		{"CANCELLED", "DRAFT"},
	}
	for _, tc := range invalid {
		if validProjectGoalTransition(tc[0], tc[1]) {
			t.Fatalf("goal transition %s->%s should be rejected", tc[0], tc[1])
		}
	}
}

func TestProjectTaskTransitionsBlockDirectDone(t *testing.T) {
	if validProjectTaskTransition("TESTING", "DONE") {
		t.Fatal("implementing path must not mark TESTING->DONE directly; tester verdict owns DONE")
	}
	if !validProjectTaskTransition("RUNNING", "REVIEWING") {
		t.Fatal("RUNNING->REVIEWING should be valid")
	}
	if !validProjectTaskTransition("REVIEWING", "TESTING") {
		t.Fatal("REVIEWING->TESTING should be valid")
	}
	if validProjectTaskTransition("PLANNED", "ASSIGNED") {
		t.Fatal("PLANNED must go through READY before ASSIGNED")
	}
	if !validProjectTaskTransition("FAILED", "RETRYING") {
		t.Fatal("FAILED->RETRYING should be valid")
	}
}

func TestProjectTaskDependencyCycleDetection(t *testing.T) {
	acyclic := map[string][]string{"a": {}, "b": {"a"}, "c": {"b"}}
	if detectTaskDependencyCycle([]string{"a", "b", "c"}, acyclic) {
		t.Fatal("acyclic graph reported as cycle")
	}
	cyclic := map[string][]string{"a": {"c"}, "b": {"a"}, "c": {"b"}}
	if !detectTaskDependencyCycle([]string{"a", "b", "c"}, cyclic) {
		t.Fatal("cyclic graph was not detected")
	}
	self := map[string][]string{"a": {"a"}}
	if !detectTaskDependencyCycle([]string{"a"}, self) {
		t.Fatal("self dependency was not detected")
	}
}

func TestProjectOSMigrationIsTenantScoped(t *testing.T) {
	lower := strings.ToLower(projectOSMigrationSQL)
	for _, token := range []string{
		"references codelocal_users(id) on delete cascade",
		"references codelocal_projects(user_id,project_id) on delete cascade",
		"user_id", "project_id", "goal_id", "plan_id", "task_id",
	} {
		if !strings.Contains(lower, token) {
			t.Fatalf("project os migration missing tenant token %q", token)
		}
	}
}

func TestProjectOSPayloadSanitizationDropsSecrets(t *testing.T) {
	raw := sanitizeProjectOSPayload(map[string]any{"title": "ok", "apiKey": "secret", "token": "secret", "password": "secret"})
	lower := strings.ToLower(raw)
	if strings.Contains(lower, "secret") {
		t.Fatalf("payload sanitization leaked secret: %s", raw)
	}
	if !strings.Contains(lower, "ok") {
		t.Fatalf("payload sanitization dropped safe field: %s", raw)
	}
}
