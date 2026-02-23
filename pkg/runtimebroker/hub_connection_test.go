// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package runtimebroker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ptone/scion-agent/pkg/api"
	"github.com/ptone/scion-agent/pkg/apiclient"
	"github.com/ptone/scion-agent/pkg/brokercredentials"
	"github.com/ptone/scion-agent/pkg/runtime"
)

// makeTestCreds creates BrokerCredentials with a base64-encoded secret key.
func makeTestCreds(name, brokerID, hubEndpoint string) *brokercredentials.BrokerCredentials {
	secretKey := base64.StdEncoding.EncodeToString([]byte("test-secret-key-32bytes!" + name + "12"))
	return &brokercredentials.BrokerCredentials{
		Name:         name,
		BrokerID:     brokerID,
		SecretKey:    secretKey,
		HubEndpoint:  hubEndpoint,
		AuthMode:     brokercredentials.AuthModeHMAC,
		RegisteredAt: time.Now(),
	}
}

// newTestServerWithInMemoryCreds creates a server with in-memory credentials for a "local" connection.
func newTestServerWithInMemoryCreds(creds *brokercredentials.BrokerCredentials) *Server {
	cfg := DefaultServerConfig()
	cfg.BrokerID = creds.BrokerID
	cfg.BrokerName = "test-host"
	cfg.HubEnabled = true
	cfg.HubEndpoint = creds.HubEndpoint
	cfg.InMemoryCredentials = creds

	mgr := &mockManager{}
	rt := &runtime.MockRuntime{}

	return New(cfg, mgr, rt)
}

func TestHubConnection_SingleConnection(t *testing.T) {
	creds := makeTestCreds("local", "broker-1", "http://localhost:8080")

	srv := newTestServerWithInMemoryCreds(creds)

	srv.hubMu.RLock()
	defer srv.hubMu.RUnlock()

	if len(srv.hubConnections) != 1 {
		t.Fatalf("expected 1 hub connection, got %d", len(srv.hubConnections))
	}

	conn, ok := srv.hubConnections["local"]
	if !ok {
		t.Fatal("expected 'local' connection to exist")
	}

	if conn.BrokerID != "broker-1" {
		t.Errorf("expected BrokerID 'broker-1', got %q", conn.BrokerID)
	}

	if conn.HubEndpoint != "http://localhost:8080" {
		t.Errorf("expected HubEndpoint 'http://localhost:8080', got %q", conn.HubEndpoint)
	}

	if conn.HubClient == nil {
		t.Error("expected HubClient to be set")
	}

	if len(conn.SecretKey) == 0 {
		t.Error("expected SecretKey to be decoded")
	}
}

func TestHubConnection_MultipleConnections(t *testing.T) {
	// Create a server with InMemory ("local") + MultiStore credentials
	tmpDir := t.TempDir()
	credDir := filepath.Join(tmpDir, "hub-credentials")
	if err := os.MkdirAll(credDir, 0700); err != nil {
		t.Fatal(err)
	}

	// Write a second credential file
	prodCreds := makeTestCreds("hub-prod", "broker-prod", "https://hub.prod.example.com")
	prodData, _ := json.MarshalIndent(prodCreds, "", "  ")
	if err := os.WriteFile(filepath.Join(credDir, "hub-prod.json"), prodData, 0600); err != nil {
		t.Fatal(err)
	}

	localCreds := makeTestCreds("local", "broker-local", "http://localhost:8080")

	cfg := DefaultServerConfig()
	cfg.BrokerID = "broker-local"
	cfg.BrokerName = "test-host"
	cfg.HubEnabled = true
	cfg.HubEndpoint = "http://localhost:8080"
	cfg.InMemoryCredentials = localCreds

	mgr := &mockManager{}
	rt := &runtime.MockRuntime{}
	srv := New(cfg, mgr, rt)

	// The multi-store was initialized but pointed to default dir.
	// Manually set up the multi-store and add the prod connection.
	srv.multiCredStore = brokercredentials.NewMultiStore(credDir)
	multiCreds, _ := srv.multiCredStore.List()
	for i := range multiCreds {
		c := &multiCreds[i]
		if _, exists := srv.hubConnections[c.Name]; exists {
			continue
		}
		conn, err := srv.createHubConnection(c.Name, c)
		if err != nil {
			t.Fatalf("Failed to create connection %q: %v", c.Name, err)
		}
		srv.hubMu.Lock()
		srv.hubConnections[c.Name] = conn
		srv.hubMu.Unlock()
	}

	srv.hubMu.RLock()
	defer srv.hubMu.RUnlock()

	if len(srv.hubConnections) != 2 {
		t.Fatalf("expected 2 hub connections, got %d", len(srv.hubConnections))
	}

	if _, ok := srv.hubConnections["local"]; !ok {
		t.Error("expected 'local' connection")
	}
	if _, ok := srv.hubConnections["hub-prod"]; !ok {
		t.Error("expected 'hub-prod' connection")
	}
}

