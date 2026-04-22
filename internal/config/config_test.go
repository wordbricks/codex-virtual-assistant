package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
	authpkg "github.com/siisee11/CodexVirtualAssistant/internal/auth"
)

func TestLoadFromEnvUsesDefaults(t *testing.T) {
	t.Parallel()

	cfg, err := loadFromEnv(
		func(string) string { return "" },
		func() (string, error) {
			return "/tmp/cva", nil
		},
		func() (string, error) {
			return "/home/test/.config", nil
		},
		func() (string, error) {
			return "/home/test", nil
		},
	)
	if err != nil {
		t.Fatalf("loadFromEnv() error = %v", err)
	}

	if cfg.HTTPAddr != "127.0.0.1:4999" {
		t.Fatalf("HTTPAddr = %q, want %q", cfg.HTTPAddr, "127.0.0.1:4999")
	}
	if cfg.ConfigDir != "/home/test/.config/cva" {
		t.Fatalf("ConfigDir = %q, want %q", cfg.ConfigDir, "/home/test/.config/cva")
	}
	if cfg.DataDir != "/home/test/.config/cva/workspace" {
		t.Fatalf("DataDir = %q, want %q", cfg.DataDir, "/home/test/.config/cva/workspace")
	}
	if cfg.ProjectsDir != "/home/test/.config/cva/workspace/projects" {
		t.Fatalf("ProjectsDir = %q, want %q", cfg.ProjectsDir, "/home/test/.config/cva/workspace/projects")
	}
	if cfg.DatabasePath != "/home/test/.config/cva/workspace/assistant.db" {
		t.Fatalf("DatabasePath = %q, want %q", cfg.DatabasePath, "/home/test/.config/cva/workspace/assistant.db")
	}
	if cfg.ArtifactDir != "/home/test/.config/cva/workspace/artifacts" {
		t.Fatalf("ArtifactDir = %q, want %q", cfg.ArtifactDir, "/home/test/.config/cva/workspace/artifacts")
	}
	if cfg.DefaultModel != FixedModel {
		t.Fatalf("DefaultModel = %q, want %q", cfg.DefaultModel, FixedModel)
	}
	if cfg.MaxGenerationAttempts != 3 {
		t.Fatalf("MaxGenerationAttempts = %d, want 3", cfg.MaxGenerationAttempts)
	}
	if cfg.RuntimeProvider != "codex" {
		t.Fatalf("RuntimeProvider = %q, want codex", cfg.RuntimeProvider)
	}
	if cfg.CodexBin != "codex" {
		t.Fatalf("CodexBin = %q, want %q", cfg.CodexBin, "codex")
	}
	if cfg.ClaudeBin != "claude" {
		t.Fatalf("ClaudeBin = %q, want %q", cfg.ClaudeBin, "claude")
	}
	if cfg.CodexCwd != "/home/test/.config/cva" {
		t.Fatalf("CodexCwd = %q, want %q", cfg.CodexCwd, "/home/test/.config/cva")
	}
	if cfg.CodexApprovalPolicy != "never" {
		t.Fatalf("CodexApprovalPolicy = %q, want %q", cfg.CodexApprovalPolicy, "never")
	}
	if cfg.CodexSandboxMode != "workspace-write" {
		t.Fatalf("CodexSandboxMode = %q, want %q", cfg.CodexSandboxMode, "workspace-write")
	}
	if !cfg.CodexNetworkAccess {
		t.Fatal("CodexNetworkAccess = false, want true")
	}
	if cfg.SchedulerInterval != 30*time.Second {
		t.Fatalf("SchedulerInterval = %s, want 30s", cfg.SchedulerInterval)
	}
}

func TestLoadFromEnvFallsBackToHomeDirWhenConfigDirUnavailable(t *testing.T) {
	t.Parallel()

	cfg, err := loadFromEnv(
		func(string) string { return "" },
		func() (string, error) {
			return "/tmp/cva", nil
		},
		func() (string, error) {
			return "", assertiveErr("no config dir")
		},
		func() (string, error) {
			return "/home/test", nil
		},
	)
	if err != nil {
		t.Fatalf("loadFromEnv() error = %v", err)
	}

	if cfg.DataDir != "/home/test/.cva/workspace" {
		t.Fatalf("DataDir = %q, want %q", cfg.DataDir, "/home/test/.cva/workspace")
	}
	if cfg.ConfigDir != "/home/test/.cva" {
		t.Fatalf("ConfigDir = %q, want %q", cfg.ConfigDir, "/home/test/.cva")
	}
	if cfg.CodexCwd != "/home/test/.cva" {
		t.Fatalf("CodexCwd = %q, want %q", cfg.CodexCwd, "/home/test/.cva")
	}
}

