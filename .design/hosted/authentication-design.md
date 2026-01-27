# Scion Authentication Design

## Status
**Proposed**

## 1. Overview

This document specifies the authentication mechanisms for Scion's hosted mode. Authentication establishes user identity across multiple client types while maintaining security and usability.

### Authentication Contexts

| Context | Client Type | Auth Method | Token Storage |
|---------|-------------|-------------|---------------|
| Web Dashboard | Browser | OAuth + Session Cookie | HTTP-only cookie |
| CLI (Hub Commands) | Terminal | OAuth + Device Flow | Local file (`~/.scion/credentials.json`) |
| API Direct | Programmatic | API Key or JWT | Client-managed |

### Goals

1. **Unified Identity** - Single user identity across all client types
2. **Secure Token Management** - Appropriate storage for each context
3. **Developer Experience** - Minimal friction for CLI authentication
4. **Standard Protocols** - OAuth 2.0 / OpenID Connect compliance

### Non-Goals

- Runtime host authentication (addressed in separate design - see Section 8)
- Service-to-service authentication between Hub components
- Multi-tenant Hub federation

---

## 2. Identity Model

### 2.1 User Identity

A user is identified by their email address, which serves as the canonical identifier across OAuth providers.

```go
type User struct {
    ID           string    `json:"id"`           // UUID primary key
    Email        string    `json:"email"`        // Canonical identifier
    DisplayName  string    `json:"displayName"`
    AvatarURL    string    `json:"avatarUrl,omitempty"`

    // OAuth provider info
    Provider     string    `json:"provider"`     // "google", "github", etc.
    ProviderID   string    `json:"providerId"`   // Provider's user ID

    // Status
    Role         string    `json:"role"`         // "admin", "member", "viewer"
    Status       string    `json:"status"`       // "active", "suspended", "pending"

    // Timestamps
    Created      time.Time `json:"created"`
    LastLogin    time.Time `json:"lastLogin"`
}
```

### 2.2 Authentication Tokens

The Hub issues JWT tokens for authenticated sessions:

```go
type TokenClaims struct {
    jwt.RegisteredClaims

    UserID      string   `json:"uid"`
    Email       string   `json:"email"`
    Role        string   `json:"role"`
    TokenType   string   `json:"type"`    // "access", "refresh", "cli"
    ClientType  string   `json:"client"`  // "web", "cli", "api"
}
```

**Token Types:**

| Type | Lifetime | Purpose |
|------|----------|---------|
| `access` | 15 minutes | Short-lived API access |
| `refresh` | 7 days | Token renewal |
| `cli` | 30 days | CLI session (longer-lived for developer convenience) |

---

## 3. Web Authentication (OAuth)

Web authentication uses standard OAuth 2.0 authorization code flow with session cookies.

### 3.1 Flow Diagram

```
┌─────────┐     ┌─────────────┐     ┌──────────────┐     ┌─────────┐
│ Browser │────►│Web Frontend │────►│OAuth Provider│────►│ Hub API │
│         │     │   :9820     │     │(Google/GitHub)│    │ :9810   │
└─────────┘     └─────────────┘     └──────────────┘     └─────────┘
     │                │                    │                  │
     │  1. GET /auth/login/google          │                  │
     │───────────────►│                    │                  │
     │                │  2. Redirect to OAuth                 │
     │◄───────────────│───────────────────►│                  │
     │  3. User authorizes                 │                  │
     │◄───────────────────────────────────►│                  │
     │                │  4. Callback with code                │
     │───────────────►│◄───────────────────│                  │
     │                │  5. Exchange code for tokens          │
     │                │───────────────────►│                  │
     │                │◄───────────────────│                  │
     │                │  6. Create/lookup user                │
     │                │────────────────────────────────────►│
     │                │◄────────────────────────────────────│
     │                │  7. Issue session token               │
     │                │────────────────────────────────────►│
     │                │◄────────────────────────────────────│
     │  8. Set session cookie                                 │
     │◄───────────────│                    │                  │
     │  9. Redirect to app                 │                  │
     │◄───────────────│                    │                  │
```

