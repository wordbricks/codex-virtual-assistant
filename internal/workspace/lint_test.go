package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLintProjectPassesForCleanProject(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	projectDir := filepath.Join(root, "x-growth")
	for _, relPath := range []string{
		"PROJECT.md",
		"AGENTS.md",
		"raw/imports",
		"raw/attachments",
		"wiki",
		"scripts",
		"runs",
	} {
		abs := filepath.Join(projectDir, relPath)
		if filepath.Ext(relPath) == ".md" {
			if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
				t.Fatalf("MkdirAll(%s): %v", abs, err)
			}
			if err := os.WriteFile(abs, []byte("# ok\n"), 0o644); err != nil {
				t.Fatalf("WriteFile(%s): %v", abs, err)
			}
			continue
		}
		if err := os.MkdirAll(abs, 0o755); err != nil {
			t.Fatalf("MkdirAll(%s): %v", abs, err)
		}
	}

	report, err := LintProject(projectDir)
	if err != nil {
		t.Fatalf("LintProject() error = %v", err)
	}
	if len(report.Failures) != 0 {
		t.Fatalf("LintProject() failures = %#v, want none", report.Failures)
	}
}

func TestLintProjectFailsForForbiddenEntriesAndRawCode(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	projectDir := filepath.Join(root, "x-growth")
	for _, relPath := range []string{
		"PROJECT.md",
		"AGENTS.md",
		"raw/imports",
		"raw/attachments",
		"wiki",
		"scripts",
		"runs",
	} {
		abs := filepath.Join(projectDir, relPath)
		if filepath.Ext(relPath) == ".md" {
			if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
				t.Fatalf("MkdirAll(%s): %v", abs, err)
			}
			if err := os.WriteFile(abs, []byte("# ok\n"), 0o644); err != nil {
				t.Fatalf("WriteFile(%s): %v", abs, err)
			}
			continue
		}
		if err := os.MkdirAll(abs, 0o755); err != nil {
			t.Fatalf("MkdirAll(%s): %v", abs, err)
		}
	}
	if err := os.MkdirAll(filepath.Join(projectDir, ".tmp"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.tmp): %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "raw", "imports", "tmp_probe.mjs"), []byte("console.log('x')\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(raw/imports/tmp_probe.mjs): %v", err)
	}

	report, err := LintProject(projectDir)
	if err != nil {
		t.Fatalf("LintProject() error = %v", err)
	}
	if len(report.Failures) != 2 {
		t.Fatalf("LintProject() failures = %#v, want 2", report.Failures)
	}
}

func TestLintProjectFailsForProceduralTopicPages(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	projectDir := filepath.Join(root, "x-growth")
	for _, relPath := range []string{
		"PROJECT.md",
		"AGENTS.md",
		"raw/imports",
		"raw/attachments",
		"wiki/topics",
		"scripts",
		"runs",
	} {
		abs := filepath.Join(projectDir, relPath)
		if filepath.Ext(relPath) == ".md" {
			if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
				t.Fatalf("MkdirAll(%s): %v", abs, err)
			}
			if err := os.WriteFile(abs, []byte("# ok\n"), 0o644); err != nil {
				t.Fatalf("WriteFile(%s): %v", abs, err)
			}
			continue
		}
		if err := os.MkdirAll(abs, 0o755); err != nil {
			t.Fatalf("MkdirAll(%s): %v", abs, err)
		}
	}
	if err := os.WriteFile(filepath.Join(projectDir, "wiki", "topics", "review-the-specified-artifacts.md"), []byte("# bad\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(procedural topic): %v", err)
	}

	report, err := LintProject(projectDir)
	if err != nil {
		t.Fatalf("LintProject() error = %v", err)
	}
	if len(report.Failures) != 1 {
		t.Fatalf("LintProject() failures = %#v, want 1", report.Failures)
	}
	if report.Failures[0].Kind != "procedural_topic_page" {
		t.Fatalf("LintProject() kind = %q, want procedural_topic_page", report.Failures[0].Kind)
	}
}

func TestLintProjectFailsForProceduralReportPages(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	projectDir := filepath.Join(root, "x-growth")
	for _, relPath := range []string{
		"PROJECT.md",
		"AGENTS.md",
		"raw/imports",
		"raw/attachments",
		"wiki/topics",
		"wiki/reports",
		"scripts",
		"runs",
	} {
		abs := filepath.Join(projectDir, relPath)
		if filepath.Ext(relPath) == ".md" {
			if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
				t.Fatalf("MkdirAll(%s): %v", abs, err)
			}
			if err := os.WriteFile(abs, []byte("# ok\n"), 0o644); err != nil {
				t.Fatalf("WriteFile(%s): %v", abs, err)
			}
			continue
		}
		if err := os.MkdirAll(abs, 0o755); err != nil {
			t.Fatalf("MkdirAll(%s): %v", abs, err)
		}
	}
	if err := os.WriteFile(filepath.Join(projectDir, "wiki", "reports", "run-run_20260410T000000Z_example.md"), []byte("# bad\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(procedural report): %v", err)
	}

	report, err := LintProject(projectDir)
	if err != nil {
		t.Fatalf("LintProject() error = %v", err)
	}
	if len(report.Failures) != 1 {
		t.Fatalf("LintProject() failures = %#v, want 1", report.Failures)
	}
	if report.Failures[0].Kind != "procedural_report_page" {
		t.Fatalf("LintProject() kind = %q, want procedural_report_page", report.Failures[0].Kind)
	}
}

func TestLintProjectsDefaultsToAllProjectDirectories(t *testing.T) {
	t.Parallel()

	projectsDir := t.TempDir()
	for _, slug := range []string{"alpha", "beta"} {
		projectDir := filepath.Join(projectsDir, slug)
		for _, relPath := range []string{
			"PROJECT.md",
			"AGENTS.md",
			"raw/imports",
			"raw/attachments",
			"wiki",
			"scripts",
			"runs",
		} {
			abs := filepath.Join(projectDir, relPath)
			if filepath.Ext(relPath) == ".md" {
				if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
					t.Fatalf("MkdirAll(%s): %v", abs, err)
				}
				if err := os.WriteFile(abs, []byte("# ok\n"), 0o644); err != nil {
					t.Fatalf("WriteFile(%s): %v", abs, err)
				}
				continue
			}
			if err := os.MkdirAll(abs, 0o755); err != nil {
				t.Fatalf("MkdirAll(%s): %v", abs, err)
			}
		}
	}

	reports, err := LintProjects(projectsDir, nil)
	if err != nil {
		t.Fatalf("LintProjects() error = %v", err)
	}
	if len(reports) != 2 {
		t.Fatalf("LintProjects() len = %d, want 2", len(reports))
	}
	if reports[0].ProjectSlug != "alpha" || reports[1].ProjectSlug != "beta" {
		t.Fatalf("LintProjects() slugs = %#v", reports)
	}
}
