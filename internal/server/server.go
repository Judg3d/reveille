package server

import (
	"context"
	"embed"
	"encoding/json"
	"html/template"
	"io/fs"
	"net/http"
	"net/url"
	"strings"
	"time"

	"reveille/internal/config"
	"reveille/internal/dockhand"
	"reveille/internal/health"
	"reveille/internal/hosts"
	"reveille/internal/leases"
	"reveille/internal/logging"
)

//go:embed templates/*.html static/*.css static/*.js
var assets embed.FS

type Dependencies struct {
	Config     config.Config
	Hosts      *hosts.Store
	Dockhand   *dockhand.Client
	Health     *health.Checker
	Leases     *leases.Manager
	Logger     *logging.Logger
	StartClock func() time.Time
}

type Server struct {
	deps Dependencies
	tpl  *template.Template
}

type statusResponse struct {
	Host           string `json:"host"`
	Healthy        bool   `json:"healthy"`
	ReturnTo       string `json:"returnTo"`
	Lease          string `json:"lease,omitempty"`
	LeaseActive    bool   `json:"leaseActive"`
	ExpiresAt      string `json:"expiresAt,omitempty"`
	Never          bool   `json:"never,omitempty"`
	StatusMessage  string `json:"statusMessage,omitempty"`
	ReadinessState string `json:"readinessState,omitempty"`
	HealthError    string `json:"healthError,omitempty"`
	LastCheck      string `json:"lastCheck,omitempty"`
	HealthStatus   int    `json:"healthStatus,omitempty"`
}

func New(deps Dependencies) *Server {
	if deps.Logger == nil {
		deps.Logger = logging.Must("info")
	}
	return &Server{
		deps: deps,
		tpl:  template.Must(template.ParseFS(assets, "templates/*.html")),
	}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.healthz)
	mux.HandleFunc("/api/traefik/forward-auth", s.forwardAuth)
	public := strings.TrimRight(s.deps.Config.Server.PublicPath, "/")
	mux.HandleFunc(public+"/wait", s.wait)
	mux.HandleFunc(public+"/api/status", s.status)
	mux.HandleFunc(public+"/api/lease", s.lease)
	mux.HandleFunc(public+"/api/stop", s.stop)
	mux.Handle(public+"/static/", http.StripPrefix(public+"/static/", http.FileServer(http.FS(mustSub(assets, "static")))))
	return mux
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func (s *Server) forwardAuth(w http.ResponseWriter, r *http.Request) {
	hostName := r.Header.Get("X-Forwarded-Host")
	host, ok := s.deps.Hosts.Lookup(hostName)
	if !ok {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	healthy, err := s.healthy(r.Context(), host)
	if err != nil {
		s.deps.Logger.Errorf("health %s: %v", host.Host, err)
		http.Error(w, "failed to check target health", http.StatusInternalServerError)
		return
	}
	if healthy {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), s.deps.Config.Defaults.StartTimeout)
	defer cancel()
	if err := s.deps.Dockhand.Start(ctx, host.Target); err != nil {
		s.deps.Logger.Errorf("start %s: %v", host.Host, err)
		http.Error(w, "failed to start target", http.StatusServiceUnavailable)
		return
	}
	returnTo := originalURL(r, host)
	http.Redirect(w, r, s.waitURL(r, host.Host, returnTo), http.StatusFound)
}

