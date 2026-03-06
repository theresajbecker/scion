/*
Copyright 2025 The Scion Authors.
*/

package telemetry

import (
	"os"
	"testing"
)

func TestLoadConfig_Defaults(t *testing.T) {
	// Clear all env vars
	clearTelemetryEnv()

	cfg := LoadConfig()

	if !cfg.Enabled {
		t.Error("Expected Enabled to be true by default")
	}
	if !cfg.CloudEnabled {
		t.Error("Expected CloudEnabled to be true by default")
	}
	if cfg.Protocol != DefaultProtocol {
		t.Errorf("Expected Protocol to be %q, got %q", DefaultProtocol, cfg.Protocol)
	}
	if cfg.GRPCPort != DefaultGRPCPort {
		t.Errorf("Expected GRPCPort to be %d, got %d", DefaultGRPCPort, cfg.GRPCPort)
	}
	if cfg.HTTPPort != DefaultHTTPPort {
		t.Errorf("Expected HTTPPort to be %d, got %d", DefaultHTTPPort, cfg.HTTPPort)
	}
	if cfg.Insecure {
		t.Error("Expected Insecure to be false by default")
	}
	// Default exclude list should be applied
	if len(cfg.Filter.Exclude) != 1 || cfg.Filter.Exclude[0] != "agent.user.prompt" {
		t.Errorf("Expected default exclude list, got %v", cfg.Filter.Exclude)
	}
}

func TestLoadConfig_EnvOverrides(t *testing.T) {
	clearTelemetryEnv()

	os.Setenv(EnvEnabled, "false")
	os.Setenv(EnvCloudEnabled, "false")
	os.Setenv(EnvEndpoint, "otel.example.com:443")
	os.Setenv(EnvProtocol, "http")
	os.Setenv(EnvInsecure, "true")
	os.Setenv(EnvGRPCPort, "14317")
	os.Setenv(EnvHTTPPort, "14318")
	os.Setenv(EnvProjectID, "my-project")
	os.Setenv(EnvFilterExclude, "event.type.a,event.type.b")
	os.Setenv(EnvFilterInclude, "event.type.c")
	defer clearTelemetryEnv()

	cfg := LoadConfig()

	if cfg.Enabled {
		t.Error("Expected Enabled to be false")
	}
	if cfg.CloudEnabled {
		t.Error("Expected CloudEnabled to be false")
	}
	if cfg.Endpoint != "otel.example.com:443" {
		t.Errorf("Expected Endpoint to be 'otel.example.com:443', got %q", cfg.Endpoint)
	}
	if cfg.Protocol != "http" {
		t.Errorf("Expected Protocol to be 'http', got %q", cfg.Protocol)
	}
	if !cfg.Insecure {
		t.Error("Expected Insecure to be true")
	}
	if cfg.GRPCPort != 14317 {
		t.Errorf("Expected GRPCPort to be 14317, got %d", cfg.GRPCPort)
	}
	if cfg.HTTPPort != 14318 {
		t.Errorf("Expected HTTPPort to be 14318, got %d", cfg.HTTPPort)
	}
	if cfg.ProjectID != "my-project" {
		t.Errorf("Expected ProjectID to be 'my-project', got %q", cfg.ProjectID)
	}
	if len(cfg.Filter.Exclude) != 2 {
		t.Errorf("Expected 2 exclude patterns, got %d", len(cfg.Filter.Exclude))
	}
	if len(cfg.Filter.Include) != 1 {
		t.Errorf("Expected 1 include pattern, got %d", len(cfg.Filter.Include))
	}
}

