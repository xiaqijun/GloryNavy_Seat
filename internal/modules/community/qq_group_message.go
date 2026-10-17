package community

import "strings"

// standaloneQQChallenge accepts exactly the alphabet emitted by newQQChallenge.
// A code embedded in a sentence, command, mention, or quoted text is not a match.
func standaloneQQChallenge(content string) string {
	code := strings.ToUpper(strings.TrimSpace(content))
	if len(code) != 8 {
		return ""
	}
	for _, c := range code {
		if !strings.ContainsRune("ABCDEFGHJKLMNPQRSTUVWXYZ23456789", c) {
			return ""
		}
	}
	return code
}
