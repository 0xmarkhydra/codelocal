package cloudserver

import (
	"fmt"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
)

func compactHealthAge(milliseconds int64) string {
	if milliseconds <= 0 {
		return "0s"
	}
	duration := time.Duration(milliseconds) * time.Millisecond
	switch {
	case duration < time.Minute:
		return fmt.Sprintf("%ds", int(duration/time.Second))
	case duration < time.Hour:
		return fmt.Sprintf("%dm", int(duration/time.Minute))
	default:
		return fmt.Sprintf("%dh%02dm", int(duration/time.Hour), int(duration%time.Hour/time.Minute))
	}
}

func durableLearningHealthCard(health cloud.DurableOutboxHealth) string {
	status := strings.ToLower(strings.TrimSpace(health.Status))
	if status == "" {
		status = "unavailable"
	}
	title := "Durable learning pipeline"
	detail := "Verified learning events are flowing normally."
	switch status {
	case "degraded":
		detail = "Learning is still available, but retries, backlog age or dead-letter evidence needs attention. User coding actions remain independent of this pipeline."
	case "critical":
		detail = "Durable learning is materially backlogged or has accumulated critical dead letters. Coding actions remain available, but Brain learning needs recovery before relying on freshness."
	case "unavailable":
		detail = "Durable learning health could not be sampled. No task payload or error content is exposed in this dashboard."
	}
	return `<div class="card" style="max-width:1600px;margin:0 auto 10px;padding:14px 18px">` +
		`<div class="section-head"><div><div class="section-kicker">Project Brain pipeline</div><div class="title">` + title + `</div><div class="label">` + detail + `</div></div><span class="badge blue">` + strings.ToUpper(status) + `</span></div>` +
		`<div class="row-meta" style="margin-top:10px">Pending ` + fmt.Sprintf("%d", health.PendingCount) +
		` · Processing ` + fmt.Sprintf("%d", health.ProcessingCount) +
		` · Retrying ` + fmt.Sprintf("%d", health.RetryingCount) +
		` · Dead ` + fmt.Sprintf("%d", health.DeadCount) +
		` · Processed last hour ` + fmt.Sprintf("%d", health.ProcessedLastHour) +
		` · Oldest active ` + compactHealthAge(health.OldestActiveAgeMS) + `</div></div>`
}
