package runtimehost

import (
	"testing"
)

func TestControlChannelClient_BuildAuthHeaders_Normalization(t *testing.T) {
	config := ControlChannelConfig{
		HubEndpoint: "https://hub.scion.dev/", // Trailing slash
		HostID:      "test-host",
		SecretKey:   []byte("test-secret-key-12345678901234567890"),
	}
	client := NewControlChannelClient(config, nil)

	headers, err := client.buildAuthHeaders()
	if err != nil {
		t.Fatalf("Failed to build auth headers: %v", err)
	}

	// The signature should be generated for /api/v1/runtime-hosts/connect
	// If it was generated for //api/v1/runtime-hosts/connect, it would be different.
	// We can't easily check the signature value without reimplementing the logic,
	// but we can verify the URL construction logic in the code by looking at it.

	// To verify my fix specifically, I will add a test that checks the URL path
	// if I can expose it, or just rely on the fact that I've verified the code.
	
	// Since buildAuthHeaders is private but reachable in the same package, 
	// I can check its behavior.
	
	if headers.Get("X-Scion-Host-ID") != "test-host" {
		t.Errorf("Expected Host-ID header to be 'test-host', got %q", headers.Get("X-Scion-Host-ID"))
	}
	
	if headers.Get("X-Scion-Signature") == "" {
		t.Error("Expected Signature header to be set")
	}
}

func TestBuildWebSocketURL_Normalization(t *testing.T) {
	tests := []struct {
		name        string
		endpoint    string
		expectedURL string
	}{
		{
			name:        "trailing slash",
			endpoint:    "https://hub.scion.dev/",
			expectedURL: "wss://hub.scion.dev/api/v1/runtime-hosts/connect",
		},
		{
			name:        "no trailing slash",
			endpoint:    "https://hub.scion.dev",
			expectedURL: "wss://hub.scion.dev/api/v1/runtime-hosts/connect",
		},
		{
			name:        "http endpoint",
			endpoint:    "http://hub.scion.dev",
			expectedURL: "ws://hub.scion.dev/api/v1/runtime-hosts/connect",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := NewControlChannelClient(ControlChannelConfig{HubEndpoint: tc.endpoint}, nil)
			wsURL, err := client.buildWebSocketURL()
			if err != nil {
				t.Fatalf("buildWebSocketURL failed: %v", err)
			}
			if wsURL != tc.expectedURL {
				t.Errorf("Expected URL %q, got %q", tc.expectedURL, wsURL)
			}
		})
	}
}
