package hub

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient_FromEnvironment(t *testing.T) {
	// Save and restore env vars
	origURL := os.Getenv(EnvHubURL)
	origToken := os.Getenv(EnvHubToken)
	origAgentID := os.Getenv(EnvAgentID)
	defer func() {
		os.Setenv(EnvHubURL, origURL)
		os.Setenv(EnvHubToken, origToken)
		os.Setenv(EnvAgentID, origAgentID)
	}()

	t.Run("missing env vars returns nil", func(t *testing.T) {
		os.Unsetenv(EnvHubURL)
		os.Unsetenv(EnvHubToken)
		os.Unsetenv(EnvAgentID)

		client := NewClient()
		assert.Nil(t, client)
	})

	t.Run("missing token returns nil", func(t *testing.T) {
		os.Setenv(EnvHubURL, "http://hub.example.com")
		os.Unsetenv(EnvHubToken)
		os.Unsetenv(EnvAgentID)

		client := NewClient()
		assert.Nil(t, client)
	})

	t.Run("with all env vars returns client", func(t *testing.T) {
		os.Setenv(EnvHubURL, "http://hub.example.com")
		os.Setenv(EnvHubToken, "test-token")
		os.Setenv(EnvAgentID, "agent-123")

		client := NewClient()
		require.NotNil(t, client)
		assert.True(t, client.IsConfigured())
	})
}

func TestNewClientWithConfig(t *testing.T) {
	client := NewClientWithConfig("http://hub.example.com", "test-token", "agent-123")

	require.NotNil(t, client)
	assert.True(t, client.IsConfigured())
}

func TestClient_UpdateStatus(t *testing.T) {
	// Create a test server
	var receivedStatus StatusUpdate
	var receivedToken string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check request
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v1/agents/agent-123/status", r.URL.Path)
		receivedToken = r.Header.Get("X-Scion-Agent-Token")

		// Parse body
		err := json.NewDecoder(r.Body).Decode(&receivedStatus)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClientWithConfig(server.URL, "test-token", "agent-123")

	err := client.UpdateStatus(context.Background(), StatusUpdate{
		Status:  StatusRunning,
		Message: "test message",
	})

	require.NoError(t, err)
	assert.Equal(t, "test-token", receivedToken)
	assert.Equal(t, StatusRunning, receivedStatus.Status)
	assert.Equal(t, "test message", receivedStatus.Message)
}

func TestClient_UpdateStatus_Errors(t *testing.T) {
	t.Run("not configured", func(t *testing.T) {
		client := &Client{}
		err := client.UpdateStatus(context.Background(), StatusUpdate{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not configured")
	})

	t.Run("no agent ID", func(t *testing.T) {
		client := NewClientWithConfig("http://hub.example.com", "test-token", "")
		err := client.UpdateStatus(context.Background(), StatusUpdate{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "agent ID not set")
	})

	t.Run("server error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("internal error"))
		}))
		defer server.Close()

		client := NewClientWithConfig(server.URL, "test-token", "agent-123")
		err := client.UpdateStatus(context.Background(), StatusUpdate{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "500")
	})
}

func TestClient_ConvenienceMethods(t *testing.T) {
	var lastStatus StatusUpdate

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&lastStatus)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClientWithConfig(server.URL, "test-token", "agent-123")
	ctx := context.Background()

	t.Run("Heartbeat", func(t *testing.T) {
		err := client.Heartbeat(ctx)
		require.NoError(t, err)
		assert.Equal(t, StatusRunning, lastStatus.Status)
		assert.True(t, lastStatus.Heartbeat)
	})

	t.Run("ReportRunning", func(t *testing.T) {
		err := client.ReportRunning(ctx, "running now")
		require.NoError(t, err)
		assert.Equal(t, StatusRunning, lastStatus.Status)
		assert.Equal(t, "running now", lastStatus.Message)
	})

	t.Run("ReportBusy", func(t *testing.T) {
		err := client.ReportBusy(ctx, "processing")
		require.NoError(t, err)
		assert.Equal(t, StatusBusy, lastStatus.Status)
	})

	t.Run("ReportIdle", func(t *testing.T) {
		err := client.ReportIdle(ctx, "waiting")
		require.NoError(t, err)
		assert.Equal(t, StatusIdle, lastStatus.Status)
	})

	t.Run("ReportError", func(t *testing.T) {
		err := client.ReportError(ctx, "something went wrong")
		require.NoError(t, err)
		assert.Equal(t, StatusError, lastStatus.Status)
	})

	t.Run("ReportShuttingDown", func(t *testing.T) {
		err := client.ReportShuttingDown(ctx, "shutting down")
		require.NoError(t, err)
		assert.Equal(t, StatusShuttingDown, lastStatus.Status)
	})

	t.Run("ReportTaskCompleted", func(t *testing.T) {
		err := client.ReportTaskCompleted(ctx, "implemented feature")
		require.NoError(t, err)
		assert.Equal(t, StatusIdle, lastStatus.Status)
		assert.Equal(t, "implemented feature", lastStatus.TaskSummary)
	})
}

func TestIsHostedMode(t *testing.T) {
	origMode := os.Getenv(EnvAgentMode)
	defer os.Setenv(EnvAgentMode, origMode)

	t.Run("not hosted mode", func(t *testing.T) {
		os.Unsetenv(EnvAgentMode)
		assert.False(t, IsHostedMode())

		os.Setenv(EnvAgentMode, "solo")
		assert.False(t, IsHostedMode())
	})

	t.Run("hosted mode", func(t *testing.T) {
		os.Setenv(EnvAgentMode, "hosted")
		assert.True(t, IsHostedMode())
	})
}

func TestGetAgentID(t *testing.T) {
	origID := os.Getenv(EnvAgentID)
	defer os.Setenv(EnvAgentID, origID)

	os.Setenv(EnvAgentID, "test-agent-id")
	assert.Equal(t, "test-agent-id", GetAgentID())

	os.Unsetenv(EnvAgentID)
	assert.Equal(t, "", GetAgentID())
}
