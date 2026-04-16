package main

import (
	"strings"
	"testing"

	"github.com/siisee11/CodexVirtualAssistant/internal/config"
)

func TestLocalStatusFromConfig(t *testing.T) {
	t.Parallel()

	status := localStatusFromConfig(config.Config{
		ConfigDir:    "/home/test/.config/cva",
		DataDir:      "/home/test/.config/cva/workspace",
		ProjectsDir:  "/home/test/.config/cva/workspace/projects",
		DatabasePath: "/home/test/.config/cva/workspace/assistant.db",
		ArtifactDir:  "/home/test/.config/cva/workspace/artifacts",
		CodexCwd:     "/home/test/.config/cva",
	})

	if status.ConfigDir != "/home/test/.config/cva" {
		t.Fatalf("ConfigDir = %q", status.ConfigDir)
	}
	if status.WorkspaceDir != "/home/test/.config/cva/workspace" {
		t.Fatalf("WorkspaceDir = %q", status.WorkspaceDir)
	}
}

func TestFormatLocalStatusIncludesConfigDirectory(t *testing.T) {
	t.Parallel()

	text := formatLocalStatus(localStatus{
		ConfigDir:    "/home/test/.config/cva",
		WorkspaceDir: "/home/test/.config/cva/workspace",
		ProjectsDir:  "/home/test/.config/cva/workspace/projects",
		DatabasePath: "/home/test/.config/cva/workspace/assistant.db",
		ArtifactDir:  "/home/test/.config/cva/workspace/artifacts",
		CodexCwd:     "/home/test/.config/cva",
	})

	for _, want := range []string{
		"CVA Status",
		"Config Directory: /home/test/.config/cva",
		"Workspace:        /home/test/.config/cva/workspace",
		"Codex CWD:        /home/test/.config/cva",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("formatted status missing %q:\n%s", want, text)
		}
	}
}