### 3.2 Session Management

Web sessions use HTTP-only cookies with the following properties:

```typescript
const sessionConfig = {
  name: 'scion:sess',
  maxAge: 24 * 60 * 60 * 1000,  // 24 hours
  httpOnly: true,
  secure: true,                  // HTTPS only in production
  sameSite: 'lax',
  signed: true
};
```

### 3.3 Hub API Endpoints

```
POST /api/v1/auth/login
  Request:  { provider, email, name, avatar, providerToken }
  Response: { user, accessToken, refreshToken }

POST /api/v1/auth/refresh
  Request:  { refreshToken }
  Response: { accessToken, refreshToken }

POST /api/v1/auth/logout
  Request:  { refreshToken? }
  Response: { success: true }

GET /api/v1/auth/me
  Response: { user }
```

---

## 4. CLI Authentication

CLI authentication enables `scion hub` commands to authenticate with a Hub server using a browser-based OAuth flow with localhost callback.

### 4.1 Commands

```bash
# Check authentication status
scion hub auth status

# Authenticate with Hub (opens browser)
scion hub auth login [--hub-url <url>]

# Clear stored credentials
scion hub auth logout
```

### 4.2 Device Authorization Flow

The CLI uses OAuth 2.0 with a localhost redirect for systems with a browser:

```
┌──────────┐     ┌─────────────┐     ┌──────────────┐     ┌─────────┐
│   CLI    │     │  Localhost  │     │OAuth Provider│     │ Hub API │
│ Terminal │     │   :18271    │     │              │     │ :9810   │
└──────────┘     └─────────────┘     └──────────────┘     └─────────┘
     │                 │                    │                  │
     │  1. scion hub auth login            │                  │
     │─────────────────┼───────────────────┼─────────────────►│
     │                 │                   │   2. Get auth URL │
     │◄────────────────┼───────────────────┼──────────────────│
     │  3. Start localhost server          │                  │
     │────────────────►│                   │                  │
     │  4. Open browser with auth URL      │                  │
     │─────────────────┼──────────────────►│                  │
     │                 │  5. User authorizes                  │
     │                 │◄─────────────────►│                  │
     │                 │  6. Redirect to localhost            │
     │                 │◄──────────────────│                  │
     │  7. Receive auth code               │                  │
     │◄────────────────│                   │                  │
     │  8. Exchange code for CLI token     │                  │
     │─────────────────┼───────────────────┼─────────────────►│
     │◄────────────────┼───────────────────┼──────────────────│
     │  9. Store credentials locally       │                  │
     │                 │                   │                  │
```

### 4.3 Implementation Details

#### Localhost Callback Server

```go
// pkg/hub/auth/localhost_server.go

const (
    CallbackPort = 18271  // Arbitrary high port for localhost callback
    CallbackPath = "/callback"
)

type LocalhostAuthServer struct {
    server     *http.Server
    codeChan   chan string
    errChan    chan error
    state      string
}

func (s *LocalhostAuthServer) Start(ctx context.Context) (string, error) {
    // Generate random state for CSRF protection
    s.state = generateRandomState()

    mux := http.NewServeMux()
    mux.HandleFunc(CallbackPath, s.handleCallback)

    s.server = &http.Server{
        Addr:    fmt.Sprintf("127.0.0.1:%d", CallbackPort),
        Handler: mux,
    }

    go s.server.ListenAndServe()

    return fmt.Sprintf("http://127.0.0.1:%d%s", CallbackPort, CallbackPath), nil
}

func (s *LocalhostAuthServer) handleCallback(w http.ResponseWriter, r *http.Request) {
    // Verify state matches
    if r.URL.Query().Get("state") != s.state {
        s.errChan <- fmt.Errorf("state mismatch")
        http.Error(w, "State mismatch", http.StatusBadRequest)
        return
    }

    code := r.URL.Query().Get("code")
    if code == "" {
        errMsg := r.URL.Query().Get("error_description")
        s.errChan <- fmt.Errorf("auth failed: %s", errMsg)
        http.Error(w, "Authentication failed", http.StatusBadRequest)
        return
    }

    // Send success page to browser
    w.Header().Set("Content-Type", "text/html")
    w.Write([]byte(authSuccessHTML))

    s.codeChan <- code
}

func (s *LocalhostAuthServer) WaitForCode(ctx context.Context) (string, error) {
    select {
    case code := <-s.codeChan:
        return code, nil
    case err := <-s.errChan:
        return "", err
    case <-ctx.Done():
        return "", ctx.Err()
    case <-time.After(5 * time.Minute):
        return "", fmt.Errorf("authentication timeout")
    }
}
```