func TestMultiKeyBrokerAuth_MatchesAnyKey(t *testing.T) {
	secret1 := []byte("secret-key-for-hub-1-32bytes!!!!")
	secret2 := []byte("secret-key-for-hub-2-32bytes!!!!")

	middleware := NewMultiKeyBrokerAuthMiddleware(true, 5*time.Minute, false)
	middleware.UpdateKeys([]secretKeyEntry{
		{hubName: "hub-1", secretKey: secret1},
		{hubName: "hub-2", secretKey: secret2},
	})

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Request signed with secret1 should pass
	req1 := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	signRequest(req1, "broker-1", secret1)
	rr1 := httptest.NewRecorder()
	middleware.Middleware(handler).ServeHTTP(rr1, req1)

	if rr1.Code != http.StatusOK {
		t.Errorf("Expected 200 for request signed with hub-1 key, got %d", rr1.Code)
	}

	// Request signed with secret2 should also pass
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	signRequest(req2, "broker-1", secret2)
	rr2 := httptest.NewRecorder()
	middleware.Middleware(handler).ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusOK {
		t.Errorf("Expected 200 for request signed with hub-2 key, got %d", rr2.Code)
	}

	// Request signed with unknown key should fail
	wrongSecret := []byte("wrong-secret-key-32bytes!!!!!!!!")
	req3 := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	signRequest(req3, "broker-1", wrongSecret)
	rr3 := httptest.NewRecorder()
	middleware.Middleware(handler).ServeHTTP(rr3, req3)

	if rr3.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401 for request signed with wrong key, got %d", rr3.Code)
	}
}

func TestMultiKeyBrokerAuth_Disabled(t *testing.T) {
	middleware := NewMultiKeyBrokerAuthMiddleware(false, 5*time.Minute, false)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	rr := httptest.NewRecorder()
	middleware.Middleware(handler).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected 200 when disabled, got %d", rr.Code)
	}
}

func TestMultiKeyBrokerAuth_AllowUnauthenticated(t *testing.T) {
	secret := []byte("test-secret-key-32bytes!12345678")
	middleware := NewMultiKeyBrokerAuthMiddleware(true, 5*time.Minute, true)
	middleware.UpdateKeys([]secretKeyEntry{
		{hubName: "hub-1", secretKey: secret},
	})

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Request without any HMAC headers should pass
	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	rr := httptest.NewRecorder()
	middleware.Middleware(handler).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected 200 for unauthenticated request, got %d", rr.Code)
	}
}

