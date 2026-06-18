package dockhand

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"reveille/internal/hosts"
)

type Client struct {
	baseURL string
	token   string
	client  *http.Client
	mu      sync.Mutex
	envs    map[string]int
}

type Container struct {
	ID     string   `json:"id"`
	Names  []string `json:"names"`
	Name   string   `json:"name"`
	Status string   `json:"status"`
	State  string   `json:"state"`
	Health string   `json:"health"`
}

type Environment struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func NewClient(baseURL, token string, timeout time.Duration) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		client:  &http.Client{Timeout: timeout},
		envs:    map[string]int{},
	}
}

func (c *Client) Start(ctx context.Context, target hosts.Target) error {
	envID, err := c.envIDFor(ctx, target)
	if err != nil {
		return err
	}
	if target.Type == "stack" {
		return c.do(ctx, http.MethodPost, "/api/stacks/"+url.PathEscape(target.Name)+"/start", envID, nil, nil)
	}
	id, err := c.resolveContainer(ctx, target.ID, envID)
	if err != nil {
		return err
	}
	return c.do(ctx, http.MethodPost, "/api/containers/"+url.PathEscape(id)+"/start", envID, nil, nil)
}

func (c *Client) Stop(ctx context.Context, target hosts.Target) error {
	envID, err := c.envIDFor(ctx, target)
	if err != nil {
		return err
	}
	if target.Type == "stack" {
		return c.do(ctx, http.MethodPost, "/api/stacks/"+url.PathEscape(target.Name)+"/stop", envID, nil, nil)
	}
	id, err := c.resolveContainer(ctx, target.ID, envID)
	if err != nil {
		return err
	}
	return c.do(ctx, http.MethodPost, "/api/containers/"+url.PathEscape(id)+"/stop", envID, nil, nil)
}

func (c *Client) Healthy(ctx context.Context, target hosts.Target) (bool, error) {
	if target.Type != "container" {
		return false, fmt.Errorf("dockhand health is only supported for container targets")
	}
	envID, err := c.envIDFor(ctx, target)
	if err != nil {
		return false, err
	}
	container, ok, err := c.findContainer(ctx, target.ID, envID)
	if err != nil || !ok {
		return false, err
	}
	state := strings.ToLower(container.State)
	status := strings.ToLower(container.Status)
	health := strings.ToLower(container.Health)
	running := state == "running" || strings.HasPrefix(status, "up")
	if !running {
		return false, nil
	}
	if health == "" || health == "none" {
		return true, nil
	}
	return health == "healthy", nil
}

func (c *Client) Containers(ctx context.Context, envID int) ([]Container, error) {
	var out []Container
	if err := c.do(ctx, http.MethodGet, "/api/containers", envID, url.Values{"all": {"true"}}, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) resolveContainer(ctx context.Context, configured string, envID int) (string, error) {
	container, ok, err := c.findContainer(ctx, configured, envID)
	if err != nil || !ok {
		return configured, err
	}
	return container.ID, nil
}

func (c *Client) findContainer(ctx context.Context, configured string, envID int) (Container, bool, error) {
	if configured == "" {
		return Container{}, false, fmt.Errorf("container id is empty")
	}
	containers, err := c.Containers(ctx, envID)
	if err != nil {
		return Container{}, false, err
	}
	want := strings.TrimPrefix(configured, "/")
	for _, container := range containers {
		if container.ID == configured || strings.HasPrefix(container.ID, configured) || container.Name == want || strings.TrimPrefix(container.Name, "/") == want {
			return container, true, nil
		}
		for _, name := range container.Names {
			if strings.TrimPrefix(name, "/") == want {
				return container, true, nil
			}
		}
	}
	return Container{}, false, nil
}

func (c *Client) envIDFor(ctx context.Context, target hosts.Target) (int, error) {
	if target.Environment == "" {
		return 0, fmt.Errorf("target environment is required")
	}
	if id, err := strconv.Atoi(target.Environment); err == nil {
		return id, nil
	}
	key := strings.ToLower(target.Environment)
	c.mu.Lock()
	if id, ok := c.envs[key]; ok {
		c.mu.Unlock()
		return id, nil
	}
	c.mu.Unlock()

	var envs []Environment
	if err := c.do(ctx, http.MethodGet, "/api/environments", 0, nil, &envs); err != nil {
		return 0, err
	}
	for _, env := range envs {
		if strings.EqualFold(env.Name, target.Environment) {
			c.mu.Lock()
			c.envs[key] = env.ID
			c.mu.Unlock()
			return env.ID, nil
		}
	}
	return 0, fmt.Errorf("dockhand environment %q not found", target.Environment)
}

func (c *Client) do(ctx context.Context, method, path string, envID int, query url.Values, out any) error {
	u, err := url.Parse(c.baseURL + path)
	if err != nil {
		return err
	}
	q := u.Query()
	for k, values := range query {
		for _, value := range values {
			q.Add(k, value)
		}
	}
	if envID > 0 {
		q.Set("env", fmt.Sprintf("%d", envID))
	}
	u.RawQuery = q.Encode()

	var body *bytes.Reader
	body = bytes.NewReader(nil)
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	res, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("%s %s: dockhand returned %s", method, path, res.Status)
	}
	if out != nil {
		return json.NewDecoder(res.Body).Decode(out)
	}
	return nil
}