func TestLoadFromEnvHonorsOverrides(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"ASSISTANT_HTTP_ADDR":               "0.0.0.0:9000",
		"ASSISTANT_DATA_DIR":                "var/state",
		"ASSISTANT_PROJECTS_DIR":            "var/projects",
		"ASSISTANT_DATABASE_PATH":           "var/sqlite/app.db",
		"ASSISTANT_ARTIFACT_DIR":            "var/artifacts",
		"ASSISTANT_MAX_GENERATION_ATTEMPTS": "5",
		"ASSISTANT_RUNTIME":                 "claude",
		"ASSISTANT_CODEX_BIN":               "/usr/local/bin/codex",
		"ASSISTANT_CLAUDE_BIN":              "/usr/local/bin/claude",
		"ASSISTANT_CLAUDE_MODEL":            "claude-sonnet-4-5",
		"ASSISTANT_CODEX_CWD":               "workspace/codex",
		"ASSISTANT_CODEX_APPROVAL_POLICY":   "on-request",
		"ASSISTANT_CODEX_SANDBOX":           "danger-full-access",
		"ASSISTANT_CODEX_NETWORK_ACCESS":    "false",
		"ASSISTANT_SCHEDULER_INTERVAL":      "45s",
	}

	cfg, err := loadFromEnv(
		func(key string) string { return env[key] },
		func() (string, error) {
			return "/workspace/project", nil
		},
		func() (string, error) {
			return "/home/test/.config", nil
		},
		func() (string, error) {
			return "/home/test", nil
		},
	)
	if err != nil {
		t.Fatalf("loadFromEnv() error = %v", err)
	}

	if cfg.HTTPAddr != "0.0.0.0:9000" {
		t.Fatalf("HTTPAddr = %q, want %q", cfg.HTTPAddr, "0.0.0.0:9000")
	}
	if cfg.ConfigDir != "/home/test/.config/cva" {
		t.Fatalf("ConfigDir = %q, want %q", cfg.ConfigDir, "/home/test/.config/cva")
	}
	if cfg.DataDir != "/home/test/.config/cva/var/state" {
		t.Fatalf("DataDir = %q, want %q", cfg.DataDir, "/home/test/.config/cva/var/state")
	}
	if cfg.ProjectsDir != "/home/test/.config/cva/var/projects" {
		t.Fatalf("ProjectsDir = %q, want %q", cfg.ProjectsDir, "/home/test/.config/cva/var/projects")
	}
	if cfg.DatabasePath != "/home/test/.config/cva/var/sqlite/app.db" {
		t.Fatalf("DatabasePath = %q, want %q", cfg.DatabasePath, "/home/test/.config/cva/var/sqlite/app.db")
	}
	if cfg.ArtifactDir != "/home/test/.config/cva/var/artifacts" {
		t.Fatalf("ArtifactDir = %q, want %q", cfg.ArtifactDir, "/home/test/.config/cva/var/artifacts")
	}
	if cfg.MaxGenerationAttempts != 5 {
		t.Fatalf("MaxGenerationAttempts = %d, want 5", cfg.MaxGenerationAttempts)
	}
	if cfg.RuntimeProvider != "claude" {
		t.Fatalf("RuntimeProvider = %q, want claude", cfg.RuntimeProvider)
	}
	if cfg.CodexBin != "/usr/local/bin/codex" {
		t.Fatalf("CodexBin = %q, want %q", cfg.CodexBin, "/usr/local/bin/codex")
	}
	if cfg.ClaudeBin != "/usr/local/bin/claude" {
		t.Fatalf("ClaudeBin = %q, want %q", cfg.ClaudeBin, "/usr/local/bin/claude")
	}
	if cfg.ClaudeModel != "claude-sonnet-4-5" {
		t.Fatalf("ClaudeModel = %q, want %q", cfg.ClaudeModel, "claude-sonnet-4-5")
	}
	if cfg.CodexCwd != "/home/test/.config/cva/workspace/codex" {
		t.Fatalf("CodexCwd = %q, want %q", cfg.CodexCwd, "/home/test/.config/cva/workspace/codex")
	}
	if cfg.CodexApprovalPolicy != "on-request" {
		t.Fatalf("CodexApprovalPolicy = %q, want %q", cfg.CodexApprovalPolicy, "on-request")
	}
	if cfg.CodexSandboxMode != "danger-full-access" {
		t.Fatalf("CodexSandboxMode = %q, want %q", cfg.CodexSandboxMode, "danger-full-access")
	}
	if cfg.CodexNetworkAccess {
		t.Fatal("CodexNetworkAccess = true, want false")
	}
	if cfg.SchedulerInterval != 45*time.Second {
		t.Fatalf("SchedulerInterval = %s, want 45s", cfg.SchedulerInterval)
	}
}

