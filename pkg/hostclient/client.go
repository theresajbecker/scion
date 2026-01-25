// Package hostclient provides a Go client for the Scion Runtime Host API.
package hostclient

import (
	"context"
	"net/http"
	"time"

	"github.com/ptone/scion-agent/pkg/apiclient"
	"github.com/ptone/scion-agent/pkg/runtimehost"
)

// Client is the interface for the Runtime Host API client.
type Client interface {
	// Agents returns the agent operations interface.
	Agents() AgentService

	// Info returns host information.
	Info(ctx context.Context) (*runtimehost.HostInfoResponse, error)

	// Health checks host availability.
	Health(ctx context.Context) (*runtimehost.HealthResponse, error)
}

// client is the concrete implementation of Client.
type client struct {
	transport *apiclient.Transport
	agents    *agentService
}

// New creates a new Runtime Host API client.
func New(baseURL string, opts ...Option) (Client, error) {
	c := &client{
		transport: apiclient.NewTransport(baseURL),
	}

	for _, opt := range opts {
		opt(c)
	}

	c.agents = &agentService{c: c}

	return c, nil
}

// Agents returns the agent operations interface.
func (c *client) Agents() AgentService {
	return c.agents
}

// Info returns host information.
func (c *client) Info(ctx context.Context) (*runtimehost.HostInfoResponse, error) {
	resp, err := c.transport.Get(ctx, "/api/v1/info", nil)
	if err != nil {
		return nil, err
	}
	return apiclient.DecodeResponse[runtimehost.HostInfoResponse](resp)
}

// Health checks host availability.
func (c *client) Health(ctx context.Context) (*runtimehost.HealthResponse, error) {
	resp, err := c.transport.Get(ctx, "/healthz", nil)
	if err != nil {
		return nil, err
	}
	return apiclient.DecodeResponse[runtimehost.HealthResponse](resp)
}

// Option configures a Runtime Host client.
type Option func(*client)

// WithBearerToken sets Bearer token authentication.
func WithBearerToken(token string) Option {
	return func(c *client) {
		c.transport.Auth = &apiclient.BearerAuth{Token: token}
	}
}

// WithHostToken sets Runtime Host token authentication.
func WithHostToken(token string) Option {
	return func(c *client) {
		c.transport.Auth = &apiclient.HostTokenAuth{Token: token}
	}
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *client) {
		c.transport.HTTPClient = hc
	}
}

// WithTimeout sets the request timeout.
func WithTimeout(d time.Duration) Option {
	return func(c *client) {
		c.transport.HTTPClient.Timeout = d
	}
}

// WithRetry configures retry behavior.
func WithRetry(maxRetries int, wait time.Duration) Option {
	return func(c *client) {
		c.transport.MaxRetries = maxRetries
		c.transport.RetryWait = wait
	}
}
