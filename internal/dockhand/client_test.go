package dockhand

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"reveille/internal/hosts"
)

func TestContainerStartResolvesNameAndSetsHeaders(t *testing.T) {
	var sawList, sawStart bool
	client := NewClient("http://dockhand.test", "dh_test", time.Second)
	client.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "Bearer dh_test" || r.Header.Get("Accept") != "application/json" {
			t.Fatalf("headers = %+v", r.Header)
		}
		if r.URL.Query().Get("env") != "3" {
			t.Fatalf("env query = %q", r.URL.RawQuery)
		}
		switch r.URL.Path {
		case "/api/containers":
			sawList = true
			if r.URL.Query().Get("all") != "true" {
				t.Fatalf("all query missing: %q", r.URL.RawQuery)
			}
			body, _ := json.Marshal([]Container{{ID: "abc123", Names: []string{"/jellyfin"}}})
			return response(http.StatusOK, body), nil
		case "/api/containers/abc123/start":
			sawStart = true
			return response(http.StatusNoContent, nil), nil
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		return response(http.StatusInternalServerError, nil), nil
	})

	if err := client.Start(context.Background(), hosts.Target{Type: "container", ID: "jellyfin", Environment: "3"}); err != nil {
		t.Fatal(err)
	}
	if !sawList || !sawStart {
		t.Fatalf("sawList=%v sawStart=%v", sawList, sawStart)
	}
}

func TestStackStopPath(t *testing.T) {
	var path string
	client := NewClient("http://dockhand.test", "", time.Second)
	client.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		path = r.URL.Path + "?" + r.URL.RawQuery
		return response(http.StatusNoContent, nil), nil
	})

	if err := client.Stop(context.Background(), hosts.Target{Type: "stack", Name: "paperless", Environment: "9"}); err != nil {
		t.Fatal(err)
	}
	if path != "/api/stacks/paperless/stop?env=9" {
		t.Fatalf("path = %s", path)
	}
}

func TestHealthyUsesDockhandContainerHealth(t *testing.T) {
	client := NewClient("http://dockhand.test", "", time.Second)
	client.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/containers" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		body, _ := json.Marshal([]Container{{
			ID:     "abc123",
			Names:  []string{"/jellyfin"},
			State:  "running",
			Status: "Up 4 seconds",
			Health: "healthy",
		}})
		return response(http.StatusOK, body), nil
	})

	healthy, err := client.Healthy(context.Background(), hosts.Target{Type: "container", ID: "jellyfin", Environment: "1"})
	if err != nil {
		t.Fatal(err)
	}
	if !healthy {
		t.Fatal("container should be healthy")
	}
}

func TestEnvironmentNameResolvesToID(t *testing.T) {
	var sawEnvLookup, sawContainerLookup bool
	client := NewClient("http://dockhand.test", "", time.Second)
	client.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/environments":
			sawEnvLookup = true
			body, _ := json.Marshal([]Environment{{ID: 7, Name: "prod"}})
			return response(http.StatusOK, body), nil
		case "/api/containers":
			sawContainerLookup = true
			if r.URL.Query().Get("env") != "7" {
				t.Fatalf("env query = %q", r.URL.RawQuery)
			}
			body, _ := json.Marshal([]Container{{ID: "abc123", Name: "app", State: "running"}})
			return response(http.StatusOK, body), nil
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		return response(http.StatusInternalServerError, nil), nil
	})

	healthy, err := client.Healthy(context.Background(), hosts.Target{Type: "container", ID: "app", Environment: "prod"})
	if err != nil {
		t.Fatal(err)
	}
	if !healthy || !sawEnvLookup || !sawContainerLookup {
		t.Fatalf("healthy=%v sawEnvLookup=%v sawContainerLookup=%v", healthy, sawEnvLookup, sawContainerLookup)
	}
}

func TestMissingEnvironmentFails(t *testing.T) {
	client := NewClient("http://dockhand.test", "", time.Second)

	err := client.Start(context.Background(), hosts.Target{Type: "container", ID: "jellyfin"})
	if err == nil || err.Error() != "target environment is required" {
		t.Fatalf("Start() err = %v, want missing environment error", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func response(status int, body []byte) *http.Response {
	if body == nil {
		body = []byte{}
	}
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Body:       io.NopCloser(bytes.NewReader(body)),
		Header:     make(http.Header),
	}
}
