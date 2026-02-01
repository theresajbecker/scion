---
title: Web Dashboard Configuration
---

This document describes the configuration for the Scion Web Dashboard frontend and its Backend-for-Frontend (BFF).

## Purpose
The Web Dashboard is configured primarily through environment variables that control its connectivity to the Scion Hub, authentication flow, and session persistence.

## Environment Variables
- **HUB_API_URL**: The endpoint of the Scion Hub API.
- **PORT / HOST**: The listening address for the web server.
- **Authentication**:
  - `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`: Credentials for Google OAuth.
  - `GITHUB_CLIENT_ID`, `GITHUB_CLIENT_SECRET`: Credentials for GitHub OAuth.
  - `SCION_AUTHORIZED_DOMAINS`: A comma-separated list of email domains allowed to sign in.
- **Development Auth**:
  - `SCION_DEV_AUTH_ENABLED`: Enables local development auto-login.
  - `SCION_DEV_TOKEN`: Explicit development token for API proxying.
- **Session Management**:
  - `SESSION_SECRET`: Secret key for signing session cookies.
  - `SESSION_MAX_AGE`: Duration of user sessions.

## Deployment
The Web Dashboard is designed to be deployed as a containerized service (e.g., on Cloud Run or Kubernetes) with these environment variables provided at runtime.
