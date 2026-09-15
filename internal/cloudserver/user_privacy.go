package cloudserver

import "strings"

func maskUserEmail(email string) string {
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

func userInitial(email string) string {
	value := []rune(strings.TrimSpace(email))
	if len(value) == 0 {
		return "C"
	}
	return strings.ToUpper(string(value[0]))
}
