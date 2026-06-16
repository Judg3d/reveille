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
	return mux
}
