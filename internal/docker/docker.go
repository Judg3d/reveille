package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"reveille/internal/hosts"
)

// Client talks to the Docker Engine API over a unix socket. It only supports
// container targets; stack targets need the Dockhand provider.
type Client struct {
	baseURL string
	client  *http.Client
}

type containerState struct {
	State struct {
		Running bool `json:"Running"`
		Health  *struct {
			Status string `json:"Status"`
		} `json:"Health"`
	} `json:"State"`
}

func NewClient(socket string, timeout time.Duration) *Client {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socket)
		},
	}
	return &Client{
		baseURL: "http://docker",
		client:  &http.Client{Transport: transport, Timeout: timeout},
	}
}

// NewClientWithBaseURL builds a client against an HTTP base URL. Used in
// tests where the Engine API is served over TCP.
func NewClientWithBaseURL(baseURL string, timeout time.Duration) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: timeout},
	}
}

func (c *Client) Name() string { return "docker" }

func (c *Client) Start(ctx context.Context, target hosts.Target) error {
	id, err := containerID(target)
	if err != nil {
		return err
	}
	return c.post(ctx, "/containers/"+url.PathEscape(id)+"/start")
}

func (c *Client) Stop(ctx context.Context, target hosts.Target) error {
	id, err := containerID(target)
	if err != nil {
		return err
	}
	return c.post(ctx, "/containers/"+url.PathEscape(id)+"/stop")
}

func (c *Client) Healthy(ctx context.Context, target hosts.Target) (bool, error) {
	id, err := containerID(target)
	if err != nil {
		return false, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/containers/"+url.PathEscape(id)+"/json", nil)
	if err != nil {
		return false, err
	}
	res, err := c.client.Do(req)
	if err != nil {
		return false, err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if res.StatusCode != http.StatusOK {
		return false, fmt.Errorf("docker inspect %s: %s", id, res.Status)
	}
	var state containerState
	if err := json.NewDecoder(res.Body).Decode(&state); err != nil {
		return false, err
	}
	if !state.State.Running {
		return false, nil
	}
	if state.State.Health == nil {
		return true, nil
	}
	status := strings.ToLower(state.State.Health.Status)
	return status == "" || status == "healthy", nil
}

func (c *Client) post(ctx context.Context, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	res, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
	// 304 means the container is already in the requested state.
	if res.StatusCode == http.StatusNotModified {
		return nil
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("POST %s: docker returned %s", path, res.Status)
	}
	return nil
}

func containerID(target hosts.Target) (string, error) {
	if target.Type != "container" {
		return "", fmt.Errorf("the docker provider only supports container targets; use dockhand for stacks")
	}
	id := strings.TrimPrefix(strings.TrimSpace(target.ID), "/")
	if id == "" {
		return "", fmt.Errorf("container id is empty")
	}
	return id, nil
}
