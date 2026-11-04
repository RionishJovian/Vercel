package broadband

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// ValidProfileName accepts a literal label within the database's 64-character
// bound, without renaming or trimming it. RouterOS builders must still quote it.
func ValidProfileName(name string) bool {
	if len(name) == 0 || !utf8.ValidString(name) || utf8.RuneCountInString(name) > 64 || strings.TrimSpace(name) != name {
		return false
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
