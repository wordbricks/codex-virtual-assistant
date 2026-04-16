package main

import (
	"strings"
	"testing"
)

func TestFormatRuntimeStatus(t *testing.T) {
	t.Parallel()

	text := formatRuntimeStatus(runtimeStatus{
		RuntimeProvider:      "claude",
		SavedRuntimeProvider: "claude",
		ConfigFile:           "/home/test/.config/cva/config.json",
	}, false)

	for _, want := range []string{
		"Runtime: claude",
		"Config File: /home/test/.config/cva/config.json",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("formatted runtime status missing %q:\n%s", want, text)
		}
	}
}

func TestFormatRuntimeStatusShowsEnvOverride(t *testing.T) {
	t.Parallel()

	text := formatRuntimeStatus(runtimeStatus{
		RuntimeProvider:      "codex",
		SavedRuntimeProvider: "claude",
		ConfigFile:           "/home/test/.config/cva/config.json",
		EnvOverride:          true,
		EnvRuntimeProvider:   "codex",
	}, true)

	for _, want := range []string{
		"Runtime saved as claude",
		"Env Override: ASSISTANT_RUNTIME=codex",
		"Effective Runtime: codex",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("formatted runtime status missing %q:\n%s", want, text)
		}
	}
}
