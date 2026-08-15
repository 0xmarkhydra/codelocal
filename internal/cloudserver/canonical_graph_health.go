package cloudserver

import (
	"fmt"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
)

func canonicalGraphFreshnessCard(summary cloud.CanonicalGraphFreshnessSummary) string {
	status := strings.ToLower(strings.TrimSpace(summary.Status))
	if status == "" {
		status = "unavailable"
	}
	detail := "Derived canonical graph is synchronized with canonical Knowledge V2."
	switch status {
	case "degraded":
		detail = "Some derived graph projections are stale or missing. Canonical knowledge remains authoritative and the maintenance worker will rebuild the graph without changing source knowledge."
	case "unavailable":
		detail = "Graph freshness could not be sampled. Canonical knowledge remains authoritative; no knowledge content is exposed by this health check."
	}
	return `<div class="card" style="max-width:1600px;margin:0 auto 10px;padding:14px 18px">` +
		`<div class="section-head"><div><div class="section-kicker">Derived index health</div><div class="title">Canonical Knowledge Graph</div><div class="label">` + detail + `</div></div><span class="badge blue">` + strings.ToUpper(status) + `</span></div>` +
		`<div class="row-meta" style="margin-top:10px">Projects ` + fmt.Sprintf("%d", summary.ProjectCount) +
		` · Current ` + fmt.Sprintf("%d", summary.CurrentProjects) +
		` · Empty ` + fmt.Sprintf("%d", summary.EmptyProjects) +
		` · Stale ` + fmt.Sprintf("%d", summary.StaleProjects) +
		` · Missing ` + fmt.Sprintf("%d", summary.MissingProjects) +
		` · Max source lag ` + compactHealthAge(summary.MaxLagMS) + `</div></div>`
}
