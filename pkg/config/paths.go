package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/ptone/scion-agent/pkg/util"
)

const (
	DotScion = ".scion"
	GlobalDir = ".scion"
)

// GetRepoDir returns the .scion directory at the root of the git repo, if it exists.
func GetRepoDir() (string, bool) {
	if !util.IsGitRepo() {
		return "", false
	}
	root, err := util.RepoRoot()
	if err != nil {
		return "", false
	}
	p := filepath.Join(root, DotScion)
	if info, err := os.Stat(p); err == nil && info.IsDir() {
		return p, true
	}
	return "", false
}

// FindProjectRoot walks up the directory tree to find the .scion directory.
func FindProjectRoot() (string, bool) {
	wd, err := os.Getwd()
	if err != nil {
		return "", false
	}

	dir := wd
	for {
		p := filepath.Join(dir, DotScion)
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			if abs, err := filepath.EvalSymlinks(p); err == nil {
				return abs, true
			}
			return p, true
		}

		parent := filepath.Dir(dir)
		if parent == dir { // Reached root
			break
		}
		dir = parent
	}
	return "", false
}

// GetResolvedProjectDir returns the active .scion directory based on precedence.
func GetResolvedProjectDir(explicitPath string) (string, error) {
	// 1. Explicitly provided via flag
	if explicitPath != "" {
		if explicitPath == "home" || explicitPath == "global" {
			return GetGlobalDir()
		}
		abs, err := filepath.Abs(explicitPath)
		if err != nil {
			return "", err
		}
		return abs, nil
	}

	// 2. Walk up to find .scion (covers repo root and current dir cases)
	if p, ok := FindProjectRoot(); ok {
		return p, nil
	}

	// 3. Fallback to global
	return GetGlobalDir()
}

func GetProjectDir() (string, error) {
	// 1. Walk up to find .scion
	if p, ok := FindProjectRoot(); ok {
		return p, nil
	}

	// 2. Fallback to current directory (legacy/non-repo behavior)
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(wd, DotScion), nil
}

// GetGroveName returns the slugified name of the grove.
func GetGroveName(projectDir string) string {
	abs, err := filepath.Abs(projectDir)
	if err != nil {
		return "unknown"
	}

	parent := filepath.Dir(abs)
	home, err := os.UserHomeDir()
	if err == nil && parent == home {
		return "global"
	}

	return slugify(filepath.Base(parent))
}

func slugify(s string) string {
	s = strings.ToLower(s)
	var res strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			res.WriteRune(r)
		} else {
			res.WriteRune('-')
		}
	}
	return strings.Trim(res.String(), "-")
}

// GetTargetProjectDir returns the directory where a grove should be initialized.
func GetTargetProjectDir() (string, error) {
	// 1. Root of the current git repo if run inside a repo
	if util.IsGitRepo() {
		root, err := util.RepoRoot()
		if err == nil {
			return filepath.Join(root, DotScion), nil
		}
	}

	// 2. Current directory
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(wd, DotScion), nil
}

func GetGlobalDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, GlobalDir), nil
}

func GetProjectTemplatesDir() (string, error) {
	p, err := GetProjectDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(p, "templates"), nil
}

func GetGlobalTemplatesDir() (string, error) {
	g, err := GetGlobalDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(g, "templates"), nil
}

func GetProjectAgentsDir() (string, error) {
	p, err := GetProjectDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(p, "agents"), nil
}

func GetProjectKubernetesConfigPath() (string, error) {
	p, err := GetProjectDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(p, "kubernetes-config.json"), nil
}

func GetGlobalAgentsDir() (string, error) {
	g, err := GetGlobalDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(g, "agents"), nil
}

// ResolveGrovePath resolves a grove path to an absolute path and indicates if it's the global grove.
// If path is empty, it attempts to find the project grove or falls back to global.
// If path is "global" or "home", it returns the global grove path.
// Returns the absolute path, whether it's the global grove, and any error.
func ResolveGrovePath(path string) (string, bool, error) {
	if path == "" {
		// Try to find project grove first
		if p, ok := FindProjectRoot(); ok {
			return p, false, nil
		}
		// Fallback to global
		g, err := GetGlobalDir()
		return g, true, err
	}

	if path == "global" || path == "home" {
		g, err := GetGlobalDir()
		return g, true, err
	}

	// Check if path is the global dir
	globalDir, _ := GetGlobalDir()

	abs, err := filepath.Abs(path)
	if err != nil {
		return "", false, err
	}

	isGlobal := abs == globalDir

	return abs, isGlobal, nil
}
