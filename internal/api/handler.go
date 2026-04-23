package api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
	"github.com/siisee11/CodexVirtualAssistant/internal/assistantapp"
	authpkg "github.com/siisee11/CodexVirtualAssistant/internal/auth"
	"github.com/siisee11/CodexVirtualAssistant/internal/config"
	"github.com/siisee11/CodexVirtualAssistant/internal/store"
	"github.com/siisee11/CodexVirtualAssistant/internal/wiki"
	"github.com/siisee11/CodexVirtualAssistant/web"
)

type BootstrapResponse struct {
	ProductName                  string                `json:"product_name"`
	ProductTagline               string                `json:"product_tagline"`
	DefaultModel                 string                `json:"default_model"`
	DefaultMaxGenerationAttempts int                   `json:"default_max_generation_attempts"`
	AuthRequired                 bool                  `json:"auth_required"`
	RunStatuses                  []assistant.RunStatus `json:"run_statuses"`
	RunPhases                    []assistant.RunPhase  `json:"run_phases"`
	APIBasePath                  string                `json:"api_base_path"`
	ChatsPath                    string                `json:"chats_path"`
	RunsPath                     string                `json:"runs_path"`
}

type RunAPI struct {
	cfg      config.Config
	runs     *assistantapp.RunService
	events   *EventBroker
	wiki     *wiki.Service
	projects projectCreator
	static   fs.FS
}

type createRunRequest struct {
	UserRequestRaw        string `json:"user_request_raw"`
	MaxGenerationAttempts int    `json:"max_generation_attempts"`
	ParentRunID           string `json:"parent_run_id"`
	ProjectSlug           string `json:"project_slug"`
}

type createProjectRequest struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type createProjectResponse struct {
	Project wiki.ProjectSummary `json:"project"`
}

type runActionRequest struct {
	Input map[string]string `json:"input"`
}

type createScheduledRunRequest struct {
	ScheduledFor          string `json:"scheduled_for"`
	CronExpr              string `json:"cron_expr"`
	Prompt                string `json:"prompt"`
	MaxGenerationAttempts int    `json:"max_generation_attempts"`
}

type updateScheduledRunRequest struct {
	ScheduledFor          string `json:"scheduled_for"`
	CronExpr              string `json:"cron_expr"`
	Prompt                string `json:"prompt"`
	MaxGenerationAttempts int    `json:"max_generation_attempts"`
}

type createRunResponse struct {
	Run       assistant.Run `json:"run"`
	ChatURL   string        `json:"chat_url"`
	StatusURL string        `json:"status_url"`
	EventsURL string        `json:"events_url"`
}

type projectRunStats struct {
	ActiveRuns    int `json:"active_runs"`
	WaitingRuns   int `json:"waiting_runs"`
	ScheduledRuns int `json:"scheduled_runs"`
	CompletedRuns int `json:"completed_runs"`
	StoppedRuns   int `json:"stopped_runs"`
	WikiPageCount int `json:"wiki_page_count"`
}

type projectDetailResponse struct {
	Project          wiki.ProjectSummary `json:"project"`
	Stats            projectRunStats     `json:"stats"`
	RecentRuns       []assistant.Run     `json:"recent_runs"`
	LatestLogEntries []string            `json:"latest_log_entries,omitempty"`
}

type projectRunsResponse struct {
	Runs       []assistant.Run         `json:"runs"`
	Pagination assistantapp.Pagination `json:"pagination"`
	RunRecords []store.RunRecord       `json:"run_records,omitempty"`
}

type projectCreator interface {
	EnsureProject(assistant.ProjectContext) (assistant.ProjectContext, error)
}

