package usage

import (
	"errors"
	"strings"
	"sync"
	"time"
)

var (
	ErrReservationExists       = errors.New("usage reservation already exists")
	ErrReservationNotFound     = errors.New("usage reservation not found")
	ErrBudgetReservationDenied = errors.New("usage reservation would exceed hard budget")
)

type ReservationState string

const (
	ReservationOpen     ReservationState = "open"
	ReservationSettled  ReservationState = "settled"
	ReservationReleased ReservationState = "released"
)

type Reservation struct {
	ID        string           `json:"id"`
	Usage     TaskUsage        `json:"usage"`
	State     ReservationState `json:"state"`
	CreatedAt time.Time        `json:"createdAt"`
	UpdatedAt time.Time        `json:"updatedAt"`
}

type LedgerSnapshot struct {
	Budget           Budget         `json:"budget"`
	Committed        TaskUsage      `json:"committed"`
	Reserved         TaskUsage      `json:"reserved"`
	Projected        TaskUsage      `json:"projected"`
	Decision         BudgetDecision `json:"decision"`
	OpenReservations int            `json:"openReservations"`
}

// Ledger provides admission control for a whole task/agent tree. Reservations
// are made before dispatch and settled with actual provider usage afterwards.
type Ledger struct {
	mu           sync.Mutex
	budget       Budget
	committed    TaskUsage
	reservations map[string]Reservation
}

func NewLedger(budget Budget, committed TaskUsage) *Ledger {
	return &Ledger{budget: budget, committed: committed.Normalize(), reservations: map[string]Reservation{}}
}

func (l *Ledger) reservedLocked() TaskUsage {
	var total TaskUsage
	for _, reservation := range l.reservations {
		if reservation.State == ReservationOpen {
			total = Add(total, reservation.Usage)
		}
	}
	return total
}

func (l *Ledger) projectedLocked(extra *TaskUsage) TaskUsage {
	projected := Add(l.committed, l.reservedLocked())
	if extra != nil {
		projected = Add(projected, *extra)
	}
	return projected
}

func (l *Ledger) Reserve(id string, estimate TaskUsage) (Reservation, BudgetDecision, error) {
	id = strings.TrimSpace(id)
	if l == nil || id == "" {
		return Reservation{}, BudgetDecision{}, ErrReservationNotFound
	}
	estimate = estimate.Normalize()
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, exists := l.reservations[id]; exists {
		return Reservation{}, EvaluateBudget(l.budget, l.projectedLocked(nil)), ErrReservationExists
	}
	projected := l.projectedLocked(&estimate)
	decision := EvaluateBudget(l.budget, projected)
	if decision.State == BudgetHard {
		return Reservation{}, decision, ErrBudgetReservationDenied
	}
	now := time.Now().UTC()
	reservation := Reservation{ID: id, Usage: estimate, State: ReservationOpen, CreatedAt: now, UpdatedAt: now}
	l.reservations[id] = reservation
	return reservation, decision, nil
}

// Settle replaces a pre-dispatch estimate with actual observed usage. Actual
// usage is always committed even when it crosses the hard limit because the
// cost has already occurred; the returned decision prevents further dispatch.
func (l *Ledger) Settle(id string, actual TaskUsage) (TaskUsage, BudgetDecision, error) {
	id = strings.TrimSpace(id)
	if l == nil || id == "" {
		return TaskUsage{}, BudgetDecision{}, ErrReservationNotFound
	}
	actual = actual.Normalize()
	l.mu.Lock()
	defer l.mu.Unlock()
	reservation, exists := l.reservations[id]
	if !exists || reservation.State != ReservationOpen {
		return l.committed, EvaluateBudget(l.budget, l.projectedLocked(nil)), ErrReservationNotFound
	}
	reservation.State = ReservationSettled
	reservation.UpdatedAt = time.Now().UTC()
	l.reservations[id] = reservation
	l.committed = Add(l.committed, actual)
	decision := EvaluateBudget(l.budget, l.projectedLocked(nil))
	return l.committed, decision, nil
}

func (l *Ledger) Release(id string) (BudgetDecision, error) {
	id = strings.TrimSpace(id)
	if l == nil || id == "" {
		return BudgetDecision{}, ErrReservationNotFound
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	reservation, exists := l.reservations[id]
	if !exists || reservation.State != ReservationOpen {
		return EvaluateBudget(l.budget, l.projectedLocked(nil)), ErrReservationNotFound
	}
	reservation.State = ReservationReleased
	reservation.UpdatedAt = time.Now().UTC()
	l.reservations[id] = reservation
	return EvaluateBudget(l.budget, l.projectedLocked(nil)), nil
}

func (l *Ledger) Snapshot() LedgerSnapshot {
	if l == nil {
		return LedgerSnapshot{}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	reserved := l.reservedLocked()
	projected := Add(l.committed, reserved)
	open := 0
	for _, reservation := range l.reservations {
		if reservation.State == ReservationOpen {
			open++
		}
	}
	return LedgerSnapshot{Budget: l.budget, Committed: l.committed, Reserved: reserved, Projected: projected, Decision: EvaluateBudget(l.budget, projected), OpenReservations: open}
}
