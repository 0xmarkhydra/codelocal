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

func mainUsageMetric(label string, value cloud.MCPUsageSummary) string {
	return ui.MetricCard(
		label,
		mainCompactNumber(value.TotalTokensEst),
		"~"+mainExactNumber(value.TotalTokensEst)+" estimated tokens · "+mainExactNumber(value.Calls)+" tool calls",
	)
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

func (s *Server) mainUsageLeaderboard(ctx context.Context) string {
	users, err := s.Store.ListAdminUsers(ctx)
	if err != nil {
		return ""
	}

	now := time.Now().UnixMilli()
	since := time.Now().Add(-30 * 24 * time.Hour).UnixMilli()
	start := mainUsageDayStart(since)
	step := int64(24 * time.Hour / time.Millisecond)
	entries := make([]mainUsageLeaderboardEntry, 0, len(users))
	commands := make([]mainUsageLeaderboardCommand, 0, len(users)*31)
	pipe := s.Store.Redis.Pipeline()
	defer pipe.Close()

	for _, user := range users {
		// The durable all-time counter tells us whether this user has any chance
		// of appearing in the rolling window. Skipping old users keeps the Redis
		// pipeline small without storing any new leaderboard data.
		if user.LastMCPUsedAt < since {
			continue
		}
		entries = append(entries, mainUsageLeaderboardEntry{Email: user.Email})
		entry := &entries[len(entries)-1]
		for at := start; at <= now; at += step {
			commands = append(commands, mainUsageLeaderboardCommand{
				entry: entry,
				cmd:   pipe.HGetAll(ctx, mainUsageDayKey(user.ID, at)),
			})
		}
	}

	if len(commands) > 0 {
		if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
			return ""
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
	if len(entries) > 10 {
		entries = entries[:10]
	}

	var rows strings.Builder
	for i, entry := range entries {
		rankClass := "muted"
		if i == 0 {
			rankClass = "blue"
		}
		rows.WriteString(`<div class="row"><div class="entity"><span class="badge ` + rankClass + `">#` + strconv.Itoa(i+1) + `</span><div class="entity-copy"><div class="row-title"><span class="row-title-text">` + ui.Escape(entry.Email) + `</span></div><div class="row-meta">` + mainExactNumber(entry.Calls) + ` tool calls in the last 30 days</div></div></div><div class="actions"><div style="text-align:right"><div class="row-title">` + mainCompactNumber(entry.Tokens) + ` tokens</div><div class="row-meta">~` + mainExactNumber(entry.Tokens) + ` estimated</div></div></div></div>`)
	}
	if rows.Len() == 0 {
		rows.WriteString(`<div class="empty">No MCP usage has been recorded in the last 30 days.</div>`)
	}

	return `<div class="card span12"><div class="section-head"><div><div class="section-kicker">Community</div><div class="title">Usage leaderboard</div><div class="label">Top CodeLocal users by estimated MCP payload in the last 30 days.</div></div><span class="badge blue">Admin only · Top 10</span></div><div class="divider"></div><div class="list">` + rows.String() + `</div><div class="divider"></div><div class="label">Ranking reuses the existing rolling usage counters. No per-tool history or additional leaderboard records are stored.</div></div>`
}
