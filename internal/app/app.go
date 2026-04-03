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
	"github.com/siisee11/CodexVirtualAssistant/internal/store"
	"github.com/siisee11/CodexVirtualAssistant/internal/wtl"
)

type App struct {
	cfg       config.Config
	store     *store.SQLiteRepository
	runtime   wtl.Runtime
	events    *api.EventBroker
	server    *http.Server
	messenger agentmessage.Service
}

func New(cfg config.Config) (*App, error) {
	executor := wtl.NewAppServerPhaseExecutor(wtl.AppServerPhaseExecutorConfig{
		BinaryPath:     cfg.CodexBin,
		Cwd:            cfg.CodexCwd,
		ArtifactDir:    cfg.ArtifactDir,
		Model:          cfg.DefaultModel,
		ApprovalPolicy: cfg.CodexApprovalPolicy,
		SandboxMode:    cfg.CodexSandboxMode,
		NetworkAccess:  cfg.CodexNetworkAccess,
	}, time.Now)
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
	projectManager := project.NewManager(cfg.DataDir, cfg.EffectiveProjectsDir())
	if err := projectManager.EnsureBaseScaffold(); err != nil {
		return nil, err
	}

	policy := gan.New(gan.Config{MaxGenerationAttempts: cfg.MaxGenerationAttempts})
	events := api.NewEventBroker()
	events.SetSnapshotLoader(repo)
	if executor == nil {
		return nil, errors.New("codex executor is required")
	}
	runtime := wtl.NewCodexRuntime(executor, cfg.DefaultModel, time.Now)
	engine := wtl.NewRunEngine(repo, runtime, events, policy, projectManager, messenger, time.Now)
	runs := assistantapp.NewRunService(context.Background(), repo, engine, policy, time.Now)

	handler, err := api.NewHandler(cfg, runs, events)
	if err != nil {
		return nil, err
	}

	registerAgentMessageHooks(events, messenger)

	return &App{
		cfg:       cfg,
		store:     repo,
		runtime:   runtime,
		events:    events,
		messenger: messenger,
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

func registerAgentMessageHooks(events *api.EventBroker, messenger agentmessage.Service) {
	if events == nil || messenger == nil {
		return
	}

	register := func(name api.HookName, build func(api.HookPayload) (string, bool)) {
		events.RegisterHook(name, func(ctx context.Context, payload api.HookPayload) error {
			if payload.Record == nil {
				return nil
			}
			spec, ok := build(payload)
			if !ok {
				return nil
			}
			return messenger.SendJSONRender(ctx, payload.Record.Run.ChatID, spec)
		})
	}

	register(api.HookOnRunStarted, func(payload api.HookPayload) (string, bool) {
		spec, err := agentmessage.RenderLifecycleCard(agentmessage.StartedCard(payload.Record.Run, payload.Event.Summary))
		return spec, err == nil
	})
	register(api.HookOnWaitEntered, func(payload api.HookPayload) (string, bool) {
		spec, err := agentmessage.RenderLifecycleCard(agentmessage.WaitingCard(payload.Record.Run))
		return spec, err == nil
	})
	register(api.HookOnRunCompleted, func(payload api.HookPayload) (string, bool) {
		spec, err := agentmessage.RenderLifecycleCard(agentmessage.CompletedCard(payload.Record.Run))
		return spec, err == nil
	})
	register(api.HookOnRunExhausted, func(payload api.HookPayload) (string, bool) {
		spec, err := agentmessage.RenderLifecycleCard(agentmessage.ExhaustedCard(payload.Record.Run))
		return spec, err == nil
	})
	register(api.HookOnRunFailed, func(payload api.HookPayload) (string, bool) {
		spec, err := agentmessage.RenderLifecycleCard(agentmessage.FailedCard(payload.Record.Run, payload.Event.Summary))
		return spec, err == nil
	})
}

func (a *App) Run(ctx context.Context) error {
	defer a.store.Close()
	defer a.runtime.Close()

	errCh := make(chan error, 1)
	go func() {
		errCh <- a.server.ListenAndServe()
	}()

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