func TestMultiKeyBrokerAuth_UpdateKeys(t *testing.T) {
	oldSecret := []byte("old-secret-key-32bytes!123456789")
	newSecret := []byte("new-secret-key-32bytes!987654321")

	middleware := NewMultiKeyBrokerAuthMiddleware(true, 5*time.Minute, false)
	middleware.UpdateKeys([]secretKeyEntry{
		{hubName: "hub-1", secretKey: oldSecret},
	})

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Request with old key should work
	req1 := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	signRequest(req1, "broker-1", oldSecret)
	rr1 := httptest.NewRecorder()
	middleware.Middleware(handler).ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Errorf("Expected 200 with old key, got %d", rr1.Code)
	}

	// Update keys to new secret only
	middleware.UpdateKeys([]secretKeyEntry{
		{hubName: "hub-1", secretKey: newSecret},
	})

	// Request with old key should now fail
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	signRequest(req2, "broker-1", oldSecret)
	rr2 := httptest.NewRecorder()
	middleware.Middleware(handler).ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401 with old key after update, got %d", rr2.Code)
	}

	// Request with new key should work
	req3 := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	signRequest(req3, "broker-1", newSecret)
	rr3 := httptest.NewRecorder()
	middleware.Middleware(handler).ServeHTTP(rr3, req3)
	if rr3.Code != http.StatusOK {
		t.Errorf("Expected 200 with new key, got %d", rr3.Code)
	}
}

func TestMultiKeyBrokerAuth_ExpiredTimestamp(t *testing.T) {
	secret := []byte("test-secret-key-32bytes!12345678")
	middleware := NewMultiKeyBrokerAuthMiddleware(true, 5*time.Minute, false)
	middleware.UpdateKeys([]secretKeyEntry{
		{hubName: "hub-1", secretKey: secret},
	})

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Create a request with old timestamp
	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	oldTimestamp := time.Now().Add(-10 * time.Minute).Unix()
	ts := fmt.Sprintf("%d", oldTimestamp)
	nonce := "test-nonce"
	req.Header.Set(apiclient.HeaderBrokerID, "broker-1")
	req.Header.Set(apiclient.HeaderTimestamp, ts)
	req.Header.Set(apiclient.HeaderNonce, nonce)
	canonical := apiclient.BuildCanonicalString(req, ts, nonce)
	sig := apiclient.ComputeHMAC(secret, canonical)
	req.Header.Set(apiclient.HeaderSignature, base64.StdEncoding.EncodeToString(sig))

	rr := httptest.NewRecorder()
	middleware.Middleware(handler).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401 for expired timestamp, got %d", rr.Code)
	}
}

func TestHeartbeatService_GroveFilter(t *testing.T) {
	client := &mockRuntimeBrokerService{}
	manager := &heartbeatMockManager{
		agents: []api.AgentInfo{
			{Name: "agent-1", GroveID: "grove-hub1", Status: "running"},
			{Name: "agent-2", GroveID: "grove-hub1", Status: "running"},
			{Name: "agent-3", GroveID: "grove-hub2", Status: "running"},
			{Name: "agent-4", GroveID: "grove-shared", Status: "running"},
		},
	}

	// Filter: only include grove-hub1 groves
	groveFilter := func(groveID string) bool {
		return groveID == "grove-hub1"
	}

	svc := NewHeartbeatService(client, "test-host", time.Hour, manager, groveFilter)
	err := svc.ForceHeartbeat(context.Background())
	if err != nil {
		t.Fatalf("ForceHeartbeat failed: %v", err)
	}

	calls := client.getHeartbeatCalls()
	if len(calls) != 1 {
		t.Fatalf("Expected 1 heartbeat call, got %d", len(calls))
	}

	heartbeat := calls[0].Heartbeat

	// Should only include grove-hub1 (2 agents), not grove-hub2 or grove-shared
	if len(heartbeat.Groves) != 1 {
		t.Errorf("Expected 1 grove in heartbeat (filtered), got %d", len(heartbeat.Groves))
	}

	if len(heartbeat.Groves) > 0 && heartbeat.Groves[0].GroveID != "grove-hub1" {
		t.Errorf("Expected grove-hub1, got %q", heartbeat.Groves[0].GroveID)
	}

	if len(heartbeat.Groves) > 0 && heartbeat.Groves[0].AgentCount != 2 {
		t.Errorf("Expected 2 agents in grove-hub1, got %d", heartbeat.Groves[0].AgentCount)
	}
}