func (s *Server) wait(w http.ResponseWriter, r *http.Request) {
	host, ok := s.hostFromRequest(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	returnTo := sanitizeReturnTo(r.URL.Query().Get("returnTo"))
	if returnTo == "" {
		returnTo = "/"
	}
	clientCfg := map[string]any{
		"host":       host.Host,
		"returnTo":   returnTo,
		"publicPath": strings.TrimRight(s.deps.Config.Server.PublicPath, "/"),
		"pollMillis": int(s.deps.Config.Defaults.PollInterval / time.Millisecond),
	}
	cfgJSON, err := json.Marshal(clientCfg)
	if err != nil {
		http.Error(w, "failed to render wait config", http.StatusInternalServerError)
		return
	}
	data := map[string]any{
		"Host":         host.Host,
		"PublicPath":   strings.TrimRight(s.deps.Config.Server.PublicPath, "/"),
		"ReturnTo":     returnTo,
		"ConfigJSON":   string(cfgJSON),
		"LeaseDefault": host.Lease.Default.Label,
		"LeaseOptions": host.Lease.Options,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tpl.ExecuteTemplate(w, "wait.html", data); err != nil {
		s.deps.Logger.Errorf("render wait: %v", err)
	}
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	host, ok := s.hostFromRequest(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	resp := statusResponse{
		Host:     host.Host,
		ReturnTo: sanitizeReturnTo(r.URL.Query().Get("returnTo")),
	}
	if host.Target.HealthURL != "" {
		check := s.deps.Health.Check(r.Context(), host.Target)
		resp.Healthy = check.Healthy
		resp.LastCheck = check.CheckedAt.Format(time.RFC3339)
		if check.StatusCode != 0 {
			resp.HealthStatus = check.StatusCode
		}
		if check.Error != "" {
			resp.HealthError = check.Error
			s.deps.Logger.Warnf("status %s: health check failed: %s", host.Host, check.Error)
		}
	} else {
		healthy, err := s.healthy(r.Context(), host)
		if err != nil {
			s.deps.Logger.Warnf("status %s: health check failed: %v", host.Host, err)
			http.Error(w, "failed to check target health", http.StatusBadGateway)
			return
		}
		resp.Healthy = healthy
	}
	if active, ok := s.deps.Leases.Get(host.Host); ok {
		resp.LeaseActive = true
		resp.Never = active.Never
		if active.Never {
			resp.Lease = "Never"
		} else {
			resp.ExpiresAt = active.ExpiresAt.Format(time.RFC3339)
		}
	}
	resp.ReadinessState = readinessState(resp)
	resp.StatusMessage = statusMessage(resp)
	s.deps.Logger.Debugf("status %s: healthy=%t leaseActive=%t readiness=%s never=%t expiresAt=%q healthStatus=%d healthError=%q", host.Host, resp.Healthy, resp.LeaseActive, resp.ReadinessState, resp.Never, resp.ExpiresAt, resp.HealthStatus, resp.HealthError)
	writeJSON(w, resp)
}

func (s *Server) healthy(ctx context.Context, host hosts.Host) (bool, error) {
	if host.Target.HealthURL != "" {
		return s.deps.Health.Healthy(ctx, host.Target), nil
	}
	return s.deps.Dockhand.Healthy(ctx, host.Target)
}

func (s *Server) lease(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	host, ok := s.hostFromRequest(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	value := r.FormValue("lease")
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		var body struct {
			Lease string `json:"lease"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		value = body.Lease
	}
	lease := host.Lease.Default
	if value != "" {
		found := false
		for _, option := range host.Lease.Options {
			if strings.EqualFold(option.Label, value) || strings.EqualFold(option.Label, "Never") && strings.EqualFold(value, "never") {
				lease = option
				found = true
				break
			}
		}
		if !found {
			s.deps.Logger.Warnf("lease rejected for %s: invalid lease %q", host.Host, value)
			http.Error(w, "invalid lease", http.StatusBadRequest)
			return
		}
	}
	active := s.deps.Leases.Set(host, lease, s.deps.StartClock())
	if active.Never {
		s.deps.Logger.Infof("lease accepted for %s: never", host.Host)
	} else {
		s.deps.Logger.Infof("lease accepted for %s: expiresAt=%s", host.Host, active.ExpiresAt.Format(time.RFC3339))
	}
	writeJSON(w, active)
}

func (s *Server) stop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	host, ok := s.hostFromRequest(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), s.deps.Config.Defaults.StopGrace)
	defer cancel()
	if err := s.deps.Leases.StopNow(ctx, host); err != nil {
		http.Error(w, "failed to stop target", http.StatusBadGateway)
		return
	}
	writeJSON(w, map[string]string{"status": "stopped"})
}

func (s *Server) hostFromRequest(r *http.Request) (hosts.Host, bool) {
	hostName := r.URL.Query().Get("host")
	if hostName == "" {
		hostName = r.Header.Get("X-Forwarded-Host")
	}
	if hostName == "" {
		hostName = r.Host
	}
	return s.deps.Hosts.Lookup(hostName)
}

func (s *Server) waitURL(r *http.Request, host, returnTo string) string {
	public := strings.TrimRight(s.deps.Config.Server.PublicPath, "/")
	q := url.Values{"host": {host}, "returnTo": {sanitizeReturnTo(returnTo)}}
	path := public + "/wait?" + q.Encode()

	base := publicBaseURL(r)
	if base == "" {
		return path
	}
	return base + path
}

func originalURL(r *http.Request, host hosts.Host) string {
	uri := r.Header.Get(host.Routing.ReturnToHeader)
	if uri == "" {
		uri = r.Header.Get("X-Forwarded-Uri")
	}
	return sanitizeReturnTo(uri)
}

func sanitizeReturnTo(raw string) string {
	if raw == "" {
		return "/"
	}
	u, err := url.Parse(raw)
	if err != nil || u.IsAbs() || strings.HasPrefix(raw, "//") || !strings.HasPrefix(raw, "/") {
		return "/"
	}
	return raw
}

func statusMessage(resp statusResponse) string {
	switch {
	case resp.Healthy:
		return "App is ready. Redirecting now."
	case resp.LeaseActive && resp.ReadinessState == "health_unreachable" && resp.Never:
		return "App start was requested, but Reveille cannot reach the health endpoint yet. Automatic stop is disabled."
	case resp.LeaseActive && resp.ReadinessState == "health_unreachable":
		return "App start was requested, but Reveille cannot reach the health endpoint yet."
	case resp.LeaseActive && resp.ReadinessState == "health_unhealthy" && resp.Never:
		return "App start was requested, but the health endpoint is responding with a non-healthy status. Automatic stop is disabled."
	case resp.LeaseActive && resp.ReadinessState == "health_unhealthy":
		return "App start was requested, but the health endpoint is responding with a non-healthy status."
	case resp.LeaseActive && resp.Never:
		return "App start was requested. Waiting for health check before redirect. Automatic stop is disabled."
	case resp.LeaseActive:
		return "App start was requested. Waiting for health check before redirect."
	default:
		return "Choose a timer to keep this app running while Reveille waits for readiness."
	}
}

func readinessState(resp statusResponse) string {
	switch {
	case resp.Healthy:
		return "ready"
	case resp.HealthError != "":
		return "health_unreachable"
	case resp.HealthStatus != 0:
		return "health_unhealthy"
	default:
		return "waiting_for_health"
	}
}

func publicBaseURL(r *http.Request) string {
	host := firstHeaderValue(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = r.Host
	}
	if host == "" {
		return ""
	}

	proto := firstHeaderValue(r.Header.Get("X-Forwarded-Proto"))
	if proto == "" {
		if r.TLS != nil {
			proto = "https"
		} else {
			proto = "http"
		}
	}

	return proto + "://" + host
}

func firstHeaderValue(value string) string {
	if i := strings.Index(value, ","); i >= 0 {
		value = value[:i]
	}
	return strings.TrimSpace(value)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func mustSub(fsys embed.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic(err)
	}
	return sub
}
