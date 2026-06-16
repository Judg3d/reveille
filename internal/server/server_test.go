package server

import (
	"net/http/httptest"
	"testing"

	"reveille/internal/config"
)

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

func TestWaitURLUsesForwardedPublicURL(t *testing.T) {
	s := &Server{
		deps: Dependencies{
			Config: config.Config{
				Server: config.ServerConfig{PublicPath: "/_reveille"},
			},
		},
	}
	r := httptest.NewRequest("GET", "http://reveille:8080/api/traefik/forward-auth", nil)
	r.Header.Set("X-Forwarded-Host", "pdf.example.com")
	r.Header.Set("X-Forwarded-Proto", "https")

	got := s.waitURL(r, "pdf.example.com", "/")
	want := "https://pdf.example.com/_reveille/wait?host=pdf.example.com&returnTo=%2F"
	if got != want {
		t.Fatalf("waitURL() = %q, want %q", got, want)
	}
}

func TestWaitURLFallsBackToRelativePath(t *testing.T) {
	s := &Server{
		deps: Dependencies{
			Config: config.Config{
				Server: config.ServerConfig{PublicPath: "/_reveille"},
			},
		},
	}
	r := httptest.NewRequest("GET", "/api/traefik/forward-auth", nil)
	r.Host = ""

	got := s.waitURL(r, "pdf.example.com", "/")
	want := "/_reveille/wait?host=pdf.example.com&returnTo=%2F"
	if got != want {
		t.Fatalf("waitURL() = %q, want %q", got, want)
	}
}
