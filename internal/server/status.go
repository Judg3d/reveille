package server

import (
	"net/http"
	"time"

	"reveille/internal/readiness"
)

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

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	host, token, ok := s.authorizedHost(w, r)
	if !ok {
		return
	}
	resp := statusResponse{
		Host:     host.Host,
		ReturnTo: token.ReturnTo,
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
	evaluation := readiness.Evaluate(readiness.Snapshot{
		Healthy:      resp.Healthy,
		LeaseActive:  resp.LeaseActive,
		Never:        resp.Never,
		HealthError:  resp.HealthError,
		HealthStatus: resp.HealthStatus,
	})
	resp.ReadinessState = string(evaluation.State)
	resp.StatusMessage = evaluation.Message
	s.deps.Logger.Debugf("status %s: healthy=%t leaseActive=%t readiness=%s never=%t expiresAt=%q healthStatus=%d healthError=%q", host.Host, resp.Healthy, resp.LeaseActive, resp.ReadinessState, resp.Never, resp.ExpiresAt, resp.HealthStatus, resp.HealthError)
	writeJSON(w, resp)
}