func NewHandler(cfg config.Config, runs *assistantapp.RunService, events *EventBroker, wikiService *wiki.Service, projectCreators ...projectCreator) (http.Handler, error) {
	staticFS, err := web.StaticFS()
	if err != nil {
		return nil, err
	}
	cfg.Auth.Enabled = true
	var projects projectCreator
	if len(projectCreators) > 0 {
		projects = projectCreators[0]
	}

	api := &RunAPI{
		cfg:      cfg,
		runs:     runs,
		events:   events,
		wiki:     wikiService,
		projects: projects,
		static:   staticFS,
	}
	authManager, err := authpkg.NewManager(authpkg.Config{
		Enabled:      cfg.Auth.Enabled,
		UserID:       cfg.Auth.UserID,
		PasswordHash: cfg.Auth.PasswordHash,
		Password:     cfg.Auth.Password,
	})
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", api.handleHealth)
	mux.HandleFunc("/api/v1/bootstrap", api.handleBootstrap)
	mux.HandleFunc("/api/v1/auth/status", authManager.HandleStatus)
	mux.HandleFunc("/api/v1/auth/login", authManager.HandleLogin)
	mux.HandleFunc("/api/v1/auth/logout", authManager.HandleLogout)

	protected := http.NewServeMux()
	protected.HandleFunc("/api/v1/chats", api.handleChats)
	protected.HandleFunc("/api/v1/chats/", api.handleChatByID)
	protected.HandleFunc("/api/v1/runs", api.handleRuns)
	protected.HandleFunc("/api/v1/runs/", api.handleRunByID)
	protected.HandleFunc("/api/v1/scheduled", api.handleScheduledRuns)
	protected.HandleFunc("/api/v1/scheduled/", api.handleScheduledRunByID)
	protected.HandleFunc("/api/v1/projects", api.handleProjects)
	protected.HandleFunc("/api/v1/projects/", api.handleProjectBySlug)
	mux.Handle("/api/v1/", authManager.Require(protected))
	mux.Handle("/artifacts/", authManager.Require(http.HandlerFunc(api.handleArtifact)))
	mux.Handle("/assets/", http.StripPrefix("/", http.FileServer(http.FS(staticFS))))
	mux.Handle("/favicon.svg", http.FileServer(http.FS(staticFS)))
	mux.Handle("/logo.svg", http.FileServer(http.FS(staticFS)))
	mux.HandleFunc("/", api.serveIndex)
	return authpkg.SecurityHeaders(mux), nil
}

func (a *RunAPI) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *RunAPI) handleBootstrap(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, BootstrapResponse{
		ProductName:                  "Codex Virtual Assistant",
		ProductTagline:               "WTL GAN-policy based web personal virtual assistant",
		DefaultModel:                 a.cfg.DefaultModel,
		DefaultMaxGenerationAttempts: a.cfg.MaxGenerationAttempts,
		AuthRequired:                 a.cfg.Auth.Enabled,
		RunStatuses:                  assistant.AllRunStatuses(),
		RunPhases:                    assistant.AllRunPhases(),
		APIBasePath:                  "/api/v1",
		ChatsPath:                    "/api/v1/chats",
		RunsPath:                     "/api/v1/runs",
	})
}

