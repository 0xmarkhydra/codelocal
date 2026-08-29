package cloudserver

import (
	"net/http"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/webutil"
)

// adminSkillEvaluationCompleteV2API accepts both the original evaluator
// contract ({passed,score,checks}) and the browser-friendly contract
// ({score,evidence}). When passed is omitted the policy derives the decision
// from the same 0.80 threshold enforced by the domain lifecycle; clients cannot
// lower the promotion threshold by choosing their own boolean.
func (s *Server) adminSkillEvaluationCompleteV2API(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.skillMutationIdentity(w, r, true, true)
	if !ok {
		return
	}
	skillID, version := strings.TrimSpace(r.PathValue("skillID")), strings.TrimSpace(r.PathValue("version"))
	if _, ok := s.verifyAdminSkillPackage(w, r, skillID, version); !ok {
		return
	}
	var input struct {
		Passed   *bool          `json:"passed"`
		Score    float64        `json:"score"`
		Evidence string         `json:"evidence"`
		Checks   map[string]any `json:"checks"`
	}
	if webutil.DecodeJSON(r, 128<<10, &input) != nil || input.Score < 0 || input.Score > 1 {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_skill_evaluation"})
		return
	}
	if input.Checks == nil {
		input.Checks = map[string]any{}
	}
	if evidence := strings.TrimSpace(input.Evidence); evidence != "" {
		input.Checks["evidence"] = evidence
	}
	passed := input.Score >= 0.80
	if input.Passed != nil {
		passed = *input.Passed
	}
	// A client may explicitly reject a high-scoring candidate, but may never
	// explicitly pass a candidate below the authoritative threshold.
	if passed && input.Score < 0.80 {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "skill_evaluation_below_threshold"})
		return
	}
	record, evaluation, err := s.Store.CompleteSkillEvaluation(r.Context(), identity.User.ID, skillID, version, passed, input.Score, input.Checks)
	if err != nil {
		webutil.JSON(w, http.StatusConflict, map[string]string{"error": "skill_evaluation_rejected", "detail": err.Error()})
		return
	}
	s.Store.Audit(cloud.AuditEvent{UserID: identity.User.ID, Event: "skill.evaluation_completed", Detail: map[string]any{"skillId": skillID, "version": version, "decision": evaluation.Decision, "score": evaluation.Score}})
	webutil.JSON(w, http.StatusOK, map[string]any{"record": record, "evaluation": evaluation})
}
