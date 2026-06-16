package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"reveille/internal/config"
	"reveille/internal/health"
	"reveille/internal/hosts"
	"reveille/internal/leases"
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

func TestStatusReportsActiveTimedLease(t *testing.T) {
	s, lease := newTestServer(t, http.StatusServiceUnavailable)
	host, ok := s.deps.Hosts.Lookup("pdf.example.com")
	if !ok {
		t.Fatal("host not loaded")
	}

	start := time.Date(2026, time.June, 16, 12, 0, 0, 0, time.UTC)
	lease.Set(host, config.LeaseDuration{Label: "1m", Duration: time.Minute}, start)

	req := httptest.NewRequest("GET", "/_reveille/api/status?host=pdf.example.com&returnTo=%2Fdocs", nil)
	rec := httptest.NewRecorder()

	s.status(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp statusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode status response: %v", err)
	}
	if !resp.LeaseActive {
		t.Fatal("leaseActive = false, want true")
	}
	if resp.Never {
		t.Fatal("never = true, want false")
	}
	if resp.ExpiresAt != start.Add(time.Minute).Format(time.RFC3339) {
		t.Fatalf("expiresAt = %q, want %q", resp.ExpiresAt, start.Add(time.Minute).Format(time.RFC3339))
	}
	if resp.StatusMessage != "App start was requested, but the health endpoint is responding with a non-healthy status." {
		t.Fatalf("statusMessage = %q", resp.StatusMessage)
	}
	if resp.ReadinessState != "health_unhealthy" {
		t.Fatalf("readinessState = %q", resp.ReadinessState)
	}
	if resp.HealthStatus != http.StatusServiceUnavailable {
		t.Fatalf("healthStatus = %d", resp.HealthStatus)
	}
	if resp.ReturnTo != "/docs" {
		t.Fatalf("returnTo = %q, want %q", resp.ReturnTo, "/docs")
	}
}

func TestStatusReportsNeverLease(t *testing.T) {
	s, lease := newTestServer(t, http.StatusServiceUnavailable)
	host, ok := s.deps.Hosts.Lookup("pdf.example.com")
	if !ok {
		t.Fatal("host not loaded")
	}

	lease.Set(host, config.LeaseDuration{Label: "Never", Never: true}, time.Now().UTC())

	req := httptest.NewRequest("GET", "/_reveille/api/status?host=pdf.example.com", nil)
	rec := httptest.NewRecorder()

	s.status(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp statusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode status response: %v", err)
	}
	if !resp.LeaseActive || !resp.Never {
		t.Fatalf("leaseActive=%t never=%t, want true/true", resp.LeaseActive, resp.Never)
	}
	if resp.Lease != "Never" {
		t.Fatalf("lease = %q, want %q", resp.Lease, "Never")
	}
	if resp.StatusMessage != "App start was requested, but the health endpoint is responding with a non-healthy status. Automatic stop is disabled." {
		t.Fatalf("statusMessage = %q", resp.StatusMessage)
	}
	if resp.ReadinessState != "health_unhealthy" {
		t.Fatalf("readinessState = %q", resp.ReadinessState)
	}
}

func TestStatusReportsHealthyRedirectState(t *testing.T) {
	s, _ := newTestServer(t, http.StatusOK)

	req := httptest.NewRequest("GET", "/_reveille/api/status?host=pdf.example.com&returnTo=%2F", nil)
	rec := httptest.NewRecorder()

	s.status(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp statusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode status response: %v", err)
	}
	if !resp.Healthy {
		t.Fatal("healthy = false, want true")
	}
	if resp.StatusMessage != "App is ready. Redirecting now." {
		t.Fatalf("statusMessage = %q", resp.StatusMessage)
	}
	if resp.ReadinessState != "ready" {
		t.Fatalf("readinessState = %q", resp.ReadinessState)
	}
}

func TestStatusReportsUnreachableHealthEndpoint(t *testing.T) {
	s, lease := newTestServerWithHealthURL(t, "://bad-health-url")
	host, ok := s.deps.Hosts.Lookup("pdf.example.com")
	if !ok {
		t.Fatal("host not loaded")
	}

	lease.Set(host, config.LeaseDuration{Label: "1m", Duration: time.Minute}, time.Now().UTC())

	req := httptest.NewRequest("GET", "/_reveille/api/status?host=pdf.example.com", nil)
	rec := httptest.NewRecorder()

	s.status(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp statusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode status response: %v", err)
	}
	if resp.ReadinessState != "health_unreachable" {
		t.Fatalf("readinessState = %q", resp.ReadinessState)
	}
	if resp.HealthError == "" {
		t.Fatal("healthError = empty, want diagnostic")
	}
	if resp.StatusMessage != "App start was requested, but Reveille cannot reach the health endpoint yet." {
		t.Fatalf("statusMessage = %q", resp.StatusMessage)
	}
}

func newTestServer(t *testing.T, healthStatus int) (*Server, *leases.Manager) {
	t.Helper()

	healthSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(healthStatus)
	}))
	t.Cleanup(healthSrv.Close)

	return newTestServerWithHealthURL(t, healthSrv.URL)
}

func newTestServerWithHealthURL(t *testing.T, healthURL string) (*Server, *leases.Manager) {
	t.Helper()

	hostDir := t.TempDir()
	configPath := filepath.Join(hostDir, "pdf.yml")

	if err := os.WriteFile(configPath, []byte(
		"target:\n  pdf:\n    type: container\n    id: pdf\n    hostname: pdf.example.com\n    healthUrl: \""+healthURL+"\"\n"), 0o644); err != nil {
		t.Fatalf("write host config: %v", err)
	}

	cfg := config.DefaultConfig()
	store, err := hosts.LoadDir(hostDir, cfg.Defaults)
	if err != nil {
		t.Fatalf("load hosts: %v", err)
	}

	leaseMgr := leases.NewManager(func(_ context.Context, _ hosts.Host) error { return nil })
	return &Server{
		deps: Dependencies{
			Config: cfg,
			Hosts:  store,
			Health: health.NewChecker(http.DefaultClient),
			Leases: leaseMgr,
		},
	}, leaseMgr
}