func (a *RunAPI) handleChats(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		chats, err := a.runs.ListChats(r.Context())
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"chats": chats})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *RunAPI) handleChatByID(w http.ResponseWriter, r *http.Request) {
	chatID, ok := parseChatPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		record, err := a.runs.GetChatRecord(r.Context(), chatID)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		for idx := range record.Runs {
			a.enrichArtifactURLs(&record.Runs[idx])
		}
		writeJSON(w, http.StatusOK, record)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *RunAPI) handleRuns(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var request createRunRequest
		if err := decodeJSONBody(r.Body, &request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		projectSlug := strings.TrimSpace(request.ProjectSlug)
		if projectSlug != "" {
			if a.wiki == nil {
				http.Error(w, "project wiki service is not configured", http.StatusServiceUnavailable)
				return
			}
			if _, err := a.findProjectSummaryBySlug(projectSlug); err != nil {
				if errors.Is(err, store.ErrNotFound) {
					http.Error(w, fmt.Sprintf("project %q not found", projectSlug), http.StatusNotFound)
					return
				}
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		run, err := a.runs.CreateRunWithProject(r.Context(), request.UserRequestRaw, request.MaxGenerationAttempts, request.ParentRunID, projectSlug)
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, store.ErrNotFound) {
				status = http.StatusNotFound
			}
			http.Error(w, err.Error(), status)
			return
		}

		writeJSON(w, http.StatusAccepted, createRunResponse{
			Run:       run,
			ChatURL:   fmt.Sprintf("/api/v1/chats/%s", run.ChatID),
			StatusURL: fmt.Sprintf("/api/v1/runs/%s", run.ID),
			EventsURL: fmt.Sprintf("/api/v1/runs/%s/events", run.ID),
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *RunAPI) handleRunByID(w http.ResponseWriter, r *http.Request) {
	runID, action, ok := parseRunPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}

	switch {
	case action == "" && r.Method == http.MethodGet:
		record, err := a.runs.GetRunRecord(r.Context(), runID)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		a.enrichArtifactURLs(&record)
		writeJSON(w, http.StatusOK, record)
	case action == "scheduled" && r.Method == http.MethodGet:
		scheduledRuns, err := a.runs.ListScheduledRunsByParent(r.Context(), runID)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"scheduled_runs": scheduledRuns})
	case action == "scheduled" && r.Method == http.MethodPost:
		var request createScheduledRunRequest
		if err := decodeJSONBody(r.Body, &request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		scheduledRun, err := a.runs.CreateScheduledRun(r.Context(), runID, request.ScheduledFor, request.CronExpr, request.Prompt, request.MaxGenerationAttempts)
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, store.ErrNotFound) {
				status = http.StatusNotFound
			}
			http.Error(w, err.Error(), status)
			return
		}
		writeJSON(w, http.StatusCreated, scheduledRun)
	case action == "events" && r.Method == http.MethodGet:
		if err := a.streamRunEvents(w, r, runID); err != nil && !errors.Is(err, io.EOF) {
			return
		}
	case action == "input" && r.Method == http.MethodPost:
		var request runActionRequest
		if err := decodeJSONBody(r.Body, &request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := a.runs.SubmitInput(r.Context(), runID, request.Input); err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"ok": true})
	case action == "resume" && r.Method == http.MethodPost:
		var request runActionRequest
		if err := decodeJSONBody(r.Body, &request); err != nil && !errors.Is(err, io.EOF) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := a.runs.ResumeRun(r.Context(), runID, request.Input); err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"ok": true})
	case action == "cancel" && r.Method == http.MethodPost:
		if err := a.runs.CancelRun(r.Context(), runID); err != nil {
			writeStoreError(w, err)
			return
		}
		record, err := a.runs.GetRunRecord(r.Context(), runID)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		a.enrichArtifactURLs(&record)
		writeJSON(w, http.StatusOK, record)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *RunAPI) handleScheduledRuns(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		chatID := strings.TrimSpace(r.URL.Query().Get("chat_id"))
		status := assistant.ScheduledRunStatus(strings.TrimSpace(r.URL.Query().Get("status")))
		scheduledRuns, err := a.runs.ListScheduledRuns(r.Context(), chatID, status)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"scheduled_runs": scheduledRuns})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *RunAPI) handleScheduledRunByID(w http.ResponseWriter, r *http.Request) {
	scheduledRunID, action, ok := parseScheduledPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}

	switch {
	case action == "" && r.Method == http.MethodGet:
		scheduledRun, err := a.runs.GetScheduledRun(r.Context(), scheduledRunID)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, scheduledRun)
	case action == "update" && r.Method == http.MethodPost:
		var request updateScheduledRunRequest
		if err := decodeJSONBody(r.Body, &request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		scheduledRun, err := a.runs.UpdateScheduledRun(r.Context(), scheduledRunID, request.ScheduledFor, request.CronExpr, request.Prompt, request.MaxGenerationAttempts)
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, store.ErrNotFound) {
				status = http.StatusNotFound
			}
			http.Error(w, err.Error(), status)
			return
		}
		writeJSON(w, http.StatusOK, scheduledRun)
	case action == "cancel" && r.Method == http.MethodPost:
		scheduledRun, err := a.runs.CancelScheduledRun(r.Context(), scheduledRunID)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, scheduledRun)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *RunAPI) handleProjects(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if a.wiki == nil {
			http.Error(w, "project wiki service is not configured", http.StatusServiceUnavailable)
			return
		}
		projects, err := a.wiki.ListProjects()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"projects": projects})
	case http.MethodPost:
		if a.projects == nil {
			http.Error(w, "project manager is not configured", http.StatusServiceUnavailable)
			return
		}
		if a.wiki == nil {
			http.Error(w, "project wiki service is not configured", http.StatusServiceUnavailable)
			return
		}

		var request createProjectRequest
		if err := decodeJSONBody(r.Body, &request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		slug := normalizeProjectSlug(request.Slug)
		if slug == "" {
			slug = normalizeProjectSlug(request.Name)
		}
		if slug == "" {
			http.Error(w, "project slug or name is required", http.StatusBadRequest)
			return
		}
		if slug == "no_project" {
			http.Error(w, "no_project is reserved", http.StatusBadRequest)
			return
		}

		projectCtx, err := a.projects.EnsureProject(assistant.ProjectContext{
			Slug:        slug,
			Name:        strings.TrimSpace(request.Name),
			Description: strings.TrimSpace(request.Description),
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		projectSummary, err := a.findProjectSummaryBySlug(projectCtx.Slug)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusCreated, createProjectResponse{Project: projectSummary})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *RunAPI) handleProjectBySlug(w http.ResponseWriter, r *http.Request) {
	slug, action, ok := parseProjectPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if a.wiki == nil {
		http.Error(w, "project wiki service is not configured", http.StatusServiceUnavailable)
		return
	}

	switch {
	case action == "" && r.Method == http.MethodGet:
		projectSummary, err := a.findProjectSummaryBySlug(slug)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				http.NotFound(w, r)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		payload, err := a.projectDetailPayload(r.Context(), projectSummary)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, payload)
	case action == "runs" && r.Method == http.MethodGet:
		if _, err := a.findProjectSummaryBySlug(slug); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				http.NotFound(w, r)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		status := assistant.RunStatus(strings.TrimSpace(r.URL.Query().Get("status")))
		page := 0
		if rawPage := strings.TrimSpace(r.URL.Query().Get("page")); rawPage != "" {
			value, err := strconv.Atoi(rawPage)
			if err != nil {
				http.Error(w, "page must be an integer", http.StatusBadRequest)
				return
			}
			page = value
		}
		pageSize := 0
		if rawPageSize := strings.TrimSpace(r.URL.Query().Get("page_size")); rawPageSize != "" {
			value, err := strconv.Atoi(rawPageSize)
			if err != nil {
				http.Error(w, "page_size must be an integer", http.StatusBadRequest)
				return
			}
			pageSize = value
		}
		includeDetails := false
		if rawInclude := strings.TrimSpace(r.URL.Query().Get("include_details")); rawInclude != "" {
			value, err := strconv.ParseBool(rawInclude)
			if err != nil {
				http.Error(w, "include_details must be a boolean", http.StatusBadRequest)
				return
			}
			includeDetails = value
		}

		runsPage, err := a.runs.ListRunsByProjectSlug(r.Context(), slug, assistantapp.ProjectRunsQuery{
			Status:   status,
			Page:     page,
			PageSize: pageSize,
		})
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "invalid run status") {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		payload := projectRunsResponse{
			Runs:       runsPage.Runs,
			Pagination: runsPage.Pagination,
		}
		if includeDetails {
			payload.RunRecords = make([]store.RunRecord, 0, len(runsPage.Runs))
			for _, run := range runsPage.Runs {
				record, err := a.runs.GetRunRecord(r.Context(), run.ID)
				if err != nil {
					writeStoreError(w, err)
					return
				}
				a.enrichArtifactURLs(&record)
				payload.RunRecords = append(payload.RunRecords, record)
			}
		}
		writeJSON(w, http.StatusOK, payload)
	case action == "wiki/index" && r.Method == http.MethodGet:
		page, err := a.wiki.ReadIndex(slug)
		if err != nil {
			writeWikiError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, page)
	case action == "wiki/page" && r.Method == http.MethodGet:
		pagePath := strings.TrimSpace(r.URL.Query().Get("path"))
		page, err := a.wiki.ReadPage(slug, pagePath)
		if err != nil {
			writeWikiError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, page)
	case action == "wiki/pages" && r.Method == http.MethodGet:
		pages, err := a.wiki.ListPages(slug)
		if err != nil {
			writeWikiError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"pages": pages})
	case action == "wiki/lint" && r.Method == http.MethodPost:
		projectCtx := assistant.ProjectContext{
			Slug:         slug,
			WorkspaceDir: filepath.Join(a.cfg.EffectiveProjectsDir(), slug),
			WikiDir:      filepath.Join(a.cfg.EffectiveProjectsDir(), slug, "wiki"),
		}
		report, err := a.wiki.LintProject(projectCtx)
		if err != nil {
			writeWikiError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, report)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *RunAPI) findProjectSummaryBySlug(slug string) (wiki.ProjectSummary, error) {
	projects, err := a.wiki.ListProjects()
	if err != nil {
		return wiki.ProjectSummary{}, err
	}
	for _, project := range projects {
		if project.Slug == slug {
			return project, nil
		}
	}
	return wiki.ProjectSummary{}, store.ErrNotFound
}

