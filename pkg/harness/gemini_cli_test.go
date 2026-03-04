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

package harness

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ptone/scion-agent/pkg/api"
)

func TestGeminiDiscoverAuth(t *testing.T) {
	// Setup a temporary home directory
	tmpHome, err := os.MkdirTemp("", "scion-home-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpHome)

	// Mock HOME environment variable
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	geminiDir := filepath.Join(tmpHome, ".gemini")
	if err := os.MkdirAll(geminiDir, 0755); err != nil {
		t.Fatal(err)
	}

	// 1. Test OAuth discovery via host settings
	settingsPath := filepath.Join(geminiDir, "settings.json")
	settingsData := `{
		"security": {
			"auth": {
				"selectedType": "oauth-personal"
			}
		}
	}`
	if err := os.WriteFile(settingsPath, []byte(settingsData), 0644); err != nil {
		t.Fatal(err)
	}

	oauthCredsPath := filepath.Join(geminiDir, "oauth_creds.json")
	if err := os.WriteFile(oauthCredsPath, []byte(`{"dummy":"creds"}`), 0644); err != nil {
		t.Fatal(err)
	}

	// Setup agent home in a dedicated directory to avoid parent dir pollution (scion-agent.json)
	baseDir := t.TempDir()
	agentHome := filepath.Join(baseDir, "agents", "test-agent", "home")
	if err := os.MkdirAll(agentHome, 0755); err != nil {
		t.Fatal(err)
	}

	g := &GeminiCLI{}
	auth := g.DiscoverAuth(agentHome)
	if auth.OAuthCreds != oauthCredsPath {
		t.Errorf("expected OAuthCreds to be %s, got %s", oauthCredsPath, auth.OAuthCreds)
	}

	// 2. Test OAuth discovery via agent settings (overriding host)
	// Create agent-specific settings.json
	agentGeminiDir := filepath.Join(agentHome, ".gemini")
	os.MkdirAll(agentGeminiDir, 0755)
	agentSettingsPath := filepath.Join(agentGeminiDir, "settings.json")
	os.WriteFile(agentSettingsPath, []byte(`{"security":{"auth":{"selectedType":"gemini-api-key"}}}`), 0644)
	
	auth = g.DiscoverAuth(agentHome)
	// wait, if agent settings says gemini-api-key, and we have oauth-personal on host,
	// DiscoverAuth currently prefers agent setting if present.
	// But it only checks agent settings for "SelectedType".
	// If agent settings has SelectedType="gemini-api-key", it will NOT return OAuthCreds.
	if auth.OAuthCreds != "" {
		t.Errorf("expected OAuthCreds to be empty when requested by agent settings, got %s", auth.OAuthCreds)
	}

	// 3. Test API Key fallback from host settings
	os.Remove(settingsPath)
	os.Remove(agentSettingsPath)
	settingsData = `{
		"apiKey": "test-api-key"
	}`
	if err := os.WriteFile(settingsPath, []byte(settingsData), 0644); err != nil {
		t.Fatal(err)
	}

	// Clear env vars that might interfere
	origApiKey := os.Getenv("GEMINI_API_KEY")
	origGoogleApiKey := os.Getenv("GOOGLE_API_KEY")
	os.Unsetenv("GEMINI_API_KEY")
	os.Unsetenv("GOOGLE_API_KEY")
	defer func() {
		os.Setenv("GEMINI_API_KEY", origApiKey)
		os.Setenv("GOOGLE_API_KEY", origGoogleApiKey)
	}()

	auth = g.DiscoverAuth(agentHome)
	if auth.GeminiAPIKey != "test-api-key" {
		t.Errorf("expected GeminiAPIKey to be 'test-api-key', got '%s'", auth.GeminiAPIKey)
	}
}

func TestGeminiGetTelemetryEnv(t *testing.T) {
	g := &GeminiCLI{}
	env := g.GetTelemetryEnv()

	expected := map[string]string{
		"GEMINI_TELEMETRY_ENABLED":       "true",
		"GEMINI_TELEMETRY_TARGET":        "local",
		"GEMINI_TELEMETRY_USE_COLLECTOR": "true",
		"GEMINI_TELEMETRY_OTLP_ENDPOINT": "http://localhost:4317",
		"GEMINI_TELEMETRY_OTLP_PROTOCOL": "grpc",
		"GEMINI_TELEMETRY_LOG_PROMPTS":   "false",
	}

	if len(env) != len(expected) {
		t.Fatalf("expected %d env vars, got %d: %v", len(expected), len(env), env)
	}

	for k, want := range expected {
		got, ok := env[k]
		if !ok {
			t.Errorf("missing env var %s", k)
			continue
		}
		if got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
}

func TestGeminiInjectAgentInstructions(t *testing.T) {
	agentHome := t.TempDir()
	g := &GeminiCLI{}
	content := []byte("# Agent Instructions\nDo good work.")

	if err := g.InjectAgentInstructions(agentHome, content); err != nil {
		t.Fatalf("InjectAgentInstructions failed: %v", err)
	}

	target := filepath.Join(agentHome, ".gemini", "GEMINI.md")
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("expected file at %s: %v", target, err)
	}
	if string(data) != string(content) {
		t.Errorf("content mismatch: got %q, want %q", string(data), string(content))
	}
}

func TestGeminiInjectAgentInstructions_RemovesLowercaseFile(t *testing.T) {
	agentHome := t.TempDir()
	g := &GeminiCLI{}

	// Simulate a harness-config home that provides gemini.md (lowercase)
	geminiDir := filepath.Join(agentHome, ".gemini")
	if err := os.MkdirAll(geminiDir, 0755); err != nil {
		t.Fatal(err)
	}
	lowercasePath := filepath.Join(geminiDir, "gemini.md")
	if err := os.WriteFile(lowercasePath, []byte("# Harness config instructions"), 0644); err != nil {
		t.Fatal(err)
	}

	// Inject agent instructions — should remove the lowercase file
	content := []byte("# Template Instructions\nFrom agents.md")
	if err := g.InjectAgentInstructions(agentHome, content); err != nil {
		t.Fatalf("InjectAgentInstructions failed: %v", err)
	}

	// Canonical GEMINI.md should exist with the injected content
	target := filepath.Join(geminiDir, "GEMINI.md")
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("expected GEMINI.md at %s: %v", target, err)
	}
	if string(data) != string(content) {
		t.Errorf("content mismatch: got %q, want %q", string(data), string(content))
	}

	// Lowercase gemini.md should no longer exist (on case-sensitive filesystems)
	if _, err := os.Lstat(lowercasePath); err == nil {
		entries, _ := os.ReadDir(geminiDir)
		for _, e := range entries {
			if strings.EqualFold(e.Name(), "GEMINI.md") && e.Name() != "GEMINI.md" {
				t.Errorf("lowercase %q should have been removed", e.Name())
			}
		}
	}
}

