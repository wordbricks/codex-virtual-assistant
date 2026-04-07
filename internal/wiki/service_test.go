package wiki_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
	"github.com/siisee11/CodexVirtualAssistant/internal/project"
	"github.com/siisee11/CodexVirtualAssistant/internal/store"
	"github.com/siisee11/CodexVirtualAssistant/internal/wiki"
)

func TestEnsureProjectScaffoldCreatesWikiLayout(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	manager := project.NewManager(dataDir, filepath.Join(dataDir, "projects"))
	projectCtx, err := manager.EnsureProject(assistant.ProjectContext{
		Slug:        "docs-bot",
		Name:        "Docs Bot",
		Description: "Maintain documentation workflows.",
	})
	if err != nil {
		t.Fatalf("EnsureProject() error = %v", err)
	}

	service := wiki.NewService(filepath.Join(dataDir, "projects"), time.Now)
	if err := service.EnsureProjectScaffold(projectCtx); err != nil {
		t.Fatalf("EnsureProjectScaffold() error = %v", err)
	}

	for _, relPath := range []string{
		"wiki/index.md",
		"wiki/overview.md",
		"wiki/log.md",
		"wiki/open-questions.md",
		"wiki/topics",
		"wiki/reports",
		"raw/imports",
		"raw/attachments",
	} {
		if _, err := os.Stat(filepath.Join(projectCtx.WorkspaceDir, relPath)); err != nil {
			t.Fatalf("Stat(%s) error = %v", relPath, err)
		}
	}
}

func TestIngestRunCreatesReportTopicAndIndexUpdates(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	manager := project.NewManager(dataDir, filepath.Join(dataDir, "projects"))
	projectCtx, err := manager.EnsureProject(assistant.ProjectContext{
		Slug:        "docs-bot",
		Name:        "Docs Bot",
		Description: "Maintain documentation workflows.",
	})
	if err != nil {
		t.Fatalf("EnsureProject() error = %v", err)
	}

	service := wiki.NewService(filepath.Join(dataDir, "projects"), func() time.Time {
		return time.Date(2026, time.April, 8, 10, 0, 0, 0, time.UTC)
	})
	record := store.RunRecord{
		Run: assistant.Run{
			ID:             "run_docs_1",
			Project:        projectCtx,
			UserRequestRaw: "Summarize the docs migration work.",
			TaskSpec: assistant.TaskSpec{
				Goal:           "Docs migration summary",
				UserRequestRaw: "Summarize the docs migration work.",
				Deliverables:   []string{"Summary"},
				ToolsAllowed:   []string{"agent-browser"},
				ToolsRequired:  []string{"agent-browser"},
				DoneDefinition: []string{"Produce the requested summary"},
				EvidenceRequired: []string{
					"Capture source evidence",
				},
				MaxGenerationAttempts: 1,
			},
			Status:    assistant.RunStatusCompleted,
			Phase:     assistant.RunPhaseCompleted,
			CreatedAt: time.Date(2026, time.April, 8, 9, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, time.April, 8, 10, 0, 0, 0, time.UTC),
		},
		Attempts: []assistant.Attempt{
			{Role: assistant.AttemptRoleReporter, OutputSummary: "Delivered docs migration summary."},
		},
		Artifacts: []assistant.Artifact{
			{ID: "artifact_1", Kind: assistant.ArtifactKindReport, Title: "Docs summary", SourceURL: "https://example.com/docs"},
		},
		Evidence: []assistant.Evidence{
			{ID: "evidence_1", Kind: assistant.EvidenceKindObservation, Summary: "Migration work finished for the setup guide."},
		},
		Evaluations: []assistant.Evaluation{
			{ID: "evaluation_1", Passed: true, Summary: "The docs migration summary is complete."},
		},
	}
	record.Run.LatestEvaluation = &record.Evaluations[0]

	result, err := service.IngestRun(record)
	if err != nil {
		t.Fatalf("IngestRun() error = %v", err)
	}
	if len(result.ChangedPages) < 4 {
		t.Fatalf("ChangedPages = %#v, want report/topic/index/log updates", result.ChangedPages)
	}

	reportContent, err := os.ReadFile(filepath.Join(projectCtx.WorkspaceDir, "wiki", "reports", "run-run_docs_1.md"))
	if err != nil {
		t.Fatalf("ReadFile(report) error = %v", err)
	}
	if !strings.Contains(string(reportContent), "The docs migration summary is complete.") {
		t.Fatalf("report content = %q, want evaluation summary", string(reportContent))
	}

	indexContent, err := os.ReadFile(filepath.Join(projectCtx.WorkspaceDir, "wiki", "index.md"))
	if err != nil {
		t.Fatalf("ReadFile(index) error = %v", err)
	}
	if !strings.Contains(string(indexContent), "reports/run-run_docs_1.md") {
		t.Fatalf("index content = %q, want report link", string(indexContent))
	}
}

func TestLintProjectCreatesHealthReport(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	service := wiki.NewService(filepath.Join(dataDir, "projects"), func() time.Time {
		return time.Date(2026, time.April, 8, 12, 0, 0, 0, time.UTC)
	})
	projectCtx := assistant.ProjectContext{
		Slug:         "docs-bot",
		WorkspaceDir: filepath.Join(dataDir, "projects", "docs-bot"),
		WikiDir:      filepath.Join(dataDir, "projects", "docs-bot", "wiki"),
	}
	if err := os.MkdirAll(filepath.Join(projectCtx.WikiDir, "topics"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := service.EnsureProjectScaffold(projectCtx); err != nil {
		t.Fatalf("EnsureProjectScaffold() error = %v", err)
	}

	page := `---
title: Missing refs
page_type: topic
updated_at: 2026-04-08T11:00:00Z
status: active
confidence: medium
source_refs:
related:
---
# Missing refs

This page has no provenance entries.
`
	if err := os.WriteFile(filepath.Join(projectCtx.WikiDir, "topics", "missing-refs.md"), []byte(page), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	report, err := service.LintProject(projectCtx)
	if err != nil {
		t.Fatalf("LintProject() error = %v", err)
	}
	if report.ReportPath == "" || len(report.Findings) == 0 {
		t.Fatalf("LintProject() = %#v, want findings and report path", report)
	}
	if !strings.Contains(report.ReportPath, "wiki-health-2026-04-08.md") {
		t.Fatalf("ReportPath = %q, want dated health report", report.ReportPath)
	}
}

func TestReadPageRejectsTraversal(t *testing.T) {
	t.Parallel()

	service := wiki.NewService(t.TempDir(), time.Now)
	if _, err := service.ReadPage("docs-bot", "../secrets.md"); err == nil {
		t.Fatal("ReadPage() error = nil, want invalid path rejection")
	}
}
