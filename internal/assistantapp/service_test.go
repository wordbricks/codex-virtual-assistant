package assistantapp

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
	"github.com/siisee11/CodexVirtualAssistant/internal/config"
	"github.com/siisee11/CodexVirtualAssistant/internal/store"
)

func TestRunServiceCreateRunSupportsParentRunID(t *testing.T) {
	t.Parallel()

	repo := openServiceTestRepository(t)
	now := time.Date(2026, time.March, 31, 3, 30, 0, 0, time.UTC)

	parent := assistant.NewRun("Initial completed request.", now.Add(-time.Hour), 2)
	parent.Status = assistant.RunStatusCompleted
	parent.Phase = assistant.RunPhaseCompleted
	parent.CompletedAt = ptrTime(now.Add(-45 * time.Minute))
	parent.UpdatedAt = now.Add(-45 * time.Minute)
	if err := repo.SaveRun(context.Background(), parent); err != nil {
		t.Fatalf("SaveRun(parent) error = %v", err)
	}

	engine := &recordingEngine{repo: repo}
	service := NewRunService(context.Background(), repo, engine, fixedPolicy{}, func() time.Time { return now })

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	run, err := service.CreateRun(ctx, "Follow-up question for the completed run.", 0, parent.ID)
	if err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}
	if run.ParentRunID != parent.ID {
		t.Fatalf("run.ParentRunID = %q, want %q", run.ParentRunID, parent.ID)
	}
	if run.ChatID != parent.ChatID {
		t.Fatalf("run.ChatID = %q, want %q", run.ChatID, parent.ChatID)
	}
	if len(engine.startedRuns) != 1 || engine.startedRuns[0].ParentRunID != parent.ID {
		t.Fatalf("started runs = %#v, want parent-linked run", engine.startedRuns)
	}
	if engine.startedRuns[0].ChatID != parent.ChatID {
		t.Fatalf("started run ChatID = %q, want %q", engine.startedRuns[0].ChatID, parent.ChatID)
	}
}

func TestRunServiceCreateRunRejectsMissingParent(t *testing.T) {
	t.Parallel()

	repo := openServiceTestRepository(t)
	engine := &recordingEngine{repo: repo}
	service := NewRunService(context.Background(), repo, engine, fixedPolicy{}, time.Now)

	_, err := service.CreateRun(context.Background(), "Follow-up question.", 0, "run_missing_parent")
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("CreateRun() error = %v, want store.ErrNotFound", err)
	}
	if len(engine.startedRuns) != 0 {
		t.Fatalf("started runs = %d, want 0 when parent is missing", len(engine.startedRuns))
	}
}

func TestRunServiceCreateRunWithProjectBindsProjectSlug(t *testing.T) {
	t.Parallel()

	repo := openServiceTestRepository(t)
	engine := &recordingEngine{repo: repo}
	now := time.Date(2026, time.April, 14, 8, 0, 0, 0, time.UTC)
	service := NewRunService(context.Background(), repo, engine, fixedPolicy{}, func() time.Time { return now })

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	run, err := service.CreateRunWithProject(ctx, "Continue docs migration work.", 0, "", "docs-bot")
	if err != nil {
		t.Fatalf("CreateRunWithProject() error = %v", err)
	}
	if run.Project.Slug != "docs-bot" {
		t.Fatalf("run.Project.Slug = %q, want %q", run.Project.Slug, "docs-bot")
	}
	if len(engine.startedRuns) != 1 {
		t.Fatalf("len(startedRuns) = %d, want 1", len(engine.startedRuns))
	}
	if engine.startedRuns[0].Project.Slug != "docs-bot" {
		t.Fatalf("started run Project.Slug = %q, want %q", engine.startedRuns[0].Project.Slug, "docs-bot")
	}
}

func TestRunServiceCreateScheduledRunSupportsCron(t *testing.T) {
	t.Parallel()

	repo := openServiceTestRepository(t)
	now := time.Date(2026, time.April, 13, 10, 15, 0, 0, time.FixedZone("PDT", -7*60*60))

	parent := assistant.NewRun("Maintain the workspace wiki.", now.Add(-time.Hour), 2)
	parent.Status = assistant.RunStatusCompleted
	parent.Phase = assistant.RunPhaseCompleted
	parent.CompletedAt = ptrTime(now.Add(-45 * time.Minute))
	parent.UpdatedAt = now.Add(-45 * time.Minute)
	if err := repo.SaveRun(context.Background(), parent); err != nil {
		t.Fatalf("SaveRun(parent) error = %v", err)
	}

	engine := &recordingEngine{repo: repo}
	service := NewRunService(context.Background(), repo, engine, fixedPolicy{}, func() time.Time { return now })

	scheduledRun, err := service.CreateScheduledRun(context.Background(), parent.ID, "", "0 0 * * *", "Perform the daily workspace wiki management pass.", 0)
	if err != nil {
		t.Fatalf("CreateScheduledRun() error = %v", err)
	}
	if scheduledRun.CronExpr != "0 0 * * *" {
		t.Fatalf("scheduledRun.CronExpr = %q, want daily cron", scheduledRun.CronExpr)
	}
}

