# Local OAuth Setup Walkthrough

This guide explains how to set up and test real OAuth authentication (Google or GitHub) in your local development environment.

## 1. OAuth Provider Setup

First, you need to create credentials in your provider's developer console.

### Google OAuth Setup
1. Go to the [Google Cloud Console](https://console.cloud.google.com/).
2. Create or select a project.
3. Navigate to **APIs & Services > Credentials**.
4. Click **Create Credentials > OAuth client ID**.
5. Select **Web application** as the application type.
6. Add the following **Authorized redirect URIs**:
   - `http://localhost:8080/auth/callback/google`
7. Note your **Client ID** and **Client Secret**.

### GitHub OAuth Setup
1. Go to your GitHub **Settings > Developer settings > OAuth Apps**.
2. Click **New OAuth App**.
3. Set **Homepage URL** to `http://localhost:8080`.
4. Set **Authorization callback URL** to `http://localhost:8080/auth/callback/github`.
5. Register the application and note your **Client ID** and **Client Secret**.

## 2. Configuration

You can configure the web server using environment variables. The easiest way is to create a shell script or use an `.env` file (if supported by your environment).

### Disable Dev Auth
By default, the web server enables "Dev Auth" which auto-logs you in as a development user. To test real OAuth, you should disable it:

```bash
export SCION_DEV_AUTH_ENABLED=false
```

### Set Provider Credentials
Set the credentials for the provider(s) you want to test:

```bash
# Google
export GOOGLE_CLIENT_ID="your-client-id"
export GOOGLE_CLIENT_SECRET="your-client-secret"

# GitHub
export GITHUB_CLIENT_ID="your-client-id"
export GITHUB_CLIENT_SECRET="your-client-secret"
```

### Optional: Authorized Domains
If you want to restrict login to specific email domains (e.g., your company domain):

```bash
export SCION_AUTHORIZED_DOMAINS="example.com,mycompany.org"
```

## 3. Running the Server

1. **Install dependencies** (if you haven't already):
   ```bash
   cd web
   npm install
   ```

2. **Build and start the server** with your environment variables:
   ```bash
   # Run with variables
   SCION_DEV_AUTH_ENABLED=false \
   GOOGLE_CLIENT_ID="xxx" \
   GOOGLE_CLIENT_SECRET="yyy" \
   npm run build && npm start
   ```

## 4. Testing the Flow

1. Open your browser to `http://localhost:8080`.
2. Since you're not authenticated, you should be redirected to the login page (`/login`).
3. Click the button for your chosen provider (e.g., "Sign in with Google").
4. Complete the OAuth flow in the provider's pop-up/redirect.
5. If successful, you will be redirected back to the Scion dashboard.

### Troubleshooting

- **Redirect URI Mismatch**: Ensure the redirect URI in your provider's console matches exactly: `http://localhost:8080/auth/callback/<provider>`.
- **Port Conflict**: If you change the `PORT` environment variable, update your redirect URIs accordingly.
- **Session Secret**: For a consistent experience across restarts, you can set a `SESSION_SECRET`:
  ```bash
  export SESSION_SECRET="a-long-random-string"
  ```
- **Authorized Domains**: If you set `SCION_AUTHORIZED_DOMAINS` and your login email doesn't match, you'll see an error message.
