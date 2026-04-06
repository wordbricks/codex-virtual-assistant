package project

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
)

const (
	DefaultProjectSlug      = "no_project"
	projectFileName         = "PROJECT.md"
	browserProfileDirName   = ".browser-profile"
	defaultBrowserCDPPort   = 9223
	defaultNoProjectName    = "No Project"
	defaultNoProjectPurpose = "Use this project for simple questions, one-off requests, and tasks that do not need long-lived project memory."
	defaultNoProjectBelongs = "Short factual questions, quick translations, and standalone instructions with no continuing context."
)

var projectSlugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

type Manager struct {
	dataDir     string
	projectsDir string
}

func NewManager(dataDir, projectsDir string) *Manager {
	return &Manager{
		dataDir:     filepath.Clean(dataDir),
		projectsDir: filepath.Clean(projectsDir),
	}
}

func (m *Manager) SelectionRoot() string {
	return m.dataDir
}

func (m *Manager) ProjectsDir() string {
	return m.projectsDir
}

func (m *Manager) EnsureBaseScaffold() error {
	if err := os.MkdirAll(m.projectsDir, 0o755); err != nil {
		return fmt.Errorf("create projects dir: %w", err)
	}
	_, err := m.EnsureProject(assistant.ProjectContext{
		Slug:        DefaultProjectSlug,
		Name:        defaultNoProjectName,
		Description: defaultNoProjectPurpose,
	})
	return err
}

func (m *Manager) EnsureProject(project assistant.ProjectContext) (assistant.ProjectContext, error) {
	if m == nil {
		return assistant.ProjectContext{}, errors.New("project manager is required")
	}
	slug := strings.TrimSpace(project.Slug)
	if slug == "" {
		return assistant.ProjectContext{}, errors.New("project slug is required")
	}
	if !projectSlugPattern.MatchString(slug) || strings.Contains(slug, "..") || strings.ContainsAny(slug, `/\`) {
		return assistant.ProjectContext{}, fmt.Errorf("invalid project slug %q", slug)
	}

	dir := filepath.Join(m.projectsDir, slug)
	project.Slug = slug
	project.Name = firstNonEmpty(project.Name, titleForSlug(slug))
	project.Description = firstNonEmpty(project.Description, defaultDescriptionForSlug(slug))
	project.WorkspaceDir = dir
	project.BrowserProfileDir = filepath.Join(dir, browserProfileDirName)
	if project.BrowserCDPPort <= 0 {
		project.BrowserCDPPort = defaultBrowserCDPPort
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return assistant.ProjectContext{}, fmt.Errorf("create project dir %s: %w", dir, err)
	}
	if err := os.MkdirAll(project.BrowserProfileDir, 0o755); err != nil {
		return assistant.ProjectContext{}, fmt.Errorf("create browser profile dir %s: %w", project.BrowserProfileDir, err)
	}

	projectFile := filepath.Join(dir, projectFileName)
	if _, err := os.Stat(projectFile); errors.Is(err, os.ErrNotExist) {
		if writeErr := os.WriteFile(projectFile, []byte(renderProjectMarkdown(project)), 0o644); writeErr != nil {
			return assistant.ProjectContext{}, fmt.Errorf("write project file %s: %w", projectFile, writeErr)
		}
	} else if err != nil {
		return assistant.ProjectContext{}, fmt.Errorf("stat project file %s: %w", projectFile, err)
	}

	return project, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func titleForSlug(slug string) string {
	if slug == DefaultProjectSlug {
		return defaultNoProjectName
	}
	parts := strings.FieldsFunc(slug, func(r rune) bool {
		return r == '-' || r == '_'
	})
	for idx := range parts {
		if parts[idx] == "" {
			continue
		}
		parts[idx] = strings.ToUpper(parts[idx][:1]) + parts[idx][1:]
	}
	return strings.Join(parts, " ")
}

func defaultDescriptionForSlug(slug string) string {
	if slug == DefaultProjectSlug {
		return defaultNoProjectPurpose
	}
	return "Track repeated work of the same kind in one persistent project workspace."
}

func renderProjectMarkdown(project assistant.ProjectContext) string {
	belongs := defaultNoProjectBelongs
	if project.Slug != DefaultProjectSlug {
		belongs = "Requests that clearly continue this same line of work should stay in this project."
	}
	return strings.TrimSpace(fmt.Sprintf(`
# Project: %s

## Purpose
%s

## Belongs Here
%s
`, strings.TrimSpace(project.Name), strings.TrimSpace(project.Description), belongs)) + "\n"
}