func TestRunServiceResumeRunRejectsNonWaitingRun(t *testing.T) {
	t.Parallel()

	repo := openServiceTestRepository(t)
	now := time.Date(2026, time.March, 31, 4, 0, 0, 0, time.UTC)

	run := assistant.NewRun("Completed request.", now, 2)
	run.Status = assistant.RunStatusCompleted
	run.Phase = assistant.RunPhaseCompleted
	run.CompletedAt = ptrTime(now.Add(time.Minute))
	run.UpdatedAt = now.Add(time.Minute)
	if err := repo.SaveRun(context.Background(), run); err != nil {
		t.Fatalf("SaveRun() error = %v", err)
	}

	engine := &recordingEngine{repo: repo}
	service := NewRunService(context.Background(), repo, engine, fixedPolicy{}, time.Now)

	err := service.ResumeRun(context.Background(), run.ID, map[string]string{"response": "follow-up"})
	if !errors.Is(err, ErrRunNotWaiting) {
		t.Fatalf("ResumeRun() error = %v, want ErrRunNotWaiting", err)
	}
	if len(engine.resumedRuns) != 0 {
		t.Fatalf("resumed runs = %d, want 0 for non-waiting run", len(engine.resumedRuns))
	}
}

func TestRunServiceResumeRunStillWorksForWaitingRun(t *testing.T) {
	t.Parallel()

	repo := openServiceTestRepository(t)
	now := time.Date(2026, time.March, 31, 4, 20, 0, 0, time.UTC)

	run := assistant.NewRun("Waiting request.", now, 2)
	run.Status = assistant.RunStatusWaiting
	run.Phase = assistant.RunPhaseWaiting
	run.UpdatedAt = now
	if err := repo.SaveRun(context.Background(), run); err != nil {
		t.Fatalf("SaveRun() error = %v", err)
	}

	engine := &recordingEngine{repo: repo, resumeCalled: make(chan struct{}, 1)}
	service := NewRunService(context.Background(), repo, engine, fixedPolicy{}, time.Now)

	input := map[string]string{"response": "approved"}
	if err := service.ResumeRun(context.Background(), run.ID, input); err != nil {
		t.Fatalf("ResumeRun() error = %v", err)
	}

	select {
	case <-engine.resumeCalled:
	case <-time.After(time.Second):
		t.Fatal("ResumeRun() did not call engine.Resume")
	}
	if len(engine.resumedRuns) != 1 || engine.resumedRuns[0].runID != run.ID {
		t.Fatalf("resumed runs = %#v, want run %q", engine.resumedRuns, run.ID)
	}
	if engine.resumedRuns[0].input["response"] != "approved" {
		t.Fatalf("resume input = %#v, want approved response", engine.resumedRuns[0].input)
	}
}

func TestRunServiceListRunsByProjectSlugFiltersStatusAndPaginates(t *testing.T) {
	t.Parallel()

	repo := openServiceTestRepository(t)
	now := time.Date(2026, time.April, 14, 9, 0, 0, 0, time.UTC)

	makeProjectRun := func(idx int, slug string, status assistant.RunStatus) assistant.Run {
		run := assistant.NewRun("project run", now.Add(time.Duration(idx)*time.Minute), 2)
		run.Project = assistant.ProjectContext{Slug: slug}
		run.Status = status
		run.UpdatedAt = run.CreatedAt.Add(5 * time.Second)
		return run
	}

	seedRuns := []assistant.Run{
		makeProjectRun(0, "alpha", assistant.RunStatusQueued),
		makeProjectRun(1, "alpha", assistant.RunStatusCompleted),
		makeProjectRun(2, "alpha", assistant.RunStatusWaiting),
		makeProjectRun(3, "beta", assistant.RunStatusCompleted),
		makeProjectRun(4, "alpha", assistant.RunStatusCompleted),
	}
	for _, run := range seedRuns {
		if err := repo.SaveRun(context.Background(), run); err != nil {
			t.Fatalf("SaveRun(%s) error = %v", run.ID, err)
		}
	}

	service := NewRunService(context.Background(), repo, &recordingEngine{repo: repo}, fixedPolicy{}, time.Now)

	completedPage, err := service.ListRunsByProjectSlug(context.Background(), "alpha", ProjectRunsQuery{
		Status:   assistant.RunStatusCompleted,
		Page:     1,
		PageSize: 1,
	})
	if err != nil {
		t.Fatalf("ListRunsByProjectSlug(alpha/completed) error = %v", err)
	}
	if completedPage.Pagination.Total != 2 {
		t.Fatalf("Total = %d, want 2 completed alpha runs", completedPage.Pagination.Total)
	}
	if completedPage.Pagination.TotalPages != 2 {
		t.Fatalf("TotalPages = %d, want 2", completedPage.Pagination.TotalPages)
	}
	if len(completedPage.Runs) != 1 || completedPage.Runs[0].Status != assistant.RunStatusCompleted {
		t.Fatalf("runs = %#v, want one completed run", completedPage.Runs)
	}

	secondPage, err := service.ListRunsByProjectSlug(context.Background(), "alpha", ProjectRunsQuery{
		Page:     2,
		PageSize: 2,
	})
	if err != nil {
		t.Fatalf("ListRunsByProjectSlug(alpha/page2) error = %v", err)
	}
	if secondPage.Pagination.Total != 4 {
		t.Fatalf("Total = %d, want 4 alpha runs", secondPage.Pagination.Total)
	}
	if secondPage.Pagination.TotalPages != 2 {
		t.Fatalf("TotalPages = %d, want 2", secondPage.Pagination.TotalPages)
	}
	if len(secondPage.Runs) != 2 {
		t.Fatalf("len(runs) = %d, want 2", len(secondPage.Runs))
	}
}