func TestHeartbeatService_NilGroveFilter(t *testing.T) {
	client := &mockRuntimeBrokerService{}
	manager := &heartbeatMockManager{
		agents: []api.AgentInfo{
			{Name: "agent-1", GroveID: "grove-1", Status: "running"},
			{Name: "agent-2", GroveID: "grove-2", Status: "running"},
		},
	}

	// Nil filter: include all groves
	svc := NewHeartbeatService(client, "test-host", time.Hour, manager, nil)
	err := svc.ForceHeartbeat(context.Background())
	if err != nil {
		t.Fatalf("ForceHeartbeat failed: %v", err)
	}

	calls := client.getHeartbeatCalls()
	if len(calls) != 1 {
		t.Fatalf("Expected 1 heartbeat call, got %d", len(calls))
	}

	heartbeat := calls[0].Heartbeat
	if len(heartbeat.Groves) != 2 {
		t.Errorf("Expected 2 groves with nil filter, got %d", len(heartbeat.Groves))
	}
}

func TestResolveHydrator_WithConnectionHeader(t *testing.T) {
	creds := makeTestCreds("local", "broker-1", "http://localhost:8080")
	srv := newTestServerWithInMemoryCreds(creds)

	// Verify the hydrator resolves via header
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents", nil)
	req.Header.Set("X-Scion-Hub-Connection", "local")

	hydrator := srv.resolveHydrator(req)
	// In test mode cache is nil, so hydrator is nil -- that's expected
	// What we're testing is the routing logic
	srv.hubMu.RLock()
	conn := srv.hubConnections["local"]
	srv.hubMu.RUnlock()

	if conn == nil {
		t.Fatal("expected 'local' connection to exist")
	}

	// The hydrator from resolveHydrator should match the connection's hydrator
	if hydrator != conn.Hydrator {
		t.Error("expected resolveHydrator to return the local connection's hydrator")
	}
}

func TestResolveHydrator_FallbackToFirstAvailable(t *testing.T) {
	creds := makeTestCreds("local", "broker-1", "http://localhost:8080")
	srv := newTestServerWithInMemoryCreds(creds)

	// Request without connection header should fall back to first available
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents", nil)
	hydrator := srv.resolveHydrator(req)

	srv.hubMu.RLock()
	conn := srv.hubConnections["local"]
	srv.hubMu.RUnlock()

	if hydrator != conn.Hydrator {
		t.Error("expected resolveHydrator to fall back to first available hydrator")
	}
}

func TestResolveHydrator_UnknownConnection(t *testing.T) {
	creds := makeTestCreds("local", "broker-1", "http://localhost:8080")
	srv := newTestServerWithInMemoryCreds(creds)

	// Request with unknown connection name should fall back
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents", nil)
	req.Header.Set("X-Scion-Hub-Connection", "nonexistent")
	hydrator := srv.resolveHydrator(req)

	// Should fall back to any available hydrator
	srv.hubMu.RLock()
	conn := srv.hubConnections["local"]
	srv.hubMu.RUnlock()

	if hydrator != conn.Hydrator {
		t.Error("expected resolveHydrator to fall back when connection not found")
	}
}

