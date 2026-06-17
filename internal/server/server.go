package server

import (
	"html/template"
	"time"

	"reveille/internal/config"
	"reveille/internal/dockhand"
	"reveille/internal/health"
	"reveille/internal/hosts"
	"reveille/internal/leases"
	"reveille/internal/logging"
)

type Dependencies struct {
	Config     config.Config
	Hosts      *hosts.Store
	Dockhand   *dockhand.Client
	Health     *health.Checker
	Leases     *leases.Manager
	Logger     *logging.Logger
	StartClock func() time.Time
	TokenKey   []byte
}

type Server struct {
	deps     Dependencies
	tpl      *template.Template
	tokenKey []byte
}

func New(deps Dependencies) *Server {
	if deps.Logger == nil {
		deps.Logger = logging.Must("info")
	}
	if deps.StartClock == nil {
		deps.StartClock = time.Now
	}
	return &Server{
		deps:     deps,
		tpl:      parseTemplates(),
		tokenKey: tokenKey(deps.TokenKey),
	}
}
