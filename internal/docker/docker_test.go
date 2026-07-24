package docker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"reveille/internal/hosts"
)

func TestStartStopAndInspectPaths(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		switch r.URL.Path {
		case "/containers/jellyfin/start":
			w.WriteHeader(http.StatusNoContent)
		case "/containers/jellyfin/stop":
			// Already stopped.
			w.WriteHeader(http.StatusNotModified)
		case "/containers/jellyfin/json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"State":{"Running":true,"Health":{"Status":"healthy"}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := NewClientWithBaseURL(srv.URL, time.Second)
	target := hosts.Target{Type: "container", ID: "/jellyfin"}

	if err := client.Start(context.Background(), target); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := client.Stop(context.Background(), target); err != nil {
		t.Fatalf("stop (304): %v", err)
	}
	healthy, err := client.Healthy(context.Background(), target)
	if err != nil {
		t.Fatalf("healthy: %v", err)
	}
	if !healthy {
		t.Fatal("healthy = false, want true")
	}
	want := []string{
		"POST /containers/jellyfin/start",
		"POST /containers/jellyfin/stop",
		"GET /containers/jellyfin/json",
	}
	if len(paths) != len(want) {
		t.Fatalf("paths = %v", paths)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Fatalf("paths[%d] = %q, want %q", i, paths[i], want[i])
		}
	}
}

func TestHealthyStatesAndMissingContainer(t *testing.T) {
	body := `{"State":{"Running":false}}`
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	client := NewClientWithBaseURL(srv.URL, time.Second)
	target := hosts.Target{Type: "container", ID: "app"}

	if healthy, err := client.Healthy(context.Background(), target); err != nil || healthy {
		t.Fatalf("stopped container: healthy=%t err=%v", healthy, err)
	}

	body = `{"State":{"Running":true,"Health":{"Status":"starting"}}}`
	if healthy, _ := client.Healthy(context.Background(), target); healthy {
		t.Fatal("starting health status must not count as healthy")
	}

	body = `{"State":{"Running":true}}`
	if healthy, _ := client.Healthy(context.Background(), target); !healthy {
		t.Fatal("running container without healthcheck must count as healthy")
	}

	status = http.StatusNotFound
	if healthy, err := client.Healthy(context.Background(), target); err != nil || healthy {
		t.Fatalf("missing container: healthy=%t err=%v, want false/nil", healthy, err)
	}
}

func TestStackTargetsRejected(t *testing.T) {
	client := NewClientWithBaseURL("http://docker.test", time.Second)
	err := client.Start(context.Background(), hosts.Target{Type: "stack", Name: "paperless"})
	if err == nil {
		t.Fatal("stack target must be rejected by the docker provider")
	}
}
