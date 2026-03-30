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
	projectFile := filepath.Join(project.WorkspaceDir, "PROJECT.md")
	content, err := os.ReadFile(projectFile)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", projectFile, err)
	}
	if !strings.Contains(string(content), "Grow the user's X.com account") {
		t.Fatalf("PROJECT.md = %q, want project description", string(content))
	}
}
