package cloudserver

import (
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type leaderboardEntry struct {
	Email  string
	Calls  int64
	Tokens int64
}

type leaderboardCommand struct {
	entry *leaderboardEntry
	cmd   *redis.MapStringStringCmd
}

func usageDayStart(createdAt int64) int64 {
	const dayMs = int64(24 * time.Hour / time.Millisecond)
	return (createdAt / dayMs) * dayMs
}

func usageDayKey(userID string, startedAt int64) string {
	return "codelocal:usage:d:" + userID + ":" + strconv.FormatInt(startedAt, 10)
}

func usageValue(values map[string]string, key string) int64 {
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

func leaderboardInitial(email string) string {
	value := []rune(strings.TrimSpace(email))
	if len(value) == 0 {
		return "C"
	}
	return strings.ToUpper(string(value[0]))
}