func TestIsCloudConfigured(t *testing.T) {
	tests := []struct {
		name     string
		config   *Config
		expected bool
	}{
		{
			name:     "nil config",
			config:   nil,
			expected: false,
		},
		{
			name: "cloud disabled",
			config: &Config{
				CloudEnabled: false,
				Endpoint:     "otel.example.com",
			},
			expected: false,
		},
		{
			name: "no endpoint",
			config: &Config{
				CloudEnabled: true,
				Endpoint:     "",
			},
			expected: false,
		},
		{
			name: "properly configured",
			config: &Config{
				CloudEnabled: true,
				Endpoint:     "otel.example.com:443",
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.config.IsCloudConfigured(); got != tt.expected {
				t.Errorf("IsCloudConfigured() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestParseBoolEnv(t *testing.T) {
	tests := []struct {
		value      string
		defaultVal bool
		expected   bool
	}{
		{"", true, true},
		{"", false, false},
		{"true", false, true},
		{"True", false, true},
		{"TRUE", false, true},
		{"1", false, true},
		{"yes", false, true},
		{"on", false, true},
		{"false", true, false},
		{"False", true, false},
		{"0", true, false},
		{"no", true, false},
		{"off", true, false},
		{"invalid", true, true},
		{"invalid", false, false},
	}

	for _, tt := range tests {
		os.Setenv("TEST_BOOL", tt.value)
		got := parseBoolEnv("TEST_BOOL", tt.defaultVal)
		if got != tt.expected {
			t.Errorf("parseBoolEnv(%q, %v) = %v, want %v", tt.value, tt.defaultVal, got, tt.expected)
		}
	}
	os.Unsetenv("TEST_BOOL")
}

func TestParseCSVEnv(t *testing.T) {
	tests := []struct {
		value    string
		expected []string
	}{
		{"", nil},
		{"a", []string{"a"}},
		{"a,b,c", []string{"a", "b", "c"}},
		{"a, b, c", []string{"a", "b", "c"}},
		{" a , b , c ", []string{"a", "b", "c"}},
		{"a,,b", []string{"a", "b"}},
	}

	for _, tt := range tests {
		os.Setenv("TEST_CSV", tt.value)
		got := parseCSVEnv("TEST_CSV")
		if len(got) != len(tt.expected) {
			t.Errorf("parseCSVEnv(%q) = %v, want %v", tt.value, got, tt.expected)
			continue
		}
		for i := range got {
			if got[i] != tt.expected[i] {
				t.Errorf("parseCSVEnv(%q)[%d] = %q, want %q", tt.value, i, got[i], tt.expected[i])
			}
		}
	}
	os.Unsetenv("TEST_CSV")
}

func TestIsCloudConfigured_GCP(t *testing.T) {
	tests := []struct {
		name     string
		config   *Config
		expected bool
	}{
		{
			name: "gcp with credentials",
			config: &Config{
				CloudEnabled:       true,
				CloudProvider:      "gcp",
				GCPCredentialsFile: "/path/to/creds.json",
			},
			expected: true,
		},
		{
			name: "gcp without credentials",
			config: &Config{
				CloudEnabled:  true,
				CloudProvider: "gcp",
			},
			expected: false,
		},
		{
			name: "gcp disabled",
			config: &Config{
				CloudEnabled:       false,
				CloudProvider:      "gcp",
				GCPCredentialsFile: "/path/to/creds.json",
			},
			expected: false,
		},
		{
			name: "gcp with credentials and no endpoint is OK",
			config: &Config{
				CloudEnabled:       true,
				CloudProvider:      "gcp",
				GCPCredentialsFile: "/path/to/creds.json",
				Endpoint:           "", // no endpoint needed for GCP
			},
			expected: true,
		},
		{
			name: "generic without endpoint",
			config: &Config{
				CloudEnabled: true,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.config.IsCloudConfigured(); got != tt.expected {
				t.Errorf("IsCloudConfigured() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestIsGCP(t *testing.T) {
	tests := []struct {
		name     string
		config   *Config
		expected bool
	}{
		{
			name:     "nil config",
			config:   nil,
			expected: false,
		},
		{
			name: "gcp with creds",
			config: &Config{
				CloudProvider:      "gcp",
				GCPCredentialsFile: "/path/to/creds.json",
			},
			expected: true,
		},
		{
			name: "gcp without creds",
			config: &Config{
				CloudProvider: "gcp",
			},
			expected: false,
		},
		{
			name: "not gcp",
			config: &Config{
				CloudProvider:      "other",
				GCPCredentialsFile: "/path/to/creds.json",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.config.IsGCP(); got != tt.expected {
				t.Errorf("IsGCP() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestReadProjectIDFromCredentials(t *testing.T) {
	// Write a temp credentials file
	tmpFile, err := os.CreateTemp("", "gcp-creds-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	credJSON := `{"type":"service_account","project_id":"test-project-123","private_key_id":"key"}`
	if _, err := tmpFile.WriteString(credJSON); err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	got := readProjectIDFromCredentials(tmpFile.Name())
	if got != "test-project-123" {
		t.Errorf("readProjectIDFromCredentials() = %q, want %q", got, "test-project-123")
	}

	// Non-existent file
	got = readProjectIDFromCredentials("/nonexistent/path.json")
	if got != "" {
		t.Errorf("readProjectIDFromCredentials(nonexistent) = %q, want empty", got)
	}

	// Invalid JSON
	badFile, _ := os.CreateTemp("", "bad-creds-*.json")
	defer os.Remove(badFile.Name())
	badFile.WriteString("not json")
	badFile.Close()
	got = readProjectIDFromCredentials(badFile.Name())
	if got != "" {
		t.Errorf("readProjectIDFromCredentials(invalid) = %q, want empty", got)
	}
}

func TestLoadConfig_ProjectIDFromCredentials(t *testing.T) {
	clearTelemetryEnv()

	// Write a temp credentials file
	tmpFile, err := os.CreateTemp("", "gcp-creds-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	credJSON := `{"type":"service_account","project_id":"creds-project"}`
	tmpFile.WriteString(credJSON)
	tmpFile.Close()

	// Set credentials file but NOT project ID
	os.Setenv(EnvGCPCredentials, tmpFile.Name())
	os.Setenv(EnvCloudProvider, "gcp")
	defer clearTelemetryEnv()

	cfg := LoadConfig()

	if cfg.ProjectID != "creds-project" {
		t.Errorf("Expected ProjectID auto-resolved from credentials, got %q", cfg.ProjectID)
	}
}

func TestLoadConfig_ProjectIDEnvTakesPriority(t *testing.T) {
	clearTelemetryEnv()

	// Write a temp credentials file
	tmpFile, err := os.CreateTemp("", "gcp-creds-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	credJSON := `{"type":"service_account","project_id":"creds-project"}`
	tmpFile.WriteString(credJSON)
	tmpFile.Close()

	// Set both env var and credentials file
	os.Setenv(EnvProjectID, "env-project")
	os.Setenv(EnvGCPCredentials, tmpFile.Name())
	defer clearTelemetryEnv()

	cfg := LoadConfig()

	// Explicit env var should win
	if cfg.ProjectID != "env-project" {
		t.Errorf("Expected ProjectID from env to take priority, got %q", cfg.ProjectID)
	}
}

func TestLoadConfig_GCPDefaults(t *testing.T) {
	clearTelemetryEnv()

	cfg := LoadConfig()

	if cfg.GCPCredentialsFile != "" {
		t.Errorf("Expected GCPCredentialsFile to be empty by default, got %q", cfg.GCPCredentialsFile)
	}
	if cfg.CloudProvider != "" {
		t.Errorf("Expected CloudProvider to be empty by default, got %q", cfg.CloudProvider)
	}
}

func TestLoadConfig_GCPEnvOverrides(t *testing.T) {
	clearTelemetryEnv()

	os.Setenv(EnvGCPCredentials, "/etc/gcp/sa.json")
	os.Setenv(EnvCloudProvider, "gcp")
	defer clearTelemetryEnv()

	cfg := LoadConfig()

	if cfg.GCPCredentialsFile != "/etc/gcp/sa.json" {
		t.Errorf("Expected GCPCredentialsFile to be '/etc/gcp/sa.json', got %q", cfg.GCPCredentialsFile)
	}
	if cfg.CloudProvider != "gcp" {
		t.Errorf("Expected CloudProvider to be 'gcp', got %q", cfg.CloudProvider)
	}
}

func clearTelemetryEnv() {
	os.Unsetenv(EnvEnabled)
	os.Unsetenv(EnvCloudEnabled)
	os.Unsetenv(EnvEndpoint)
	os.Unsetenv(EnvProtocol)
	os.Unsetenv(EnvInsecure)
	os.Unsetenv(EnvGRPCPort)
	os.Unsetenv(EnvHTTPPort)
	os.Unsetenv(EnvFilterExclude)
	os.Unsetenv(EnvFilterInclude)
	os.Unsetenv(EnvProjectID)
	os.Unsetenv(EnvGCPCredentials)
	os.Unsetenv(EnvCloudProvider)
}
