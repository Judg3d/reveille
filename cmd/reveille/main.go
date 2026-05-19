package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"reveille/internal/config"
	"reveille/internal/dockhand"
	"reveille/internal/health"
	"reveille/internal/hosts"
	"reveille/internal/leases"
	"reveille/internal/server"
)

func main() {
	configPath := flag.String("config", envDefault("REVEILLE_CONFIG", "config.yml"), "path to config.yml")
	hostsDir := flag.String("hosts", envDefault("REVEILLE_HOSTS_DIR", "hosts"), "path to dynamic host config directory")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	store, err := hosts.LoadDir(*hostsDir, cfg.Defaults)
	if err != nil {
		log.Fatalf("load hosts: %v", err)
	}

	dh := dockhand.NewClient(cfg.Dockhand.BaseURL, cfg.Dockhand.APIToken, cfg.Dockhand.EnvironmentID, cfg.Dockhand.Timeout)
	checker := health.NewChecker(http.DefaultClient)
	leases := leases.NewManager(func(ctx context.Context, host hosts.Host) error {
		return dh.Stop(ctx, host.Target)
	})
	ctx, stopWatch := context.WithCancel(context.Background())
	defer stopWatch()
	go store.Watch(ctx, 5*time.Second, func(err error) {
		log.Printf("reload hosts: %v", err)
	})

	app := server.New(server.Dependencies{
		Config:     cfg,
		Hosts:      store,
		Dockhand:   dh,
		Health:     checker,
		Leases:     leases,
		StartClock: time.Now,
	})

	srv := &http.Server{Addr: cfg.Server.Listen, Handler: app.Routes()}
	go func() {
		log.Printf("reveille listening on %s", cfg.Server.Listen)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	leases.Close()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}

func envDefault(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}
