package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type LintFailure struct {
	Kind    string `json:"kind"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

type LintReport struct {
	ProjectSlug string        `json:"project_slug"`
	ProjectDir  string        `json:"project_dir"`
	Failures    []LintFailure `json:"failures"`
}

var (
	requiredPaths = []string{
		"PROJECT.md",
		"AGENTS.md",
		"raw",
		"raw/imports",
		"raw/attachments",
		"wiki",
		"scripts",
		"runs",
	}
	forbiddenRootEntries = []string{
		".agent-browser",
		".agent-browser-home",
		".agent-home",
		".agent-message",
		".agent-message-config.json",
		".agent-message-config.runtime.json",
		".agent-message-runtime.json",
		".chrome-home",
		".home",
		".home-install",
		".home-run",
		".home2",
		".home3",
		".npm-cache",
		".tmp",
		".tmp-home",
		".tmp_agent_message",
		"artifacts",
		"evidence",
		"tmp",
	}
	forbiddenRawExtensions = map[string]struct{}{
		".js":  {},
		".cjs": {},
		".mjs": {},
		".sh":  {},
		".py":  {},
		".ts":  {},
	}
	proceduralTopicPrefixes = []string{
		"continue-",
		"execute-",
		"finish-",
		"normalize-",
		"preserve-",
		"produce-",
		"re-evaluate-",
		"re-review-",
		"re-run-",
		"recover-",
		"repeat-",
		"retry-",
		"return-",
		"review-",
		"run-",
		"safely-",
		"schedule-",
		"set-the-",
	}
	proceduralReportPrefixes = []string{
		"run-run_",
	}
)

func LintProject(projectDir string) (LintReport, error) {
	projectDir = filepath.Clean(projectDir)
	info, err := os.Stat(projectDir)
	if err != nil {
		return LintReport{}, fmt.Errorf("stat project dir: %w", err)
	}
	if !info.IsDir() {
		return LintReport{}, fmt.Errorf("project path is not a directory: %s", projectDir)
	}

	report := LintReport{
		ProjectSlug: filepath.Base(projectDir),
		ProjectDir:  projectDir,
	}

	for _, relPath := range requiredPaths {
		if _, err := os.Stat(filepath.Join(projectDir, relPath)); err != nil {
			if os.IsNotExist(err) {
				report.Failures = append(report.Failures, LintFailure{
					Kind:    "missing_required_path",
					Path:    relPath,
					Message: "required project path is missing",
				})
				continue
			}
			return LintReport{}, fmt.Errorf("stat %s: %w", relPath, err)
		}
	}

	rootEntries, err := os.ReadDir(projectDir)
	if err != nil {
		return LintReport{}, fmt.Errorf("read project root: %w", err)
	}
	rootSet := make(map[string]struct{}, len(rootEntries))
	for _, entry := range rootEntries {
		rootSet[entry.Name()] = struct{}{}
	}
	for _, name := range forbiddenRootEntries {
		if _, ok := rootSet[name]; ok {
			report.Failures = append(report.Failures, LintFailure{
				Kind:    "forbidden_root_entry",
				Path:    name,
				Message: "root contains temporary or operational clutter that should live in runs/ or .cache/",
			})
		}
	}

	rawImportsDir := filepath.Join(projectDir, "raw", "imports")
	importEntries, err := os.ReadDir(rawImportsDir)
	if err == nil {
		for _, entry := range importEntries {
			if entry.IsDir() {
				continue
			}
			ext := strings.ToLower(filepath.Ext(entry.Name()))
			if _, ok := forbiddenRawExtensions[ext]; ok {
				report.Failures = append(report.Failures, LintFailure{
					Kind:    "raw_imports_contains_code",
					Path:    filepath.ToSlash(filepath.Join("raw", "imports", entry.Name())),
					Message: "executable code should live in scripts/ rather than raw/imports/",
				})
			}
		}
	} else if !os.IsNotExist(err) {
		return LintReport{}, fmt.Errorf("read raw/imports: %w", err)
	}

	topicsDir := filepath.Join(projectDir, "wiki", "topics")
	topicEntries, err := os.ReadDir(topicsDir)
	if err == nil {
		for _, entry := range topicEntries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
				continue
			}
			name := strings.ToLower(entry.Name())
			for _, prefix := range proceduralTopicPrefixes {
				if strings.HasPrefix(name, prefix) {
					report.Failures = append(report.Failures, LintFailure{
						Kind:    "procedural_topic_page",
						Path:    filepath.ToSlash(filepath.Join("wiki", "topics", entry.Name())),
						Message: "topic pages should be durable concepts, not prompt-like procedural tasks",
					})
					break
				}
			}
		}
	} else if !os.IsNotExist(err) {
		return LintReport{}, fmt.Errorf("read wiki/topics: %w", err)
	}

	reportsDir := filepath.Join(projectDir, "wiki", "reports")
	reportEntries, err := os.ReadDir(reportsDir)
	if err == nil {
		for _, entry := range reportEntries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
				continue
			}
			name := strings.ToLower(entry.Name())
			for _, prefix := range proceduralReportPrefixes {
				if strings.HasPrefix(name, prefix) {
					report.Failures = append(report.Failures, LintFailure{
						Kind:    "procedural_report_page",
						Path:    filepath.ToSlash(filepath.Join("wiki", "reports", entry.Name())),
						Message: "run-history dump pages should be archived under runs/ rather than kept as durable wiki reports",
					})
					break
				}
			}
		}
	} else if !os.IsNotExist(err) {
		return LintReport{}, fmt.Errorf("read wiki/reports: %w", err)
	}

	sort.Slice(report.Failures, func(i, j int) bool {
		if report.Failures[i].Kind == report.Failures[j].Kind {
			return report.Failures[i].Path < report.Failures[j].Path
		}
		return report.Failures[i].Kind < report.Failures[j].Kind
	})

	return report, nil
}

func LintProjects(projectsDir string, slugs []string) ([]LintReport, error) {
	projectsDir = filepath.Clean(projectsDir)
	targets := slugs
	if len(targets) == 0 {
		entries, err := os.ReadDir(projectsDir)
		if err != nil {
			return nil, fmt.Errorf("read projects dir: %w", err)
		}
		for _, entry := range entries {
			if entry.IsDir() {
				targets = append(targets, entry.Name())
			}
		}
		sort.Strings(targets)
	}

	reports := make([]LintReport, 0, len(targets))
	for _, slug := range targets {
		report, err := LintProject(filepath.Join(projectsDir, slug))
		if err != nil {
			return nil, fmt.Errorf("lint project %s: %w", slug, err)
		}
		reports = append(reports, report)
	}
	return reports, nil
}
