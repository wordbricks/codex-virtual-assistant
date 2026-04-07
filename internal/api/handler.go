package api

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
	"github.com/siisee11/CodexVirtualAssistant/internal/assistantapp"
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
	RunStatuses                  []assistant.RunStatus `json:"run_statuses"`
	RunPhases                    []assistant.RunPhase  `json:"run_phases"`
	APIBasePath                  string                `json:"api_base_path"`
	ChatsPath                    string                `json:"chats_path"`
	RunsPath                     string                `json:"runs_path"`
}

type RunAPI struct {
	cfg    config.Config
	runs   *assistantapp.RunService
	events *EventBroker
	wiki   *wiki.Service
	static fs.FS
}

type createRunRequest struct {
	UserRequestRaw        string `json:"user_request_raw"`
	MaxGenerationAttempts int    `json:"max_generation_attempts"`
	ParentRunID           string `json:"parent_run_id"`
}

type runActionRequest struct {
	Input map[string]string `json:"input"`
}

type createScheduledRunRequest struct {
	ScheduledFor          string `json:"scheduled_for"`
	Prompt                string `json:"prompt"`
	MaxGenerationAttempts int    `json:"max_generation_attempts"`
}

type updateScheduledRunRequest struct {
	ScheduledFor          string `json:"scheduled_for"`
	Prompt                string `json:"prompt"`
	MaxGenerationAttempts int    `json:"max_generation_attempts"`
}

type createRunResponse struct {
	Run       assistant.Run `json:"run"`
	ChatURL   string        `json:"chat_url"`
	StatusURL string        `json:"status_url"`
	EventsURL string        `json:"events_url"`
}

func NewHandler(cfg config.Config, runs *assistantapp.RunService, events *EventBroker, wikiService *wiki.Service) (http.Handler, error) {
	staticFS, err := web.StaticFS()
	if err != nil {
		return nil, err
	}

	api := &RunAPI{
		cfg:    cfg,
		runs:   runs,
		events: events,
		wiki:   wikiService,
		static: staticFS,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", api.handleHealth)
	mux.HandleFunc("/api/v1/bootstrap", api.handleBootstrap)
	mux.HandleFunc("/api/v1/chats", api.handleChats)
	mux.HandleFunc("/api/v1/chats/", api.handleChatByID)
	mux.HandleFunc("/api/v1/runs", api.handleRuns)
	mux.HandleFunc("/api/v1/runs/", api.handleRunByID)
	mux.HandleFunc("/api/v1/scheduled", api.handleScheduledRuns)
	mux.HandleFunc("/api/v1/scheduled/", api.handleScheduledRunByID)
	mux.HandleFunc("/api/v1/projects", api.handleProjects)
	mux.HandleFunc("/api/v1/projects/", api.handleProjectBySlug)
	mux.Handle("/artifacts/", http.StripPrefix("/artifacts/", http.FileServer(http.Dir(cfg.ArtifactDir))))
	mux.Handle("/assets/", http.StripPrefix("/", http.FileServer(http.FS(staticFS))))
	mux.HandleFunc("/", api.serveIndex)
	return mux, nil
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

		run, err := a.runs.CreateRun(r.Context(), request.UserRequestRaw, request.MaxGenerationAttempts, request.ParentRunID)
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
		scheduledRun, err := a.runs.CreateScheduledRun(r.Context(), runID, request.ScheduledFor, request.Prompt, request.MaxGenerationAttempts)
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
		scheduledRun, err := a.runs.UpdateScheduledRun(r.Context(), scheduledRunID, request.ScheduledFor, request.Prompt, request.MaxGenerationAttempts)
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
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
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

func (a *RunAPI) enrichArtifactURLs(record *store.RunRecord) {
	for idx := range record.Artifacts {
		record.Artifacts[idx].URL = a.artifactURL(record.Artifacts[idx].Path)
	}
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
		rel, err := filepath.Rel(a.cfg.ArtifactDir, cleaned)
		if err != nil || strings.HasPrefix(rel, "..") {
			return ""
		}
		cleaned = rel
	}
	cleaned = filepath.ToSlash(cleaned)
	if cleaned == "." || cleaned == "" || strings.HasPrefix(cleaned, "../") {
		return ""
	}
	if _, err := os.Stat(filepath.Join(a.cfg.ArtifactDir, filepath.FromSlash(cleaned))); err != nil {
		return ""
	}
	return "/artifacts/" + cleaned
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
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	index, err := fs.ReadFile(a.static, "index.html")
	if err != nil {
		http.Error(w, fmt.Sprintf("read index: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
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
	if len(parts) < 2 || parts[0] == "" {
		return "", "", false
	}
	slug = parts[0]
	action = strings.Join(parts[1:], "/")
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
	_, err = fmt.Fprintf(w, "event: run_event\ndata: %s\n\n", payload)
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
