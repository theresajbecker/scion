package hubclient

import (
	"context"
	"fmt"
	"net/url"

	"github.com/ptone/scion-agent/pkg/apiclient"
)

// AgentService handles agent operations.
type AgentService interface {
	// List returns agents matching the filter criteria.
	List(ctx context.Context, opts *ListAgentsOptions) (*ListAgentsResponse, error)

	// Get returns a single agent by ID.
	Get(ctx context.Context, agentID string) (*Agent, error)

	// Create creates a new agent.
	Create(ctx context.Context, req *CreateAgentRequest) (*CreateAgentResponse, error)

	// Update updates an agent's metadata.
	Update(ctx context.Context, agentID string, req *UpdateAgentRequest) (*Agent, error)

	// Delete removes an agent.
	Delete(ctx context.Context, agentID string, opts *DeleteAgentOptions) error

	// Start starts a stopped agent.
	Start(ctx context.Context, agentID string) error

	// Stop stops a running agent.
	Stop(ctx context.Context, agentID string) error

	// Restart restarts an agent.
	Restart(ctx context.Context, agentID string) error

	// SendMessage sends a message to an agent.
	SendMessage(ctx context.Context, agentID string, message string, interrupt bool) error

	// Exec executes a command in an agent container.
	Exec(ctx context.Context, agentID string, command []string, timeout int) (*ExecResponse, error)

	// GetLogs retrieves agent logs.
	GetLogs(ctx context.Context, agentID string, opts *GetLogsOptions) (string, error)
}

// agentService is the implementation of AgentService.
type agentService struct {
	c *client
}

// ListAgentsOptions configures agent list filtering.
type ListAgentsOptions struct {
	GroveID       string            // Filter by grove
	Status        string            // Filter by status
	RuntimeHostID string            // Filter by runtime host
	Labels        map[string]string // Label selector
	Page          apiclient.PageOptions
}

// ListAgentsResponse is the response from listing agents.
type ListAgentsResponse struct {
	Agents []Agent
	Page   apiclient.PageResult
}

// CreateAgentRequest is the request body for creating an agent.
type CreateAgentRequest struct {
	Name          string            `json:"name"`
	GroveID       string            `json:"groveId"`
	Template      string            `json:"template,omitempty"`
	RuntimeHostID string            `json:"runtimeHostId,omitempty"`
	Task          string            `json:"task,omitempty"`
	Branch        string            `json:"branch,omitempty"`
	Workspace     string            `json:"workspace,omitempty"`
	Labels        map[string]string `json:"labels,omitempty"`
	Annotations   map[string]string `json:"annotations,omitempty"`
	Config        *AgentConfig      `json:"config,omitempty"`
	Resume        bool              `json:"resume,omitempty"`
}

// CreateAgentResponse is the response from creating an agent.
type CreateAgentResponse struct {
	Agent    *Agent   `json:"agent"`
	Warnings []string `json:"warnings,omitempty"`
}

// UpdateAgentRequest is the request body for updating an agent.
type UpdateAgentRequest struct {
	Name         string            `json:"name,omitempty"`
	Labels       map[string]string `json:"labels,omitempty"`
	Annotations  map[string]string `json:"annotations,omitempty"`
	TaskSummary  string            `json:"taskSummary,omitempty"`
	StateVersion int64             `json:"stateVersion"` // Required for optimistic locking
}

// DeleteAgentOptions configures agent deletion.
type DeleteAgentOptions struct {
	DeleteFiles  bool // Also delete agent files
	RemoveBranch bool // Remove git branch
}

// GetLogsOptions configures log retrieval.
type GetLogsOptions struct {
	Tail  int    // Number of lines from end
	Since string // RFC3339 timestamp
}

// ExecResponse is the response from executing a command.
type ExecResponse struct {
	Output   string `json:"output"`
	ExitCode int    `json:"exitCode"`
}

// List returns agents matching the filter criteria.
func (s *agentService) List(ctx context.Context, opts *ListAgentsOptions) (*ListAgentsResponse, error) {
	query := url.Values{}
	if opts != nil {
		if opts.GroveID != "" {
			query.Set("groveId", opts.GroveID)
		}
		if opts.Status != "" {
			query.Set("status", opts.Status)
		}
		if opts.RuntimeHostID != "" {
			query.Set("runtimeHostId", opts.RuntimeHostID)
		}
		for k, v := range opts.Labels {
			query.Add("label", fmt.Sprintf("%s=%s", k, v))
		}
		opts.Page.ToQuery(query)
	}

	resp, err := s.c.transport.GetWithQuery(ctx, "/api/v1/agents", query, nil)
	if err != nil {
		return nil, err
	}

	type listResponse struct {
		Agents     []Agent `json:"agents"`
		NextCursor string  `json:"nextCursor,omitempty"`
		TotalCount int     `json:"totalCount,omitempty"`
	}

	result, err := apiclient.DecodeResponse[listResponse](resp)
	if err != nil {
		return nil, err
	}

	return &ListAgentsResponse{
		Agents: result.Agents,
		Page: apiclient.PageResult{
			NextCursor: result.NextCursor,
			TotalCount: result.TotalCount,
		},
	}, nil
}