func TestLoadFromEnvEnablesAuthFromPassword(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"ASSISTANT_AUTH_ID":       "operator",
		"ASSISTANT_AUTH_PASSWORD": "correct horse battery staple",
	}
	cfg, err := loadFromEnv(
		func(key string) string { return env[key] },
		func() (string, error) {
			return "/workspace/project", nil
		},
		func() (string, error) {
			return "/home/test/.config", nil
		},
		func() (string, error) {
			return "/home/test", nil
		},
	)
	if err != nil {
		t.Fatalf("loadFromEnv() error = %v", err)
	}

	if !cfg.Auth.Enabled {
		t.Fatal("Auth.Enabled = false, want true")
	}
	if cfg.Auth.UserID != "operator" {
		t.Fatalf("Auth.UserID = %q, want operator", cfg.Auth.UserID)
	}
	if cfg.Auth.Password != "correct horse battery staple" {
		t.Fatal("Auth.Password was not loaded from environment")
	}
}

func TestLoadFromSourcesAutoEnablesFileAuthHash(t *testing.T) {
	t.Parallel()

	hash, err := authpkg.HashPasswordWithParams("correct horse battery staple", authpkg.PasswordParams{
		Memory:      8 * 1024,
		Iterations:  1,
		Parallelism: 1,
		SaltLength:  16,
		KeyLength:   16,
	})
	if err != nil {
		t.Fatalf("HashPasswordWithParams() error = %v", err)
	}
	cfg, err := loadFromSources(
		func(string) string { return "" },
		func() (string, error) {
			return "/workspace/project", nil
		},
		func() (string, error) {
			return "/home/test/.config", nil
		},
		func() (string, error) {
			return "/home/test", nil
		},
		func(string) (FileConfig, error) {
			return FileConfig{Auth: FileAuthConfig{UserID: "operator", PasswordHash: hash}}, nil
		},
	)
	if err != nil {
		t.Fatalf("loadFromSources() error = %v", err)
	}

	if !cfg.Auth.Enabled {
		t.Fatal("Auth.Enabled = false, want true")
	}
	if cfg.Auth.UserID != "operator" {
		t.Fatalf("Auth.UserID = %q, want operator", cfg.Auth.UserID)
	}
	if cfg.Auth.PasswordHash != hash {
		t.Fatal("Auth.PasswordHash was not loaded from file config")
	}
}

func TestLoadFromSourcesHonorsFileRuntimeProvider(t *testing.T) {
	t.Parallel()

	configRoot := t.TempDir()
	cfg, err := loadFromSources(
		func(string) string { return "" },
		func() (string, error) {
			return "/workspace/project", nil
		},
		func() (string, error) {
			return configRoot, nil
		},
		func() (string, error) {
			return "/home/test", nil
		},
		func(path string) (FileConfig, error) {
			want := filepath.Join(configRoot, "cva", "config.json")
			if path != want {
				t.Fatalf("config file path = %q, want %q", path, want)
			}
			return FileConfig{RuntimeProvider: "claude"}, nil
		},
	)
	if err != nil {
		t.Fatalf("loadFromSources() error = %v", err)
	}

	if cfg.RuntimeProvider != "claude" {
		t.Fatalf("RuntimeProvider = %q, want claude", cfg.RuntimeProvider)
	}
}

func TestLoadFromSourcesEnvRuntimeOverridesFile(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"ASSISTANT_RUNTIME": "codex",
	}
	cfg, err := loadFromSources(
		func(key string) string { return env[key] },
		func() (string, error) {
			return "/workspace/project", nil
		},
		func() (string, error) {
			return "/home/test/.config", nil
		},
		func() (string, error) {
			return "/home/test", nil
		},
		func(string) (FileConfig, error) {
			return FileConfig{RuntimeProvider: "claude"}, nil
		},
	)
	if err != nil {
		t.Fatalf("loadFromSources() error = %v", err)
	}

	if cfg.RuntimeProvider != "codex" {
		t.Fatalf("RuntimeProvider = %q, want codex", cfg.RuntimeProvider)
	}
}

