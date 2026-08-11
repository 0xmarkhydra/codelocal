package cloudserver

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/ui"
	"github.com/redis/go-redis/v9"
)

func mainCompactNumber(value int64) string {
	abs := value
	if abs < 0 {
		abs = -abs
	}
	format := func(divisor float64, suffix string, decimals int) string {
		text := fmt.Sprintf("%.*f", decimals, float64(value)/divisor)
		text = strings.TrimRight(strings.TrimRight(text, "0"), ".")
		return text + suffix
	}
	switch {
	case abs >= 1_000_000_000:
		return format(1_000_000_000, "B", 2)
	case abs >= 1_000_000:
		return format(1_000_000, "M", 2)
	case abs >= 1_000:
		return format(1_000, "K", 1)
	default:
		return strconv.FormatInt(value, 10)
	}
}

func mainExactNumber(value int64) string {
	negative := value < 0
	if negative {
		value = -value
	}
	raw := strconv.FormatInt(value, 10)
	for i := len(raw) - 3; i > 0; i -= 3 {
		raw = raw[:i] + "," + raw[i:]
	}
	if negative {
		return "-" + raw
	}
	return raw
}

const (
	mainUsageReferenceInputUSDPerMillion  = 5.0
	mainUsageReferenceOutputUSDPerMillion = 30.0
	mainUsageReferenceInputShare          = 0.5
	mainUsageReferenceOutputShare         = 0.5
)

func mainUsageReferenceUSD(tokens int64) string {
	blendedPerMillion := mainUsageReferenceInputUSDPerMillion*mainUsageReferenceInputShare + mainUsageReferenceOutputUSDPerMillion*mainUsageReferenceOutputShare
	value := float64(tokens) / 1_000_000 * blendedPerMillion
	switch {
	case value >= 10:
		return fmt.Sprintf("≈ $%.0f ref.", value)
	case value >= 1:
		return fmt.Sprintf("≈ $%.1f ref.", value)
	default:
		return fmt.Sprintf("≈ $%.2f ref.", value)
	}
}

func mainUsageMetric(label string, value cloud.MCPUsageSummary) string {
	return `<div class="card metric-card usage-metric-card span4"><div class="metric-label">` + ui.Escape(label) + `</div><div class="usage-metric-main"><div class="metric">` + ui.Escape(mainCompactNumber(value.TotalTokensEst)) + `</div><div class="usage-reference">` + ui.Escape(mainUsageReferenceUSD(value.TotalTokensEst)) + `</div></div><div class="metric-sub">~` + ui.Escape(mainExactNumber(value.TotalTokensEst)) + ` estimated tokens · ` + ui.Escape(mainExactNumber(value.Calls)) + ` tool calls</div></div>`
}

type mainUsageLeaderboardEntry struct {
	Email  string
	Calls  int64
	Tokens int64
}

type mainUsageLeaderboardCommand struct {
	entry *mainUsageLeaderboardEntry
	cmd   *redis.MapStringStringCmd
}

func mainUsageDayStart(createdAt int64) int64 {
	const dayMs = int64(24 * time.Hour / time.Millisecond)
	return (createdAt / dayMs) * dayMs
}

func mainUsageDayKey(userID string, startedAt int64) string {
	return "codelocal:usage:d:" + userID + ":" + strconv.FormatInt(startedAt, 10)
}

func mainUsageValue(values map[string]string, key string) int64 {
	value, _ := strconv.ParseInt(values[key], 10, 64)
	return value
}

func maskLeaderboardEmail(email string) string {
	parts := strings.SplitN(strings.TrimSpace(email), "@", 2)
	if len(parts) != 2 || parts[0] == "" {
		return "CodeLocal user"
	}
	local := []rune(parts[0])
	visible := 1
	if len(local) >= 4 {
		visible = 2
	}
	return string(local[:visible]) + "***@" + parts[1]
}

func mainLeaderboardInitial(email string) string {
	value := []rune(strings.TrimSpace(email))
	if len(value) == 0 {
		return "C"
	}
	return strings.ToUpper(string(value[0]))
}

