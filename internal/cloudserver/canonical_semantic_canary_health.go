package cloudserver

import (
	"fmt"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
)

func canonicalSemanticCanaryCard(metrics cloud.CanonicalSemanticCanaryMetrics) string {
	status := "COLLECTING"
	detail := "Semantic Hybrid live canary has no aggregate samples yet. Deterministic Hybrid remains the fallback."
	if metrics.AttemptsTotal > 0 {
		status = "OBSERVING"
		detail = "Aggregate-only live canary outcomes. Query text, task content, summaries, repository paths and symbols are never stored here."
	}
	errors := metrics.ReadinessErrorCount + metrics.RecallErrorCount
	return `<div class="card" style="max-width:1600px;margin:0 auto 10px;padding:14px 18px">` +
		`<div class="section-head"><div><div class="section-kicker">Live retrieval canary</div><div class="title">Semantic Hybrid Outcomes</div><div class="label">` + detail + `</div></div><span class="badge blue">` + status + `</span></div>` +
		`<div class="row-meta" style="margin-top:10px">Attempts ` + fmt.Sprintf("%d", metrics.AttemptsTotal) +
		` · Applied ` + fmt.Sprintf("%d", metrics.AppliedCount) +
		` · Claims ` + fmt.Sprintf("%d", metrics.AppliedClaimsTotal) +
		` · Deterministic fallback ` + fmt.Sprintf("%d", metrics.DeterministicFallbackCount) +
		` · Readiness blocked ` + fmt.Sprintf("%d", metrics.ReadinessBlockedCount) +
		` · Errors ` + fmt.Sprintf("%d", errors) +
		` · Timeout ` + fmt.Sprintf("%d", metrics.TimeoutCount) +
		` · Slow ` + fmt.Sprintf("%d", metrics.SlowCount) + `</div></div>`
}