#### CLI Auth Command

```go
// cmd/hub_auth.go

var hubAuthCmd = &cobra.Command{
    Use:   "auth",
    Short: "Manage Hub authentication",
}

var hubAuthLoginCmd = &cobra.Command{
    Use:   "login",
    Short: "Authenticate with Hub server",
    RunE: func(cmd *cobra.Command, args []string) error {
        hubURL, _ := cmd.Flags().GetString("hub-url")
        if hubURL == "" {
            hubURL = config.DefaultHubURL()
        }

        // Start localhost callback server
        authServer := auth.NewLocalhostAuthServer()
        callbackURL, err := authServer.Start(cmd.Context())
        if err != nil {
            return fmt.Errorf("failed to start auth server: %w", err)
        }
        defer authServer.Shutdown()

        // Get OAuth URL from Hub
        client := hub.NewClient(hubURL)
        authURL, err := client.GetAuthURL(cmd.Context(), callbackURL)
        if err != nil {
            return fmt.Errorf("failed to get auth URL: %w", err)
        }

        // Open browser
        fmt.Println("Opening browser for authentication...")
        if err := openBrowser(authURL); err != nil {
            fmt.Printf("Please open this URL in your browser:\n%s\n", authURL)
        }

        // Wait for callback
        fmt.Println("Waiting for authentication...")
        code, err := authServer.WaitForCode(cmd.Context())
        if err != nil {
            return fmt.Errorf("authentication failed: %w", err)
        }

        // Exchange code for token
        token, err := client.ExchangeCode(cmd.Context(), code, callbackURL)
        if err != nil {
            return fmt.Errorf("failed to get token: %w", err)
        }

        // Store credentials
        if err := credentials.Store(hubURL, token); err != nil {
            return fmt.Errorf("failed to store credentials: %w", err)
        }

        fmt.Println("Authentication successful!")
        return nil
    },
}

var hubAuthStatusCmd = &cobra.Command{
    Use:   "status",
    Short: "Show authentication status",
    RunE: func(cmd *cobra.Command, args []string) error {
        hubURL := config.DefaultHubURL()

        creds, err := credentials.Load(hubURL)
        if err != nil {
            fmt.Println("Not authenticated")
            return nil
        }

        // Verify token is still valid
        client := hub.NewClient(hubURL)
        client.SetToken(creds.AccessToken)

        user, err := client.GetCurrentUser(cmd.Context())
        if err != nil {
            fmt.Println("Authentication expired. Run 'scion hub auth login' to re-authenticate.")
            return nil
        }

        fmt.Printf("Authenticated as: %s (%s)\n", user.DisplayName, user.Email)
        fmt.Printf("Hub: %s\n", hubURL)
        return nil
    },
}

var hubAuthLogoutCmd = &cobra.Command{
    Use:   "logout",
    Short: "Clear stored credentials",
    RunE: func(cmd *cobra.Command, args []string) error {
        hubURL := config.DefaultHubURL()

        if err := credentials.Remove(hubURL); err != nil {
            return fmt.Errorf("failed to remove credentials: %w", err)
        }

        fmt.Println("Logged out successfully.")
        return nil
    },
}
```

### 4.4 Credential Storage

CLI credentials are stored in `~/.scion/credentials.json`:

