package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitProject_CreatesClaudeTemplate(t *testing.T) {
	// Create a temporary directory for the project
	tempDir, err := os.MkdirTemp("", "scion-init-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Run InitProject
	err = InitProject(tempDir)
	if err != nil {
		t.Fatalf("InitProject failed: %v", err)
	}

	// Verify that templates/claude exists
	claudeDir := filepath.Join(tempDir, "templates", "claude")
	if _, err := os.Stat(claudeDir); os.IsNotExist(err) {
		t.Errorf("Expected templates/claude to be created, but it was not")
	}

	// Verify a file inside templates/claude exists to be sure
	claudeSettings := filepath.Join(claudeDir, "scion-agent.json")
	if _, err := os.Stat(claudeSettings); os.IsNotExist(err) {
		t.Errorf("Expected templates/claude/scion-agent.json to be created, but it was not")
	}
}
