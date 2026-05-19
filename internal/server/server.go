package server

import (
	"context"
	"embed"
	"encoding/json"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"reveille/internal/config"
	"reveille/internal/dockhand"
	"reveille/internal/health"
	"reveille/internal/hosts"
	"reveille/internal/leases"
)

//go:embed templates/*.html static/*.css static/*.js
var assets embed.FS

type Dependencies struct {
	Config     config.Config
	Hosts      *hosts.Store
	Dockhand   *dockhand.Client
	Health     *health.Checker
	Leases     *leases.Manager
	StartClock func() time.Time
}

type Server struct {
	deps Dependencies
	tpl  *template.Template
}

type statusResponse struct {
	Host      string `json:"host"`
	Healthy   bool   `json:"healthy"`
	ReturnTo  string `json:"returnTo"`
	Lease     string `json:"lease,omitempty"`
	ExpiresAt string `json:"expiresAt,omitempty"`
	Never     bool   `json:"never,omitempty"`
}

func New(deps Dependencies) *Server {
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
	if s.deps.Health.Healthy(r.Context(), host.Target) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), s.deps.Config.Defaults.StartTimeout)
	defer cancel()
	if err := s.deps.Dockhand.Start(ctx, host.Target); err != nil {
		log.Printf("start %s: %v", host.Host, err)
		http.Error(w, "failed to start target", http.StatusServiceUnavailable)
		return
	}
	returnTo := originalURL(r, host)
	http.Redirect(w, r, s.waitURL(host.Host, returnTo), http.StatusFound)
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
	data := map[string]any{
		"Host":         host.Host,
		"PublicPath":   strings.TrimRight(s.deps.Config.Server.PublicPath, "/"),
		"ReturnTo":     returnTo,
		"PollMillis":   int(s.deps.Config.Defaults.PollInterval / time.Millisecond),
		"LeaseDefault": host.Lease.Default.Label,
		"LeaseOptions": host.Lease.Options,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tpl.ExecuteTemplate(w, "wait.html", data); err != nil {
		log.Printf("render wait: %v", err)
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
		Healthy:  s.deps.Health.Healthy(r.Context(), host.Target),
		ReturnTo: sanitizeReturnTo(r.URL.Query().Get("returnTo")),
	}
	if active, ok := s.deps.Leases.Get(host.Host); ok {
		resp.Never = active.Never
		if active.Never {
			resp.Lease = "Never"
		} else {
			resp.ExpiresAt = active.ExpiresAt.Format(time.RFC3339)
		}
	}
	writeJSON(w, resp)
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
			http.Error(w, "invalid lease", http.StatusBadRequest)
			return
		}
	}
	active := s.deps.Leases.Set(host, lease, s.deps.StartClock())
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

func (s *Server) waitURL(host, returnTo string) string {
	public := strings.TrimRight(s.deps.Config.Server.PublicPath, "/")
	q := url.Values{"host": {host}, "returnTo": {sanitizeReturnTo(returnTo)}}
	return public + "/wait?" + q.Encode()
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
