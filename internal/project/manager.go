package project

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
	"github.com/siisee11/CodexVirtualAssistant/internal/wiki"
)

const (
	DefaultProjectSlug      = "no_project"
	projectFileName         = "PROJECT.md"
	agentsFileName          = "AGENTS.md"
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
	if slug != DefaultProjectSlug {
		project.WikiDir = filepath.Join(dir, "wiki")
	}
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
	if slug != DefaultProjectSlug {
		agentsFile := filepath.Join(dir, agentsFileName)
		if _, err := os.Stat(agentsFile); errors.Is(err, os.ErrNotExist) {
			if writeErr := os.WriteFile(agentsFile, []byte(renderAgentsMarkdown(project)), 0o644); writeErr != nil {
				return assistant.ProjectContext{}, fmt.Errorf("write agents file %s: %w", agentsFile, writeErr)
			}
		} else if err != nil {
			return assistant.ProjectContext{}, fmt.Errorf("stat agents file %s: %w", agentsFile, err)
		}
		if err := wiki.NewService(m.projectsDir, time.Now).EnsureProjectScaffold(project); err != nil {
			return assistant.ProjectContext{}, fmt.Errorf("ensure wiki scaffold for %s: %w", slug, err)
		}
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

func renderAgentsMarkdown(project assistant.ProjectContext) string {
	return strings.TrimSpace(fmt.Sprintf(`
# AGENTS.md

Operational schema for maintaining the %q project as an LLM-managed wiki.

## Purpose

This project exists to accumulate durable knowledge and working context for this line of work over time.

Project name: %s
Project description: %s

The agent should maintain a clear separation between:

- ` + "`raw/`" + `: immutable source material
- ` + "`wiki/`" + `: durable synthesized knowledge
- ` + "`scripts/`" + `: reusable execution helpers
- ` + "`runs/`" + `: run-specific evidence and artifacts
- ` + "`.cache/`" + `: disposable runtime state

## Default Rules

- Read ` + "`PROJECT.md`" + `, ` + "`AGENTS.md`" + `, and ` + "`wiki/index.md`" + ` first when orienting.
- Prefer updating existing wiki pages over creating new ones.
- Store durable insights in ` + "`wiki/`" + `.
- Store run-specific proof and temporary outputs outside the wiki.
- Do not place executable code in ` + "`raw/`" + `.
- Do not create topic pages whose titles are just the user's full prompt.

## Required Wiki Files

The project wiki should preserve and maintain:

- ` + "`wiki/index.md`" + `
- ` + "`wiki/log.md`" + `
- ` + "`wiki/overview.md`" + `
- ` + "`wiki/open-questions.md`" + `

## Page Types

Use the wiki to maintain these durable page categories:

- ` + "`entities/`" + ` for recurring actors, accounts, audiences, tools, or named things
- ` + "`topics/`" + ` for reusable concepts and themes
- ` + "`playbooks/`" + ` for repeatable procedures
- ` + "`decisions/`" + ` for lasting policies or interpretations
- ` + "`sources/`" + ` for source sets and source summaries
- ` + "`reports/`" + ` for durable analyses worth revisiting

## Ingest Rule

When new source material arrives, the agent should update the wiki, refresh cross-references, and append a concise entry to ` + "`wiki/log.md`" + `.

## Query Rule

When answering questions, the agent should treat the wiki as the primary knowledge layer and update it when the answer creates durable value.

## Lint Rule

The agent should periodically look for duplicate pages, stale claims, weak cross-references, orphan pages, and operational material that should be moved out of the wiki.
`, project.Slug, strings.TrimSpace(project.Name), strings.TrimSpace(project.Description))) + "\n"
}
