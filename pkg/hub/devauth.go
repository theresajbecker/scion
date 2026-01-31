package hub

import (
	"context"
	"log"
	"net/http"
	"strings"

	"github.com/ptone/scion-agent/pkg/apiclient"
)

// DevUser represents the pseudo-user for development authentication.
type DevUser struct {
	id string
}

// ID returns the user ID.
func (u *DevUser) ID() string { return u.id }

// Email returns the user email.
func (u *DevUser) Email() string { return "dev@localhost" }

// DisplayName returns the user display name.
func (u *DevUser) DisplayName() string { return "Development User" }

// Role returns the user role.
func (u *DevUser) Role() string { return "admin" }

// userContextKey is the key for storing the user in the request context.
type userContextKey struct{}

// DevAuthMiddleware creates middleware that validates development tokens.
// If the token is valid, it adds a DevUser to the request context.
// Use DevAuthMiddlewareWithDebug for verbose logging of auth failures.
func DevAuthMiddleware(validToken string) func(http.Handler) http.Handler {
	return DevAuthMiddlewareWithDebug(validToken, false)
}

// DevAuthMiddlewareWithDebug creates middleware with optional debug logging.
func DevAuthMiddlewareWithDebug(validToken string, debug bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip auth for health endpoints
			if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
				next.ServeHTTP(w, r)
				return
			}

			// Check if already authenticated by agent token middleware
			if GetAgentFromContext(r.Context()) != nil {
				if debug {
					log.Printf("[Hub] Auth success: agent token already validated")
				}
				next.ServeHTTP(w, r)
				return
			}

			// Check for X-Scion-Agent-Token header - if present, skip dev auth
			// (the agent token middleware will have validated it or rejected it)
			if r.Header.Get("X-Scion-Agent-Token") != "" {
				// Agent token was present but not validated - reject
				if debug {
					log.Printf("[Hub] Auth failed: X-Scion-Agent-Token present but not validated")
				}
				writeError(w, http.StatusUnauthorized, ErrCodeUnauthorized,
					"invalid agent token", nil)
				return
			}

			// Extract token from Authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				if debug {
					log.Printf("[Hub] Auth failed: missing Authorization header for %s %s", r.Method, r.URL.Path)
				}
				writeError(w, http.StatusUnauthorized, ErrCodeUnauthorized,
					"missing authorization header", nil)
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
				if debug {
					log.Printf("[Hub] Auth failed: invalid Authorization header format (expected 'Bearer <token>')")
				}
				writeError(w, http.StatusUnauthorized, ErrCodeUnauthorized,
					"invalid authorization header format", nil)
				return
			}

			token := parts[1]

			// Validate token (constant-time comparison)
			if !apiclient.ValidateDevToken(token, validToken) {
				if debug {
					// Log token prefix for debugging (safe: only shows first chars)
					tokenPrefix := token
					if len(tokenPrefix) > 20 {
						tokenPrefix = tokenPrefix[:20] + "..."
					}
					expectedPrefix := validToken
					if len(expectedPrefix) > 20 {
						expectedPrefix = expectedPrefix[:20] + "..."
					}
					log.Printf("[Hub] Auth failed: token mismatch (provided: %s, expected: %s)", tokenPrefix, expectedPrefix)
				}
				writeError(w, http.StatusUnauthorized, ErrCodeUnauthorized,
					"invalid token", nil)
				return
			}

			if debug {
				log.Printf("[Hub] Auth success: dev-user authenticated")
			}

			// Add dev user context
			ctx := context.WithValue(r.Context(), userContextKey{}, &DevUser{id: "dev-user"})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetUserFromContext retrieves the user from the request context.
func GetUserFromContext(ctx context.Context) *DevUser {
	if user, ok := ctx.Value(userContextKey{}).(*DevUser); ok {
		return user
	}
	return nil
}