func (a *RunAPI) projectDetailPayload(ctx context.Context, project wiki.ProjectSummary) (projectDetailResponse, error) {
	runs, err := a.runs.ListAllRunsByProjectSlug(ctx, project.Slug)
	if err != nil {
		return projectDetailResponse{}, err
	}

	stats := projectRunStats{WikiPageCount: project.WikiPageCount}
	projectRunIDs := make(map[string]struct{}, len(runs))
	for _, run := range runs {
		projectRunIDs[run.ID] = struct{}{}
		switch {
		case isActiveRunStatus(run.Status):
			stats.ActiveRuns++
		case run.Status == assistant.RunStatusWaiting:
			stats.WaitingRuns++
		case run.Status == assistant.RunStatusCompleted:
			stats.CompletedRuns++
		case isStoppedRunStatus(run.Status):
			stats.StoppedRuns++
		}
	}

	scheduledRuns, err := a.runs.ListScheduledRuns(ctx, "", assistant.ScheduledRunStatusPending)
	if err != nil {
		return projectDetailResponse{}, err
	}
	for _, scheduledRun := range scheduledRuns {
		if _, ok := projectRunIDs[scheduledRun.ParentRunID]; ok {
			stats.ScheduledRuns++
		}
	}

	sort.Slice(runs, func(i, j int) bool {
		left := runSortTimestamp(runs[i])
		right := runSortTimestamp(runs[j])
		if left.Equal(right) {
			return runs[i].ID > runs[j].ID
		}
		return left.After(right)
	})
	recentRuns := runs
	if len(recentRuns) > 5 {
		recentRuns = recentRuns[:5]
	}

	var latestLogEntries []string
	if project.WikiEnabled {
		wikiContext, err := a.wiki.LoadContext(assistant.ProjectContext{
			Slug:         project.Slug,
			Name:         project.Name,
			Description:  project.Description,
			WorkspaceDir: project.WorkspaceDir,
			WikiDir:      filepath.Join(project.WorkspaceDir, "wiki"),
		})
		if err != nil && !errors.Is(err, wiki.ErrWikiDisabled) {
			return projectDetailResponse{}, err
		}
		latestLogEntries = wikiContext.RecentLogEntries
	}

	return projectDetailResponse{
		Project:          project,
		Stats:            stats,
		RecentRuns:       recentRuns,
		LatestLogEntries: latestLogEntries,
	}, nil
}

