package server

import (
	"embed"
	"encoding/json"
	"html/template"
	"io/fs"
	"net/http"
	"time"

	"reveille/internal/config"
)

//go:embed templates/*.html static/*.css static/*.js
var assets embed.FS

type waitView struct {
	Host         string
	PublicPath   string
	ReturnTo     string
	Token        string
	ConfigJSON   template.JS
	LeaseDefault string
	LeaseOptions []config.LeaseDuration
}

func parseTemplates() *template.Template {
	return template.Must(template.ParseFS(assets, "templates/*.html"))
}

func staticHandler(public string) http.Handler {
	return http.StripPrefix(public+"/static/", http.FileServer(http.FS(mustSub(assets, "static"))))
}

func (s *Server) wait(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet && r.URL.Query().Get("format") == "status" {
		s.status(w, r)
		return
	}
	if r.Method == http.MethodPost {
		if r.FormValue("action") == "stop" {
			s.stop(w, r)
			return
		}
		s.lease(w, r)
		return
	}

	host, token, ok := s.authorizedHost(w, r)
	if !ok {
		return
	}
	returnTo := token.ReturnTo
	clientCfg := map[string]any{
		"host":       host.Host,
		"returnTo":   returnTo,
		"token":      requestToken(r),
		"publicPath": publicPath(s.deps.Config.Server.PublicPath),
		"waitPath":   publicPath(s.deps.Config.Server.PublicPath) + "/wait",
		"pollMillis": int(s.deps.Config.Defaults.PollInterval / time.Millisecond),
	}
	cfgJSON, err := json.Marshal(clientCfg)
	if err != nil {
		http.Error(w, "failed to render wait config", http.StatusInternalServerError)
		return
	}
	data := waitView{
		Host:         host.Host,
		PublicPath:   publicPath(s.deps.Config.Server.PublicPath),
		ReturnTo:     returnTo,
		Token:        requestToken(r),
		ConfigJSON:   template.JS(cfgJSON),
		LeaseDefault: host.Lease.Default.Label,
		LeaseOptions: host.Lease.Options,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tpl.ExecuteTemplate(w, "wait.html", data); err != nil {
		s.deps.Logger.Errorf("render wait: %v", err)
	}
}

func mustSub(fsys embed.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic(err)
	}
	return sub
}
