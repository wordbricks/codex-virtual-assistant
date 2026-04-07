package wiki

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
	"github.com/siisee11/CodexVirtualAssistant/internal/store"
)

const (
	defaultProjectSlug    = "no_project"
	wikiDirName           = "wiki"
	rawDirName            = "raw"
	importsDirName        = "imports"
	attachmentsDirName    = "attachments"
	indexFileName         = "index.md"
	logFileName           = "log.md"
	overviewFileName      = "overview.md"
	openQuestionsFileName = "open-questions.md"
)

var (
	ErrWikiDisabled = errors.New("wiki: project wiki is disabled")
	linkPattern     = regexp.MustCompile(`\[[^\]]+\]\(([^)#]+?\.md)\)`)
	slugSanitizer   = regexp.MustCompile(`[^a-z0-9]+`)
)

type Service struct {
	projectsDir string
	now         func() time.Time
}

type Page struct {
	Meta    assistant.WikiPageMeta `json:"meta"`
	Path    string                 `json:"path"`
	Content string                 `json:"content"`
}

type ProjectSummary struct {
	Slug          string    `json:"slug"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	WorkspaceDir  string    `json:"workspace_dir"`
	WikiEnabled   bool      `json:"wiki_enabled"`
	WikiPageCount int       `json:"wiki_page_count"`
	UpdatedAt     time.Time `json:"updated_at,omitempty"`
}

type IngestResult struct {
	ChangedPages []string `json:"changed_pages"`
	Summary      string   `json:"summary"`
}

type LintFinding struct {
	Severity string `json:"severity"`
	PagePath string `json:"page_path,omitempty"`
	Message  string `json:"message"`
}

type LintReport struct {
	ReportPath string        `json:"report_path"`
	Findings   []LintFinding `json:"findings"`
}

type pageMeta struct {
	Title      string
	PageType   string
	UpdatedAt  string
	Status     string
	Confidence string
	SourceRefs []string
	Related    []string
}

type pageData struct {
	Meta assistant.WikiPageMeta
	Body string
}

func NewService(projectsDir string, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{
		projectsDir: filepath.Clean(projectsDir),
		now:         now,
	}
}

func (s *Service) EnsureProjectScaffold(projectCtx assistant.ProjectContext) error {
	if !wikiEnabled(projectCtx) {
		return nil
	}
	projectCtx.WikiDir = wikiDir(projectCtx)

	dirs := []string{
		projectCtx.WikiDir,
		filepath.Join(projectCtx.WikiDir, "entities"),
		filepath.Join(projectCtx.WikiDir, "topics"),
		filepath.Join(projectCtx.WikiDir, "decisions"),
		filepath.Join(projectCtx.WikiDir, "reports"),
		filepath.Join(projectCtx.WikiDir, "sources"),
		filepath.Join(projectCtx.WikiDir, "playbooks"),
		filepath.Join(projectCtx.WorkspaceDir, rawDirName, importsDirName),
		filepath.Join(projectCtx.WorkspaceDir, rawDirName, attachmentsDirName),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create wiki dir %s: %w", dir, err)
		}
	}

	now := s.now().UTC()
	initial := []struct {
		path string
		body string
	}{
		{
			path: filepath.Join(projectCtx.WikiDir, overviewFileName),
			body: renderPage(
				pageMeta{
					Title:      "Project Overview",
					PageType:   "overview",
					UpdatedAt:  now.Format(time.RFC3339),
					Status:     "active",
					Confidence: "medium",
					SourceRefs: []string{"PROJECT.md"},
					Related:    []string{indexFileName, openQuestionsFileName},
				},
				fmt.Sprintf(
					"# %s\n\n%s\n\n## Memory Scope\nThis wiki tracks durable project knowledge, recent progress, and follow-up questions for `%s`.\n",
					firstNonEmpty(projectCtx.Name, projectCtx.Slug),
					firstNonEmpty(projectCtx.Description, "Project memory has not been synthesized yet."),
					projectCtx.Slug,
				),
			),
		},
		{
			path: filepath.Join(projectCtx.WikiDir, indexFileName),
			body: renderPage(
				pageMeta{
					Title:      "Wiki Index",
					PageType:   "overview",
					UpdatedAt:  now.Format(time.RFC3339),
					Status:     "active",
					Confidence: "high",
					SourceRefs: []string{"PROJECT.md"},
					Related:    []string{overviewFileName, logFileName, openQuestionsFileName},
				},
				"# Wiki Index\n\n## Core\n- [Project Overview](overview.md): High-level project summary.\n- [Run Log](log.md): Chronological history of wiki updates.\n- [Open Questions](open-questions.md): Gaps, follow-ups, and uncertainties.\n",
			),
		},
		{
			path: filepath.Join(projectCtx.WikiDir, logFileName),
			body: renderPage(
				pageMeta{
					Title:      "Run Log",
					PageType:   "report",
					UpdatedAt:  now.Format(time.RFC3339),
					Status:     "active",
					Confidence: "high",
					SourceRefs: []string{"PROJECT.md"},
					Related:    []string{overviewFileName, indexFileName},
				},
				"# Run Log\n\nNo runs have been ingested yet.\n",
			),
		},
		{
			path: filepath.Join(projectCtx.WikiDir, openQuestionsFileName),
			body: renderPage(
				pageMeta{
					Title:      "Open Questions",
					PageType:   "question",
					UpdatedAt:  now.Format(time.RFC3339),
					Status:     "active",
					Confidence: "medium",
					SourceRefs: []string{"PROJECT.md"},
					Related:    []string{overviewFileName, indexFileName},
				},
				"# Open Questions\n\n- No open questions recorded yet.\n",
			),
		},
	}
	for _, file := range initial {
		if _, err := os.Stat(file.path); errors.Is(err, os.ErrNotExist) {
			if writeErr := os.WriteFile(file.path, []byte(file.body), 0o644); writeErr != nil {
				return fmt.Errorf("write wiki scaffold file %s: %w", file.path, writeErr)
			}
		} else if err != nil {
			return fmt.Errorf("stat wiki scaffold file %s: %w", file.path, err)
		}
	}
	return nil
}

func (s *Service) LoadContext(projectCtx assistant.ProjectContext) (assistant.WikiContext, error) {
	if !wikiEnabled(projectCtx) {
		return assistant.WikiContext{}, nil
	}
	projectCtx.WikiDir = wikiDir(projectCtx)
	if err := s.EnsureProjectScaffold(projectCtx); err != nil {
		return assistant.WikiContext{}, err
	}

	overview, _ := s.readPageFromProject(projectCtx, overviewFileName)
	indexPage, _ := s.readPageFromProject(projectCtx, indexFileName)
	openQuestions, _ := s.readPageFromProject(projectCtx, openQuestionsFileName)
	logPage, _ := s.readPageFromProject(projectCtx, logFileName)

	pages, err := s.listPages(projectCtx)
	if err != nil {
		return assistant.WikiContext{}, err
	}

	context := assistant.WikiContext{
		Enabled:              true,
		OverviewSummary:      summarizeWikiText(overview.Content, 700),
		IndexSummary:         summarizeWikiText(indexPage.Content, 900),
		OpenQuestionsSummary: summarizeWikiText(openQuestions.Content, 500),
		RecentLogEntries:     extractRecentLogEntries(logPage.Content, 4),
	}
	for _, page := range pages {
		if page.Path == indexFileName || page.Path == overviewFileName || page.Path == logFileName || page.Path == openQuestionsFileName {
			continue
		}
		context.RelevantPages = append(context.RelevantPages, page.Meta)
		if len(context.RelevantPages) >= 6 {
			break
		}
	}
	return context, nil
}

func (s *Service) IngestRun(record store.RunRecord) (IngestResult, error) {
	projectCtx := record.Run.Project
	if !wikiEnabled(projectCtx) {
		return IngestResult{}, nil
	}
	projectCtx.WikiDir = wikiDir(projectCtx)
	if err := s.EnsureProjectScaffold(projectCtx); err != nil {
		return IngestResult{}, err
	}

	now := s.now().UTC()
	summary := runSummary(record)
	reportRelPath := filepath.ToSlash(filepath.Join("reports", fmt.Sprintf("run-%s.md", record.Run.ID)))
	reportPath := filepath.Join(projectCtx.WikiDir, filepath.FromSlash(reportRelPath))
	reportContent := renderRunReport(record, reportRelPath, now)
	if err := os.WriteFile(reportPath, []byte(reportContent), 0o644); err != nil {
		return IngestResult{}, fmt.Errorf("write run report page: %w", err)
	}

	changedPages := []string{reportRelPath}

	topicRelPath := ""
	if topicSlug := topicSlugForRun(record.Run); topicSlug != "" {
		topicRelPath = filepath.ToSlash(filepath.Join("topics", topicSlug+".md"))
		topicPath := filepath.Join(projectCtx.WikiDir, filepath.FromSlash(topicRelPath))
		topicContent, err := s.renderTopicPage(projectCtx, record, topicRelPath, now)
		if err != nil {
			return IngestResult{}, err
		}
		if err := os.WriteFile(topicPath, []byte(topicContent), 0o644); err != nil {
			return IngestResult{}, fmt.Errorf("write topic page: %w", err)
		}
		changedPages = append(changedPages, topicRelPath)
	}

	if err := s.updateLogPage(projectCtx, record, changedPages, now); err != nil {
		return IngestResult{}, err
	}
	changedPages = append(changedPages, logFileName)

	if err := s.rebuildOverviewPage(projectCtx, now); err != nil {
		return IngestResult{}, err
	}
	changedPages = append(changedPages, overviewFileName)

	if err := s.rebuildIndexPage(projectCtx, now); err != nil {
		return IngestResult{}, err
	}
	changedPages = append(changedPages, indexFileName)

	return IngestResult{
		ChangedPages: dedupeStrings(changedPages),
		Summary:      firstNonEmpty(summary, "Project wiki updated."),
	}, nil
}

func (s *Service) LintProject(projectCtx assistant.ProjectContext) (LintReport, error) {
	if !wikiEnabled(projectCtx) {
		return LintReport{}, ErrWikiDisabled
	}
	projectCtx.WikiDir = wikiDir(projectCtx)
	if err := s.EnsureProjectScaffold(projectCtx); err != nil {
		return LintReport{}, err
	}

	pages, err := s.listPageData(projectCtx)
	if err != nil {
		return LintReport{}, err
	}
	inbound := inboundLinks(pages)
	findings := make([]LintFinding, 0)
	for _, page := range pages {
		if isCorePage(page.Meta.Path) {
			continue
		}
		if len(page.Meta.SourceRefs) == 0 {
			findings = append(findings, LintFinding{
				Severity: "warning",
				PagePath: page.Meta.Path,
				Message:  "Page has no source_refs entries in frontmatter.",
			})
		}
		if inbound[page.Meta.Path] == 0 {
			findings = append(findings, LintFinding{
				Severity: "warning",
				PagePath: page.Meta.Path,
				Message:  "Page is orphaned outside of the generated index and overview pages.",
			})
		}
		bodyLower := strings.ToLower(page.Body)
		switch {
		case strings.Contains(page.Body, "⚠️ CONFLICT"):
			findings = append(findings, LintFinding{
				Severity: "info",
				PagePath: page.Meta.Path,
				Message:  "Conflict note present; manual reconciliation may still be required.",
			})
		case strings.Contains(bodyLower, "stale") || page.Meta.Status == "stale":
			findings = append(findings, LintFinding{
				Severity: "info",
				PagePath: page.Meta.Path,
				Message:  "Page appears stale and should be reviewed against newer run evidence.",
			})
		}
	}
	if len(findings) == 0 {
		findings = append(findings, LintFinding{
			Severity: "info",
			Message:  "No structural wiki issues were detected in this lint pass.",
		})
	}

	now := s.now().UTC()
	reportRelPath := filepath.ToSlash(filepath.Join("reports", fmt.Sprintf("wiki-health-%s.md", now.Format("2006-01-02"))))
	reportPath := filepath.Join(projectCtx.WikiDir, filepath.FromSlash(reportRelPath))
	if err := os.WriteFile(reportPath, []byte(renderLintReport(projectCtx, findings, now)), 0o644); err != nil {
		return LintReport{}, fmt.Errorf("write lint report: %w", err)
	}
	if err := s.updateOpenQuestionsPage(projectCtx, findings, reportRelPath, now); err != nil {
		return LintReport{}, err
	}
	if err := s.rebuildIndexPage(projectCtx, now); err != nil {
		return LintReport{}, err
	}
	if err := s.rebuildOverviewPage(projectCtx, now); err != nil {
		return LintReport{}, err
	}

	return LintReport{
		ReportPath: reportRelPath,
		Findings:   findings,
	}, nil
}

func (s *Service) ListProjects() ([]ProjectSummary, error) {
	entries, err := os.ReadDir(s.projectsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	summaries := make([]ProjectSummary, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		slug := entry.Name()
		workspaceDir := filepath.Join(s.projectsDir, slug)
		projectFile := filepath.Join(workspaceDir, "PROJECT.md")
		name, description := slug, ""
		if content, readErr := os.ReadFile(projectFile); readErr == nil {
			name, description = parseProjectMarkdown(string(content), slug)
		}
		wikiDir := filepath.Join(workspaceDir, wikiDirName)
		pageCount, updatedAt := countMarkdownPages(wikiDir)
		summaries = append(summaries, ProjectSummary{
			Slug:          slug,
			Name:          name,
			Description:   description,
			WorkspaceDir:  workspaceDir,
			WikiEnabled:   slug != defaultProjectSlug && pageCount > 0,
			WikiPageCount: pageCount,
			UpdatedAt:     updatedAt,
		})
	}
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].Slug < summaries[j].Slug
	})
	return summaries, nil
}

func (s *Service) ReadIndex(slug string) (Page, error) {
	return s.ReadPage(slug, indexFileName)
}

func (s *Service) ReadPage(slug, relPath string) (Page, error) {
	projectCtx, err := s.projectBySlug(slug)
	if err != nil {
		return Page{}, err
	}
	return s.readPageFromProject(projectCtx, relPath)
}

func (s *Service) projectBySlug(slug string) (assistant.ProjectContext, error) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return assistant.ProjectContext{}, errors.New("wiki: project slug is required")
	}
	projectCtx := assistant.ProjectContext{
		Slug:         slug,
		Name:         slug,
		WorkspaceDir: filepath.Join(s.projectsDir, slug),
		WikiDir:      filepath.Join(s.projectsDir, slug, wikiDirName),
	}
	if _, err := os.Stat(projectCtx.WorkspaceDir); err != nil {
		return assistant.ProjectContext{}, err
	}
	if slug == defaultProjectSlug {
		return projectCtx, ErrWikiDisabled
	}
	return projectCtx, nil
}

func (s *Service) readPageFromProject(projectCtx assistant.ProjectContext, relPath string) (Page, error) {
	if !wikiEnabled(projectCtx) {
		return Page{}, ErrWikiDisabled
	}
	cleanRelPath, err := sanitizeWikiRelativePath(relPath)
	if err != nil {
		return Page{}, err
	}
	fullPath := filepath.Join(wikiDir(projectCtx), filepath.FromSlash(cleanRelPath))
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return Page{}, err
	}
	meta, body := parsePage(string(content))
	return Page{
		Meta: pageMetaToAssistant(meta, cleanRelPath),
		Path: cleanRelPath,
		Content: strings.TrimSpace(strings.Join([]string{
			renderFrontmatter(meta),
			strings.TrimSpace(body),
		}, "\n")),
	}, nil
}

func (s *Service) renderTopicPage(projectCtx assistant.ProjectContext, record store.RunRecord, relPath string, now time.Time) (string, error) {
	title := firstNonEmpty(record.Run.TaskSpec.Goal, record.Run.UserRequestRaw, relPath)
	existing, err := s.readPageFromProject(projectCtx, relPath)
	body := ""
	meta := pageMeta{
		Title:      title,
		PageType:   "topic",
		UpdatedAt:  now.Format(time.RFC3339),
		Status:     "active",
		Confidence: confidenceForRun(record),
		SourceRefs: collectSourceRefs(record),
		Related:    []string{filepath.ToSlash(filepath.Join("reports", fmt.Sprintf("run-%s.md", record.Run.ID)))},
	}
	if err == nil {
		existingMeta, existingBody := parsePage(existing.Content)
		meta = mergePageMeta(existingMeta, meta)
		body = strings.TrimSpace(existingBody)
	}

	section := renderTopicSection(record)
	if body == "" {
		body = fmt.Sprintf("# %s\n\n%s", title, section)
	} else {
		body = fmt.Sprintf("%s\n\n%s", body, section)
	}
	return renderPage(meta, body), nil
}

func (s *Service) updateLogPage(projectCtx assistant.ProjectContext, record store.RunRecord, changedPages []string, now time.Time) error {
	logPath := filepath.Join(wikiDir(projectCtx), logFileName)
	meta, body := readPageFile(logPath)
	if meta.Title == "" {
		meta = pageMeta{
			Title:      "Run Log",
			PageType:   "report",
			Status:     "active",
			Confidence: "high",
			SourceRefs: []string{"PROJECT.md"},
		}
	}
	meta.UpdatedAt = now.Format(time.RFC3339)
	meta.Related = dedupeStrings(append(meta.Related, overviewFileName, indexFileName))
	entry := renderLogEntry(record, changedPages, now)
	trimmedBody := strings.TrimSpace(body)
	if trimmedBody == "" || trimmedBody == "# Run Log" {
		trimmedBody = "# Run Log"
	}
	if strings.Contains(trimmedBody, "No runs have been ingested yet.") {
		trimmedBody = "# Run Log"
	}
	updatedBody := fmt.Sprintf("%s\n\n%s", trimmedBody, entry)
	return os.WriteFile(logPath, []byte(renderPage(meta, updatedBody)), 0o644)
}

func (s *Service) rebuildOverviewPage(projectCtx assistant.ProjectContext, now time.Time) error {
	pages, err := s.listPages(projectCtx)
	if err != nil {
		return err
	}
	logPage, _ := s.readPageFromProject(projectCtx, logFileName)
	openQuestionsPage, _ := s.readPageFromProject(projectCtx, openQuestionsFileName)

	projectTitle := firstNonEmpty(projectCtx.Name, projectCtx.Slug)
	body := &strings.Builder{}
	fmt.Fprintf(body, "# %s\n\n", projectTitle)
	fmt.Fprintf(body, "%s\n\n", firstNonEmpty(projectCtx.Description, "Project memory is active."))
	fmt.Fprintf(body, "## Snapshot\n")
	fmt.Fprintf(body, "- Wiki pages tracked: %d\n", len(pages))
	fmt.Fprintf(body, "- Reports available: %d\n", countPagesByType(pages, "report"))
	fmt.Fprintf(body, "- Topics available: %d\n", countPagesByType(pages, "topic"))
	fmt.Fprintf(body, "- Decisions available: %d\n", countPagesByType(pages, "decision"))
	fmt.Fprintf(body, "\n## Recent Log Entries\n")
	for _, entry := range extractRecentLogEntries(logPage.Content, 4) {
		fmt.Fprintf(body, "- %s\n", entry)
	}
	if strings.TrimSpace(openQuestionsPage.Content) != "" {
		fmt.Fprintf(body, "\n## Open Questions Snapshot\n%s\n", summarizeWikiText(openQuestionsPage.Content, 500))
	}

	meta := pageMeta{
		Title:      "Project Overview",
		PageType:   "overview",
		UpdatedAt:  now.Format(time.RFC3339),
		Status:     "active",
		Confidence: "medium",
		SourceRefs: []string{"PROJECT.md", logFileName},
		Related:    []string{indexFileName, openQuestionsFileName},
	}
	return os.WriteFile(filepath.Join(wikiDir(projectCtx), overviewFileName), []byte(renderPage(meta, body.String())), 0o644)
}

func (s *Service) rebuildIndexPage(projectCtx assistant.ProjectContext, now time.Time) error {
	pages, err := s.listPages(projectCtx)
	if err != nil {
		return err
	}

	grouped := make(map[string][]assistant.WikiPageMeta)
	for _, page := range pages {
		grouped[page.Meta.PageType] = append(grouped[page.Meta.PageType], page.Meta)
	}

	order := []string{"overview", "topic", "entity", "decision", "playbook", "source", "question", "report"}
	body := &strings.Builder{}
	fmt.Fprintf(body, "# Wiki Index\n\n")
	for _, pageType := range order {
		items := grouped[pageType]
		if len(items) == 0 {
			continue
		}
		sort.Slice(items, func(i, j int) bool {
			return items[i].Path < items[j].Path
		})
		fmt.Fprintf(body, "## %s\n", strings.Title(pageType))
		for _, item := range items {
			description := firstNonEmpty(item.Status, item.Confidence, "tracked page")
			fmt.Fprintf(body, "- [%s](%s): %s\n", firstNonEmpty(item.Title, item.Path), item.Path, description)
		}
		fmt.Fprintln(body)
	}

	meta := pageMeta{
		Title:      "Wiki Index",
		PageType:   "overview",
		UpdatedAt:  now.Format(time.RFC3339),
		Status:     "active",
		Confidence: "high",
		SourceRefs: []string{"PROJECT.md", logFileName},
		Related:    []string{overviewFileName, logFileName, openQuestionsFileName},
	}
	return os.WriteFile(filepath.Join(wikiDir(projectCtx), indexFileName), []byte(renderPage(meta, body.String())), 0o644)
}

func (s *Service) updateOpenQuestionsPage(projectCtx assistant.ProjectContext, findings []LintFinding, reportPath string, now time.Time) error {
	meta := pageMeta{
		Title:      "Open Questions",
		PageType:   "question",
		UpdatedAt:  now.Format(time.RFC3339),
		Status:     "active",
		Confidence: "medium",
		SourceRefs: []string{reportPath},
		Related:    []string{overviewFileName, indexFileName, reportPath},
	}
	body := &strings.Builder{}
	fmt.Fprintf(body, "# Open Questions\n\n")
	fmt.Fprintf(body, "Latest lint report: [%s](%s)\n\n", filepath.Base(reportPath), reportPath)
	for _, finding := range findings {
		fmt.Fprintf(body, "- [%s] %s", strings.ToUpper(finding.Severity), finding.Message)
		if finding.PagePath != "" {
			fmt.Fprintf(body, " (%s)", finding.PagePath)
		}
		fmt.Fprintln(body)
	}
	return os.WriteFile(filepath.Join(wikiDir(projectCtx), openQuestionsFileName), []byte(renderPage(meta, body.String())), 0o644)
}

func (s *Service) listPages(projectCtx assistant.ProjectContext) ([]Page, error) {
	pageDataList, err := s.listPageData(projectCtx)
	if err != nil {
		return nil, err
	}
	pages := make([]Page, 0, len(pageDataList))
	for _, page := range pageDataList {
		pages = append(pages, Page{
			Meta: page.Meta,
			Path: page.Meta.Path,
			Content: strings.TrimSpace(strings.Join([]string{
				renderFrontmatter(pageMeta{
					Title:      page.Meta.Title,
					PageType:   page.Meta.PageType,
					UpdatedAt:  page.Meta.UpdatedAt,
					Status:     page.Meta.Status,
					Confidence: page.Meta.Confidence,
					SourceRefs: page.Meta.SourceRefs,
					Related:    page.Meta.Related,
				}),
				strings.TrimSpace(page.Body),
			}, "\n")),
		})
	}
	sort.Slice(pages, func(i, j int) bool {
		return pages[i].Path < pages[j].Path
	})
	return pages, nil
}

func (s *Service) listPageData(projectCtx assistant.ProjectContext) ([]pageData, error) {
	root := wikiDir(projectCtx)
	pages := make([]pageData, 0)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".md" {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		meta, body := parsePage(string(content))
		relPath, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		pages = append(pages, pageData{
			Meta: pageMetaToAssistant(meta, filepath.ToSlash(relPath)),
			Body: body,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(pages, func(i, j int) bool {
		return pages[i].Meta.Path < pages[j].Meta.Path
	})
	return pages, nil
}

func renderRunReport(record store.RunRecord, reportPath string, now time.Time) string {
	meta := pageMeta{
		Title:      fmt.Sprintf("Run %s Report", record.Run.ID),
		PageType:   "report",
		UpdatedAt:  now.Format(time.RFC3339),
		Status:     strings.ToLower(string(record.Run.Status)),
		Confidence: confidenceForRun(record),
		SourceRefs: collectSourceRefs(record),
		Related:    relatedRefsForRun(record),
	}

	body := &strings.Builder{}
	fmt.Fprintf(body, "# Run %s\n\n", record.Run.ID)
	fmt.Fprintf(body, "- Status: `%s`\n", record.Run.Status)
	fmt.Fprintf(body, "- Phase: `%s`\n", record.Run.Phase)
	if !record.Run.CreatedAt.IsZero() {
		fmt.Fprintf(body, "- Created: %s\n", record.Run.CreatedAt.UTC().Format(time.RFC3339))
	}
	if record.Run.CompletedAt != nil {
		fmt.Fprintf(body, "- Completed: %s\n", record.Run.CompletedAt.UTC().Format(time.RFC3339))
	}
	if goal := strings.TrimSpace(record.Run.TaskSpec.Goal); goal != "" {
		fmt.Fprintf(body, "- Goal: %s\n", goal)
	}
	fmt.Fprintf(body, "\n## User Request\n%s\n", strings.TrimSpace(record.Run.UserRequestRaw))
	if summary := strings.TrimSpace(runSummary(record)); summary != "" {
		fmt.Fprintf(body, "\n## Outcome Summary\n%s\n", summary)
	}
	if len(record.Artifacts) > 0 {
		fmt.Fprintf(body, "\n## Artifacts\n")
		for _, artifact := range record.Artifacts {
			fmt.Fprintf(body, "- [%s] %s", artifact.Kind, firstNonEmpty(artifact.Title, artifact.ID))
			if source := firstNonEmpty(artifact.SourceURL, artifact.Path); source != "" {
				fmt.Fprintf(body, " (%s)", source)
			}
			fmt.Fprintln(body)
		}
	}
	if len(record.Evidence) > 0 {
		fmt.Fprintf(body, "\n## Evidence Highlights\n")
		for _, evidence := range tailEvidence(record.Evidence, 8) {
			fmt.Fprintf(body, "- [%s] %s\n", evidence.Kind, firstNonEmpty(evidence.Summary, evidence.Detail))
		}
	}
	if record.Run.LatestEvaluation != nil {
		fmt.Fprintf(body, "\n## Latest Evaluation\n%s\n", strings.TrimSpace(record.Run.LatestEvaluation.Summary))
	}
	fmt.Fprintf(body, "\n## Provenance\n")
	fmt.Fprintf(body, "- Run record: `run:%s`\n", record.Run.ID)
	for _, ref := range collectSourceRefs(record) {
		fmt.Fprintf(body, "- %s\n", ref)
	}
	fmt.Fprintf(body, "\n## Related\n- [Wiki Index](../index.md)\n- [Project Overview](../overview.md)\n")
	_ = reportPath
	return renderPage(meta, body.String())
}

func renderTopicSection(record store.RunRecord) string {
	builder := &strings.Builder{}
	timestamp := record.Run.UpdatedAt
	if timestamp.IsZero() {
		timestamp = record.Run.CreatedAt
	}
	fmt.Fprintf(builder, "## Update from %s (%s)\n\n", timestamp.UTC().Format("2006-01-02"), record.Run.ID)
	fmt.Fprintf(builder, "%s\n", firstNonEmpty(runSummary(record), "Run completed without a synthesized summary."))
	if len(record.Evidence) > 0 {
		fmt.Fprintf(builder, "\n### Evidence Highlights\n")
		for _, evidence := range tailEvidence(record.Evidence, 5) {
			fmt.Fprintf(builder, "- %s\n", firstNonEmpty(evidence.Summary, evidence.Detail))
		}
	}
	if len(record.Artifacts) > 0 {
		fmt.Fprintf(builder, "\n### Artifacts\n")
		for _, artifact := range tailArtifacts(record.Artifacts, 4) {
			fmt.Fprintf(builder, "- %s\n", firstNonEmpty(artifact.Title, artifact.ID))
		}
	}
	return strings.TrimSpace(builder.String())
}

func renderLintReport(projectCtx assistant.ProjectContext, findings []LintFinding, now time.Time) string {
	meta := pageMeta{
		Title:      fmt.Sprintf("Wiki Health %s", now.Format("2006-01-02")),
		PageType:   "report",
		UpdatedAt:  now.Format(time.RFC3339),
		Status:     "active",
		Confidence: "medium",
		SourceRefs: []string{"wiki:index", "wiki:pages"},
		Related:    []string{openQuestionsFileName, indexFileName, overviewFileName},
	}
	body := &strings.Builder{}
	fmt.Fprintf(body, "# Wiki Health Report\n\n")
	fmt.Fprintf(body, "- Project: `%s`\n", projectCtx.Slug)
	fmt.Fprintf(body, "- Generated: %s\n\n", now.Format(time.RFC3339))
	for _, finding := range findings {
		fmt.Fprintf(body, "## %s\n\n", strings.ToUpper(finding.Severity))
		fmt.Fprintf(body, "%s\n", finding.Message)
		if finding.PagePath != "" {
			fmt.Fprintf(body, "\nAffected page: `%s`\n", finding.PagePath)
		}
		fmt.Fprintln(body)
	}
	return renderPage(meta, body.String())
}

func renderLogEntry(record store.RunRecord, changedPages []string, now time.Time) string {
	builder := &strings.Builder{}
	fmt.Fprintf(builder, "## %s | %s\n\n", now.Format("2006-01-02"), record.Run.ID)
	fmt.Fprintf(builder, "- Status: `%s`\n", record.Run.Status)
	fmt.Fprintf(builder, "- Summary: %s\n", firstNonEmpty(runSummary(record), "No summary available."))
	if len(changedPages) > 0 {
		fmt.Fprintf(builder, "- Changed pages: %s\n", strings.Join(changedPages, ", "))
	}
	return strings.TrimSpace(builder.String())
}

func renderPage(meta pageMeta, body string) string {
	return strings.TrimSpace(renderFrontmatter(meta)+"\n"+strings.TrimSpace(body)) + "\n"
}

func renderFrontmatter(meta pageMeta) string {
	builder := &strings.Builder{}
	fmt.Fprintln(builder, "---")
	fmt.Fprintf(builder, "title: %s\n", yamlScalar(meta.Title))
	fmt.Fprintf(builder, "page_type: %s\n", yamlScalar(meta.PageType))
	fmt.Fprintf(builder, "updated_at: %s\n", yamlScalar(meta.UpdatedAt))
	fmt.Fprintf(builder, "status: %s\n", yamlScalar(meta.Status))
	fmt.Fprintf(builder, "confidence: %s\n", yamlScalar(meta.Confidence))
	fmt.Fprintf(builder, "source_refs:\n")
	for _, ref := range dedupeStrings(meta.SourceRefs) {
		fmt.Fprintf(builder, "  - %s\n", yamlScalar(ref))
	}
	fmt.Fprintf(builder, "related:\n")
	for _, ref := range dedupeStrings(meta.Related) {
		fmt.Fprintf(builder, "  - %s\n", yamlScalar(ref))
	}
	fmt.Fprintln(builder, "---")
	return builder.String()
}

func parsePage(content string) (pageMeta, string) {
	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, "---") {
		return pageMeta{}, trimmed
	}
	lines := strings.Split(trimmed, "\n")
	if len(lines) < 3 || strings.TrimSpace(lines[0]) != "---" {
		return pageMeta{}, trimmed
	}
	meta := pageMeta{}
	currentListKey := ""
	bodyStart := 0
	for i := 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "---" {
			bodyStart = i + 1
			break
		}
		if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "-\t") {
			value := strings.TrimSpace(strings.TrimPrefix(line, "- "))
			switch currentListKey {
			case "source_refs":
				meta.SourceRefs = append(meta.SourceRefs, trimYAMLScalar(value))
			case "related":
				meta.Related = append(meta.Related, trimYAMLScalar(value))
			}
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		currentListKey = ""
		switch key {
		case "title":
			meta.Title = trimYAMLScalar(value)
		case "page_type":
			meta.PageType = trimYAMLScalar(value)
		case "updated_at":
			meta.UpdatedAt = trimYAMLScalar(value)
		case "status":
			meta.Status = trimYAMLScalar(value)
		case "confidence":
			meta.Confidence = trimYAMLScalar(value)
		case "source_refs", "related":
			currentListKey = key
		}
	}
	body := ""
	if bodyStart > 0 && bodyStart < len(lines) {
		body = strings.TrimSpace(strings.Join(lines[bodyStart:], "\n"))
	}
	return meta, body
}

func sanitizeWikiRelativePath(relPath string) (string, error) {
	relPath = filepath.ToSlash(strings.TrimSpace(relPath))
	if relPath == "" {
		return "", errors.New("wiki: page path is required")
	}
	if strings.HasPrefix(relPath, "/") || strings.Contains(relPath, "..") || strings.Contains(relPath, "\\") || strings.Contains(relPath, "?") || strings.ContainsRune(relPath, 0) {
		return "", fmt.Errorf("wiki: invalid page path %q", relPath)
	}
	cleaned := pathClean(relPath)
	if cleaned == "." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("wiki: invalid page path %q", relPath)
	}
	return cleaned, nil
}

func wikiEnabled(projectCtx assistant.ProjectContext) bool {
	return strings.TrimSpace(projectCtx.Slug) != "" && projectCtx.Slug != defaultProjectSlug && strings.TrimSpace(projectCtx.WorkspaceDir) != ""
}

func wikiDir(projectCtx assistant.ProjectContext) string {
	if strings.TrimSpace(projectCtx.WikiDir) != "" {
		return filepath.Clean(projectCtx.WikiDir)
	}
	return filepath.Join(projectCtx.WorkspaceDir, wikiDirName)
}

func pageMetaToAssistant(meta pageMeta, relPath string) assistant.WikiPageMeta {
	return assistant.WikiPageMeta{
		Path:       relPath,
		Title:      firstNonEmpty(meta.Title, relPath),
		PageType:   firstNonEmpty(meta.PageType, inferPageType(relPath)),
		UpdatedAt:  meta.UpdatedAt,
		Status:     meta.Status,
		Confidence: meta.Confidence,
		SourceRefs: dedupeStrings(meta.SourceRefs),
		Related:    dedupeStrings(meta.Related),
	}
}

func inferPageType(relPath string) string {
	switch {
	case strings.HasPrefix(relPath, "topics/"):
		return "topic"
	case strings.HasPrefix(relPath, "entities/"):
		return "entity"
	case strings.HasPrefix(relPath, "decisions/"):
		return "decision"
	case strings.HasPrefix(relPath, "playbooks/"):
		return "playbook"
	case strings.HasPrefix(relPath, "sources/"):
		return "source"
	case strings.HasPrefix(relPath, "reports/"):
		return "report"
	case relPath == openQuestionsFileName:
		return "question"
	default:
		return "overview"
	}
}

func mergePageMeta(existing, incoming pageMeta) pageMeta {
	return pageMeta{
		Title:      firstNonEmpty(incoming.Title, existing.Title),
		PageType:   firstNonEmpty(incoming.PageType, existing.PageType),
		UpdatedAt:  firstNonEmpty(incoming.UpdatedAt, existing.UpdatedAt),
		Status:     firstNonEmpty(incoming.Status, existing.Status),
		Confidence: firstNonEmpty(incoming.Confidence, existing.Confidence),
		SourceRefs: dedupeStrings(append(existing.SourceRefs, incoming.SourceRefs...)),
		Related:    dedupeStrings(append(existing.Related, incoming.Related...)),
	}
}

func readPageFile(path string) (pageMeta, string) {
	content, err := os.ReadFile(path)
	if err != nil {
		return pageMeta{}, ""
	}
	return parsePage(string(content))
}

func collectSourceRefs(record store.RunRecord) []string {
	refs := []string{fmt.Sprintf("run:%s", record.Run.ID)}
	for _, artifact := range record.Artifacts {
		refs = append(refs, fmt.Sprintf("artifact:%s", artifact.ID))
		if strings.TrimSpace(artifact.SourceURL) != "" {
			refs = append(refs, artifact.SourceURL)
		}
	}
	for _, evidence := range record.Evidence {
		refs = append(refs, fmt.Sprintf("evidence:%s", evidence.ID))
	}
	return dedupeStrings(refs)
}

func relatedRefsForRun(record store.RunRecord) []string {
	related := []string{indexFileName, overviewFileName}
	if topicSlug := topicSlugForRun(record.Run); topicSlug != "" {
		related = append(related, filepath.ToSlash(filepath.Join("topics", topicSlug+".md")))
	}
	return dedupeStrings(related)
}

func confidenceForRun(record store.RunRecord) string {
	if record.Run.LatestEvaluation != nil && record.Run.LatestEvaluation.Passed {
		return "high"
	}
	if len(record.Evidence) > 0 || len(record.Artifacts) > 0 {
		return "medium"
	}
	return "low"
}

func runSummary(record store.RunRecord) string {
	if record.Run.LatestEvaluation != nil && strings.TrimSpace(record.Run.LatestEvaluation.Summary) != "" {
		return strings.TrimSpace(record.Run.LatestEvaluation.Summary)
	}
	if len(record.Attempts) > 0 {
		last := record.Attempts[len(record.Attempts)-1]
		if strings.TrimSpace(last.OutputSummary) != "" {
			return strings.TrimSpace(last.OutputSummary)
		}
	}
	if len(record.Events) > 0 {
		last := record.Events[len(record.Events)-1]
		if strings.TrimSpace(last.Summary) != "" {
			return strings.TrimSpace(last.Summary)
		}
	}
	return strings.TrimSpace(record.Run.UserRequestRaw)
}

func topicSlugForRun(run assistant.Run) string {
	base := firstNonEmpty(run.TaskSpec.Goal, run.UserRequestRaw)
	base = strings.ToLower(strings.TrimSpace(base))
	if base == "" {
		return ""
	}
	base = slugSanitizer.ReplaceAllString(base, "-")
	base = strings.Trim(base, "-")
	if len(base) > 64 {
		base = strings.Trim(base[:64], "-")
	}
	return base
}

func summarizeWikiText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return strings.TrimSpace(value[:limit]) + "..."
}

func extractRecentLogEntries(content string, limit int) []string {
	lines := strings.Split(content, "\n")
	entries := make([]string, 0, limit)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "## ") {
			entries = append(entries, strings.TrimSpace(strings.TrimPrefix(line, "## ")))
		}
	}
	if len(entries) > limit {
		entries = entries[len(entries)-limit:]
	}
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}
	return entries
}

func inboundLinks(pages []pageData) map[string]int {
	inbound := make(map[string]int)
	for _, page := range pages {
		matches := linkPattern.FindAllStringSubmatch(page.Body, -1)
		for _, match := range matches {
			if len(match) < 2 {
				continue
			}
			target := pathClean(match[1])
			if target == "" || target == "." {
				continue
			}
			if isCorePage(page.Meta.Path) {
				continue
			}
			inbound[target]++
		}
	}
	return inbound
}

func isCorePage(relPath string) bool {
	switch relPath {
	case indexFileName, logFileName, overviewFileName, openQuestionsFileName:
		return true
	default:
		return false
	}
}

func countPagesByType(pages []Page, pageType string) int {
	count := 0
	for _, page := range pages {
		if page.Meta.PageType == pageType {
			count++
		}
	}
	return count
}

func countMarkdownPages(root string) (int, time.Time) {
	count := 0
	var latest time.Time
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".md" {
			return nil
		}
		count++
		if info, statErr := d.Info(); statErr == nil && info.ModTime().After(latest) {
			latest = info.ModTime()
		}
		return nil
	})
	return count, latest
}

func parseProjectMarkdown(content, fallbackSlug string) (string, string) {
	name := fallbackSlug
	description := ""
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# Project:") {
			name = strings.TrimSpace(strings.TrimPrefix(trimmed, "# Project:"))
		}
		if trimmed == "## Purpose" && i+1 < len(lines) {
			description = strings.TrimSpace(lines[i+1])
		}
	}
	return firstNonEmpty(name, fallbackSlug), description
}

func yamlScalar(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, `"`, `'`)
	return value
}

func trimYAMLScalar(value string) string {
	return strings.Trim(strings.TrimSpace(value), `"'`)
}

func dedupeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func tailEvidence(values []assistant.Evidence, n int) []assistant.Evidence {
	if len(values) <= n {
		return values
	}
	return values[len(values)-n:]
}

func tailArtifacts(values []assistant.Artifact, n int) []assistant.Artifact {
	if len(values) <= n {
		return values
	}
	return values[len(values)-n:]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func pathClean(value string) string {
	return filepath.ToSlash(filepath.Clean(strings.TrimSpace(value)))
}
