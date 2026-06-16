package health

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"reveille/internal/hosts"
)

type Result struct {
	Healthy    bool
	Reachable  bool
	StatusCode int
	Error      string
	CheckedAt  time.Time
}

type Checker struct {
	client *http.Client
}

func NewChecker(client *http.Client) *Checker {
	return &Checker{client: client}
}

func (c *Checker) Healthy(ctx context.Context, target hosts.Target) bool {
	return c.Check(ctx, target).Healthy
}

func (c *Checker) Check(ctx context.Context, target hosts.Target) Result {
	result := Result{CheckedAt: time.Now().UTC()}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.HealthURL, nil)
	if err != nil {
		result.Error = fmt.Sprintf("build request: %v", err)
		return result
	}
	res, err := c.client.Do(req)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	defer res.Body.Close()
	result.Reachable = true
	result.StatusCode = res.StatusCode
	for _, code := range target.HealthyStatus {
		if res.StatusCode == code {
			result.Healthy = true
			return result
		}
	}
	return result
}
