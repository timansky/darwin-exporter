//go:build darwin

package collector

import "testing"

func TestNormalizeSensitiveValue(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"<redacted>", ""},
		{"<private>", ""},
		{"(redacted)", ""},
		{"MyNetwork", "MyNetwork"},
		{"  HomeWiFi  ", "HomeWiFi"},
	}
	for _, tc := range cases {
		got := normalizeSensitiveValue(tc.in)
		if got != tc.want {
			t.Fatalf("normalizeSensitiveValue(%q)=%q, want %q", tc.in, got, tc.want)
		}
	}
}
