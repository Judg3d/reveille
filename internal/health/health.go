package health

import (
	"context"
	"net/http"
	"time"

	"reveille/internal/hosts"
)

type Checker struct {
	client *http.Client
}

func NewChecker(client *http.Client) *Checker {
	return &Checker{client: client}
}

func (c *Checker) Healthy(ctx context.Context, target hosts.Target) bool {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.HealthURL, nil)
	if err != nil {
		return false
	}
	res, err := c.client.Do(req)
	if err != nil {
		return false
	}
	defer res.Body.Close()
	for _, code := range target.HealthyStatus {
		if res.StatusCode == code {
			return true
		}
	}
	return false
}
