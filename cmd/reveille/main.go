package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"reveille/internal/config"
	"reveille/internal/docker"
	"reveille/internal/dockhand"
	"reveille/internal/health"
	"reveille/internal/hosts"
	"reveille/internal/leases"
	"reveille/internal/logging"
	"reveille/internal/notify"
	"reveille/internal/provider"
	"reveille/internal/server"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	args := os.Args[1:]
	validateOnly := false
	if len(args) > 0 && args[0] == "validate" {
		validateOnly = true
		args = args[1:]
	}

	flags := flag.NewFlagSet("reveille", flag.ExitOnError)
	configPath := flags.String("config", envDefault("REVEILLE_CONFIG", "reveille.yml"), "path to reveille.yml")
	hostsDir := flags.String("hosts", envDefault("REVEILLE_HOSTS_DIR", "hosts"), "path to dynamic host config directory")
	showVersion := flags.Bool("version", false, "print version and exit")
	_ = flags.Parse(args)

	if *showVersion {
		fmt.Println("reveille " + version)
		return
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	logger, err := logging.NewWithFormat(cfg.Log.Level, cfg.Log.Format)
	if err != nil {
		log.Fatalf("init logger: %v", err)
	}
	store, err := hosts.LoadDir(*hostsDir, cfg.Defaults, logger)
	if err != nil {
		logger.Errorf("load hosts: %v", err)
		os.Exit(1)
	}

	if validateOnly {
		count := len(store.All())
		fmt.Printf("config %s: ok\nhosts %s: %d target(s)\n", *configPath, *hostsDir, count)
		return
	}

	prov := buildProvider(cfg)
	healthClient := &http.Client{
		// Redirects are not followed: a redirecting health endpoint counts
		// as its raw status code against healthyStatus.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	checker := health.NewChecker(healthClient)

	dispatcher := notify.NewDispatcher(cfg.Notify, logger)
	leaseMgr := leases.NewManager(func(ctx context.Context, host hosts.Host) error {
		return prov.Stop(ctx, host.Target)
	}, logger)
	leaseMgr.SetOnEvent(func(event string, host hosts.Host, err error) {
		e := notify.Event{Type: event, Host: host.Host, Target: host.Target.Name}
		switch event {
		case notify.EventLeaseExpiredStop:
			e.Message = "Lease expired; target was stopped."
		case notify.EventStopFailed:
			e.Message = "Lease expired but the stop failed; the target may still be running."
		}
		if err != nil {
			e.Error = err.Error()
		}
		dispatcher.Send(e)
	})
	if cfg.Server.StateFile != "" {
		if err := leases.EnsureStateDir(cfg.Server.StateFile); err != nil {
			logger.Warnf("lease persistence disabled: state file %s not writable: %v", cfg.Server.StateFile, err)
		} else {
			leaseMgr.SetStatePath(cfg.Server.StateFile)
			if err := leaseMgr.Load(time.Now()); err != nil {
				logger.Errorf("restore leases: %v", err)
			}
		}
	}

	store.SetOnChange(func() {
		leaseMgr.UpdateHosts(func(name string) (hosts.Host, bool) {
			return store.Lookup(name)
		})
	})
	ctx, stopWatch := context.WithCancel(context.Background())
	defer stopWatch()
	go store.Watch(ctx, cfg.Server.HostsReloadInterval, func(err error) {
		logger.Errorf("reload hosts: %v", err)
	})

	app := server.New(server.Dependencies{
		Config:     cfg,
		Hosts:      store,
		Provider:   prov,
		Health:     checker,
		Leases:     leaseMgr,
		Notify:     dispatcher,
		Logger:     logger,
		StartClock: time.Now,
	})

	srv := &http.Server{
		Addr:              cfg.Server.Listen,
		Handler:           app.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	go func() {
		logger.Infof("reveille %s (%s provider) listening on %s", version, prov.Name(), cfg.Server.Listen)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Errorf("server: %v", err)
			os.Exit(1)
		}
	}()

	var adminSrv *http.Server
	if cfg.Admin.Listen != "" {
		adminSrv = &http.Server{
			Addr:              cfg.Admin.Listen,
			Handler:           app.AdminRoutes(),
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       2 * time.Minute,
		}
		go func() {
			logger.Infof("reveille admin dashboard listening on %s", cfg.Admin.Listen)
			if err := adminSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logger.Errorf("admin server: %v", err)
			}
		}()
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	leaseMgr.Close()
	if adminSrv != nil {
		_ = adminSrv.Shutdown(shutdownCtx)
	}
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Warnf("shutdown: %v", err)
	}
}

func buildProvider(cfg config.Config) provider.Provider {
	if cfg.Provider == "docker" {
		return docker.NewClient(cfg.Docker.Socket, cfg.Docker.Timeout)
	}
	return dockhand.NewClient(cfg.Dockhand.BaseURL, cfg.Dockhand.APIToken, cfg.Dockhand.Timeout)
}

func envDefault(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}
