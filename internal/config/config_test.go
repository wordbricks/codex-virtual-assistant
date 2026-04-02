package config

import "testing"

func TestLoadFromEnvUsesDefaults(t *testing.T) {
	t.Parallel()

	cfg, err := LoadFromEnv(func(string) string { return "" }, func() (string, error) {
		return "/tmp/cva", nil
	})
	if err != nil {
		t.Fatalf("LoadFromEnv() error = %v", err)
	}

	if cfg.HTTPAddr != "127.0.0.1:8080" {
		t.Fatalf("HTTPAddr = %q, want %q", cfg.HTTPAddr, "127.0.0.1:8080")
	}
	if cfg.DataDir != "/tmp/cva/workspace" {
		t.Fatalf("DataDir = %q, want %q", cfg.DataDir, "/tmp/cva/workspace")
	}
	if cfg.ProjectsDir != "/tmp/cva/workspace/projects" {
		t.Fatalf("ProjectsDir = %q, want %q", cfg.ProjectsDir, "/tmp/cva/workspace/projects")
	}
	if cfg.DatabasePath != "/tmp/cva/workspace/assistant.db" {
		t.Fatalf("DatabasePath = %q, want %q", cfg.DatabasePath, "/tmp/cva/workspace/assistant.db")
	}
	if cfg.ArtifactDir != "/tmp/cva/workspace/artifacts" {
		t.Fatalf("ArtifactDir = %q, want %q", cfg.ArtifactDir, "/tmp/cva/workspace/artifacts")
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
	if cfg.CodexCwd != "/tmp/cva" {
		t.Fatalf("CodexCwd = %q, want %q", cfg.CodexCwd, "/tmp/cva")
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
	}

	cfg, err := LoadFromEnv(func(key string) string { return env[key] }, func() (string, error) {
		return "/workspace/project", nil
	})
	if err != nil {
		t.Fatalf("LoadFromEnv() error = %v", err)
	}

	if cfg.HTTPAddr != "0.0.0.0:9000" {
		t.Fatalf("HTTPAddr = %q, want %q", cfg.HTTPAddr, "0.0.0.0:9000")
	}
	if cfg.DataDir != "/workspace/project/var/state" {
		t.Fatalf("DataDir = %q, want %q", cfg.DataDir, "/workspace/project/var/state")
	}
	if cfg.ProjectsDir != "/workspace/project/var/projects" {
		t.Fatalf("ProjectsDir = %q, want %q", cfg.ProjectsDir, "/workspace/project/var/projects")
	}
	if cfg.DatabasePath != "/workspace/project/var/sqlite/app.db" {
		t.Fatalf("DatabasePath = %q, want %q", cfg.DatabasePath, "/workspace/project/var/sqlite/app.db")
	}
	if cfg.ArtifactDir != "/workspace/project/var/artifacts" {
		t.Fatalf("ArtifactDir = %q, want %q", cfg.ArtifactDir, "/workspace/project/var/artifacts")
	}
	if cfg.MaxGenerationAttempts != 5 {
		t.Fatalf("MaxGenerationAttempts = %d, want 5", cfg.MaxGenerationAttempts)
	}
	if cfg.CodexBin != "/usr/local/bin/codex" {
		t.Fatalf("CodexBin = %q, want %q", cfg.CodexBin, "/usr/local/bin/codex")
	}
	if cfg.CodexCwd != "/workspace/project/workspace/codex" {
		t.Fatalf("CodexCwd = %q, want %q", cfg.CodexCwd, "/workspace/project/workspace/codex")
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
}