func TestRunServiceListRunsByProjectSlugRejectsInvalidStatus(t *testing.T) {
	t.Parallel()

	repo := openServiceTestRepository(t)
	now := time.Date(2026, time.April, 14, 9, 30, 0, 0, time.UTC)
	run := assistant.NewRun("project run", now, 2)
	run.Project = assistant.ProjectContext{Slug: "alpha"}
	if err := repo.SaveRun(context.Background(), run); err != nil {
		t.Fatalf("SaveRun() error = %v", err)
	}

	service := NewRunService(context.Background(), repo, &recordingEngine{repo: repo}, fixedPolicy{}, time.Now)
	_, err := service.ListRunsByProjectSlug(context.Background(), "alpha", ProjectRunsQuery{
		Status: assistant.RunStatus("unknown"),
	})
	if err == nil {
		t.Fatal("ListRunsByProjectSlug() error = nil, want invalid run status error")
	}
}

func TestRunServiceListAllRunsByProjectSlug(t *testing.T) {
	t.Parallel()

	repo := openServiceTestRepository(t)
	now := time.Date(2026, time.April, 14, 10, 0, 0, 0, time.UTC)

	alphaA := assistant.NewRun("alpha A", now, 2)
	alphaA.Project = assistant.ProjectContext{Slug: "alpha"}
	if err := repo.SaveRun(context.Background(), alphaA); err != nil {
		t.Fatalf("SaveRun(alphaA) error = %v", err)
	}

	beta := assistant.NewRun("beta", now.Add(time.Minute), 2)
	beta.Project = assistant.ProjectContext{Slug: "beta"}
	if err := repo.SaveRun(context.Background(), beta); err != nil {
		t.Fatalf("SaveRun(beta) error = %v", err)
	}

	alphaB := assistant.NewRun("alpha B", now.Add(2*time.Minute), 2)
	alphaB.Project = assistant.ProjectContext{Slug: "alpha"}
	if err := repo.SaveRun(context.Background(), alphaB); err != nil {
		t.Fatalf("SaveRun(alphaB) error = %v", err)
	}

	service := NewRunService(context.Background(), repo, &recordingEngine{repo: repo}, fixedPolicy{}, time.Now)
	runs, err := service.ListAllRunsByProjectSlug(context.Background(), "alpha")
	if err != nil {
		t.Fatalf("ListAllRunsByProjectSlug(alpha) error = %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("len(runs) = %d, want 2", len(runs))
	}
	if runs[0].ID != alphaA.ID || runs[1].ID != alphaB.ID {
		t.Fatalf("runs = %#v, want alphaA then alphaB", runs)
	}
}

type fixedPolicy struct{}

func (fixedPolicy) InitialRun(userRequest string, now time.Time) assistant.Run {
	return assistant.NewRun(userRequest, now, 3)
}

type resumedRun struct {
	runID string
	input map[string]string
}

type recordingEngine struct {
	repo         *store.SQLiteRepository
	startedRuns  []assistant.Run
	resumedRuns  []resumedRun
	resumeCalled chan struct{}
}

func (e *recordingEngine) Start(ctx context.Context, run assistant.Run) error {
	e.startedRuns = append(e.startedRuns, run)
	return e.repo.SaveRun(ctx, run)
}

func (e *recordingEngine) Resume(_ context.Context, runID string, input map[string]string) error {
	clone := make(map[string]string, len(input))
	for key, value := range input {
		clone[key] = value
	}
	e.resumedRuns = append(e.resumedRuns, resumedRun{runID: runID, input: clone})
	if e.resumeCalled != nil {
		select {
		case e.resumeCalled <- struct{}{}:
		default:
		}
	}
	return nil
}

func (e *recordingEngine) Cancel(context.Context, string) error {
	return nil
}

func openServiceTestRepository(t *testing.T) *store.SQLiteRepository {
	t.Helper()

	dataDir := t.TempDir()
	cfg := config.Config{
		HTTPAddr:              "127.0.0.1:0",
		DataDir:               dataDir,
		DatabasePath:          filepath.Join(dataDir, "assistant.db"),
		ArtifactDir:           filepath.Join(dataDir, "artifacts"),
		DefaultModel:          config.FixedModel,
		MaxGenerationAttempts: 3,
	}
	repo, err := store.OpenSQLite(cfg)
	if err != nil {
		t.Fatalf("OpenSQLite() error = %v", err)
	}
	return repo
}

func ptrTime(v time.Time) *time.Time {
	return &v
}
