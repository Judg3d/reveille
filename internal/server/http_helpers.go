package server

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"reveille/internal/hosts"
)

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
	public := publicPath(s.deps.Config.Server.PublicPath)
	returnTo = sanitizeReturnTo(returnTo)
	q := url.Values{"host": {host}, "returnTo": {returnTo}}
	if token, err := s.newWaitToken(host, returnTo); err == nil {
		q.Set("token", token)
	} else {
		s.deps.Logger.Errorf("sign wait token for %s: %v", host, err)
	}
	return public + "/wait?" + q.Encode()
}

func publicPath(raw string) string {
	path := "/" + strings.Trim(raw, "/")
	if path == "/" {
		return ""
	}
	return path
}

func originalURL(r *http.Request, host hosts.Host) string {
	uri := r.Header.Get(host.Routing.ReturnToHeader)
	if uri == "" {
		uri = r.Header.Get("X-Forwarded-Uri")
	}
	return sanitizeReturnTo(uri)
}

func (s *Server) authorizedHost(w http.ResponseWriter, r *http.Request) (hosts.Host, waitTokenClaims, bool) {
	token, err := s.verifyWaitToken(requestToken(r))
	if err != nil {
		http.Error(w, "invalid wait token", http.StatusForbidden)
		return hosts.Host{}, waitTokenClaims{}, false
	}
	host, ok := s.hostFromRequest(r)
	if !ok {
		http.NotFound(w, r)
		return hosts.Host{}, waitTokenClaims{}, false
	}
	if !strings.EqualFold(host.Host, token.Host) {
		http.Error(w, "wait token host mismatch", http.StatusForbidden)
		return hosts.Host{}, waitTokenClaims{}, false
	}
	return host, token, true
}

func (s *Server) validateRequestOrigin(w http.ResponseWriter, r *http.Request, host hosts.Host) bool {
	expected := s.expectedOrigin(r, host)
	for _, header := range []string{"Origin", "Referer"} {
		value := r.Header.Get(header)
		if value == "" {
			continue
		}
		if originMatches(value, expected) {
			continue
		}
		s.deps.Logger.Warnf("rejected %s for %s: %s does not match %s", r.Method, host.Host, header, expected)
		http.Error(w, "invalid request origin", http.StatusForbidden)
		return false
	}
	return true
}

func (s *Server) expectedOrigin(r *http.Request, host hosts.Host) string {
	proto := firstHeaderValue(r.Header.Get("X-Forwarded-Proto"))
	if proto == "" {
		if r.TLS != nil {
			proto = "https"
		} else {
			proto = "http"
		}
	}
	if proto != "http" && proto != "https" {
		proto = "https"
	}
	return proto + "://" + host.Host
}

func originMatches(raw, expected string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return false
	}
	origin := parsed.Scheme + "://" + strings.ToLower(parsed.Host)
	return origin == strings.ToLower(expected)
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
