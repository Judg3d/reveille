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
	q := url.Values{"host": {host}, "returnTo": {sanitizeReturnTo(returnTo)}}
	path := public + "/wait?" + q.Encode()

	base := publicBaseURL(r)
	if base == "" {
		return path
	}
	return base + path
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
