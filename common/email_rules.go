package common

import "strings"

func IsQQEmailNumericAddress(email string) bool {
	parts := strings.Split(email, "@")
	if len(parts) != 2 || !strings.EqualFold(parts[1], "qq.com") {
		return true
	}
	localPart := strings.TrimSpace(parts[0])
	if localPart == "" {
		return false
	}
	for _, r := range localPart {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
