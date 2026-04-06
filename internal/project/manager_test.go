package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
)

func TestEnsureBaseScaffoldCreatesNoProject(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	manager := NewManager(dataDir, filepath.Join(dataDir, "projects"))

	if err := manager.EnsureBaseScaffold(); err != nil {
		t.Fatalf("EnsureBaseScaffold() error = %v", err)
	}

	projectFile := filepath.Join(dataDir, "projects", DefaultProjectSlug, "PROJECT.md")
	content, err := os.ReadFile(projectFile)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", projectFile, err)
	}
	if !strings.Contains(string(content), "No Project") {
		t.Fatalf("PROJECT.md = %q, want no_project title", string(content))
	}
}

func TestEnsureProjectCreatesProjectMarkdown(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	manager := NewManager(dataDir, filepath.Join(dataDir, "projects"))

	project, err := manager.EnsureProject(assistant.ProjectContext{
		Slug:        "x-growth",
		Name:        "X Growth",
		Description: "Grow the user's X.com account over repeated work.",
	})
	if err != nil {
		t.Fatalf("EnsureProject() error = %v", err)
	}

	if project.WorkspaceDir != filepath.Join(dataDir, "projects", "x-growth") {
		t.Fatalf("WorkspaceDir = %q, want project directory", project.WorkspaceDir)
	}
	if project.BrowserProfileDir != filepath.Join(project.WorkspaceDir, ".browser-profile") {
		t.Fatalf("BrowserProfileDir = %q, want project browser profile directory", project.BrowserProfileDir)
	}
	if project.BrowserCDPPort != 9223 {
		t.Fatalf("BrowserCDPPort = %d, want 9223", project.BrowserCDPPort)
	}
	if info, err := os.Stat(project.BrowserProfileDir); err != nil {
		t.Fatalf("Stat(%s) error = %v", project.BrowserProfileDir, err)
	} else if !info.IsDir() {
		t.Fatalf("%s is not a directory", project.BrowserProfileDir)
	}
	projectFile := filepath.Join(project.WorkspaceDir, "PROJECT.md")
	content, err := os.ReadFile(projectFile)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", projectFile, err)
	}
	if !strings.Contains(string(content), "Grow the user's X.com account") {
		t.Fatalf("PROJECT.md = %q, want project description", string(content))
	}
}