func TestWriteRuntimeProviderPersistsFileConfig(t *testing.T) {
	t.Parallel()

	configDir := t.TempDir()
	if err := WriteRuntimeProvider(configDir, "claude"); err != nil {
		t.Fatalf("WriteRuntimeProvider() error = %v", err)
	}

	cfg, err := ReadFileConfig(ConfigFilePath(configDir))
	if err != nil {
		t.Fatalf("ReadFileConfig() error = %v", err)
	}
	if cfg.RuntimeProvider != "claude" {
		t.Fatalf("RuntimeProvider = %q, want claude", cfg.RuntimeProvider)
	}
}

func TestWriteAuthConfigPersistsAndPreservesFileConfig(t *testing.T) {
	t.Parallel()

	configDir := t.TempDir()
	if err := WriteRuntimeProvider(configDir, "claude"); err != nil {
		t.Fatalf("WriteRuntimeProvider() error = %v", err)
	}
	hash, err := authpkg.HashPasswordWithParams("correct horse battery staple", authpkg.PasswordParams{
		Memory:      8 * 1024,
		Iterations:  1,
		Parallelism: 1,
		SaltLength:  16,
		KeyLength:   16,
	})
	if err != nil {
		t.Fatalf("HashPasswordWithParams() error = %v", err)
	}

	if err := WriteAuthConfig(configDir, "operator", hash); err != nil {
		t.Fatalf("WriteAuthConfig() error = %v", err)
	}

	cfg, err := ReadFileConfig(ConfigFilePath(configDir))
	if err != nil {
		t.Fatalf("ReadFileConfig() error = %v", err)
	}
	if cfg.RuntimeProvider != "claude" {
		t.Fatalf("RuntimeProvider = %q, want claude", cfg.RuntimeProvider)
	}
	if cfg.Auth.Enabled == nil || !*cfg.Auth.Enabled {
		t.Fatalf("Auth.Enabled = %#v, want true", cfg.Auth.Enabled)
	}
	if cfg.Auth.UserID != "operator" {
		t.Fatalf("Auth.UserID = %q, want operator", cfg.Auth.UserID)
	}
	if cfg.Auth.PasswordHash != hash {
		t.Fatal("Auth.PasswordHash was not persisted")
	}
}

func TestLoadFromEnvUsesWorkingDirectoryAsLastFallback(t *testing.T) {
	t.Parallel()

	cfg, err := loadFromEnv(
		func(string) string { return "" },
		func() (string, error) {
			return "/tmp/cva", nil
		},
		func() (string, error) {
			return "", assertiveErr("no config dir")
		},
		func() (string, error) {
			return "", assertiveErr("no home dir")
		},
	)
	if err != nil {
		t.Fatalf("loadFromEnv() error = %v", err)
	}

	if cfg.DataDir != "/tmp/cva/workspace" {
		t.Fatalf("DataDir = %q, want %q", cfg.DataDir, "/tmp/cva/workspace")
	}
	if cfg.ConfigDir != "/tmp/cva" {
		t.Fatalf("ConfigDir = %q, want %q", cfg.ConfigDir, "/tmp/cva")
	}
	if cfg.CodexCwd != "/tmp/cva" {
		t.Fatalf("CodexCwd = %q, want %q", cfg.CodexCwd, "/tmp/cva")
	}
}

type assertiveErr string

func (e assertiveErr) Error() string {
	return string(e)
}

func TestProjectArtifactDirUsesProjectsDir(t *testing.T) {
	t.Parallel()

	cfg := Config{
		DataDir:     "/tmp/cva/workspace",
		ProjectsDir: "/tmp/cva/workspace/projects",
	}

	if got := cfg.ProjectArtifactDir("docs-bot"); got != "/tmp/cva/workspace/projects/docs-bot/artifacts" {
		t.Fatalf("ProjectArtifactDir(docs-bot) = %q", got)
	}
	if got := cfg.ProjectArtifactDir(""); got != "/tmp/cva/workspace/projects/no_project/artifacts" {
		t.Fatalf("ProjectArtifactDir(\"\") = %q", got)
	}
}

func TestReadFileConfigRejectsInvalidAutomationSafetyDefaultProfile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	payload := `{"automation_safety":{"defaults":{"not-a-profile":{"enforcement":"advisory"}}}}`
	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	_, err := ReadFileConfig(path)
	if err == nil {
		t.Fatal("ReadFileConfig() error = nil, want invalid automation safety profile error")
	}
	if !strings.Contains(err.Error(), "automation safety") {
		t.Fatalf("ReadFileConfig() error = %v, want automation safety context", err)
	}
}

