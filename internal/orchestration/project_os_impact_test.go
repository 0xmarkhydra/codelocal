package orchestration

import "testing"

func TestAnalyzeRequirementImpactScoresOverlap(t *testing.T) {
	tasks := []ImpactTask{
		{ID: "task_a", Title: "Triển khai payment intent", Description: "Tạo payment intent cho BIDDI", Status: "RUNNING"},
		{ID: "task_b", Title: "Viết docs onboarding", Description: "Hướng dẫn onboarding", Status: "READY"},
		{ID: "task_c", Title: "Xong việc cũ", Description: "Không liên quan", Status: "DONE"},
	}
	hits := AnalyzeRequirementImpact([]string{"docs/payment-requirements.md", "payment intent webhook"}, tasks)
	if len(hits) == 0 {
		t.Fatal("expected impact hits for payment tasks")
	}
	if hits[0].TaskID != "task_a" {
		t.Fatalf("strongest hit should be task_a, got %s", hits[0].TaskID)
	}
	for _, h := range hits {
		if h.TaskID == "task_c" {
			t.Fatal("terminal DONE task must never be reported")
		}
	}
}

func TestAnalyzeRequirementImpactEmptyRefs(t *testing.T) {
	hits := AnalyzeRequirementImpact(nil, []ImpactTask{{ID: "task_a", Title: "Something", Status: "READY"}})
	if len(hits) != 0 {
		t.Fatalf("empty refs should yield no hits, got %v", hits)
	}
	hits = AnalyzeRequirementImpact([]string{"payment"}, []ImpactTask{{ID: "task_a", Title: "Unrelated docs", Status: "READY"}})
	if len(hits) != 0 {
		t.Fatalf("no overlap should yield no hits, got %v", hits)
	}
}