func isActiveRunStatus(status assistant.RunStatus) bool {
	switch status {
	case assistant.RunStatusQueued,
		assistant.RunStatusGating,
		assistant.RunStatusAnswering,
		assistant.RunStatusSelectingProject,
		assistant.RunStatusPlanning,
		assistant.RunStatusContracting,
		assistant.RunStatusGenerating,
		assistant.RunStatusEvaluating,
		assistant.RunStatusScheduling,
		assistant.RunStatusWikiIngesting,
		assistant.RunStatusReporting:
		return true
	default:
		return false
	}
}

var projectSlugUnsafeChars = regexp.MustCompile(`[^a-z0-9_-]+`)

func normalizeProjectSlug(value string) string {
	slug := strings.ToLower(strings.TrimSpace(value))
	slug = projectSlugUnsafeChars.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-_")
	return slug
}

func isStoppedRunStatus(status assistant.RunStatus) bool {
	return status == assistant.RunStatusFailed || status == assistant.RunStatusExhausted || status == assistant.RunStatusCancelled
}

func runSortTimestamp(run assistant.Run) time.Time {
	if run.UpdatedAt.IsZero() {
		return run.CreatedAt
	}
	return run.UpdatedAt
}

func (a *RunAPI) enrichArtifactURLs(record *store.RunRecord) {
	for idx := range record.Artifacts {
		record.Artifacts[idx].URL = a.artifactURL(record.Artifacts[idx].Path)
	}
}

