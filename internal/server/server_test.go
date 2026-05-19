package server

import "testing"

func TestSanitizeReturnToBlocksOpenRedirects(t *testing.T) {
	tests := map[string]string{
		"/":                    "/",
		"/library?tab=latest":  "/library?tab=latest",
		"https://example.com/": "/",
		"//example.com/path":   "/",
		"not/a/path":           "/",
	}
	for input, want := range tests {
		if got := sanitizeReturnTo(input); got != want {
			t.Fatalf("sanitizeReturnTo(%q) = %q, want %q", input, got, want)
		}
	}
}
