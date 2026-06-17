package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"reveille/internal/hosts"
)

const leaseJSONBodyLimit = 1 << 20

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func (s *Server) forwardAuth(w http.ResponseWriter, r *http.Request) {
	hostName := r.Header.Get("X-Forwarded-Host")
	host, ok := s.deps.Hosts.Lookup(hostName)
	if !ok {
		if s.deps.Config.Server.FailClosedUnknownHosts {
			s.deps.Logger.Warnf("forward-auth rejected unknown host %q", hostName)
			http.NotFound(w, r)
			return
		}
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
	host, _, ok := s.authorizedHost(w, r)
	if !ok {
		return
	}
	if !s.validateRequestOrigin(w, r, host) {
		return
	}
	value := r.FormValue("lease")
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		var body struct {
			Lease string `json:"lease"`
		}
		limited := http.MaxBytesReader(w, r.Body, leaseJSONBodyLimit)
		defer limited.Close()
		if err := json.NewDecoder(limited).Decode(&body); err != nil {
			if errors.Is(err, io.EOF) {
				http.Error(w, "empty JSON body", http.StatusBadRequest)
				return
			}
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
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
	host, _, ok := s.authorizedHost(w, r)
	if !ok {
		return
	}
	if !s.validateRequestOrigin(w, r, host) {
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
