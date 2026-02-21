//go:build darwin

package collector

import "strings"

// normalizeSensitiveValue converts macOS redaction placeholders to empty
// strings so fallback logic can treat them as missing values.
func normalizeSensitiveValue(v string) string {
	if isRedactedValue(v) {
		return ""
	}
	return strings.TrimSpace(v)
}

func isRedactedValue(v string) bool {
	s := strings.ToLower(strings.TrimSpace(v))
	switch s {
	case "<redacted>", "redacted", "<private>", "private", "(redacted)", "(private)", "<hidden>":
		return true
	default:
		return false
	}
}
