package skills

import "fmt"

type Outcome struct {
	SkillID          string `json:"skillId"`
	TechnicalSuccess bool   `json:"technicalSuccess"`
	UserAccepted     bool   `json:"userAccepted"`
	TaskCompleted    bool   `json:"taskCompleted"`
	RetryCount       int    `json:"retryCount"`
	Reverted         bool   `json:"reverted"`
	ExplicitRating   int    `json:"explicitRating,omitempty"`
}

func (o Outcome) Validate() error {
	if o.SkillID == "" {
		return fmt.Errorf("skill id is required")
	}
	if o.RetryCount < 0 {
		return fmt.Errorf("retry count cannot be negative")
	}
	if o.ExplicitRating < 0 || o.ExplicitRating > 5 {
		return fmt.Errorf("explicit rating must be between 0 and 5")
	}
	return nil
}

type CandidateState string

const (
	CandidateProposed   CandidateState = "proposed"
	CandidateEvaluating CandidateState = "evaluating"
	CandidateCanary     CandidateState = "canary"
	CandidatePromoted   CandidateState = "promoted"
	CandidateRejected   CandidateState = "rejected"
	CandidateRolledBack CandidateState = "rolled_back"
)

func CanTransition(from, to CandidateState) bool {
	switch from {
	case CandidateProposed:
		return to == CandidateEvaluating || to == CandidateRejected
	case CandidateEvaluating:
		return to == CandidateCanary || to == CandidateRejected
	case CandidateCanary:
		return to == CandidatePromoted || to == CandidateRejected
	case CandidatePromoted:
		return to == CandidateRolledBack
	default:
		return false
	}
}

type RankingSignal struct {
	SkillID string  `json:"skillId"`
	Delta   float64 `json:"delta"`
}

func RankingSignalFromOutcome(outcome Outcome) (RankingSignal, error) {
	if err := outcome.Validate(); err != nil {
		return RankingSignal{}, err
	}
	delta := 0.0
	if outcome.TechnicalSuccess {
		delta += 0.10
	} else {
		delta -= 0.12
	}
	if outcome.TaskCompleted {
		delta += 0.12
	}
	if outcome.UserAccepted {
		delta += 0.15
	} else if outcome.TaskCompleted {
		delta -= 0.08
	}
	if outcome.Reverted {
		delta -= 0.25
	}
	if outcome.RetryCount > 0 {
		delta -= minFloat(float64(outcome.RetryCount)*0.03, 0.15)
	}
	if outcome.ExplicitRating > 0 {
		delta += (float64(outcome.ExplicitRating) - 3) * 0.03
	}
	return RankingSignal{SkillID: outcome.SkillID, Delta: round(clamp(delta, -0.5, 0.5))}, nil
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
