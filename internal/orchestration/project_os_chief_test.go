package orchestration

import (
	"strings"
	"testing"
)

func TestClassifyProjectOSChatIntent(t *testing.T) {
	if got := ClassifyProjectOSChatIntent("Làm payment mới cho BIDDI."); got != ChatIntentGoalWork {
		t.Fatalf("goal work misclassified: %s", got)
	}
	if got := ClassifyProjectOSChatIntent("Fix đoạn này."); got != ChatIntentDirectWork {
		t.Fatalf("direct work misclassified: %s", got)
	}
	if got := ClassifyProjectOSChatIntent("Payment đến đâu rồi?"); got != ChatIntentStatusControl {
		t.Fatalf("status misclassified: %s", got)
	}
	if got := ClassifyProjectOSChatIntent("Giải thích hàm này."); got != ChatIntentAsk {
		t.Fatalf("ask misclassified: %s", got)
	}
}

func TestDecomposeApprovedPlanBuildsVerifyChain(t *testing.T) {
	specs, err := DecomposeApprovedPlan(ChiefDecompositionInput{
		PlanSummary:        "Payment mới cho BIDDI",
		AcceptanceCriteria: []string{"Tạo payment intent", "Webhook xác nhận"},
	})
	if err != nil {
		t.Fatalf("decompose failed: %v", err)
	}
	if len(specs) != 4 {
		t.Fatalf("spec count=%d want 4", len(specs))
	}
	if specs[1].TaskKind != "testing" || len(specs[1].DependsOn) != 1 {
		t.Fatalf("verify task must depend on implement task: %#v", specs[1])
	}
	if err := ValidateChiefTaskGraph(specs); err != nil {
		t.Fatalf("valid graph rejected: %v", err)
	}
}

func TestValidateChiefTaskGraphRejectsCycleAndDuplicates(t *testing.T) {
	dup := []ChiefTaskSpec{{Title: "A", TaskKind: "coding"}, {Title: "a", TaskKind: "coding"}}
	if err := ValidateChiefTaskGraph(dup); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate titles should be rejected, got %v", err)
	}
	cycle := []ChiefTaskSpec{
		{Title: "A", TaskKind: "coding", DependsOn: []string{"B"}},
		{Title: "B", TaskKind: "coding", DependsOn: []string{"A"}},
	}
	if err := ValidateChiefTaskGraph(cycle); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("cycle should be rejected, got %v", err)
	}
	unknown := []ChiefTaskSpec{{Title: "A", TaskKind: "coding", DependsOn: []string{"missing"}}}
	if err := ValidateChiefTaskGraph(unknown); err == nil {
		t.Fatal("unknown dependency should be rejected")
	}
}
