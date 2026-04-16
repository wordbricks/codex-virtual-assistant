package config

import (
	"testing"
	"time"
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

	if cfg.HTTPAddr != "127.0.0.1:8080" {
		t.Fatalf("HTTPAddr = %q, want %q", cfg.HTTPAddr, "127.0.0.1:8080")
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
	if cfg.CodexBin != "codex" {
		t.Fatalf("CodexBin = %q, want %q", cfg.CodexBin, "codex")
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
		"ASSISTANT_CODEX_BIN":               "/usr/local/bin/codex",
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
	if cfg.CodexBin != "/usr/local/bin/codex" {
		t.Fatalf("CodexBin = %q, want %q", cfg.CodexBin, "/usr/local/bin/codex")
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
