package releasegate

import "testing"

func TestSummarizeBoardFlows(t *testing.T) {
	if _, err := SummarizeBoard(nil, BoardSignals{}); err == nil {
		t.Fatal("empty samples accepted")
	}
	samples := suiteSamples(4, true)
	samples = append(samples, TaskSample{Tokens: 200, LatencyMillis: 60, Verified: false, ResumeAttempted: true})
	signals := BoardSignals{CostPerVerifiedTask: 0.42, ModelCallsPerTask: 3, ToolCallsPerTask: 9, RetryWaste: 2, BrowserVerificationRate: 0.5}
	board, err := SummarizeBoard(samples, signals)
	if err != nil {
		t.Fatal(err)
	}
	if board.VerifiedSuccessRate != 0.8 {
		t.Fatalf("success rate = %v, want 0.8", board.VerifiedSuccessRate)
	}
	if board.TokensPerVerifiedTask != 100 || board.CostPerVerifiedTask != 0.42 || board.RetryWaste != 2 {
		t.Fatalf("board mismatch: %+v", board)
	}
	if board.BrowserVerificationRate != 0.5 || board.MobileVerificationRate != 0 {
		t.Fatalf("lane rates mismatch: %+v", board)
	}
}
