package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/siisee11/CodexVirtualAssistant/internal/config"
)

type runtimeStatus struct {
	RuntimeProvider      string `json:"runtime_provider"`
	SavedRuntimeProvider string `json:"saved_runtime_provider,omitempty"`
	ConfigFile           string `json:"config_file"`
	EnvOverride          bool   `json:"env_override"`
	EnvRuntimeProvider   string `json:"env_runtime_provider,omitempty"`
}

func cmdRuntime(args []string, jsonMode bool) error {
	if len(args) > 1 {
		return fmt.Errorf("usage: cva runtime [codex|claude|zai]")
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if len(args) == 1 {
		provider := strings.TrimSpace(args[0])
		if !config.ValidRuntimeProvider(provider) {
			return fmt.Errorf("runtime provider must be %q, %q, or %q", "codex", "claude", "zai")
		}
		if err := config.WriteRuntimeProvider(cfg.ConfigDir, provider); err != nil {
			return err
		}
	}

	status, err := runtimeStatusFromConfig(cfg.ConfigDir)
	if err != nil {
		return err
	}
	if jsonMode {
		return printJSON(status)
	}
	fmt.Print(formatRuntimeStatus(status, len(args) == 1))
	return nil
}

func runtimeStatusFromConfig(configDir string) (runtimeStatus, error) {
	cfg, err := config.Load()
	if err != nil {
		return runtimeStatus{}, fmt.Errorf("load config: %w", err)
	}
	path := config.ConfigFilePath(configDir)
	fileConfig, err := config.ReadFileConfig(path)
	if err != nil {
		return runtimeStatus{}, err
	}

	envRuntime := strings.TrimSpace(os.Getenv("ASSISTANT_RUNTIME"))
	status := runtimeStatus{
		RuntimeProvider:      cfg.RuntimeProvider,
		SavedRuntimeProvider: fileConfig.RuntimeProvider,
		ConfigFile:           path,
		EnvOverride:          envRuntime != "",
		EnvRuntimeProvider:   envRuntime,
	}
	return status, nil
}

func formatRuntimeStatus(status runtimeStatus, changed bool) string {
	var b strings.Builder
	if changed {
		fmt.Fprintf(&b, "Runtime saved as %s\n", status.SavedRuntimeProvider)
	} else {
		fmt.Fprintf(&b, "Runtime: %s\n", status.RuntimeProvider)
	}
	fmt.Fprintf(&b, "Config File: %s\n", status.ConfigFile)
	if status.EnvOverride {
		fmt.Fprintf(&b, "Env Override: ASSISTANT_RUNTIME=%s\n", status.EnvRuntimeProvider)
		if changed && status.RuntimeProvider != status.SavedRuntimeProvider {
			fmt.Fprintf(&b, "Effective Runtime: %s\n", status.RuntimeProvider)
		}
	}
	return b.String()
}
