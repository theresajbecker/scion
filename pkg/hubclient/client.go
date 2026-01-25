// Package hubclient provides a Go client for the Scion Hub API.
package hubclient

import (
	"context"
	"net/http"
	"time"

	"github.com/ptone/scion-agent/pkg/apiclient"
)

// Client is the interface for the Hub API client.
// This interface enables mocking for tests.
type Client interface {
	// Agents returns the agent operations interface.
	Agents() AgentService

	// Groves returns the grove operations interface.
	Groves() GroveService

	// RuntimeHosts returns the runtime host operations interface.
	RuntimeHosts() RuntimeHostService

	// Templates returns the template operations interface.
	Templates() TemplateService

	// Users returns the user operations interface.
	Users() UserService

	// Auth returns the authentication operations interface.
	Auth() AuthService

	// Health checks API availability.
	Health(ctx context.Context) (*HealthResponse, error)
}

// client is the concrete implementation of Client.
type client struct {
	transport *apiclient.Transport

	agents       *agentService
	groves       *groveService
	runtimeHosts *runtimeHostService
	templates    *templateService
	users        *userService
	authService  *authService
}

// New creates a new Hub API client.
func New(baseURL string, opts ...Option) (Client, error) {
	c := &client{
		transport: apiclient.NewTransport(baseURL),
	}

	for _, opt := range opts {
		opt(c)
	}

	// Initialize service implementations
	c.agents = &agentService{c: c}
	c.groves = &groveService{c: c}
	c.runtimeHosts = &runtimeHostService{c: c}
	c.templates = &templateService{c: c}
	c.users = &userService{c: c}
	c.authService = &authService{c: c}

	return c, nil
}

// Agents returns the agent operations interface.
func (c *client) Agents() AgentService {
	return c.agents
}

// Groves returns the grove operations interface.
func (c *client) Groves() GroveService {
	return c.groves
}

// RuntimeHosts returns the runtime host operations interface.
func (c *client) RuntimeHosts() RuntimeHostService {
	return c.runtimeHosts
}

// Templates returns the template operations interface.
func (c *client) Templates() TemplateService {
	return c.templates
}

// Users returns the user operations interface.
func (c *client) Users() UserService {
	return c.users
}

// Auth returns the authentication operations interface.
func (c *client) Auth() AuthService {
	return c.authService
}

// Health checks API availability.
func (c *client) Health(ctx context.Context) (*HealthResponse, error) {
	resp, err := c.transport.Get(ctx, "/healthz", nil)
	if err != nil {
		return nil, err
	}
	return apiclient.DecodeResponse[HealthResponse](resp)
}

// HealthResponse is the response from health check.
type HealthResponse struct {
	Status  string            `json:"status"`
	Version string            `json:"version"`
	Uptime  string            `json:"uptime"`
	Checks  map[string]string `json:"checks,omitempty"`
}

// Option configures a Hub client.
type Option func(*client)

// WithBearerToken sets Bearer token authentication.
func WithBearerToken(token string) Option {
	return func(c *client) {
		c.transport.Auth = &apiclient.BearerAuth{Token: token}
	}
}

// WithAPIKey sets API key authentication.
func WithAPIKey(key string) Option {
	return func(c *client) {
		c.transport.Auth = &apiclient.APIKeyAuth{Key: key}
	}
}

// WithAuthenticator sets a custom authenticator.
func WithAuthenticator(auth apiclient.Authenticator) Option {
	return func(c *client) {
		c.transport.Auth = auth
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