func (a *RunAPI) handleArtifact(w http.ResponseWriter, r *http.Request) {
	relPath := strings.TrimPrefix(r.URL.Path, "/artifacts/")
	absPath, ok := a.resolveArtifactPath(relPath)
	if !ok {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, absPath)
}

func (a *RunAPI) artifactURL(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	cleaned := filepath.Clean(path)
	if filepath.IsAbs(cleaned) {
		rel, ok := a.artifactRelativePath(cleaned)
		if !ok {
			return ""
		}
		cleaned = rel
	}
	cleaned = filepath.ToSlash(cleaned)
	if cleaned == "." || cleaned == "" || strings.HasPrefix(cleaned, "../") {
		return ""
	}
	if _, ok := a.resolveArtifactPath(cleaned); !ok {
		return ""
	}
	return "/artifacts/" + cleaned
}

func (a *RunAPI) artifactRelativePath(absPath string) (string, bool) {
	cleaned := filepath.Clean(absPath)
	projectsRoot := filepath.Clean(a.cfg.EffectiveProjectsDir())
	if relToProjects, err := filepath.Rel(projectsRoot, cleaned); err == nil && relToProjects != "." && !strings.HasPrefix(relToProjects, "..") {
		parts := strings.Split(filepath.ToSlash(relToProjects), "/")
		if len(parts) >= 3 && parts[1] == "artifacts" {
			return path.Join(append([]string{parts[0]}, parts[2:]...)...), true
		}
	}
	legacyRoot := filepath.Clean(a.cfg.ArtifactDir)
	if relToLegacy, err := filepath.Rel(legacyRoot, cleaned); err == nil && relToLegacy != "." && !strings.HasPrefix(relToLegacy, "..") {
		return filepath.ToSlash(relToLegacy), true
	}
	return "", false
}

func (a *RunAPI) resolveArtifactPath(relPath string) (string, bool) {
	cleaned := filepath.ToSlash(strings.TrimSpace(relPath))
	cleaned = path.Clean(strings.TrimPrefix(cleaned, "/"))
	if cleaned == "." || cleaned == "" || strings.HasPrefix(cleaned, "../") {
		return "", false
	}

	parts := strings.Split(cleaned, "/")
	if len(parts) > 1 {
		projectPath := filepath.Join(a.cfg.ProjectArtifactDir(parts[0]), filepath.Join(parts[1:]...))
		if info, err := os.Stat(projectPath); err == nil && !info.IsDir() {
			return projectPath, true
		}
	}

	legacyPath := filepath.Join(a.cfg.ArtifactDir, filepath.FromSlash(cleaned))
	if info, err := os.Stat(legacyPath); err == nil && !info.IsDir() {
		return legacyPath, true
	}
	return "", false
}

