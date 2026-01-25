package apiclient

import "net/http"

// Authenticator provides authentication credentials for API requests.
type Authenticator interface {
	// ApplyAuth adds authentication to the request (header, query param, etc.)
	ApplyAuth(req *http.Request) error

	// Refresh refreshes expired credentials if supported.
	// Returns false if refresh is not supported.
	Refresh() (bool, error)
}

// BearerAuth implements Bearer token authentication.
type BearerAuth struct {
	Token string
}

// ApplyAuth adds the Bearer token to the Authorization header.
func (a *BearerAuth) ApplyAuth(req *http.Request) error {
	if a.Token != "" {
		req.Header.Set("Authorization", "Bearer "+a.Token)
	}
	return nil
}

// Refresh indicates that refresh is not supported for static tokens.
func (a *BearerAuth) Refresh() (bool, error) { return false, nil }

// APIKeyAuth implements API key authentication.
type APIKeyAuth struct {
	Key string
}

// ApplyAuth adds the API key to the X-API-Key header.
func (a *APIKeyAuth) ApplyAuth(req *http.Request) error {
	if a.Key != "" {
		req.Header.Set("X-API-Key", a.Key)
	}
	return nil
}

// Refresh indicates that refresh is not supported for API keys.
func (a *APIKeyAuth) Refresh() (bool, error) { return false, nil }

// HostTokenAuth implements Runtime Host token authentication.
type HostTokenAuth struct {
	Token string
}

// ApplyAuth adds the host token to the X-Scion-Host-Token header.
func (a *HostTokenAuth) ApplyAuth(req *http.Request) error {
	if a.Token != "" {
		req.Header.Set("X-Scion-Host-Token", a.Token)
	}
	return nil
}

// Refresh indicates that refresh is not supported for host tokens.
func (a *HostTokenAuth) Refresh() (bool, error) { return false, nil }

// AgentTokenAuth implements Agent token authentication.
type AgentTokenAuth struct {
	Token string
}

// ApplyAuth adds the agent token to the X-Scion-Agent-Token header.
func (a *AgentTokenAuth) ApplyAuth(req *http.Request) error {
	if a.Token != "" {
		req.Header.Set("X-Scion-Agent-Token", a.Token)
	}
	return nil
}

// Refresh indicates that refresh is not supported for agent tokens.
func (a *AgentTokenAuth) Refresh() (bool, error) { return false, nil }
