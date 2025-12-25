package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateTemplate(t *testing.T) {
	// Setup a temporary directory for the test
	tmpDir, err := os.MkdirTemp("", "scion-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Override home dir for global templates
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Create a mock project structure
	projectDir := filepath.Join(tmpDir, "project", DotScion)
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Helper to change current working directory
	origWd, _ := os.Getwd()
	defer os.Chdir(origWd)
	if err := os.Chdir(filepath.Dir(projectDir)); err != nil {
		t.Fatal(err)
	}

	// Test creating a project template
	tplName := "test-tpl"
	err = CreateTemplate(tplName, false)
	if err != nil {
		t.Fatalf("failed to create project template: %v", err)
	}

	expectedPath := filepath.Join(projectDir, "templates", tplName)
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Errorf("expected template directory %s to exist", expectedPath)
	}

	// Verify key files exist
	files := []string{
		"scion.json",
		".bashrc",
		filepath.Join(".gemini", "settings.json"),
	}
	for _, f := range files {
		if _, err := os.Stat(filepath.Join(expectedPath, f)); os.IsNotExist(err) {
			t.Errorf("expected file %s to exist in template", f)
		}
	}

	// Test creating a global template
	globalTplName := "global-tpl"
	err = CreateTemplate(globalTplName, true)
	if err != nil {
		t.Fatalf("failed to create global template: %v", err)
	}

	globalExpectedPath := filepath.Join(tmpDir, GlobalDir, "templates", globalTplName)
	if _, err := os.Stat(globalExpectedPath); os.IsNotExist(err) {
		t.Errorf("expected global template directory %s to exist", globalExpectedPath)
	}

	// Test duplicate template creation fails
	err = CreateTemplate(tplName, false)
	if err == nil {
		t.Error("expected error when creating duplicate template, got nil")
	}
}

func TestDeleteTemplate(t *testing.T) {
	// Setup a temporary directory for the test
	tmpDir, err := os.MkdirTemp("", "scion-test-delete-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Override home dir for global templates
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Create a mock project structure
	projectDir := filepath.Join(tmpDir, "project", DotScion)
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Helper to change current working directory
	origWd, _ := os.Getwd()
	defer os.Chdir(origWd)
	if err := os.Chdir(filepath.Dir(projectDir)); err != nil {
		t.Fatal(err)
	}

	// Create templates to delete
	tplName := "test-tpl-delete"
	if err := CreateTemplate(tplName, false); err != nil {
		t.Fatal(err)
	}
	globalTplName := "global-tpl-delete"
	if err := CreateTemplate(globalTplName, true); err != nil {
		t.Fatal(err)
	}

	// Test deleting project template
	if err := DeleteTemplate(tplName, false); err != nil {
		t.Fatalf("failed to delete project template: %v", err)
	}
	expectedPath := filepath.Join(projectDir, "templates", tplName)
	if _, err := os.Stat(expectedPath); !os.IsNotExist(err) {
		t.Errorf("expected template directory %s to be gone", expectedPath)
	}

	// Test deleting global template
	if err := DeleteTemplate(globalTplName, true); err != nil {
		t.Fatalf("failed to delete global template: %v", err)
	}
	globalExpectedPath := filepath.Join(tmpDir, GlobalDir, "templates", globalTplName)
	if _, err := os.Stat(globalExpectedPath); !os.IsNotExist(err) {
		t.Errorf("expected global template directory %s to be gone", globalExpectedPath)
	}

	// Test deleting "default" fails
	if err := DeleteTemplate("default", false); err == nil {
		t.Error("expected error when deleting default template, got nil")
	}

	// Test deleting non-existent template fails
	if err := DeleteTemplate("no-such-template", false); err == nil {
		t.Error("expected error when deleting non-existent template, got nil")
	}
}

func TestUpdateDefaultTemplate(t *testing.T) {
	// Setup a temporary directory for the test
	tmpDir, err := os.MkdirTemp("", "scion-test-update-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Override home dir
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Create a mock project structure
	projectDir := filepath.Join(tmpDir, "project", DotScion)
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Helper to change current working directory
	origWd, _ := os.Getwd()
	defer os.Chdir(origWd)
	if err := os.Chdir(filepath.Dir(projectDir)); err != nil {
		t.Fatal(err)
	}

	// Initialize project (creates default template)
	if err := InitProject(""); err != nil {
		t.Fatal(err)
	}

	defaultTemplateScionJSON := filepath.Join(projectDir, "templates", "default", "scion.json")
	
	// Corrupt the default template file
	corruptContent := "CORRUPT"
	if err := os.WriteFile(defaultTemplateScionJSON, []byte(corruptContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Update default template
	if err := UpdateDefaultTemplate(false); err != nil {
		t.Fatalf("failed to update default template: %v", err)
	}

	// Verify it was restored
	data, err := os.ReadFile(defaultTemplateScionJSON)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == corruptContent {
		t.Error("expected scion.json to be overwritten, but it still contains corrupt content")
	}
}

func TestMergeScionConfig(t *testing.T) {
	trueVal := true
	falseVal := false

	tests := []struct {
		name     string
		base     *ScionConfig
		override *ScionConfig
		wantTmux bool
	}{
		{
			name:     "override false to true",
			base:     &ScionConfig{UseTmux: &falseVal},
			override: &ScionConfig{UseTmux: &trueVal},
			wantTmux: true,
		},
		{
			name:     "override true to false",
			base:     &ScionConfig{UseTmux: &trueVal},
			override: &ScionConfig{UseTmux: &falseVal},
			wantTmux: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MergeScionConfig(tt.base, tt.override)
			if got.UseTmux == nil || *got.UseTmux != tt.wantTmux {
				t.Errorf("MergeScionConfig() UseTmux = %v, want %v", got.UseTmux, tt.wantTmux)
			}
		})
	}

	t.Run("detached merge", func(t *testing.T) {
		base := &ScionConfig{Detached: &trueVal}
		override := &ScionConfig{Detached: &falseVal}
		got := MergeScionConfig(base, override)
		if got.Detached == nil || *got.Detached != false {
			t.Errorf("expected detached to be false, got %v", got.Detached)
		}
	})
}