func TestReadFileConfigRejectsInvalidAutomationSafetyEnforcement(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	payload := `{"automation_safety":{"projects":{"demo":{"profile_override":"browser_mutating","enforcement":"invalid"}}}}`
	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	_, err := ReadFileConfig(path)
	if err == nil {
		t.Fatal("ReadFileConfig() error = nil, want invalid enforcement error")
	}
	if !strings.Contains(err.Error(), "enforcement") {
		t.Fatalf("ReadFileConfig() error = %v, want enforcement context", err)
	}
}

func TestResolveAutomationSafetyPolicyAppliesMergePrecedence(t *testing.T) {
	t.Parallel()

	cfg := Config{
		AutomationSafety: AutomationSafetyConfig{
			Defaults: map[string]AutomationSafetyPolicyOverride{
				string(assistant.AutomationSafetyProfileBrowserHighRiskEngagement): {
					RateLimits: AutomationSafetyRateLimitsOverride{
						MaxAccountChangingActionsPerRun: intPtr(2),
						MaxRepliesPer24h:                intPtr(12),
						MinSpacingMinutes:               intPtr(25),
					},
					ModePolicy: AutomationSafetyModePolicyOverride{
						AllowNoActionSuccess: boolPtr(true),
					},
				},
			},
			Projects: map[string]AutomationSafetyProjectOverride{
				"proj-a": {
					ProfileOverride: assistant.AutomationSafetyProfileBrowserHighRiskEngagement,
					AutomationSafetyPolicyOverride: AutomationSafetyPolicyOverride{
						RateLimits: AutomationSafetyRateLimitsOverride{
							MaxAccountChangingActionsPerRun: intPtr(1),
						},
						ModePolicy: AutomationSafetyModePolicyOverride{
							RequireNoActionEvidence: boolPtr(true),
						},
					},
				},
			},
		},
	}

	engineDefault := &assistant.AutomationSafetyPolicy{
		Profile: assistant.AutomationSafetyProfileBrowserMutating,
		ModePolicy: assistant.AutomationSafetyModePolicy{
			AllowNoActionSuccess: false,
		},
		RateLimits: assistant.AutomationSafetyRateLimits{
			MaxAccountChangingActionsPerRun: 4,
			MaxRepliesPer24h:                30,
			MinSpacingMinutes:               10,
		},
	}

	policy := cfg.ResolveAutomationSafetyPolicy(engineDefault, assistant.AutomationSafetyProfileBrowserMutating, "proj-a")
	if policy == nil {
		t.Fatal("ResolveAutomationSafetyPolicy() = nil, want merged policy")
	}
	if policy.Profile != assistant.AutomationSafetyProfileBrowserHighRiskEngagement {
		t.Fatalf("Profile = %q, want browser_high_risk_engagement", policy.Profile)
	}
	if policy.Enforcement != assistant.AutomationSafetyEnforcementEngineBlocking {
		t.Fatalf("Enforcement = %q, want engine_blocking", policy.Enforcement)
	}
	if policy.RateLimits.MaxAccountChangingActionsPerRun != 1 {
		t.Fatalf("MaxAccountChangingActionsPerRun = %d, want 1", policy.RateLimits.MaxAccountChangingActionsPerRun)
	}
	if policy.RateLimits.MaxRepliesPer24h != 12 {
		t.Fatalf("MaxRepliesPer24h = %d, want 12", policy.RateLimits.MaxRepliesPer24h)
	}
	if policy.RateLimits.MinSpacingMinutes != 25 {
		t.Fatalf("MinSpacingMinutes = %d, want 25", policy.RateLimits.MinSpacingMinutes)
	}
	if !policy.ModePolicy.AllowNoActionSuccess {
		t.Fatal("AllowNoActionSuccess = false, want true from defaults")
	}
	if !policy.ModePolicy.RequireNoActionEvidence {
		t.Fatal("RequireNoActionEvidence = false, want true from project override")
	}
}

func TestResolveAutomationSafetyPolicyReturnsNilWithoutInputs(t *testing.T) {
	t.Parallel()

	cfg := Config{}
	policy := cfg.ResolveAutomationSafetyPolicy(nil, "", "")
	if policy != nil {
		t.Fatalf("ResolveAutomationSafetyPolicy() = %#v, want nil", policy)
	}
}

func boolPtr(value bool) *bool {
	return &value
}

func intPtr(value int) *int {
	return &value
}