func TestGlobalGroveRejection_MultiHub(t *testing.T) {
	// Create a server with two connections to simulate multi-hub mode
	creds := makeTestCreds("local", "broker-1", "http://localhost:8080")
	srv := newTestServerWithInMemoryCreds(creds)

	// Add a second connection to enable multi-hub mode
	creds2 := makeTestCreds("hub-prod", "broker-2", "https://hub.prod.example.com")
	conn2, err := srv.createHubConnection("hub-prod", creds2)
	if err != nil {
		t.Fatal(err)
	}
	srv.hubMu.Lock()
	srv.hubConnections["hub-prod"] = conn2
	srv.hubMu.Unlock()

	if !srv.isMultiHubMode() {
		t.Fatal("expected multi-hub mode with 2 connections")
	}

	// Try to create an agent with empty groveID (global grove)
	body := `{"name": "global-agent", "config": {"template": "claude"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected status %d for global grove in multi-hub mode, got %d: %s",
			http.StatusConflict, w.Code, w.Body.String())
	}

	// Verify error code
	var errResp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}

	errObj, ok := errResp["error"].(map[string]interface{})
	if !ok {
		t.Fatal("expected error object in response")
	}
	if errObj["code"] != "global_grove_disabled" {
		t.Errorf("expected error code 'global_grove_disabled', got %q", errObj["code"])
	}
}

func TestGlobalGroveRejection_SingleHub_Allowed(t *testing.T) {
	// Single-hub mode: global grove should be allowed
	creds := makeTestCreds("local", "broker-1", "http://localhost:8080")
	srv := newTestServerWithInMemoryCreds(creds)

	if srv.isMultiHubMode() {
		t.Fatal("expected single-hub mode with 1 connection")
	}

	body := `{"name": "global-agent", "config": {"template": "claude"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.Handler().ServeHTTP(w, req)

	// In single-hub mode, empty groveID should NOT be rejected
	if w.Code == http.StatusConflict {
		t.Error("single-hub mode should not reject global grove agents")
	}
}

func TestGlobalGroveRejection_WithGroveID_MultiHub(t *testing.T) {
	// Multi-hub mode: agents with a specific groveID should be allowed
	creds := makeTestCreds("local", "broker-1", "http://localhost:8080")
	srv := newTestServerWithInMemoryCreds(creds)

	creds2 := makeTestCreds("hub-prod", "broker-2", "https://hub.prod.example.com")
	conn2, _ := srv.createHubConnection("hub-prod", creds2)
	srv.hubMu.Lock()
	srv.hubConnections["hub-prod"] = conn2
	srv.hubMu.Unlock()

	body := `{
		"name": "scoped-agent",
		"groveId": "my-project",
		"grovePath": "/some/path/.scion",
		"config": {"template": "claude"}
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.Handler().ServeHTTP(w, req)

	// Should NOT be rejected (has explicit groveID and grovePath)
	if w.Code == http.StatusConflict {
		t.Errorf("expected non-global grove to be allowed in multi-hub mode, got %d: %s",
			w.Code, w.Body.String())
	}
}

func TestIsMultiHubMode(t *testing.T) {
	srv := newTestServer()

	// No connections = not multi-hub
	if srv.isMultiHubMode() {
		t.Error("expected single-hub mode with no connections")
	}

	// Add one connection
	srv.hubMu.Lock()
	srv.hubConnections["hub-1"] = &HubConnection{Name: "hub-1"}
	srv.hubMu.Unlock()

	if srv.isMultiHubMode() {
		t.Error("expected single-hub mode with 1 connection")
	}

	// Add second connection
	srv.hubMu.Lock()
	srv.hubConnections["hub-2"] = &HubConnection{Name: "hub-2"}
	srv.hubMu.Unlock()

	if !srv.isMultiHubMode() {
		t.Error("expected multi-hub mode with 2 connections")
	}
}

func TestIsGlobalGrove(t *testing.T) {
	srv := newTestServer()

	tests := []struct {
		name      string
		groveID   string
		grovePath string
		expected  bool
	}{
		{"empty both", "", "", true},
		{"global id", "global", "/some/path", true},
		{"empty id", "", "/some/path", true},
		{"empty path", "my-project", "", true},
		{"both set", "my-project", "/some/path", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := srv.isGlobalGrove(tc.groveID, tc.grovePath)
			if result != tc.expected {
				t.Errorf("isGlobalGrove(%q, %q) = %v, expected %v",
					tc.groveID, tc.grovePath, result, tc.expected)
			}
		})
	}
}

func TestConnectionStatus(t *testing.T) {
	conn := &HubConnection{
		Name:   "test",
		Status: ConnectionStatusDisconnected,
	}

	if conn.GetStatus() != ConnectionStatusDisconnected {
		t.Errorf("expected disconnected, got %v", conn.GetStatus())
	}

	conn.setStatus(ConnectionStatusConnected)

	if conn.GetStatus() != ConnectionStatusConnected {
		t.Errorf("expected connected, got %v", conn.GetStatus())
	}

	conn.setStatus(ConnectionStatusError)

	if conn.GetStatus() != ConnectionStatusError {
		t.Errorf("expected error, got %v", conn.GetStatus())
	}
}

func TestCredentialWatcher_AddConnection(t *testing.T) {
	tmpDir := t.TempDir()
	credDir := filepath.Join(tmpDir, "hub-credentials")
	if err := os.MkdirAll(credDir, 0700); err != nil {
		t.Fatal(err)
	}

	// Start with one connection file
	creds1 := makeTestCreds("hub-one", "broker-1", "http://hub1.example.com")
	data1, _ := json.MarshalIndent(creds1, "", "  ")
	if err := os.WriteFile(filepath.Join(credDir, "hub-one.json"), data1, 0600); err != nil {
		t.Fatal(err)
	}

	// Create server without hub integration to avoid auto-creating connections
	cfg := DefaultServerConfig()
	cfg.BrokerID = "broker-1"
	cfg.BrokerName = "test-host"

	mgr := &mockManager{}
	rt := &runtime.MockRuntime{}
	srv := New(cfg, mgr, rt)

	// Manually set up multi-store and load initial connections
	srv.multiCredStore = brokercredentials.NewMultiStore(credDir)
	multiCreds, _ := srv.multiCredStore.List()
	for i := range multiCreds {
		c := &multiCreds[i]
		conn, err := srv.createHubConnection(c.Name, c)
		if err != nil {
			t.Fatal(err)
		}
		srv.hubMu.Lock()
		srv.hubConnections[c.Name] = conn
		srv.hubMu.Unlock()
	}

	srv.hubMu.RLock()
	initialCount := len(srv.hubConnections)
	srv.hubMu.RUnlock()

	if initialCount != 1 {
		t.Fatalf("expected 1 initial connection, got %d", initialCount)
	}

	// Add a new credential file
	creds2 := makeTestCreds("hub-two", "broker-2", "http://hub2.example.com")
	data2, _ := json.MarshalIndent(creds2, "", "  ")
	if err := os.WriteFile(filepath.Join(credDir, "hub-two.json"), data2, 0600); err != nil {
		t.Fatal(err)
	}

	// Trigger credential reload
	ctx := context.Background()
	if err := srv.checkAndReloadCredentials(ctx); err != nil {
		t.Fatalf("checkAndReloadCredentials failed: %v", err)
	}

	srv.hubMu.RLock()
	newCount := len(srv.hubConnections)
	_, exists := srv.hubConnections["hub-two"]
	srv.hubMu.RUnlock()

	if newCount != 2 {
		t.Errorf("expected 2 connections after add, got %d", newCount)
	}

	if !exists {
		t.Error("expected 'hub-two' connection to exist after reload")
	}
}

func TestCredentialWatcher_RemoveConnection(t *testing.T) {
	tmpDir := t.TempDir()
	credDir := filepath.Join(tmpDir, "hub-credentials")
	if err := os.MkdirAll(credDir, 0700); err != nil {
		t.Fatal(err)
	}

	// Start with two connections
	creds1 := makeTestCreds("hub-one", "broker-1", "http://hub1.example.com")
	data1, _ := json.MarshalIndent(creds1, "", "  ")
	if err := os.WriteFile(filepath.Join(credDir, "hub-one.json"), data1, 0600); err != nil {
		t.Fatal(err)
	}

	creds2 := makeTestCreds("hub-two", "broker-2", "http://hub2.example.com")
	data2, _ := json.MarshalIndent(creds2, "", "  ")
	if err := os.WriteFile(filepath.Join(credDir, "hub-two.json"), data2, 0600); err != nil {
		t.Fatal(err)
	}

	// Create server without hub integration
	cfg := DefaultServerConfig()
	cfg.BrokerID = "broker-1"

	mgr := &mockManager{}
	rt := &runtime.MockRuntime{}
	srv := New(cfg, mgr, rt)

	srv.multiCredStore = brokercredentials.NewMultiStore(credDir)
	multiCreds, _ := srv.multiCredStore.List()
	for i := range multiCreds {
		c := &multiCreds[i]
		conn, err := srv.createHubConnection(c.Name, c)
		if err != nil {
			t.Fatal(err)
		}
		srv.hubMu.Lock()
		srv.hubConnections[c.Name] = conn
		srv.hubMu.Unlock()
	}

	srv.hubMu.RLock()
	if len(srv.hubConnections) != 2 {
		t.Fatalf("expected 2 initial connections, got %d", len(srv.hubConnections))
	}
	srv.hubMu.RUnlock()

	// Remove hub-two credential file
	if err := os.Remove(filepath.Join(credDir, "hub-two.json")); err != nil {
		t.Fatal(err)
	}

	// Trigger credential reload
	ctx := context.Background()
	if err := srv.checkAndReloadCredentials(ctx); err != nil {
		t.Fatalf("checkAndReloadCredentials failed: %v", err)
	}

	srv.hubMu.RLock()
	_, exists := srv.hubConnections["hub-two"]
	count := len(srv.hubConnections)
	srv.hubMu.RUnlock()

	if exists {
		t.Error("expected 'hub-two' connection to be removed after credential deletion")
	}

	if count != 1 {
		t.Errorf("expected 1 connection after removal, got %d", count)
	}
}

func TestBuildAuthMiddleware_NoKeys(t *testing.T) {
	cfg := DefaultServerConfig()
	cfg.BrokerAuthEnabled = true
	srv := New(cfg, &mockManager{}, &runtime.MockRuntime{})

	// With no connections and no keys, middleware should be nil
	if srv.brokerAuthMiddleware != nil {
		t.Error("expected nil middleware when no keys available")
	}
}

func TestBuildAuthMiddleware_WithKeys(t *testing.T) {
	creds := makeTestCreds("local", "broker-1", "http://localhost:8080")

	cfg := DefaultServerConfig()
	cfg.BrokerID = "broker-1"
	cfg.BrokerName = "test-host"
	cfg.HubEnabled = true
	cfg.HubEndpoint = "http://localhost:8080"
	cfg.InMemoryCredentials = creds
	cfg.BrokerAuthEnabled = true

	srv := New(cfg, &mockManager{}, &runtime.MockRuntime{})

	if srv.brokerAuthMiddleware == nil {
		t.Error("expected middleware to be created when keys available")
	}
}

func TestGetFirstHeartbeat_NoConnections(t *testing.T) {
	srv := newTestServer()

	hb := srv.getFirstHeartbeat()
	if hb != nil {
		t.Error("expected nil heartbeat when no connections")
	}
}

func TestHubConnection_Stop(t *testing.T) {
	conn := &HubConnection{
		Name:   "test",
		Status: ConnectionStatusConnected,
	}

	conn.Stop()

	if conn.GetStatus() != ConnectionStatusDisconnected {
		t.Errorf("expected disconnected after Stop, got %v", conn.GetStatus())
	}
}

func TestControlChannel_ConnectionNameHeader(t *testing.T) {
	// Verify that NewControlChannelClient stores the connectionName
	config := ControlChannelConfig{
		HubEndpoint: "https://hub.example.com",
		BrokerID:    "test-broker",
		SecretKey:   []byte("test-secret-key-12345678901234567890"),
	}

	cc := NewControlChannelClient(config, nil, nil, "hub-prod")

	if cc.connectionName != "hub-prod" {
		t.Errorf("expected connectionName 'hub-prod', got %q", cc.connectionName)
	}
}

func TestControlChannel_EmptyConnectionName(t *testing.T) {
	config := ControlChannelConfig{
		HubEndpoint: "https://hub.example.com",
		BrokerID:    "test-broker",
		SecretKey:   []byte("test-secret-key-12345678901234567890"),
	}

	cc := NewControlChannelClient(config, nil, nil, "")

	if cc.connectionName != "" {
		t.Errorf("expected empty connectionName, got %q", cc.connectionName)
	}
}
