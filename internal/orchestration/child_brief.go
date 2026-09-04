package orchestration

import (
	"errors"
	"sort"
	"strings"
	"time"
)

var ErrInvalidChildBrief = errors.New("invalid child brief")

type JoinVerdict string

const (
	JoinComplete    JoinVerdict = "complete"
	JoinNeedsReview JoinVerdict = "needs_review"
	JoinFailed      JoinVerdict = "failed"
)

// ChildBrief is the durable contract a parent hands a dispatched child: what
// to do, where it may read and write, how many tokens it may spend, and how
// its result is verified at join time. The brief is the join key — results
// that do not reference a known brief are ignored, never merged silently.
type ChildBrief struct {
	ID            string     "id"
	TaskID        string     "taskId"
	ParentAgentID string     "parentAgentId"
	Role          Specialist "role"
	Objective     string     "objective"
	ReadPaths     []string   "readPaths,omitempty"
	WritePaths    []string   "writePaths,omitempty"
	TokenBudget   int64      "tokenBudget"
	Verification  []string   "verification,omitempty"
	EdgeID        string     "edgeId,omitempty"
	CreatedAt     time.Time  "createdAt"
}

func IssueChildBrief(brief ChildBrief) (ChildBrief, error) {
	brief.ID = strings.TrimSpace(brief.ID)
	brief.TaskID = strings.TrimSpace(brief.TaskID)
	brief.ParentAgentID = strings.TrimSpace(brief.ParentAgentID)
	brief.Objective = strings.TrimSpace(brief.Objective)
	brief.ReadPaths = normalizeTaskStrings(brief.ReadPaths)
	brief.WritePaths = normalizeTaskStrings(brief.WritePaths)
	brief.Verification = normalizeTaskStrings(brief.Verification)
	brief.EdgeID = strings.TrimSpace(brief.EdgeID)
	if brief.ID == "" || brief.TaskID == "" || brief.ParentAgentID == "" || brief.Objective == "" {
		return ChildBrief{}, ErrInvalidChildBrief
	}
	if _, ok := DefaultSpecialistPolicies()[brief.Role]; !ok {
		return ChildBrief{}, ErrInvalidChildBrief
	}
	if brief.TokenBudget <= 0 {
		return ChildBrief{}, ErrInvalidChildBrief
	}
	brief.CreatedAt = time.Now().UTC()
	return brief, nil
}

type MemberOutcome struct {
	BriefID            string "briefId"
	Status             string "status"
	Summary            string "summary,omitempty"
	VerificationPassed bool   "verificationPassed"
}

type TeamJoin struct {
	Verdict  JoinVerdict "verdict"
	Complete []string    "complete,omitempty"
	Failed   []string    "failed,omitempty"
	Unvered  []string    "unverified,omitempty"
	Unknown  []string    "unknown,omitempty"
}

// JoinTeamResults aggregates child outcomes against issued briefs. The join
// is fail-closed: unknown brief references are reported, never merged; any
// failed member fails the join; completed-but-unverified members force a
// human review instead of a silent pass.
func JoinTeamResults(briefs []ChildBrief, outcomes []MemberOutcome) TeamJoin {
	known := make(map[string]struct{}, len(briefs))
	for _, brief := range briefs {
		if id := strings.TrimSpace(brief.ID); id != "" {
			known[id] = struct{}{}
		}
	}
	join := TeamJoin{Verdict: JoinComplete}
	for _, outcome := range outcomes {
		id := strings.TrimSpace(outcome.BriefID)
		if _, ok := known[id]; !ok {
			join.Unknown = append(join.Unknown, id)
			continue
		}
		switch strings.ToLower(strings.TrimSpace(outcome.Status)) {
		case "completed", "complete", "done":
			if outcome.VerificationPassed {
				join.Complete = append(join.Complete, id)
			} else {
				join.Unvered = append(join.Unvered, id)
			}
		default:
			join.Failed = append(join.Failed, id)
		}
	}
	sort.Strings(join.Complete)
	sort.Strings(join.Failed)
	sort.Strings(join.Unvered)
	sort.Strings(join.Unknown)
	switch {
	case len(join.Failed) > 0 || len(join.Unknown) > 0:
		join.Verdict = JoinFailed
	case len(join.Unvered) > 0:
		join.Verdict = JoinNeedsReview
	}
	return join
}
