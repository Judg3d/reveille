package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reveille/internal/config"
	"reveille/internal/health"
	"reveille/internal/hosts"
	"reveille/internal/leases"
)

// newProviderTestServer builds a server whose host has no healthUrl, so
// health flows through the stub provider.
func newProviderTestServer(t *testing.T, prov *stubProvider, extraTarget string) (*Server, *leases.Manager) {
	t.Helper()

	hostDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(hostDir, "pdf.yml"), []byte(
		"target:\n  pdf:\n    type: container\n    id: pdf\n    environment: homelab\n    hostname: pdf.example.com\n"+extraTarget), 0o644); err != nil {
		t.Fatalf("write host config: %v", err)
	}

	cfg := config.DefaultConfig()
	store, err := hosts.LoadDir(hostDir, cfg.Defaults)
	if err != nil {
		t.Fatalf("load hosts: %v", err)
	}
	leaseMgr := leases.NewManager(func(ctx context.Context, host hosts.Host) error {
		return prov.Stop(ctx, host.Target)
	})
	return New(Dependencies{
		Config:   cfg,
		Hosts:    store,
		Provider: prov,
		Health:   health.NewChecker(http.DefaultClient),
		Leases:   leaseMgr,
	}), leaseMgr
}

func TestForwardAuthStartsTargetAndArmsProvisionalLease(t *testing.T) {
	prov := &stubProvider{healthy: false}
	s, leaseMgr := newProviderTestServer(t, prov, "")
	handler := s.Routes()

	req := httptest.NewRequest("GET", "/api/traefik/forward-auth", nil)
	req.Header.Set("X-Forwarded-Host", "pdf.example.com")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status code = %d, want %d; body=%q", rec.Code, http.StatusFound, rec.Body.String())
	}
	if prov.starts.Load() != 1 {
		t.Fatalf("starts = %d, want 1", prov.starts.Load())
	}
	lease, ok := leaseMgr.Get("pdf.example.com")
	if !ok {
		t.Fatal("no lease armed after start")
	}
	if !lease.Provisional {
		t.Fatalf("lease = %+v, want provisional", lease)
	}
	if lease.ExpiresAt.IsZero() {
		t.Fatal("provisional lease has no expiry")
	}
}

func TestForwardAuthManualModeDoesNotStart(t *testing.T) {
	prov := &stubProvider{healthy: false}
	s, leaseMgr := newProviderTestServer(t, prov, "    startMode: manual\n")
	handler := s.Routes()

	req := httptest.NewRequest("GET", "/api/traefik/forward-auth", nil)
	req.Header.Set("X-Forwarded-Host", "pdf.example.com")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusFound)
	}
	if prov.starts.Load() != 0 {
		t.Fatalf("starts = %d, want 0 in manual mode", prov.starts.Load())
	}
	if _, ok := leaseMgr.Get("pdf.example.com"); ok {
		t.Fatal("manual mode must not arm a lease")
	}
}

func TestForwardAuthTouchesLeaseWhenHealthy(t *testing.T) {
	prov := &stubProvider{healthy: true}
	s, _ := newProviderTestServer(t, prov, "")
	handler := s.Routes()

	req := httptest.NewRequest("GET", "/api/traefik/forward-auth", nil)
	req.Header.Set("X-Forwarded-Host", "pdf.example.com")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if prov.starts.Load() != 0 {
		t.Fatalf("starts = %d, want 0 for healthy target", prov.starts.Load())
	}
}

func TestForwardAuthSharedSecret(t *testing.T) {
	prov := &stubProvider{healthy: true}
	s, _ := newProviderTestServer(t, prov, "")
	s.deps.Config.Server.ForwardAuthSecret = "s3cret"
	handler := s.Routes()

	req := httptest.NewRequest("GET", "/api/traefik/forward-auth", nil)
	req.Header.Set("X-Forwarded-Host", "pdf.example.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("without secret: status = %d, want %d", rec.Code, http.StatusForbidden)
	}

	req = httptest.NewRequest("GET", "/api/traefik/forward-auth", nil)
	req.Header.Set("X-Forwarded-Host", "pdf.example.com")
	req.Header.Set("X-Reveille-Auth", "s3cret")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("with secret: status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestForwardAuthDeduplicatesHealthChecksViaCache(t *testing.T) {
	prov := &stubProvider{healthy: true}
	s, _ := newProviderTestServer(t, prov, "")
	handler := s.Routes()

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/api/traefik/forward-auth", nil)
		req.Header.Set("X-Forwarded-Host", "pdf.example.com")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d", rec.Code)
		}
	}
	if got := prov.healthChecks.Load(); got != 1 {
		t.Fatalf("health checks = %d, want 1 (cached)", got)
	}
}