func (a *RunAPI) streamRunEvents(w http.ResponseWriter, r *http.Request, runID string) error {
	if _, err := a.runs.GetRunRecord(r.Context(), runID); err != nil {
		writeStoreError(w, err)
		return err
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return errors.New("streaming unsupported")
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	events, err := a.runs.ListRunEvents(r.Context(), runID)
	if err != nil {
		writeStoreError(w, err)
		return err
	}
	for _, event := range events {
		if err := writeSSEEvent(w, event); err != nil {
			return err
		}
	}
	flusher.Flush()

	stream, unsubscribe := a.events.Subscribe(runID)
	defer unsubscribe()

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case event, ok := <-stream:
			if !ok {
				return nil
			}
			if err := writeSSEEvent(w, event); err != nil {
				return err
			}
			flusher.Flush()
		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": keep-alive\n\n"); err != nil {
				return err
			}
			flusher.Flush()
		case <-r.Context().Done():
			return r.Context().Err()
		}
	}
}

func (a *RunAPI) serveIndex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.NotFound(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/artifacts/") {
		http.NotFound(w, r)
		return
	}
	index, err := fs.ReadFile(a.static, "index.html")
	if err != nil {
		http.Error(w, fmt.Sprintf("read index: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(index)
}

func parseRunPath(path string) (runID, action string, ok bool) {
	const prefix = "/api/v1/runs/"
	if !strings.HasPrefix(path, prefix) {
		return "", "", false
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(path, prefix), "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		return "", "", false
	}
	runID = parts[0]
	if len(parts) > 1 {
		action = parts[1]
	}
	return runID, action, true
}

func parseChatPath(path string) (chatID string, ok bool) {
	const prefix = "/api/v1/chats/"
	if !strings.HasPrefix(path, prefix) {
		return "", false
	}
	chatID = strings.Trim(strings.TrimPrefix(path, prefix), "/")
	if chatID == "" || strings.Contains(chatID, "/") {
		return "", false
	}
	return chatID, true
}

func parseScheduledPath(path string) (scheduledRunID, action string, ok bool) {
	const prefix = "/api/v1/scheduled/"
	if !strings.HasPrefix(path, prefix) {
		return "", "", false
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(path, prefix), "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		return "", "", false
	}
	scheduledRunID = parts[0]
	if len(parts) > 1 {
		action = parts[1]
	}
	return scheduledRunID, action, true
}

func parseProjectPath(path string) (slug, action string, ok bool) {
	const prefix = "/api/v1/projects/"
	if !strings.HasPrefix(path, prefix) {
		return "", "", false
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(path, prefix), "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		return "", "", false
	}
	slug = parts[0]
	if len(parts) > 1 {
		action = strings.Join(parts[1:], "/")
	}
	return slug, action, true
}

func decodeJSONBody(body io.ReadCloser, target any) error {
	defer body.Close()
	if body == nil {
		return io.EOF
	}
	buffered := bufio.NewReader(body)
	if _, err := buffered.Peek(1); err != nil {
		return err
	}
	return json.NewDecoder(buffered).Decode(target)
}

func writeSSEEvent(w io.Writer, event assistant.RunEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprint(w, "event: run_event\n"); err != nil {
		return err
	}
	const chunkSize = 16 * 1024
	for len(payload) > 0 {
		n := len(payload)
		if n > chunkSize {
			n = chunkSize
		}
		if _, err := fmt.Fprintf(w, "data: %s\n", payload[:n]); err != nil {
			return err
		}
		payload = payload[n:]
	}
	_, err = fmt.Fprint(w, "\n")
	return err
}

func writeStoreError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	if errors.Is(err, store.ErrNotFound) {
		status = http.StatusNotFound
	} else if errors.Is(err, assistantapp.ErrRunNotWaiting) {
		status = http.StatusConflict
	} else if errors.Is(err, assistantapp.ErrScheduledRunNotPending) {
		status = http.StatusConflict
	}
	http.Error(w, err.Error(), status)
}

func writeWikiError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, os.ErrNotExist):
		status = http.StatusNotFound
	case errors.Is(err, wiki.ErrWikiDisabled):
		status = http.StatusBadRequest
	default:
		if strings.Contains(strings.ToLower(err.Error()), "invalid page path") || strings.Contains(strings.ToLower(err.Error()), "project slug is required") {
			status = http.StatusBadRequest
		}
	}
	http.Error(w, err.Error(), status)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
