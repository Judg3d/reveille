package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"reveille/internal/config"
	"reveille/internal/logging"
)

func TestGotifyNotify(t *testing.T) {
	var gotPath, gotKey string
	var payload map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotKey = r.Header.Get("X-Gotify-Key")
		_ = json.NewDecoder(r.Body).Decode(&payload)
	}))
	defer srv.Close()

	g := &Gotify{
		cfg:    config.GotifyConfig{URL: srv.URL, Token: "app-token", Priority: 5},
		client: srv.Client(),
	}
	err := g.Notify(context.Background(), Event{Type: EventWake, Host: "app.example.com", Message: "started"})
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/message" || gotKey != "app-token" {
		t.Fatalf("path=%q key=%q", gotPath, gotKey)
	}
	if payload["message"] != "started" || payload["priority"] != float64(5) {
		t.Fatalf("payload = %+v", payload)
	}
}

func TestTelegramNotify(t *testing.T) {
	var gotPath string
	var form url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		form, _ = url.ParseQuery(string(body))
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	tg := &Telegram{
		cfg:     config.TelegramConfig{Token: "12345:abc", ChatID: "-100200"},
		client:  srv.Client(),
		baseURL: srv.URL,
	}
	err := tg.Notify(context.Background(), Event{Type: EventStopFailed, Host: "app.example.com", Message: "stop failed"})
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/bot12345:abc/sendMessage" {
		t.Fatalf("path = %q", gotPath)
	}
	if form.Get("chat_id") != "-100200" {
		t.Fatalf("chat_id = %q", form.Get("chat_id"))
	}
	if form.Get("text") == "" {
		t.Fatal("text is empty")
	}
}

func TestNtfyNotify(t *testing.T) {
	var gotPath, gotTitle, gotAuth, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotTitle = r.Header.Get("Title")
		gotAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
	}))
	defer srv.Close()

	n := &Ntfy{
		cfg:    config.NtfyConfig{URL: srv.URL, Topic: "reveille", Token: "tk"},
		client: srv.Client(),
	}
	err := n.Notify(context.Background(), Event{Type: EventManualStop, Host: "app.example.com", Message: "stopped"})
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/reveille" || gotBody != "stopped" || gotAuth != "Bearer tk" || gotTitle == "" {
		t.Fatalf("path=%q body=%q auth=%q title=%q", gotPath, gotBody, gotAuth, gotTitle)
	}
}

func TestDispatcherFiltersEvents(t *testing.T) {
	received := make(chan Event, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var event Event
		_ = json.NewDecoder(r.Body).Decode(&event)
		received <- event
	}))
	defer srv.Close()

	d := NewDispatcher(config.NotifyConfig{
		Events:  []string{EventStopFailed},
		Webhook: config.WebhookConfig{URL: srv.URL},
	}, logging.Must("info"))
	if d == nil {
		t.Fatal("dispatcher is nil despite webhook config")
	}

	d.Send(Event{Type: EventWake, Host: "app.example.com"})
	d.Send(Event{Type: EventStopFailed, Host: "app.example.com", Message: "boom"})

	select {
	case event := <-received:
		if event.Type != EventStopFailed {
			t.Fatalf("received filtered event %q", event.Type)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("webhook never received the stop_failed event")
	}
	select {
	case event := <-received:
		t.Fatalf("unexpected extra event %q", event.Type)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestNilDispatcherIsSafe(t *testing.T) {
	var d *Dispatcher
	d.Send(Event{Type: EventWake, Host: "app.example.com"})
}
