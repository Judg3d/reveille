package dockhand

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"reveille/internal/hosts"
)

type Client struct {
	baseURL string
	token   string
	envID   int
	client  *http.Client
}

type Container struct {
	ID     string   `json:"id"`
	Names  []string `json:"names"`
	Name   string   `json:"name"`
	Status string   `json:"status"`
	State  string   `json:"state"`
}

func NewClient(baseURL, token string, envID int, timeout time.Duration) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		envID:   envID,
		client:  &http.Client{Timeout: timeout},
	}
}

func (c *Client) Start(ctx context.Context, target hosts.Target) error {
	if target.Type == "stack" {
		return c.do(ctx, http.MethodPost, "/api/stacks/"+url.PathEscape(target.Name)+"/start", nil, nil)
	}
	id, err := c.resolveContainer(ctx, target.ID)
	if err != nil {
		return err
	}
	return c.do(ctx, http.MethodPost, "/api/containers/"+url.PathEscape(id)+"/start", nil, nil)
}

func (c *Client) Stop(ctx context.Context, target hosts.Target) error {
	if target.Type == "stack" {
		return c.do(ctx, http.MethodPost, "/api/stacks/"+url.PathEscape(target.Name)+"/stop", nil, nil)
	}
	id, err := c.resolveContainer(ctx, target.ID)
	if err != nil {
		return err
	}
	return c.do(ctx, http.MethodPost, "/api/containers/"+url.PathEscape(id)+"/stop", nil, nil)
}

func (c *Client) Containers(ctx context.Context) ([]Container, error) {
	var out []Container
	if err := c.do(ctx, http.MethodGet, "/api/containers", url.Values{"all": {"true"}}, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) resolveContainer(ctx context.Context, configured string) (string, error) {
	if configured == "" {
		return "", fmt.Errorf("container id is empty")
	}
	containers, err := c.Containers(ctx)
	if err != nil {
		return "", err
	}
	want := strings.TrimPrefix(configured, "/")
	for _, container := range containers {
		if container.ID == configured || strings.HasPrefix(container.ID, configured) || container.Name == want || strings.TrimPrefix(container.Name, "/") == want {
			return container.ID, nil
		}
		for _, name := range container.Names {
			if strings.TrimPrefix(name, "/") == want {
				return container.ID, nil
			}
		}
	}
	return configured, nil
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, out any) error {
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
	q.Set("env", fmt.Sprintf("%d", c.envID))
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