func TestStatusHidesHealthErrorByDefault(t *testing.T) {
	s, _ := newTestServerWithHealthURL(t, "http://127.0.0.1:1/health")
	handler := s.Routes()

	req := httptest.NewRequest("GET", "/_reveille/api/status?host=pdf.example.com&token="+url.QueryEscape(waitToken(t, s, "pdf.example.com", "/")), nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var resp statusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if resp.HealthError != "health endpoint unreachable" {
		t.Fatalf("healthError = %q, want generic message", resp.HealthError)
	}
}

func TestStatusExposesHealthErrorWhenEnabled(t *testing.T) {
	s, _ := newTestServerWithHealthURL(t, "http://127.0.0.1:1/health")
	s.deps.Config.Server.ExposeHealthDetail = true
	handler := s.Routes()

	req := httptest.NewRequest("GET", "/_reveille/api/status?host=pdf.example.com&token="+url.QueryEscape(waitToken(t, s, "pdf.example.com", "/")), nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var resp statusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if resp.HealthError == "" || resp.HealthError == "health endpoint unreachable" {
		t.Fatalf("healthError = %q, want raw diagnostic", resp.HealthError)
	}
}

func TestWaitPageSetsSessionCookie(t *testing.T) {
	s, _ := newTestServer(t, http.StatusServiceUnavailable)
	handler := s.Routes()

	req := httptest.NewRequest("GET", "/_reveille/wait?host=pdf.example.com&token="+url.QueryEscape(waitToken(t, s, "pdf.example.com", "/")), nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	cookie := findCookie(rec.Result().Cookies(), waitCookieName)
	if cookie == nil {
		t.Fatal("wait page did not set session cookie")
	}
	if !cookie.HttpOnly {
		t.Fatal("session cookie must be HttpOnly")
	}
	if _, err := s.verifyWaitToken(cookie.Value); err != nil {
		t.Fatalf("cookie token invalid: %v", err)
	}
}

func TestStatusAcceptsCookieToken(t *testing.T) {
	s, _ := newTestServer(t, http.StatusServiceUnavailable)
	handler := s.Routes()

	req := httptest.NewRequest("GET", "/_reveille/api/status?host=pdf.example.com", nil)
	req.AddCookie(&http.Cookie{Name: waitCookieName, Value: waitToken(t, s, "pdf.example.com", "/")})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%q", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestSecurityHeadersPresent(t *testing.T) {
	s, _ := newTestServer(t, http.StatusServiceUnavailable)
	handler := s.Routes()

	req := httptest.NewRequest("GET", "/healthz", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	headers := rec.Result().Header
	for _, name := range []string{"Content-Security-Policy", "Referrer-Policy", "X-Content-Type-Options"} {
		if headers.Get(name) == "" {
			t.Fatalf("missing %s header", name)
		}
	}
}

func TestLeaseMutationsAreRateLimited(t *testing.T) {
	s, _ := newTestServer(t, http.StatusServiceUnavailable)
	handler := s.Routes()
	token := waitToken(t, s, "pdf.example.com", "/")

	var lastCode int
	for i := 0; i < 12; i++ {
		req := httptest.NewRequest("POST", "/_reveille/wait?host=pdf.example.com", strings.NewReader("action=lease&lease=30m&token="+url.QueryEscape(token)))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		lastCode = rec.Code
	}
	if lastCode != http.StatusTooManyRequests {
		t.Fatalf("12th mutation status = %d, want %d", lastCode, http.StatusTooManyRequests)
	}
}

func TestLeaseMatchesCanonicalValue(t *testing.T) {
	options, err := config.ParseLeaseDurations([]string{"30m", "1h", "idle:20m", "never"})
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]string{
		"30m":      "30m",
		"0.5h":     "30m",
		"Never":    "Never",
		"idle:20m": "idle:20m",
		"IDLE:20m": "idle:20m",
	}
	for value, wantLabel := range tests {
		option, ok := matchLeaseOption(options, value)
		if !ok {
			t.Fatalf("matchLeaseOption(%q) not found", value)
		}
		if option.Label != wantLabel {
			t.Fatalf("matchLeaseOption(%q) = %q, want %q", value, option.Label, wantLabel)
		}
	}
	if _, ok := matchLeaseOption(options, "45m"); ok {
		t.Fatal("matchLeaseOption(45m) matched, want rejection")
	}
}

func TestAdminRoutesRequireToken(t *testing.T) {
	prov := &stubProvider{healthy: true}
	s, _ := newProviderTestServer(t, prov, "")
	s.deps.Config.Admin.Token = "admin-token"
	handler := s.AdminRoutes()

	req := httptest.NewRequest("GET", "/api/hosts", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("without token: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	req = httptest.NewRequest("GET", "/api/hosts", nil)
	req.Header.Set("X-Reveille-Admin-Token", "admin-token")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("with token: status = %d, want %d; body=%q", rec.Code, http.StatusOK, rec.Body.String())
	}
	var out []adminHostView
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode hosts: %v", err)
	}
	if len(out) != 1 || out[0].Host != "pdf.example.com" || !out[0].Healthy {
		t.Fatalf("hosts = %+v", out)
	}
}

func TestAdminStartAndStop(t *testing.T) {
	prov := &stubProvider{healthy: false}
	s, leaseMgr := newProviderTestServer(t, prov, "")
	handler := s.AdminRoutes()

	req := httptest.NewRequest("POST", "/api/hosts/pdf.example.com/start", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("start status = %d; body=%q", rec.Code, rec.Body.String())
	}
	if prov.starts.Load() != 1 {
		t.Fatalf("starts = %d, want 1", prov.starts.Load())
	}
	if lease, ok := leaseMgr.Get("pdf.example.com"); !ok || !lease.Provisional {
		t.Fatalf("lease after admin start = %+v ok=%t, want provisional", lease, ok)
	}

	req = httptest.NewRequest("POST", "/api/hosts/pdf.example.com/stop", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("stop status = %d; body=%q", rec.Code, rec.Body.String())
	}
	if prov.stops.Load() != 1 {
		t.Fatalf("stops = %d, want 1", prov.stops.Load())
	}
	if _, ok := leaseMgr.Get("pdf.example.com"); ok {
		t.Fatal("lease still active after admin stop")
	}
}

func findCookie(cookies []*http.Cookie, name string) *http.Cookie {
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
}
