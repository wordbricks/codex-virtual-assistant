package main

import (
	"testing"

	"github.com/siisee11/CodexVirtualAssistant/internal/config"
)

func TestParseStartupOptions(t *testing.T) {
	t.Parallel()

	options, err := parseStartupOptions([]string{"--yolo"})
	if err != nil {
		t.Fatalf("parseStartupOptions() error = %v", err)
	}
	if !options.yolo {
		t.Fatal("options.yolo = false, want true")
	}
}

func TestParseStartupOptionsRejectsUnknownArgs(t *testing.T) {
	t.Parallel()

	if _, err := parseStartupOptions([]string{"--nope"}); err == nil {
		t.Fatal("parseStartupOptions() error = nil, want unknown argument error")
	}
}

func TestApplyStartupOptionsForcesDangerFullAccessInYoloMode(t *testing.T) {
	t.Parallel()

	cfg := config.Config{CodexSandboxMode: "workspace-write"}
	cfg = applyStartupOptions(cfg, startupOptions{yolo: true})
	if cfg.CodexSandboxMode != "danger-full-access" {
		t.Fatalf("CodexSandboxMode = %q, want %q", cfg.CodexSandboxMode, "danger-full-access")
	}
}

func TestApplyStartupOptionsLeavesSandboxUnchangedWithoutYolo(t *testing.T) {
	t.Parallel()

	cfg := config.Config{CodexSandboxMode: "workspace-write"}
	cfg = applyStartupOptions(cfg, startupOptions{})
	if cfg.CodexSandboxMode != "workspace-write" {
		t.Fatalf("CodexSandboxMode = %q, want %q", cfg.CodexSandboxMode, "workspace-write")
	}
}