// Get returns a single agent by ID.
func (s *agentService) Get(ctx context.Context, agentID string) (*Agent, error) {
	resp, err := s.c.transport.Get(ctx, "/api/v1/agents/"+agentID, nil)
	if err != nil {
		return nil, err
	}
	return apiclient.DecodeResponse[Agent](resp)
}

// Create creates a new agent.
func (s *agentService) Create(ctx context.Context, req *CreateAgentRequest) (*CreateAgentResponse, error) {
	resp, err := s.c.transport.Post(ctx, "/api/v1/agents", req, nil)
	if err != nil {
		return nil, err
	}
	return apiclient.DecodeResponse[CreateAgentResponse](resp)
}

// Update updates an agent's metadata.
func (s *agentService) Update(ctx context.Context, agentID string, req *UpdateAgentRequest) (*Agent, error) {
	resp, err := s.c.transport.Patch(ctx, "/api/v1/agents/"+agentID, req, nil)
	if err != nil {
		return nil, err
	}
	return apiclient.DecodeResponse[Agent](resp)
}

// Delete removes an agent.
func (s *agentService) Delete(ctx context.Context, agentID string, opts *DeleteAgentOptions) error {
	path := "/api/v1/agents/" + agentID
	if opts != nil {
		query := url.Values{}
		if opts.DeleteFiles {
			query.Set("deleteFiles", "true")
		}
		if opts.RemoveBranch {
			query.Set("removeBranch", "true")
		}
		if len(query) > 0 {
			path += "?" + query.Encode()
		}
	}

	resp, err := s.c.transport.Delete(ctx, path, nil)
	if err != nil {
		return err
	}
	return apiclient.CheckResponse(resp)
}

// Start starts a stopped agent.
func (s *agentService) Start(ctx context.Context, agentID string) error {
	resp, err := s.c.transport.Post(ctx, "/api/v1/agents/"+agentID+"/start", nil, nil)
	if err != nil {
		return err
	}
	return apiclient.CheckResponse(resp)
}

// Stop stops a running agent.
func (s *agentService) Stop(ctx context.Context, agentID string) error {
	resp, err := s.c.transport.Post(ctx, "/api/v1/agents/"+agentID+"/stop", nil, nil)
	if err != nil {
		return err
	}
	return apiclient.CheckResponse(resp)
}

// Restart restarts an agent.
func (s *agentService) Restart(ctx context.Context, agentID string) error {
	resp, err := s.c.transport.Post(ctx, "/api/v1/agents/"+agentID+"/restart", nil, nil)
	if err != nil {
		return err
	}
	return apiclient.CheckResponse(resp)
}

// SendMessage sends a message to an agent.
func (s *agentService) SendMessage(ctx context.Context, agentID string, message string, interrupt bool) error {
	body := struct {
		Message   string `json:"message"`
		Interrupt bool   `json:"interrupt,omitempty"`
	}{
		Message:   message,
		Interrupt: interrupt,
	}
	resp, err := s.c.transport.Post(ctx, "/api/v1/agents/"+agentID+"/message", body, nil)
	if err != nil {
		return err
	}
	return apiclient.CheckResponse(resp)
}

// Exec executes a command in an agent container.
func (s *agentService) Exec(ctx context.Context, agentID string, command []string, timeout int) (*ExecResponse, error) {
	body := struct {
		Command []string `json:"command"`
		Timeout int      `json:"timeout,omitempty"`
	}{
		Command: command,
		Timeout: timeout,
	}
	resp, err := s.c.transport.Post(ctx, "/api/v1/agents/"+agentID+"/exec", body, nil)
	if err != nil {
		return nil, err
	}
	return apiclient.DecodeResponse[ExecResponse](resp)
}

// GetLogs retrieves agent logs.
func (s *agentService) GetLogs(ctx context.Context, agentID string, opts *GetLogsOptions) (string, error) {
	query := url.Values{}
	if opts != nil {
		if opts.Tail > 0 {
			query.Set("tail", fmt.Sprintf("%d", opts.Tail))
		}
		if opts.Since != "" {
			query.Set("since", opts.Since)
		}
	}

	resp, err := s.c.transport.GetWithQuery(ctx, "/api/v1/agents/"+agentID+"/logs", query, nil)
	if err != nil {
		return "", err
	}

	type logsResponse struct {
		Logs string `json:"logs"`
	}

	result, err := apiclient.DecodeResponse[logsResponse](resp)
	if err != nil {
		return "", err
	}
	return result.Logs, nil
}
