package hubclient

import (
	"context"
	"net/url"

	"github.com/ptone/scion-agent/pkg/apiclient"
)

// TemplateService handles template operations.
type TemplateService interface {
	// List returns templates matching the filter criteria.
	List(ctx context.Context, opts *ListTemplatesOptions) (*ListTemplatesResponse, error)

	// Get returns a single template by ID.
	Get(ctx context.Context, templateID string) (*Template, error)

	// Create creates a new template.
	Create(ctx context.Context, req *CreateTemplateRequest) (*Template, error)

	// Update updates a template.
	Update(ctx context.Context, templateID string, req *UpdateTemplateRequest) (*Template, error)

	// Delete removes a template.
	Delete(ctx context.Context, templateID string) error

	// Clone creates a copy of a template.
	Clone(ctx context.Context, templateID string, req *CloneTemplateRequest) (*Template, error)
}

// templateService is the implementation of TemplateService.
type templateService struct {
	c *client
}

// ListTemplatesOptions configures template list filtering.
type ListTemplatesOptions struct {
	Scope   string // Filter by scope (global, grove, user)
	GroveID string // Filter by grove
	Harness string // Filter by harness type
	Page    apiclient.PageOptions
}

// ListTemplatesResponse is the response from listing templates.
type ListTemplatesResponse struct {
	Templates []Template
	Page      apiclient.PageResult
}

// CreateTemplateRequest is the request for creating a template.
type CreateTemplateRequest struct {
	Name       string          `json:"name"`
	Harness    string          `json:"harness"`
	Scope      string          `json:"scope"`
	GroveID    string          `json:"groveId,omitempty"`
	Config     *TemplateConfig `json:"config,omitempty"`
	Visibility string          `json:"visibility,omitempty"`
}

// UpdateTemplateRequest is the request for updating a template.
type UpdateTemplateRequest struct {
	Name       string          `json:"name,omitempty"`
	Config     *TemplateConfig `json:"config,omitempty"`
	Visibility string          `json:"visibility,omitempty"`
}

// CloneTemplateRequest is the request for cloning a template.
type CloneTemplateRequest struct {
	Name    string `json:"name"`
	Scope   string `json:"scope"`
	GroveID string `json:"groveId,omitempty"`
}

// List returns templates matching the filter criteria.
func (s *templateService) List(ctx context.Context, opts *ListTemplatesOptions) (*ListTemplatesResponse, error) {
	query := url.Values{}
	if opts != nil {
		if opts.Scope != "" {
			query.Set("scope", opts.Scope)
		}
		if opts.GroveID != "" {
			query.Set("groveId", opts.GroveID)
		}
		if opts.Harness != "" {
			query.Set("harness", opts.Harness)
		}
		opts.Page.ToQuery(query)
	}

	resp, err := s.c.transport.GetWithQuery(ctx, "/api/v1/templates", query, nil)
	if err != nil {
		return nil, err
	}

	type listResponse struct {
		Templates  []Template `json:"templates"`
		NextCursor string     `json:"nextCursor,omitempty"`
		TotalCount int        `json:"totalCount,omitempty"`
	}

	result, err := apiclient.DecodeResponse[listResponse](resp)
	if err != nil {
		return nil, err
	}

	return &ListTemplatesResponse{
		Templates: result.Templates,
		Page: apiclient.PageResult{
			NextCursor: result.NextCursor,
			TotalCount: result.TotalCount,
		},
	}, nil
}

// Get returns a single template by ID.
func (s *templateService) Get(ctx context.Context, templateID string) (*Template, error) {
	resp, err := s.c.transport.Get(ctx, "/api/v1/templates/"+templateID, nil)
	if err != nil {
		return nil, err
	}
	return apiclient.DecodeResponse[Template](resp)
}

// Create creates a new template.
func (s *templateService) Create(ctx context.Context, req *CreateTemplateRequest) (*Template, error) {
	resp, err := s.c.transport.Post(ctx, "/api/v1/templates", req, nil)
	if err != nil {
		return nil, err
	}
	return apiclient.DecodeResponse[Template](resp)
}

// Update updates a template.
func (s *templateService) Update(ctx context.Context, templateID string, req *UpdateTemplateRequest) (*Template, error) {
	resp, err := s.c.transport.Patch(ctx, "/api/v1/templates/"+templateID, req, nil)
	if err != nil {
		return nil, err
	}
	return apiclient.DecodeResponse[Template](resp)
}

// Delete removes a template.
func (s *templateService) Delete(ctx context.Context, templateID string) error {
	resp, err := s.c.transport.Delete(ctx, "/api/v1/templates/"+templateID, nil)
	if err != nil {
		return err
	}
	return apiclient.CheckResponse(resp)
}

// Clone creates a copy of a template.
func (s *templateService) Clone(ctx context.Context, templateID string, req *CloneTemplateRequest) (*Template, error) {
	resp, err := s.c.transport.Post(ctx, "/api/v1/templates/"+templateID+"/clone", req, nil)
	if err != nil {
		return nil, err
	}
	return apiclient.DecodeResponse[Template](resp)
}
