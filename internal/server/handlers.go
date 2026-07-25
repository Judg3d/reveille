package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"reveille/internal/config"
	"reveille/internal/hosts"
	"reveille/internal/notify"
)

const leaseJSONBodyLimit = 1 << 20

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func (s *Server) forwardAuth(w http.ResponseWriter, r *http.Request) {
	if secret := s.deps.Config.Server.ForwardAuthSecret; secret != "" {
		provided := r.Header.Get("X-Reveille-Auth")
		if subtle.ConstantTimeCompare([]byte(provided), []byte(secret)) != 1 {
			s.deps.Logger.Warnf("forward-auth rejected: missing or invalid shared secret")
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
	}
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
	healthy, err := s.cachedHealthy(r.Context(), host)
	if err != nil {
		s.deps.Logger.Errorf("health %s: %v", host.Host, err)
		http.Error(w, "failed to check target health", http.StatusInternalServerError)
		return
	}
	if healthy {
		s.deps.Leases.Touch(host.Host, s.now())
		w.WriteHeader(http.StatusNoContent)
		return
	}
	returnTo := originalURL(r, host)
	if host.Target.StartMode == "manual" {
		http.Redirect(w, r, s.waitURL(w, r, host.Host, returnTo), http.StatusFound)
		return
	}
	if !s.limiter.Allow("start:" + host.Host) {
		// Too many start attempts; send the visitor to the wait page where
		// polling picks up once the pending start lands.
		http.Redirect(w, r, s.waitURL(w, r, host.Host, returnTo), http.StatusFound)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), s.deps.Config.Defaults.StartTimeout)
	defer cancel()
	if err := s.startTarget(ctx, host); err != nil {
		s.deps.Logger.Errorf("start %s: %v", host.Host, err)
		http.Error(w, "failed to start target", http.StatusServiceUnavailable)
		return
	}
	s.ensureProvisionalLease(host)
	http.Redirect(w, r, s.waitURL(w, r, host.Host, returnTo), http.StatusFound)
}

// ensureProvisionalLease guarantees a started target always has a stop timer
// armed, so wakes that never reach the wait page (crawlers, closed tabs) do
// not leave the target running forever.
func (s *Server) ensureProvisionalLease(host hosts.Host) {
	grace := config.LeaseDuration{
		Label:    "orphan-grace",
		Duration: s.deps.Config.Defaults.OrphanGrace,
	}
	lease, created := s.deps.Leases.EnsureProvisional(host, grace, s.now())
	if !created {
		return
	}
	s.deps.Logger.Infof("provisional lease for %s until %s", host.Host, lease.ExpiresAt.Format(time.RFC3339))
	s.deps.Notify.Send(notify.Event{
		Type:    notify.EventWake,
		Host:    host.Host,
		Target:  host.Target.Name,
		Message: "Target was started on demand. It stops at " + lease.ExpiresAt.Format(time.RFC3339) + " unless a timer is chosen.",
	})
}

func (s *Server) healthy(ctx context.Context, host hosts.Host) (bool, error) {
	if host.Target.HealthURL != "" {
		return s.deps.Health.Healthy(ctx, host.Target), nil
	}
	return s.deps.Provider.Healthy(ctx, host.Target)
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
	if !s.allowMutation(w, host) {
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
		option, found := matchLeaseOption(host.Lease.Options, value)
		if !found {
			s.deps.Logger.Warnf("lease rejected for %s: invalid lease %q", host.Host, value)
			http.Error(w, "invalid lease", http.StatusBadRequest)
			return
		}
		lease = option
	}
	active := s.deps.Leases.Set(host, lease, s.now())
	if active.Never {
		s.deps.Logger.Infof("lease accepted for %s: never", host.Host)
	} else {
		s.deps.Logger.Infof("lease accepted for %s: expiresAt=%s", host.Host, active.ExpiresAt.Format(time.RFC3339))
	}
	s.startIfStopped(host)
	writeJSON(w, active)
}

// matchLeaseOption finds the configured option the submitted value refers to,
// matching the display label or the canonical parsed lease.
func matchLeaseOption(options []config.LeaseDuration, value string) (config.LeaseDuration, bool) {
	parsed, parseErr := config.ParseLeaseDuration(value)
	for _, option := range options {
		if strings.EqualFold(option.Label, strings.TrimSpace(value)) {
			return option, true
		}
		if parseErr == nil && option.Equal(parsed) {
			return option, true
		}
	}
	return config.LeaseDuration{}, false
}

// startIfStopped kicks off a background start when a lease is created while
// the target is down. This covers manual startMode and re-starting after a
// stop without leaving the wait page.
func (s *Server) startIfStopped(host hosts.Host) {
	ctx, cancel := context.WithTimeout(context.Background(), s.deps.Config.Defaults.StartTimeout)
	healthy, err := s.cachedHealthy(ctx, host)
	if err == nil && healthy {
		cancel()
		return
	}
	go func() {
		defer cancel()
		if err := s.startTarget(ctx, host); err != nil {
			s.deps.Logger.Errorf("start %s after lease: %v", host.Host, err)
		}
	}()
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
	if !s.allowMutation(w, host) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), s.deps.Config.Defaults.StopGrace)
	defer cancel()
	if err := s.deps.Leases.StopNow(ctx, host); err != nil {
		s.deps.Logger.Errorf("stop %s: %v", host.Host, err)
		s.deps.Notify.Send(notify.Event{
			Type:    notify.EventStopFailed,
			Host:    host.Host,
			Target:  host.Target.Name,
			Message: "Manual stop failed; the target may still be running.",
			Error:   err.Error(),
		})
		http.Error(w, "failed to stop target", http.StatusBadGateway)
		return
	}
	s.healthCache.invalidate(host.Host)
	s.deps.Notify.Send(notify.Event{
		Type:    notify.EventManualStop,
		Host:    host.Host,
		Target:  host.Target.Name,
		Message: "Target was stopped from the wait page.",
	})
	writeJSON(w, map[string]string{"status": "stopped"})
}

func (s *Server) allowMutation(w http.ResponseWriter, host hosts.Host) bool {
	if s.limiter.Allow("mutate:" + host.Host) {
		return true
	}
	s.deps.Logger.Warnf("rate limited wait-control request for %s", host.Host)
	http.Error(w, "too many requests", http.StatusTooManyRequests)
	return false
}
