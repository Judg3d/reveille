package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"reveille/internal/config"
	"reveille/internal/logging"
)

const (
	EventWake             = "wake"
	EventLeaseExpiredStop = "lease_expired_stop"
	EventManualStop       = "manual_stop"
	EventStopFailed       = "stop_failed"
)

type Event struct {
	Type    string    `json:"type"`
	Host    string    `json:"host"`
	Target  string    `json:"target"`
	Message string    `json:"message"`
	Error   string    `json:"error,omitempty"`
	Time    time.Time `json:"time"`
}

// Title renders a short human-readable subject line for the event.
func (e Event) Title() string {
	switch e.Type {
	case EventWake:
		return fmt.Sprintf("Reveille: %s woke up", e.Host)
	case EventLeaseExpiredStop:
		return fmt.Sprintf("Reveille: %s stopped (timer expired)", e.Host)
	case EventManualStop:
		return fmt.Sprintf("Reveille: %s stopped manually", e.Host)
	case EventStopFailed:
		return fmt.Sprintf("Reveille: failed to stop %s", e.Host)
	default:
		return fmt.Sprintf("Reveille: %s %s", e.Host, e.Type)
	}
}

type Notifier interface {
	Name() string
	Notify(ctx context.Context, event Event) error
}

// Dispatcher fans events out to all configured notifiers. Delivery is
// best-effort and asynchronous: a slow or down notification service must
// never block or delay a start/stop.
type Dispatcher struct {
	notifiers []Notifier
	events    map[string]bool
	logger    *logging.Logger
	timeout   time.Duration
}

// NewDispatcher builds a dispatcher from config. It returns nil when no
// channel is configured; a nil dispatcher is safe to call.
func NewDispatcher(cfg config.NotifyConfig, logger *logging.Logger) *Dispatcher {
	client := &http.Client{Timeout: 10 * time.Second}
	var notifiers []Notifier
	if cfg.Gotify.URL != "" {
		notifiers = append(notifiers, &Gotify{cfg: cfg.Gotify, client: client})
	}
	if cfg.Telegram.Token != "" {
		notifiers = append(notifiers, &Telegram{cfg: cfg.Telegram, client: client})
	}
	if cfg.Ntfy.URL != "" {
		notifiers = append(notifiers, &Ntfy{cfg: cfg.Ntfy, client: client})
	}
	if cfg.Webhook.URL != "" {
		notifiers = append(notifiers, &Webhook{cfg: cfg.Webhook, client: client})
	}
	if len(notifiers) == 0 {
		return nil
	}
	var events map[string]bool
	if len(cfg.Events) > 0 {
		events = map[string]bool{}
		for _, event := range cfg.Events {
			events[event] = true
		}
	}
	return &Dispatcher{
		notifiers: notifiers,
		events:    events,
		logger:    logger,
		timeout:   10 * time.Second,
	}
}

// Send delivers the event to every configured channel in the background.
func (d *Dispatcher) Send(event Event) {
	if d == nil {
		return
	}
	if d.events != nil && !d.events[event.Type] {
		return
	}
	if event.Time.IsZero() {
		event.Time = time.Now().UTC()
	}
	for _, notifier := range d.notifiers {
		notifier := notifier
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), d.timeout)
			defer cancel()
			if err := notifier.Notify(ctx, event); err != nil {
				d.logger.Warnf("notify %s: %v", notifier.Name(), err)
			}
		}()
	}
}

type Gotify struct {
	cfg    config.GotifyConfig
	client *http.Client
}

func (g *Gotify) Name() string { return "gotify" }

func (g *Gotify) Notify(ctx context.Context, event Event) error {
	body, err := json.Marshal(map[string]any{
		"title":    event.Title(),
		"message":  event.Message,
		"priority": g.cfg.Priority,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.cfg.URL+"/message", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Gotify-Key", g.cfg.Token)
	return doRequest(g.client, req)
}

type Telegram struct {
	cfg    config.TelegramConfig
	client *http.Client
	// baseURL overrides the Telegram API endpoint in tests.
	baseURL string
}

func (t *Telegram) Name() string { return "telegram" }

func (t *Telegram) Notify(ctx context.Context, event Event) error {
	base := t.baseURL
	if base == "" {
		base = "https://api.telegram.org"
	}
	form := url.Values{
		"chat_id": {t.cfg.ChatID},
		"text":    {event.Title() + "\n" + event.Message},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		base+"/bot"+url.PathEscape(t.cfg.Token)+"/sendMessage",
		strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return doRequest(t.client, req)
}

type Ntfy struct {
	cfg    config.NtfyConfig
	client *http.Client
}

func (n *Ntfy) Name() string { return "ntfy" }

func (n *Ntfy) Notify(ctx context.Context, event Event) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		n.cfg.URL+"/"+url.PathEscape(n.cfg.Topic), strings.NewReader(event.Message))
	if err != nil {
		return err
	}
	req.Header.Set("Title", event.Title())
	if n.cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+n.cfg.Token)
	}
	return doRequest(n.client, req)
}

type Webhook struct {
	cfg    config.WebhookConfig
	client *http.Client
}

func (w *Webhook) Name() string { return "webhook" }

func (w *Webhook) Notify(ctx context.Context, event Event) error {
	body, err := json.Marshal(event)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.cfg.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return doRequest(w.client, req)
}

func doRequest(client *http.Client, req *http.Request) error {
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("%s returned %s", req.URL.Host, res.Status)
	}
	return nil
}