func TestGeminiRequiredEnvKeys(t *testing.T) {
	g := &GeminiCLI{}

	tests := []struct {
		name             string
		authSelectedType string
		want             []string
	}{
		{"gemini-api-key", "gemini-api-key", []string{"GEMINI_API_KEY"}},
		{"vertex-ai", "vertex-ai", []string{"GOOGLE_CLOUD_PROJECT"}},
		{"oauth-personal", "oauth-personal", nil},
		{"compute-default-credentials", "compute-default-credentials", nil},
		{"empty (default)", "", []string{"GEMINI_API_KEY"}},
		{"unknown type", "something-else", []string{"GEMINI_API_KEY"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := g.RequiredEnvKeys(tt.authSelectedType)
			if len(got) != len(tt.want) {
				t.Fatalf("RequiredEnvKeys(%q) = %v, want %v", tt.authSelectedType, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("RequiredEnvKeys(%q)[%d] = %q, want %q", tt.authSelectedType, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestGeminiResolveAuth_ExplicitAPIKey(t *testing.T) {
	g := &GeminiCLI{}
	auth := api.AuthConfig{
		SelectedType: "gemini-api-key",
		GeminiAPIKey: "test-key",
	}
	result, err := g.ResolveAuth(auth)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Method != "gemini-api-key" {
		t.Errorf("Method = %q, want %q", result.Method, "gemini-api-key")
	}
	if result.EnvVars["GEMINI_API_KEY"] != "test-key" {
		t.Errorf("GEMINI_API_KEY = %q, want %q", result.EnvVars["GEMINI_API_KEY"], "test-key")
	}
	if result.EnvVars["GEMINI_DEFAULT_AUTH_TYPE"] != "gemini-api-key" {
		t.Errorf("GEMINI_DEFAULT_AUTH_TYPE = %q, want %q", result.EnvVars["GEMINI_DEFAULT_AUTH_TYPE"], "gemini-api-key")
	}
}

func TestGeminiResolveAuth_ExplicitAPIKeyFallbackGoogle(t *testing.T) {
	g := &GeminiCLI{}
	auth := api.AuthConfig{
		SelectedType: "gemini-api-key",
		GoogleAPIKey: "google-key",
	}
	result, err := g.ResolveAuth(auth)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.EnvVars["GOOGLE_API_KEY"] != "google-key" {
		t.Errorf("GOOGLE_API_KEY = %q, want %q", result.EnvVars["GOOGLE_API_KEY"], "google-key")
	}
}

func TestGeminiResolveAuth_ExplicitAPIKeyMissing(t *testing.T) {
	g := &GeminiCLI{}
	auth := api.AuthConfig{SelectedType: "gemini-api-key"}
	_, err := g.ResolveAuth(auth)
	if err == nil {
		t.Fatal("expected error for explicit api-key with no key")
	}
}

func TestGeminiResolveAuth_ExplicitOAuth(t *testing.T) {
	g := &GeminiCLI{}
	auth := api.AuthConfig{
		SelectedType:     "oauth-personal",
		OAuthCreds:       "/path/to/oauth.json",
		GoogleCloudProject: "proj",
	}
	result, err := g.ResolveAuth(auth)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Method != "oauth-personal" {
		t.Errorf("Method = %q, want %q", result.Method, "oauth-personal")
	}
	if len(result.Files) != 1 {
		t.Fatalf("expected 1 file mapping, got %d", len(result.Files))
	}
	if result.EnvVars["GOOGLE_CLOUD_PROJECT"] != "proj" {
		t.Errorf("GOOGLE_CLOUD_PROJECT = %q, want %q", result.EnvVars["GOOGLE_CLOUD_PROJECT"], "proj")
	}
}

func TestGeminiResolveAuth_ExplicitOAuthMissing(t *testing.T) {
	g := &GeminiCLI{}
	auth := api.AuthConfig{SelectedType: "oauth-personal"}
	_, err := g.ResolveAuth(auth)
	if err == nil {
		t.Fatal("expected error for explicit oauth-personal with no creds")
	}
}

func TestGeminiResolveAuth_ExplicitVertexAI(t *testing.T) {
	g := &GeminiCLI{}
	auth := api.AuthConfig{
		SelectedType:         "vertex-ai",
		GoogleCloudProject:   "proj",
		GoogleCloudRegion:    "us-east1",
		GoogleAppCredentials: "/path/to/adc.json",
	}
	result, err := g.ResolveAuth(auth)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Method != "vertex-ai" {
		t.Errorf("Method = %q, want %q", result.Method, "vertex-ai")
	}
	if result.EnvVars["GOOGLE_CLOUD_PROJECT"] != "proj" {
		t.Errorf("GOOGLE_CLOUD_PROJECT = %q, want %q", result.EnvVars["GOOGLE_CLOUD_PROJECT"], "proj")
	}
	if result.EnvVars["GOOGLE_CLOUD_REGION"] != "us-east1" {
		t.Errorf("GOOGLE_CLOUD_REGION = %q, want %q", result.EnvVars["GOOGLE_CLOUD_REGION"], "us-east1")
	}
	if len(result.Files) != 1 {
		t.Fatalf("expected 1 file mapping, got %d", len(result.Files))
	}
}

func TestGeminiResolveAuth_ExplicitVertexMissingProject(t *testing.T) {
	g := &GeminiCLI{}
	auth := api.AuthConfig{SelectedType: "vertex-ai"}
	_, err := g.ResolveAuth(auth)
	if err == nil {
		t.Fatal("expected error for explicit vertex-ai with no project")
	}
}

func TestGeminiResolveAuth_ExplicitComputeDefault(t *testing.T) {
	g := &GeminiCLI{}
	auth := api.AuthConfig{
		SelectedType:         "compute-default-credentials",
		GoogleAppCredentials: "/path/to/adc.json",
	}
	result, err := g.ResolveAuth(auth)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Method != "compute-default-credentials" {
		t.Errorf("Method = %q, want %q", result.Method, "compute-default-credentials")
	}
	if len(result.Files) != 1 {
		t.Fatalf("expected 1 file mapping, got %d", len(result.Files))
	}
}

func TestGeminiResolveAuth_ExplicitComputeDefaultNoADC(t *testing.T) {
	g := &GeminiCLI{}
	auth := api.AuthConfig{SelectedType: "compute-default-credentials"}
	result, err := g.ResolveAuth(auth)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Method != "compute-default-credentials" {
		t.Errorf("Method = %q, want %q", result.Method, "compute-default-credentials")
	}
	if len(result.Files) != 0 {
		t.Errorf("expected no file mappings without ADC, got %d", len(result.Files))
	}
}

func TestGeminiResolveAuth_UnknownType(t *testing.T) {
	g := &GeminiCLI{}
	auth := api.AuthConfig{SelectedType: "foobar"}
	_, err := g.ResolveAuth(auth)
	if err == nil {
		t.Fatal("expected error for unknown selected type")
	}
	if !strings.Contains(err.Error(), "foobar") {
		t.Errorf("error should mention the unknown type: %v", err)
	}
}

func TestGeminiResolveAuth_AutoDetectAPIKey(t *testing.T) {
	g := &GeminiCLI{}
	auth := api.AuthConfig{GeminiAPIKey: "auto-key"}
	result, err := g.ResolveAuth(auth)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Method != "gemini-api-key" {
		t.Errorf("Method = %q, want %q", result.Method, "gemini-api-key")
	}
}

func TestGeminiResolveAuth_AutoDetectADC(t *testing.T) {
	g := &GeminiCLI{}
	auth := api.AuthConfig{GoogleAppCredentials: "/path/to/adc.json"}
	result, err := g.ResolveAuth(auth)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Method != "compute-default-credentials" {
		t.Errorf("Method = %q, want %q", result.Method, "compute-default-credentials")
	}
}

func TestGeminiResolveAuth_AutoDetectOAuth(t *testing.T) {
	g := &GeminiCLI{}
	auth := api.AuthConfig{OAuthCreds: "/path/to/oauth.json"}
	result, err := g.ResolveAuth(auth)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Method != "oauth-personal" {
		t.Errorf("Method = %q, want %q", result.Method, "oauth-personal")
	}
}

func TestGeminiResolveAuth_AutoDetectPriority(t *testing.T) {
	g := &GeminiCLI{}
	// API key should win over ADC and OAuth
	auth := api.AuthConfig{
		GeminiAPIKey:         "key",
		GoogleAppCredentials: "/adc.json",
		OAuthCreds:           "/oauth.json",
	}
	result, err := g.ResolveAuth(auth)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Method != "gemini-api-key" {
		t.Errorf("API key should win; Method = %q, want %q", result.Method, "gemini-api-key")
	}
}

func TestGeminiResolveAuth_NoCreds(t *testing.T) {
	g := &GeminiCLI{}
	_, err := g.ResolveAuth(api.AuthConfig{})
	if err == nil {
		t.Fatal("expected error for empty AuthConfig")
	}
}

func TestGeminiInjectSystemPrompt(t *testing.T) {
	agentHome := t.TempDir()
	g := &GeminiCLI{}
	content := []byte("You are a helpful coding assistant.")

	if err := g.InjectSystemPrompt(agentHome, content); err != nil {
		t.Fatalf("InjectSystemPrompt failed: %v", err)
	}

	target := filepath.Join(agentHome, ".gemini", "system_prompt.md")
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("expected file at %s: %v", target, err)
	}
	if string(data) != string(content) {
		t.Errorf("content mismatch: got %q, want %q", string(data), string(content))
	}
}
