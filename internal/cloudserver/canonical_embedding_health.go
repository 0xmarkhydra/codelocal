package cloudserver

import (
	"fmt"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
)

func canonicalEmbeddingFreshnessCard(summary cloud.CanonicalEmbeddingFreshnessSummary) string {
	status := strings.ToLower(strings.TrimSpace(summary.Status))
	if status == "" {
		status = "unavailable"
	}
	detail := "Canonical semantic index is synchronized with active private Knowledge V2 revisions."
	switch status {
	case "disabled":
		detail = "Canonical semantic indexing is disabled. Deterministic canonical retrieval remains available and authoritative."
	case "degraded":
		detail = "Some semantic projections are stale or missing. Canonical knowledge remains authoritative; semantic shadow retrieval stays fail-closed until the index is current."
	case "unavailable":
		detail = "Semantic index health could not be sampled. Canonical knowledge remains authoritative and no knowledge content is exposed by this health check."
	}
	return `<div class="card" style="max-width:1600px;margin:0 auto 10px;padding:14px 18px">` +
		`<div class="section-head"><div><div class="section-kicker">Derived index health</div><div class="title">Canonical Semantic Index</div><div class="label">` + detail + `</div></div><span class="badge blue">` + strings.ToUpper(status) + `</span></div>` +
		`<div class="row-meta" style="margin-top:10px">Projects ` + fmt.Sprintf("%d", summary.ProjectCount) +
		` · Current ` + fmt.Sprintf("%d", summary.CurrentProjects) +
		` · Empty ` + fmt.Sprintf("%d", summary.EmptyProjects) +
		` · Stale ` + fmt.Sprintf("%d", summary.StaleProjects) +
		` · Missing ` + fmt.Sprintf("%d", summary.MissingProjects) +
		` · Max source lag ` + compactHealthAge(summary.MaxLagMS) + `</div></div>`
}
