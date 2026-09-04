package usage

import (
	"errors"
	"testing"
)

func TestLedgerPreventsParallelOverReservation(t *testing.T) {
	ledger := NewLedger(Budget{MaxTotalTokens: 1000}, TaskUsage{TaskID: "task", Tokens: TokenBreakdown{Code: 200}})
	if _, _, err := ledger.Reserve("a", TaskUsage{Tokens: TokenBreakdown{Code: 500}}); err != nil {
		t.Fatal(err)
	}
	_, decision, err := ledger.Reserve("b", TaskUsage{Tokens: TokenBreakdown{Code: 400}})
	if !errors.Is(err, ErrBudgetReservationDenied) || decision.State != BudgetHard {
		t.Fatalf("expected hard admission denial, decision=%+v err=%v", decision, err)
	}
}

func TestLedgerSettlementReplacesEstimateWithActual(t *testing.T) {
	ledger := NewLedger(Budget{MaxTotalTokens: 1000}, TaskUsage{})
	if _, _, err := ledger.Reserve("a", TaskUsage{Tokens: TokenBreakdown{Code: 600}}); err != nil {
		t.Fatal(err)
	}
	committed, decision, err := ledger.Settle("a", TaskUsage{Tokens: TokenBreakdown{Code: 250}})
	if err != nil || committed.Tokens.Total() != 250 || decision.State != BudgetWithin {
		t.Fatalf("unexpected settlement committed=%+v decision=%+v err=%v", committed, decision, err)
	}
	snapshot := ledger.Snapshot()
	if snapshot.OpenReservations != 0 || snapshot.Projected.Tokens.Total() != 250 {
		t.Fatalf("estimate was not replaced by actual: %+v", snapshot)
	}
}

func TestLedgerReleaseReturnsReservedCapacity(t *testing.T) {
	ledger := NewLedger(Budget{MaxTotalTokens: 1000}, TaskUsage{})
	if _, _, err := ledger.Reserve("a", TaskUsage{Tokens: TokenBreakdown{Code: 800}}); err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Release("a"); err != nil {
		t.Fatal(err)
	}
	if _, decision, err := ledger.Reserve("b", TaskUsage{Tokens: TokenBreakdown{Code: 800}}); err != nil || decision.State == BudgetHard {
		t.Fatalf("released capacity should be reusable, decision=%+v err=%v", decision, err)
	}
}