```json
{
  "version": 1,
  "hubs": {
    "https://hub.example.com": {
      "accessToken": "eyJ...",
      "refreshToken": "eyJ...",
      "expiresAt": "2025-02-01T12:00:00Z",
      "user": {
        "id": "user-uuid",
        "email": "user@example.com",
        "displayName": "User Name"
      }
    }
  }
}
```

**Security Considerations:**
- File permissions set to `0600` (owner read/write only)
- Tokens are not encrypted at rest (relies on filesystem permissions)
- Refresh tokens enable automatic token renewal

```go
// pkg/credentials/store.go

const (
    CredentialsFile = "credentials.json"
    FileMode        = 0600
)

type Credentials struct {
    Version int                        `json:"version"`
    Hubs    map[string]*HubCredentials `json:"hubs"`
}

type HubCredentials struct {
    AccessToken  string    `json:"accessToken"`
    RefreshToken string    `json:"refreshToken"`
    ExpiresAt    time.Time `json:"expiresAt"`
    User         *User     `json:"user"`
}

func Store(hubURL string, token *TokenResponse) error {
    path := filepath.Join(config.ScionDir(), CredentialsFile)

    creds, _ := load(path)
    if creds == nil {
        creds = &Credentials{Version: 1, Hubs: make(map[string]*HubCredentials)}
    }

    creds.Hubs[hubURL] = &HubCredentials{
        AccessToken:  token.AccessToken,
        RefreshToken: token.RefreshToken,
        ExpiresAt:    time.Now().Add(token.ExpiresIn),
        User:         token.User,
    }

    data, err := json.MarshalIndent(creds, "", "  ")
    if err != nil {
        return err
    }

    return os.WriteFile(path, data, FileMode)
}

func Load(hubURL string) (*HubCredentials, error) {
    path := filepath.Join(config.ScionDir(), CredentialsFile)
    creds, err := load(path)
    if err != nil {
        return nil, err
    }

    hubCreds, ok := creds.Hubs[hubURL]
    if !ok {
        return nil, ErrNotAuthenticated
    }

    // Check if token needs refresh
    if time.Now().After(hubCreds.ExpiresAt.Add(-5 * time.Minute)) {
        return refreshToken(hubURL, hubCreds)
    }

    return hubCreds, nil
}
```

### 4.5 Headless Authentication

For systems without a browser (CI/CD, remote servers), support API key authentication:

```bash
# Set API key via environment variable
export SCION_API_KEY="sk_live_..."

# Or via config file
scion hub auth set-key <api-key>
```

API keys are created via the web dashboard and stored in the same credentials file.

---

## 5. API Key Authentication

For programmatic access and CI/CD pipelines, users can create API keys.

### 5.1 API Key Format

```
sk_live_<base64-encoded-payload>
```

Payload structure:
```json
{
  "kid": "key-uuid",
  "uid": "user-uuid",
  "created": "2025-01-01T00:00:00Z"
}
```

### 5.2 API Key Management

```
POST /api/v1/auth/api-keys
  Request:  { name, expiresAt?, scopes? }
  Response: { key, keyId, name, createdAt }

GET /api/v1/auth/api-keys
  Response: { keys: [{ keyId, name, lastUsed, createdAt }] }

DELETE /api/v1/auth/api-keys/{keyId}
  Response: { success: true }
```

### 5.3 API Key Usage

API keys are passed via the `Authorization` header:

```
Authorization: Bearer sk_live_...
```

Or via `X-API-Key` header:

```
X-API-Key: sk_live_...
```

---

## 6. Hub API Auth Endpoints

### 6.1 OAuth Initiation (for CLI)

```
GET /api/v1/auth/authorize
  Query: redirect_uri, state
  Response: { authUrl, state }
```

### 6.2 Token Exchange

```
POST /api/v1/auth/token
  Request:  { code, redirectUri, grantType: "authorization_code" }
  Response: { accessToken, refreshToken, expiresIn, user }

POST /api/v1/auth/token
  Request:  { refreshToken, grantType: "refresh_token" }
  Response: { accessToken, refreshToken, expiresIn }
```

