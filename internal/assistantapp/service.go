package assistantapp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
	"github.com/siisee11/CodexVirtualAssistant/internal/store"
	"github.com/siisee11/CodexVirtualAssistant/internal/wtl"
)

var ErrRunNotWaiting = errors.New("assistant: run is not waiting")
var ErrScheduledRunNotPending = errors.New("assistant: scheduled run is not pending")

const DefaultProjectRunsPage = 1
const DefaultProjectRunsPageSize = 20
const MaxProjectRunsPageSize = 200

type InitialRunPolicy interface {
	InitialRun(string, time.Time) assistant.Run
}

type ProjectRunsQuery struct {
	Status   assistant.RunStatus
	Page     int
	PageSize int
}

type Pagination struct {
	Page       int `json:"page"`
	PageSize   int `json:"page_size"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

type ProjectRunsPage struct {
	Runs       []assistant.Run `json:"runs"`
	Pagination Pagination      `json:"pagination"`
}

type RunService struct {
	repo   *store.SQLiteRepository
	engine wtl.Engine
	policy InitialRunPolicy
	now    func() time.Time
	bgCtx  context.Context
}

func NewRunService(bgCtx context.Context, repo *store.SQLiteRepository, engine wtl.Engine, policy InitialRunPolicy, now func() time.Time) *RunService {
	if bgCtx == nil {
		bgCtx = context.Background()
	}
	if now == nil {
		now = time.Now
	}
	return &RunService{
		repo:   repo,
		engine: engine,
		policy: policy,
		now:    now,
		bgCtx:  bgCtx,
	}
}

func (s *RunService) CreateRun(ctx context.Context, userRequest string, maxGenerationAttempts int, parentRunID string) (assistant.Run, error) {
	if strings.TrimSpace(userRequest) == "" {
		return assistant.Run{}, errors.New("assistant: user request is required")
	}
	parentRunID = strings.TrimSpace(parentRunID)
	var parentRun assistant.Run
	if parentRunID != "" {
		var err error
		parentRun, err = s.repo.GetRun(ctx, parentRunID)
		if err != nil {
			return assistant.Run{}, err
		}
	}

	now := s.now().UTC()
	run := s.policy.InitialRun(userRequest, now)
	if maxGenerationAttempts > 0 {
		run = assistant.NewRun(userRequest, now, maxGenerationAttempts)
	}
	run.ParentRunID = parentRunID
	if parentRunID != "" {
		run.ChatID = firstNonEmpty(parentRun.ChatID, parentRun.ID)
	}

	go func(run assistant.Run) {
		_ = s.engine.Start(s.bgCtx, run)
	}(run)

	if err := s.waitForRunVisible(ctx, run.ID); err != nil {
		return assistant.Run{}, err
	}
	return s.repo.GetRun(ctx, run.ID)
}

func (s *RunService) GetRunRecord(ctx context.Context, runID string) (store.RunRecord, error) {
	return s.repo.GetRunRecord(ctx, runID)
}

func (s *RunService) GetChatRecord(ctx context.Context, chatID string) (store.ChatRecord, error) {
	return s.repo.GetChatRecord(ctx, chatID)
}

func (s *RunService) ListChats(ctx context.Context) ([]assistant.Chat, error) {
	return s.repo.ListChats(ctx)
}

func (s *RunService) ListRunEvents(ctx context.Context, runID string) ([]assistant.RunEvent, error) {
	return s.repo.ListRunEvents(ctx, runID)
}

func (s *RunService) ListRunsByProjectSlug(ctx context.Context, slug string, query ProjectRunsQuery) (ProjectRunsPage, error) {
	runs, err := s.repo.ListRunsByProjectSlug(ctx, slug)
	if err != nil {
		return ProjectRunsPage{}, err
	}

	if trimmedStatus := strings.TrimSpace(string(query.Status)); trimmedStatus != "" {
		if !isValidRunStatus(assistant.RunStatus(trimmedStatus)) {
			return ProjectRunsPage{}, fmt.Errorf("assistant: invalid run status %q", trimmedStatus)
		}

		status := assistant.RunStatus(trimmedStatus)
		filtered := make([]assistant.Run, 0, len(runs))
		for _, run := range runs {
			if run.Status == status {
				filtered = append(filtered, run)
			}
		}
		runs = filtered
	}

	page, pageSize := normalizeProjectRunsPage(query.Page, query.PageSize)
	total := len(runs)
	totalPages := total / pageSize
	if total%pageSize != 0 {
		totalPages++
	}

	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}

	return ProjectRunsPage{
		Runs: runs[start:end],
		Pagination: Pagination{
			Page:       page,
			PageSize:   pageSize,
			Total:      total,
			TotalPages: totalPages,
		},
	}, nil
}

func (s *RunService) SubmitInput(ctx context.Context, runID string, input map[string]string) error {
	return s.ResumeRun(ctx, runID, input)
}

func (s *RunService) ResumeRun(ctx context.Context, runID string, input map[string]string) error {
	record, err := s.repo.GetRunRecord(ctx, runID)
	if err != nil {
		return err
	}
	if record.Run.Status != assistant.RunStatusWaiting {
		return fmt.Errorf("%w: run %s; create a follow-up run with parent_run_id instead", ErrRunNotWaiting, runID)
	}
	go func() {
		_ = s.engine.Resume(s.bgCtx, runID, input)
	}()
	return nil
}

func (s *RunService) CancelRun(ctx context.Context, runID string) error {
	return s.engine.Cancel(ctx, runID)
}

func (s *RunService) ListScheduledRuns(ctx context.Context, chatID string, status assistant.ScheduledRunStatus) ([]assistant.ScheduledRun, error) {
	return s.repo.ListScheduledRuns(ctx, chatID, status)
}

func (s *RunService) GetScheduledRun(ctx context.Context, scheduledRunID string) (assistant.ScheduledRun, error) {
	return s.repo.GetScheduledRun(ctx, scheduledRunID)
}

func (s *RunService) ListScheduledRunsByParent(ctx context.Context, parentRunID string) ([]assistant.ScheduledRun, error) {
	return s.repo.ListScheduledRunsByParent(ctx, parentRunID)
}

func (s *RunService) CreateScheduledRun(ctx context.Context, parentRunID, scheduledForRaw, cronExpr, prompt string, maxGenerationAttempts int) (assistant.ScheduledRun, error) {
	parentRunID = strings.TrimSpace(parentRunID)
	if parentRunID == "" {
		return assistant.ScheduledRun{}, errors.New("assistant: parent run id is required")
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return assistant.ScheduledRun{}, errors.New("assistant: scheduled prompt is required")
	}

	parentRun, err := s.repo.GetRun(ctx, parentRunID)
	if err != nil {
		return assistant.ScheduledRun{}, err
	}

	cronExpr = strings.TrimSpace(cronExpr)
	scheduledFor, err := resolveScheduledFor(s.now(), strings.TrimSpace(scheduledForRaw), cronExpr)
	if err != nil {
		return assistant.ScheduledRun{}, err
	}
	if maxGenerationAttempts <= 0 {
		maxGenerationAttempts = parentRun.MaxGenerationAttempts
	}
	if maxGenerationAttempts <= 0 {
		maxGenerationAttempts = 3
	}

	now := s.now().UTC()
	scheduledRun := assistant.ScheduledRun{
		ID:                    assistant.NewID("scheduled", now),
		ChatID:                firstNonEmpty(parentRun.ChatID, parentRun.ID),
		ParentRunID:           parentRun.ID,
		UserRequestRaw:        prompt,
		MaxGenerationAttempts: maxGenerationAttempts,
		CronExpr:              cronExpr,
		ScheduledFor:          scheduledFor,
		Status:                assistant.ScheduledRunStatusPending,
		CreatedAt:             now,
	}
	if err := s.repo.SaveScheduledRun(ctx, scheduledRun); err != nil {
		return assistant.ScheduledRun{}, err
	}
	return s.repo.GetScheduledRun(ctx, scheduledRun.ID)
}

func (s *RunService) UpdateScheduledRun(ctx context.Context, scheduledRunID, scheduledForRaw, cronExpr, prompt string, maxGenerationAttempts int) (assistant.ScheduledRun, error) {
	scheduledRun, err := s.repo.GetScheduledRun(ctx, scheduledRunID)
	if err != nil {
		return assistant.ScheduledRun{}, err
	}
	if scheduledRun.Status != assistant.ScheduledRunStatusPending {
		return assistant.ScheduledRun{}, fmt.Errorf("%w: %s is %s", ErrScheduledRunNotPending, scheduledRunID, scheduledRun.Status)
	}

	if strings.TrimSpace(prompt) != "" {
		scheduledRun.UserRequestRaw = strings.TrimSpace(prompt)
	}
	if strings.TrimSpace(scheduledForRaw) != "" || strings.TrimSpace(cronExpr) != "" {
		nextCronExpr := scheduledRun.CronExpr
		if strings.TrimSpace(cronExpr) != "" {
			nextCronExpr = strings.TrimSpace(cronExpr)
		}
		scheduledFor, err := resolveScheduledFor(s.now(), strings.TrimSpace(scheduledForRaw), nextCronExpr)
		if err != nil {
			return assistant.ScheduledRun{}, err
		}
		scheduledRun.CronExpr = nextCronExpr
		scheduledRun.ScheduledFor = scheduledFor
	}
	if maxGenerationAttempts > 0 {
		scheduledRun.MaxGenerationAttempts = maxGenerationAttempts
	}

	if err := s.repo.UpdateScheduledRun(ctx, scheduledRun); err != nil {
		return assistant.ScheduledRun{}, err
	}
	return s.repo.GetScheduledRun(ctx, scheduledRunID)
}

func resolveScheduledFor(now time.Time, scheduledForRaw, cronExpr string) (time.Time, error) {
	if cronExpr != "" {
		return assistant.NextCronOccurrence(cronExpr, now.In(time.Local))
	}
	return assistant.ParseScheduledFor(scheduledForRaw, now.UTC())
}

func (s *RunService) CancelScheduledRun(ctx context.Context, scheduledRunID string) (assistant.ScheduledRun, error) {
	scheduledRun, err := s.repo.GetScheduledRun(ctx, scheduledRunID)
	if err != nil {
		return assistant.ScheduledRun{}, err
	}
	if scheduledRun.Status != assistant.ScheduledRunStatusPending {
		return assistant.ScheduledRun{}, fmt.Errorf("%w: %s is %s", ErrScheduledRunNotPending, scheduledRunID, scheduledRun.Status)
	}
	if err := s.repo.UpdateScheduledRunStatus(ctx, scheduledRunID, assistant.ScheduledRunStatusCancelled, ""); err != nil {
		return assistant.ScheduledRun{}, err
	}
	return s.repo.GetScheduledRun(ctx, scheduledRunID)
}

func (s *RunService) waitForRunVisible(ctx context.Context, runID string) error {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		_, err := s.repo.GetRun(ctx, runID)
		if err == nil {
			return nil
		}
		if !errors.Is(err, store.ErrNotFound) {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func isValidRunStatus(status assistant.RunStatus) bool {
	for _, candidate := range assistant.AllRunStatuses() {
		if candidate == status {
			return true
		}
	}
	return false
}

func normalizeProjectRunsPage(page, pageSize int) (int, int) {
	if page <= 0 {
		page = DefaultProjectRunsPage
	}
	if pageSize <= 0 {
		pageSize = DefaultProjectRunsPageSize
	}
	if pageSize > MaxProjectRunsPageSize {
		pageSize = MaxProjectRunsPageSize
	}
	return page, pageSize
}
