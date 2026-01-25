package hubclient

import (
	"context"

	"github.com/ptone/scion-agent/pkg/apiclient"
)

// AuthService handles authentication operations.
type AuthService interface {
	// Login performs user login.
	Login(ctx context.Context, req *LoginRequest) (*LoginResponse, error)

	// Logout invalidates the current session.
	Logout(ctx context.Context) error

	// Refresh refreshes an access token.
	Refresh(ctx context.Context, refreshToken string) (*TokenResponse, error)

	// Me returns the current authenticated user.
	Me(ctx context.Context) (*User, error)

	// GetWSTicket gets a short-lived WebSocket authentication ticket.
	GetWSTicket(ctx context.Context) (*WSTicketResponse, error)
}

// authService is the implementation of AuthService.
type authService struct {
	c *client
}

// LoginRequest is the request for user login.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// LoginResponse is the response from login.
type LoginResponse struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresAt    string `json:"expiresAt"`
	User         *User  `json:"user"`
}

// TokenResponse is the response from token refresh.
type TokenResponse struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken,omitempty"`
	ExpiresAt    string `json:"expiresAt"`
}

// WSTicketResponse is the response for WebSocket ticket.
type WSTicketResponse struct {
	Ticket    string `json:"ticket"`
	ExpiresAt string `json:"expiresAt"`
}

// Login performs user login.
func (s *authService) Login(ctx context.Context, req *LoginRequest) (*LoginResponse, error) {
	resp, err := s.c.transport.Post(ctx, "/api/v1/auth/login", req, nil)
	if err != nil {
		return nil, err
	}
	return apiclient.DecodeResponse[LoginResponse](resp)
}

// Logout invalidates the current session.
func (s *authService) Logout(ctx context.Context) error {
	resp, err := s.c.transport.Post(ctx, "/api/v1/auth/logout", nil, nil)
	if err != nil {
		return err
	}
	return apiclient.CheckResponse(resp)
}

// Refresh refreshes an access token.
func (s *authService) Refresh(ctx context.Context, refreshToken string) (*TokenResponse, error) {
	body := struct {
		RefreshToken string `json:"refreshToken"`
	}{
		RefreshToken: refreshToken,
	}
	resp, err := s.c.transport.Post(ctx, "/api/v1/auth/refresh", body, nil)
	if err != nil {
		return nil, err
	}
	return apiclient.DecodeResponse[TokenResponse](resp)
}

// Me returns the current authenticated user.
func (s *authService) Me(ctx context.Context) (*User, error) {
	resp, err := s.c.transport.Get(ctx, "/api/v1/auth/me", nil)
	if err != nil {
		return nil, err
	}
	return apiclient.DecodeResponse[User](resp)
}

// GetWSTicket gets a short-lived WebSocket authentication ticket.
func (s *authService) GetWSTicket(ctx context.Context) (*WSTicketResponse, error) {
	resp, err := s.c.transport.Post(ctx, "/api/v1/auth/ws-ticket", nil, nil)
	if err != nil {
		return nil, err
	}
	return apiclient.DecodeResponse[WSTicketResponse](resp)
}