func (s *Server) mainUsageLeaderboard(ctx context.Context, currentEmail string) string {
	now := time.Now().UnixMilli()
	since := time.Now().Add(-30 * 24 * time.Hour).UnixMilli()
	users, err := s.Store.ListUsageLeaderboardUsers(ctx, since)
	if err != nil {
		return `<div class="card span12"><div class="empty">Unable to load leaderboard right now.</div></div>`
	}

	start := mainUsageDayStart(since)
	step := int64(24 * time.Hour / time.Millisecond)
	entries := make([]mainUsageLeaderboardEntry, 0, len(users))
	commands := make([]mainUsageLeaderboardCommand, 0, len(users)*31)
	pipe := s.Store.Redis.Pipeline()

	for _, user := range users {
		entries = append(entries, mainUsageLeaderboardEntry{Email: user.Email})
		entry := &entries[len(entries)-1]
		for at := start; at <= now; at += step {
			commands = append(commands, mainUsageLeaderboardCommand{entry: entry, cmd: pipe.HGetAll(ctx, mainUsageDayKey(user.ID, at))})
		}
	}
	if len(commands) > 0 {
		if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
			return `<div class="card span12"><div class="empty">Unable to load leaderboard right now.</div></div>`
		}
	}
	for _, pending := range commands {
		values := pending.cmd.Val()
		pending.entry.Calls += mainUsageValue(values, "calls")
		pending.entry.Tokens += mainUsageValue(values, "input_tokens") + mainUsageValue(values, "output_tokens")
	}

	active := entries[:0]
	for _, entry := range entries {
		if entry.Calls > 0 || entry.Tokens > 0 {
			active = append(active, entry)
		}
	}
	entries = active
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Tokens != entries[j].Tokens {
			return entries[i].Tokens > entries[j].Tokens
		}
		if entries[i].Calls != entries[j].Calls {
			return entries[i].Calls > entries[j].Calls
		}
		return strings.ToLower(entries[i].Email) < strings.ToLower(entries[j].Email)
	})

	currentRank := 0
	currentTokens := int64(0)
	for i, entry := range entries {
		if strings.EqualFold(entry.Email, currentEmail) {
			currentRank = i + 1
			currentTokens = entry.Tokens
			break
		}
	}
	if len(entries) == 0 {
		return `<div class="card span12"><div class="section-head"><div><div class="title">Top users</div><div class="label">Ranked by estimated MCP payload during the last 30 days. Emails are masked for privacy.</div></div><span class="badge muted">No rank yet</span></div><div class="empty">No MCP usage has been recorded in the last 30 days.</div></div>`
	}

	podiumOrder := []int{0}
	if len(entries) == 2 {
		podiumOrder = []int{0, 1}
	} else if len(entries) >= 3 {
		podiumOrder = []int{1, 0, 2}
	}
	var podium strings.Builder
	podium.WriteString(`<div class="leaderboard-podium">`)
	for _, index := range podiumOrder {
		entry := entries[index]
		className := "podium-card rank-" + strconv.Itoa(index+1)
		medalIcon := ui.Icon("medal")
		if index == 0 {
			className += " first"
			medalIcon = ui.Icon("crown")
		}
		name := maskLeaderboardEmail(entry.Email)
		if strings.EqualFold(entry.Email, currentEmail) {
			name = "You · " + name
		}
		podium.WriteString(`<div class="` + className + `"><div class="podium-top"><div class="podium-medal">` + medalIcon + `</div><div class="podium-avatar">` + ui.Escape(mainLeaderboardInitial(entry.Email)) + `</div></div><div class="podium-rank">#` + strconv.Itoa(index+1) + ` · Top 30 days</div><div class="podium-name">` + ui.Escape(name) + `</div><div class="podium-value">` + mainCompactNumber(entry.Tokens) + `</div><div class="podium-meta">estimated MCP tokens · ` + mainExactNumber(entry.Calls) + ` tool calls</div></div>`)
	}
	podium.WriteString(`</div>`)

	listStart := 3
	if len(entries) < listStart {
		listStart = len(entries)
	}
	listEnd := len(entries)
	if listEnd > 10 {
		listEnd = 10
	}
	var rows strings.Builder
	for i := listStart; i < listEnd; i++ {
		entry := entries[i]
		name := maskLeaderboardEmail(entry.Email)
		if strings.EqualFold(entry.Email, currentEmail) {
			name = "You · " + name
		}
		rows.WriteString(`<div class="row"><div class="entity"><span class="badge muted">#` + strconv.Itoa(i+1) + `</span><div class="entity-copy"><div class="row-title"><span class="row-title-text">` + ui.Escape(name) + `</span></div><div class="row-meta">` + mainExactNumber(entry.Calls) + ` tool calls · last 30 days</div></div></div><div class="actions"><div style="text-align:right"><div class="row-title">` + mainCompactNumber(entry.Tokens) + `</div><div class="row-meta">estimated MCP tokens</div></div></div></div>`)
	}
	if rows.Len() == 0 {
		rows.WriteString(`<div class="empty">No additional ranked users yet.</div>`)
	}

	yourRank := `<span class="badge muted">No rank yet</span>`
	if currentRank > 0 {
		yourRank = `<span class="badge blue">Your rank #` + strconv.Itoa(currentRank) + ` · ` + mainCompactNumber(currentTokens) + ` tokens</span>`
	}
	return podium.String() + `<div class="card span12"><div class="section-head"><div><div class="title">Leaderboard</div><div class="label">Ranks 4–10 by estimated MCP payload during the last 30 days. Emails are masked for privacy.</div></div>` + yourRank + `</div><div class="divider"></div><div class="list">` + rows.String() + `</div></div>`
}
