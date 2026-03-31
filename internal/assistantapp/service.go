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

type InitialRunPolicy interface {
	InitialRun(string, time.Time) assistant.Run
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
	if parentRunID != "" {
		if _, err := s.repo.GetRun(ctx, parentRunID); err != nil {
			return assistant.Run{}, err
		}
	}

	now := s.now().UTC()
	run := s.policy.InitialRun(userRequest, now)
	if maxGenerationAttempts > 0 {
		run = assistant.NewRun(userRequest, now, maxGenerationAttempts)
	}
	run.ParentRunID = parentRunID

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

func (s *RunService) ListRunEvents(ctx context.Context, runID string) ([]assistant.RunEvent, error) {
	return s.repo.ListRunEvents(ctx, runID)
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
