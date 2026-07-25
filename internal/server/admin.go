package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"html/template"
	"net/http"
	"time"

	"reveille/internal/hosts"
	"reveille/internal/leases"
	"reveille/internal/notify"
)

type adminHostView struct {
	Host        string        `json:"host"`
	Type        string        `json:"type"`
	Name        string        `json:"name"`
	ID          string        `json:"id,omitempty"`
	Environment string        `json:"environment"`
	StartMode   string        `json:"startMode"`
	Healthy     bool          `json:"healthy"`
	HealthError string        `json:"healthError,omitempty"`
	Lease       *leases.Lease `json:"lease,omitempty"`
	LeaseLabels []string      `json:"leaseLabels"`
}

// AdminRoutes serves the operator dashboard. It is meant for a separate,
// internal-only listener configured through admin.listen.
func (s *Server) AdminRoutes() http.Handler {
	protected := http.NewServeMux()
	protected.HandleFunc("GET /{$}", s.adminPage)
	protected.HandleFunc("GET /api/hosts", s.adminHosts)
	protected.HandleFunc("POST /api/hosts/{host}/start", s.adminStart)
	protected.HandleFunc("POST /api/hosts/{host}/stop", s.adminStop)
	protected.HandleFunc("POST /api/hosts/{host}/lease", s.adminLease)

	mux := http.NewServeMux()
	// Embedded CSS/JS stay outside auth: the browser cannot attach the admin
	// token to <link>/<script> requests, and the assets hold no secrets.
	mux.Handle("GET /static/", staticHandler(""))
	mux.Handle("/", s.adminAuth(protected))
	return securityHeaders(mux)
}

func (s *Server) adminAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := s.deps.Config.Admin.Token
		if token == "" {
			next.ServeHTTP(w, r)
			return
		}
		provided := r.Header.Get("X-Reveille-Admin-Token")
		if provided == "" {
			if auth := r.Header.Get("Authorization"); len(auth) > 7 && auth[:7] == "Bearer " {
				provided = auth[7:]
			}
		}
		if provided == "" {
			provided = r.URL.Query().Get("token")
		}
		if subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type adminView struct {
	ConfigJSON template.JS
}

func (s *Server) adminPage(w http.ResponseWriter, r *http.Request) {
	cfg := map[string]any{
		"token":      r.URL.Query().Get("token"),
		"pollMillis": int(s.deps.Config.Defaults.PollInterval / time.Millisecond),
	}
	cfgJSON, err := json.Marshal(cfg)
	if err != nil {
		http.Error(w, "failed to render admin config", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tpl.ExecuteTemplate(w, "admin.html", adminView{ConfigJSON: template.JS(cfgJSON)}); err != nil {
		s.deps.Logger.Errorf("render admin: %v", err)
	}
}

func (s *Server) adminHosts(w http.ResponseWriter, r *http.Request) {
	all := s.deps.Hosts.All()
	out := make([]adminHostView, 0, len(all))
	for _, host := range all {
		view := adminHostView{
			Host:        host.Host,
			Type:        host.Target.Type,
			Name:        host.Target.Name,
			ID:          host.Target.ID,
			Environment: host.Target.Environment,
			StartMode:   host.Target.StartMode,
			LeaseLabels: leaseLabels(host),
		}
		healthy, err := s.cachedHealthy(r.Context(), host)
		if err != nil {
			view.HealthError = err.Error()
		}
		view.Healthy = healthy
		if lease, ok := s.deps.Leases.Get(host.Host); ok {
			lease := lease
			view.Lease = &lease
		}
		out = append(out, view)
	}
	writeJSON(w, out)
}

func leaseLabels(host hosts.Host) []string {
	labels := make([]string, 0, len(host.Lease.Options))
	for _, option := range host.Lease.Options {
		labels = append(labels, option.Label)
	}
	return labels
}

func (s *Server) adminHost(w http.ResponseWriter, r *http.Request) (hosts.Host, bool) {
	host, ok := s.deps.Hosts.Lookup(r.PathValue("host"))
	if !ok {
		http.NotFound(w, r)
	}
	return host, ok
}

func (s *Server) adminStart(w http.ResponseWriter, r *http.Request) {
	host, ok := s.adminHost(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), s.deps.Config.Defaults.StartTimeout)
	defer cancel()
	if err := s.startTarget(ctx, host); err != nil {
		s.deps.Logger.Errorf("admin start %s: %v", host.Host, err)
		http.Error(w, "failed to start target", http.StatusBadGateway)
		return
	}
	s.ensureProvisionalLease(host)
	writeJSON(w, map[string]string{"status": "started"})
}

func (s *Server) adminStop(w http.ResponseWriter, r *http.Request) {
	host, ok := s.adminHost(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), s.deps.Config.Defaults.StopGrace)
	defer cancel()
	if err := s.deps.Leases.StopNow(ctx, host); err != nil {
		s.deps.Logger.Errorf("admin stop %s: %v", host.Host, err)
		http.Error(w, "failed to stop target", http.StatusBadGateway)
		return
	}
	s.healthCache.invalidate(host.Host)
	s.deps.Notify.Send(notify.Event{
		Type:    notify.EventManualStop,
		Host:    host.Host,
		Target:  host.Target.Name,
		Message: "Target was stopped from the admin dashboard.",
	})
	writeJSON(w, map[string]string{"status": "stopped"})
}

func (s *Server) adminLease(w http.ResponseWriter, r *http.Request) {
	host, ok := s.adminHost(w, r)
	if !ok {
		return
	}
	value := r.FormValue("lease")
	if value == "" {
		http.Error(w, "lease is required", http.StatusBadRequest)
		return
	}
	option, found := matchLeaseOption(host.Lease.Options, value)
	if !found {
		http.Error(w, "invalid lease", http.StatusBadRequest)
		return
	}
	active := s.deps.Leases.Set(host, option, s.now())
	s.startIfStopped(host)
	writeJSON(w, active)
}
