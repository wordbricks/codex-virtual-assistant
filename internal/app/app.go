package app

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/siisee11/CodexVirtualAssistant/internal/agentmessage"
	"github.com/siisee11/CodexVirtualAssistant/internal/api"
	"github.com/siisee11/CodexVirtualAssistant/internal/assistantapp"
	"github.com/siisee11/CodexVirtualAssistant/internal/config"
	"github.com/siisee11/CodexVirtualAssistant/internal/policy/gan"
	"github.com/siisee11/CodexVirtualAssistant/internal/project"
	"github.com/siisee11/CodexVirtualAssistant/internal/scheduler"
	"github.com/siisee11/CodexVirtualAssistant/internal/store"
	"github.com/siisee11/CodexVirtualAssistant/internal/wiki"
	"github.com/siisee11/CodexVirtualAssistant/internal/wtl"
)

type App struct {
	cfg       config.Config
	store     *store.SQLiteRepository
	runtime   wtl.Runtime
	events    *api.EventBroker
	server    *http.Server
	messenger agentmessage.Service
	scheduler *scheduler.Scheduler
	replies   *replyBridge
}

func New(cfg config.Config) (*App, error) {
	var executor wtl.CodexPhaseExecutor
	if cfg.RuntimeProvider == "claude" || cfg.RuntimeProvider == "zai" {
		executor = wtl.NewClaudeHeadlessPhaseExecutor(wtl.ClaudeHeadlessPhaseExecutorConfig{
			BinaryPath:      cfg.ClaudeBin,
			UsePrintWrapper: cfg.RuntimeProvider == "zai",
			Cwd:             cfg.CodexCwd,
			Model:           cfg.ClaudeModel,
			ProjectsDir:     cfg.EffectiveProjectsDir(),
			ArtifactDir:     cfg.ArtifactDir,
		}, time.Now)
	} else {
		executor = wtl.NewAppServerPhaseExecutor(wtl.AppServerPhaseExecutorConfig{
			BinaryPath:     cfg.CodexBin,
			Cwd:            cfg.CodexCwd,
			ProjectsDir:    cfg.EffectiveProjectsDir(),
			ArtifactDir:    cfg.ArtifactDir,
			Model:          cfg.DefaultModel,
			ApprovalPolicy: cfg.CodexApprovalPolicy,
			SandboxMode:    cfg.CodexSandboxMode,
			NetworkAccess:  cfg.CodexNetworkAccess,
		}, time.Now)
	}
	return NewWithExecutorAndMessenger(cfg, executor, agentmessage.NewClient())
}

func NewWithExecutor(cfg config.Config, executor wtl.CodexPhaseExecutor) (*App, error) {
	return NewWithExecutorAndMessenger(cfg, executor, agentmessage.NewClient())
}

func NewWithExecutorAndMessenger(cfg config.Config, executor wtl.CodexPhaseExecutor, messenger agentmessage.Service) (*App, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	repo, err := store.OpenSQLite(cfg)
	if err != nil {
		return nil, err
	}
	if err := ensureWorkspaceWikiManagementSchedule(context.Background(), repo, cfg, time.Now()); err != nil {
		return nil, err
	}
	projectManager := project.NewManager(cfg.DataDir, cfg.EffectiveProjectsDir())
	if err := projectManager.EnsureBaseScaffold(); err != nil {
		return nil, err
	}
	wikiService := wiki.NewService(cfg.EffectiveProjectsDir(), time.Now)

	policy := gan.New(gan.Config{MaxGenerationAttempts: cfg.MaxGenerationAttempts})
	events := api.NewEventBroker()
	events.SetSnapshotLoader(repo)
	if executor == nil {
		return nil, errors.New("codex executor is required")
	}
	runtime := wtl.NewCodexRuntime(executor, cfg.DefaultModel, time.Now)
	engine := wtl.NewRunEngine(repo, runtime, events, policy, projectManager, wikiService, messenger, time.Now)
	runs := assistantapp.NewRunService(context.Background(), repo, engine, policy, time.Now)
	backgroundScheduler := scheduler.New(repo, runs, events, cfg.SchedulerInterval, time.Now)

	handler, err := api.NewHandler(cfg, runs, events, wikiService)
	if err != nil {
		return nil, err
	}

	return &App{
		cfg:       cfg,
		store:     repo,
		runtime:   runtime,
		events:    events,
		messenger: messenger,
		scheduler: backgroundScheduler,
		replies:   newReplyBridge(runs, messenger, 5*time.Second),
		server: &http.Server{
			Addr:              cfg.HTTPAddr,
			Handler:           handler,
			ReadHeaderTimeout: 5 * time.Second,
		},
	}, nil
}

func (a *App) Handler() http.Handler {
	return a.server.Handler
}

func (a *App) RegisterHook(name api.HookName, hook api.HookFunc) func() {
	if a.events == nil {
		return func() {}
	}
	return a.events.RegisterHook(name, hook)
}

func (a *App) Run(ctx context.Context) error {
	defer a.store.Close()
	defer a.runtime.Close()

	errCh := make(chan error, 1)
	go func() {
		errCh <- a.server.ListenAndServe()
	}()
	if a.scheduler != nil {
		go func() {
			if err := a.scheduler.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				select {
				case errCh <- err:
				default:
				}
			}
		}()
	}
	if a.replies != nil {
		go func() {
			if err := a.replies.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				select {
				case errCh <- err:
				default:
				}
			}
		}()
	}

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := a.server.Shutdown(shutdownCtx); err != nil {
		return err
	}

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	default:
	}

	return nil
}
