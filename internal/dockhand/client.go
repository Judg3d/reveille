package dockhand

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	baseURL    string
	token      string
	client     *http.Client
	mu         sync.Mutex
	envs       map[string]int
	containers map[string]string
}

type apiError struct {
	Method     string
	Path       string
	Status     string
	StatusCode int
}

func (e *apiError) Error() string {
	return fmt.Sprintf("%s %s: dockhand returned %s", e.Method, e.Path, e.Status)
}

func isNotFound(err error) bool {
	var apiErr *apiError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound
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
		baseURL:    strings.TrimRight(baseURL, "/"),
		token:      token,
		client:     &http.Client{Timeout: timeout},
		envs:       map[string]int{},
		containers: map[string]string{},
	}
}

func (c *Client) Name() string { return "dockhand" }

func (c *Client) Start(ctx context.Context, target hosts.Target) error {
	return c.containerAction(ctx, target, "start")
}

func (c *Client) Stop(ctx context.Context, target hosts.Target) error {
	return c.containerAction(ctx, target, "stop")
}

func (c *Client) containerAction(ctx context.Context, target hosts.Target, action string) error {
	envID, err := c.envIDFor(ctx, target)
	if err != nil {
		return err
	}
	if target.Type == "stack" {
		return c.do(ctx, http.MethodPost, "/api/stacks/"+url.PathEscape(target.Name)+"/"+action, envID, nil, nil)
	}
	id, err := c.resolveContainer(ctx, target.ID, envID)
	if err != nil {
		return err
	}
	err = c.do(ctx, http.MethodPost, "/api/containers/"+url.PathEscape(id)+"/"+action, envID, nil, nil)
	if !isNotFound(err) {
		return err
	}
	// A 404 usually means the cached container ID went stale after a
	// recreate; drop the caches and retry once with fresh lookups.
	c.invalidate(target, envID)
	envID, retryErr := c.envIDFor(ctx, target)
	if retryErr != nil {
		return retryErr
	}
	id, retryErr = c.resolveContainer(ctx, target.ID, envID)
	if retryErr != nil {
		return retryErr
	}
	return c.do(ctx, http.MethodPost, "/api/containers/"+url.PathEscape(id)+"/"+action, envID, nil, nil)
}

func (c *Client) invalidate(target hosts.Target, envID int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.envs, strings.ToLower(target.Environment))
	delete(c.containers, containerCacheKey(target.ID, envID))
}

func containerCacheKey(configured string, envID int) string {
	return fmt.Sprintf("%d|%s", envID, configured)
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
	key := containerCacheKey(configured, envID)
	c.mu.Lock()
	if id, ok := c.containers[key]; ok {
		c.mu.Unlock()
		return id, nil
	}
	c.mu.Unlock()
	container, ok, err := c.findContainer(ctx, configured, envID)
	if err != nil || !ok {
		return configured, err
	}
	c.mu.Lock()
	c.containers[key] = container.ID
	c.mu.Unlock()
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
		return &apiError{Method: method, Path: path, Status: res.Status, StatusCode: res.StatusCode}
	}
	if out != nil {
		return json.NewDecoder(res.Body).Decode(out)
	}
	return nil
}
