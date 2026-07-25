package server

import (
	"html/template"
	"time"

	"reveille/internal/config"
	"reveille/internal/health"
	"reveille/internal/hosts"
	"reveille/internal/leases"
	"reveille/internal/logging"
	"reveille/internal/notify"
	"reveille/internal/provider"
	"reveille/internal/ratelimit"
)

type Dependencies struct {
	Config     config.Config
	Hosts      *hosts.Store
	Provider   provider.Provider
	Health     *health.Checker
	Leases     *leases.Manager
	Notify     *notify.Dispatcher
	Logger     *logging.Logger
	StartClock func() time.Time
	TokenKey   []byte
}

type Server struct {
	deps        Dependencies
	tpl         *template.Template
	tokenKey    []byte
	flights     *flightGroup
	healthCache *healthCache
	limiter     *ratelimit.Limiter
}

func New(deps Dependencies) *Server {
	if deps.Logger == nil {
		deps.Logger = logging.Must("info")
	}
	if deps.StartClock == nil {
		deps.StartClock = time.Now
	}
	if len(deps.TokenKey) == 0 && deps.Config.Server.TokenSecret != "" {
		deps.TokenKey = []byte(deps.Config.Server.TokenSecret)
	}
	return &Server{
		deps:        deps,
		tpl:         parseTemplates(),
		tokenKey:    tokenKey(deps.TokenKey),
		flights:     newFlightGroup(),
		healthCache: newHealthCache(),
		// Mutating wait-control requests: 1/s sustained, bursts of 10 per host.
		limiter: ratelimit.New(1, 10),
	}
}
