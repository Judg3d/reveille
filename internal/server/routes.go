package server

import "net/http"

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.healthz)
	mux.HandleFunc("/api/traefik/forward-auth", s.forwardAuth)

	public := publicPath(s.deps.Config.Server.PublicPath)
	mux.HandleFunc(public+"/wait", s.wait)
	mux.HandleFunc(public+"/api/status", s.status)
	mux.HandleFunc(public+"/api/lease", s.lease)
	mux.HandleFunc(public+"/api/stop", s.stop)
	mux.Handle(public+"/static/", staticHandler(public))
	return securityHeaders(mux)
}

// securityHeaders hardens every response. Assets are fully self-hosted, so
// the CSP can stay strict; no-referrer keeps wait tokens out of Referer
// headers while they can still appear in URLs.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self'; connect-src 'self'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}
