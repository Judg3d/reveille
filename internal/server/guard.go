package server

import (
	"context"
	"sync"
	"time"

	"reveille/internal/hosts"
)

// flightGroup deduplicates concurrent calls that share a key, so a burst of
// requests produces a single upstream health check or start call.
type flightGroup struct {
	mu    sync.Mutex
	calls map[string]*flightCall
}

type flightCall struct {
	done chan struct{}
	val  any
	err  error
}

func newFlightGroup() *flightGroup {
	return &flightGroup{calls: map[string]*flightCall{}}
}

func (g *flightGroup) Do(ctx context.Context, key string, fn func() (any, error)) (any, error) {
	g.mu.Lock()
	if c, ok := g.calls[key]; ok {
		g.mu.Unlock()
		select {
		case <-c.done:
			return c.val, c.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	c := &flightCall{done: make(chan struct{})}
	g.calls[key] = c
	g.mu.Unlock()

	c.val, c.err = fn()

	g.mu.Lock()
	delete(g.calls, key)
	g.mu.Unlock()
	close(c.done)
	return c.val, c.err
}

type healthEntry struct {
	healthy bool
	err     error
	at      time.Time
}

type healthCache struct {
	mu      sync.Mutex
	entries map[string]healthEntry
}

func newHealthCache() *healthCache {
	return &healthCache{entries: map[string]healthEntry{}}
}

func (c *healthCache) get(key string, now time.Time, ttl time.Duration) (healthEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok || now.Sub(entry.at) > ttl {
		return healthEntry{}, false
	}
	return entry, true
}

func (c *healthCache) put(key string, healthy bool, err error, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = healthEntry{healthy: healthy, err: err, at: now}
}

func (c *healthCache) invalidate(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
}

// cachedHealthy answers the health question from a short-TTL cache, sharing a
// single upstream check between concurrent requests for the same host.
func (s *Server) cachedHealthy(ctx context.Context, host hosts.Host) (bool, error) {
	ttl := s.deps.Config.Defaults.HealthCacheTTL
	if ttl <= 0 {
		return s.healthy(ctx, host)
	}
	if entry, ok := s.healthCache.get(host.Host, s.now(), ttl); ok {
		return entry.healthy, entry.err
	}
	val, err := s.flights.Do(ctx, "health:"+host.Host, func() (any, error) {
		healthy, err := s.healthy(ctx, host)
		s.healthCache.put(host.Host, healthy, err, s.now())
		return healthy, err
	})
	healthy, _ := val.(bool)
	return healthy, err
}

// startTarget starts the host's target, deduplicating concurrent start calls
// and dropping the stale cached health state on success.
func (s *Server) startTarget(ctx context.Context, host hosts.Host) error {
	_, err := s.flights.Do(ctx, "start:"+host.Host, func() (any, error) {
		err := s.deps.Provider.Start(ctx, host.Target)
		if err == nil {
			s.healthCache.invalidate(host.Host)
		}
		return nil, err
	})
	return err
}