### 6.3 Token Validation

```
POST /api/v1/auth/validate
  Request:  { token }
  Response: { valid: true, user, expiresAt }
```

---

## 7. Security Considerations

### 7.1 Token Security

| Aspect | Web | CLI | API Key |
|--------|-----|-----|---------|
| Storage | HTTP-only cookie | Local file (0600) | Local file or env var |
| Transmission | HTTPS only | HTTPS only | HTTPS only |
| Lifetime | 24 hours (session) | 30 days (renewable) | Configurable |
| Revocation | Logout endpoint | Logout command | Dashboard |

### 7.2 PKCE for CLI

CLI authentication uses PKCE (Proof Key for Code Exchange) for additional security:

```go
type PKCEChallenge struct {
    Verifier  string  // Random 43-128 character string
    Challenge string  // SHA256(verifier), base64url encoded
    Method    string  // "S256"
}

func GeneratePKCE() *PKCEChallenge {
    verifier := generateRandomString(64)
    hash := sha256.Sum256([]byte(verifier))
    challenge := base64.RawURLEncoding.EncodeToString(hash[:])

    return &PKCEChallenge{
        Verifier:  verifier,
        Challenge: challenge,
        Method:    "S256",
    }
}
```

### 7.3 Rate Limiting

Authentication endpoints are rate-limited to prevent brute force attacks:

| Endpoint | Limit | Window |
|----------|-------|--------|
| `/auth/login` | 10 | 1 minute |
| `/auth/token` | 20 | 1 minute |
| `/auth/authorize` | 10 | 1 minute |

### 7.4 Audit Logging

All authentication events are logged:

```go
type AuthEvent struct {
    EventType   string    `json:"eventType"`   // login, logout, token_refresh, api_key_created
    UserID      string    `json:"userId"`
    ClientType  string    `json:"clientType"`  // web, cli, api
    IPAddress   string    `json:"ipAddress"`
    UserAgent   string    `json:"userAgent"`
    Success     bool      `json:"success"`
    FailReason  string    `json:"failReason,omitempty"`
    Timestamp   time.Time `json:"timestamp"`
}
```

---

## 8. Future Work: Runtime Host Authentication

> **TODO:** Runtime host authentication will be addressed in a separate design document.

Runtime hosts (Docker, Apple Virtualization, Kubernetes) require a different trust model:

- **Host Registration** - How hosts register with the Hub
- **Host Identity** - Certificates or tokens for host identification
- **Mutual TLS** - Secure communication between Hub and hosts
- **Host Capabilities** - What operations hosts can perform

This is distinct from user authentication and will be designed separately to address the unique security requirements of distributed compute resources.

---

## 9. Implementation Phases

### Phase 1: Web OAuth
- [x] OAuth provider integration (Google, GitHub)
- [x] Session cookie management
- [x] User creation/lookup on login
- [ ] Hub auth endpoints (`/api/v1/auth/*`)

### Phase 2: CLI Authentication
- [ ] `scion hub auth login` command
- [ ] Localhost callback server
- [ ] PKCE implementation
- [ ] Credential storage (`~/.scion/credentials.json`)
- [ ] `scion hub auth status` command
- [ ] `scion hub auth logout` command

### Phase 3: API Keys
- [ ] API key generation endpoint
- [ ] API key validation middleware
- [ ] Key management UI in dashboard
- [ ] `scion hub auth set-key` command

### Phase 4: Security Hardening
- [ ] Rate limiting on auth endpoints
- [ ] Audit logging
- [ ] Token revocation lists
- [ ] Session invalidation on password change

---

## 10. References

- **Permissions System:** `permissions-design.md`
- **Web Frontend:** `web-frontend-design.md`
- **Hub API:** `hub-api.md`
- **OAuth 2.0 RFC:** https://datatracker.ietf.org/doc/html/rfc6749
- **PKCE RFC:** https://datatracker.ietf.org/doc/html/rfc7636
