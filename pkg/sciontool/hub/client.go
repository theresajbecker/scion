// Package hub provides a client for sciontool to communicate with the Scion Hub.
// It uses the SCION_HUB_TOKEN environment variable for authentication.
package hub

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const (
	// EnvHubURL is the environment variable for the Hub URL.
	EnvHubURL = "SCION_HUB_URL"
	// EnvHubToken is the environment variable for the Hub JWT token.
	EnvHubToken = "SCION_HUB_TOKEN"
	// EnvAgentID is the environment variable for the agent ID.
	EnvAgentID = "SCION_AGENT_ID"
	// EnvAgentMode is the environment variable for the agent mode.
	EnvAgentMode = "SCION_AGENT_MODE"

	// AgentModeHosted indicates the agent is running in hosted mode.
	AgentModeHosted = "hosted"

	// DefaultTimeout is the default HTTP request timeout.
	DefaultTimeout = 30 * time.Second
)

// AgentStatus represents the status of an agent.
type AgentStatus string

const (
	StatusPending      AgentStatus = "pending"
	StatusProvisioning AgentStatus = "provisioning"
	StatusStarting     AgentStatus = "starting"
	StatusRunning      AgentStatus = "running"
	StatusBusy         AgentStatus = "busy"
	StatusIdle         AgentStatus = "idle"
	StatusStopping     AgentStatus = "stopping"
	StatusStopped      AgentStatus = "stopped"
	StatusError        AgentStatus = "error"
	StatusShuttingDown AgentStatus = "shutting_down"
)

// StatusUpdate represents a status update request.
type StatusUpdate struct {
	Status        AgentStatus `json:"status"`
	Message       string      `json:"message,omitempty"`
	TaskSummary   string      `json:"taskSummary,omitempty"`
	Heartbeat     bool        `json:"heartbeat,omitempty"`
}

// Client is a Hub API client for sciontool.
type Client struct {
	hubURL   string
	token    string
	agentID  string
	client   *http.Client
}

// NewClient creates a new Hub client from environment variables.
// Returns nil if the required environment variables are not set.
func NewClient() *Client {
	hubURL := os.Getenv(EnvHubURL)
	token := os.Getenv(EnvHubToken)
	agentID := os.Getenv(EnvAgentID)

	if hubURL == "" || token == "" {
		return nil
	}

	return &Client{
		hubURL:  hubURL,
		token:   token,
		agentID: agentID,
		client: &http.Client{
			Timeout: DefaultTimeout,
		},
	}
}

// NewClientWithConfig creates a new Hub client with explicit configuration.
func NewClientWithConfig(hubURL, token, agentID string) *Client {
	return &Client{
		hubURL:  hubURL,
		token:   token,
		agentID: agentID,
		client: &http.Client{
			Timeout: DefaultTimeout,
		},
	}
}

// IsConfigured returns true if the client is properly configured.
func (c *Client) IsConfigured() bool {
	return c != nil && c.hubURL != "" && c.token != ""
}

// IsHostedMode returns true if the agent is running in hosted mode.
func IsHostedMode() bool {
	return os.Getenv(EnvAgentMode) == AgentModeHosted
}

// GetAgentID returns the agent ID from environment.
func GetAgentID() string {
	return os.Getenv(EnvAgentID)
}

// UpdateStatus sends a status update to the Hub.
func (c *Client) UpdateStatus(ctx context.Context, status StatusUpdate) error {
	if !c.IsConfigured() {
		return fmt.Errorf("hub client not configured")
	}

	if c.agentID == "" {
		return fmt.Errorf("agent ID not set")
	}

	endpoint := fmt.Sprintf("%s/api/v1/agents/%s/status", c.hubURL, c.agentID)

	body, err := json.Marshal(status)
	if err != nil {
		return fmt.Errorf("failed to marshal status: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Scion-Agent-Token", c.token)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("hub returned error %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// Heartbeat sends a heartbeat to the Hub.
func (c *Client) Heartbeat(ctx context.Context) error {
	return c.UpdateStatus(ctx, StatusUpdate{
		Status:    StatusRunning,
		Heartbeat: true,
	})
}

// ReportRunning reports that the agent is running.
func (c *Client) ReportRunning(ctx context.Context, message string) error {
	return c.UpdateStatus(ctx, StatusUpdate{
		Status:  StatusRunning,
		Message: message,
	})
}

// ReportBusy reports that the agent is busy with a task.
func (c *Client) ReportBusy(ctx context.Context, message string) error {
	return c.UpdateStatus(ctx, StatusUpdate{
		Status:  StatusBusy,
		Message: message,
	})
}

// ReportIdle reports that the agent is idle.
func (c *Client) ReportIdle(ctx context.Context, message string) error {
	return c.UpdateStatus(ctx, StatusUpdate{
		Status:  StatusIdle,
		Message: message,
	})
}

// ReportError reports an error status.
func (c *Client) ReportError(ctx context.Context, message string) error {
	return c.UpdateStatus(ctx, StatusUpdate{
		Status:  StatusError,
		Message: message,
	})
}

// ReportShuttingDown reports that the agent is shutting down.
func (c *Client) ReportShuttingDown(ctx context.Context, message string) error {
	return c.UpdateStatus(ctx, StatusUpdate{
		Status:  StatusShuttingDown,
		Message: message,
	})
}

// ReportTaskCompleted reports that a task has been completed.
func (c *Client) ReportTaskCompleted(ctx context.Context, taskSummary string) error {
	return c.UpdateStatus(ctx, StatusUpdate{
		Status:      StatusIdle,
		TaskSummary: taskSummary,
	})
}
