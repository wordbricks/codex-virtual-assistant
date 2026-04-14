package main

import (
	"os"
	"testing"
)

func TestSelectRunOutputMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		jsonMode    bool
		interactive bool
		stdinTTY    bool
		stdoutTTY   bool
		wantOutput  runOutputMode
	}{
		{
			name:        "json wins over tty",
			jsonMode:    true,
			interactive: true,
			stdinTTY:    true,
			stdoutTTY:   true,
			wantOutput:  runOutputModeJSON,
		},
		{
			name:        "json wins over non tty",
			jsonMode:    true,
			interactive: true,
			stdinTTY:    false,
			stdoutTTY:   false,
			wantOutput:  runOutputModeJSON,
		},
		{
			name:        "interactive flag with tty uses tui",
			interactive: true,
			stdinTTY:    true,
			stdoutTTY:   true,
			wantOutput:  runOutputModeTUI,
		},
		{
			name:       "tty without interactive flag uses plain",
			stdinTTY:   true,
			stdoutTTY:  true,
			wantOutput: runOutputModePlain,
		},
		{
			name:        "interactive flag without stdin tty uses plain",
			interactive: true,
			stdoutTTY:   true,
			wantOutput:  runOutputModePlain,
		},
		{
			name:        "interactive flag without stdout tty uses plain",
			interactive: true,
			stdinTTY:    true,
			wantOutput:  runOutputModePlain,
		},
		{
			name:       "non tty uses plain",
			wantOutput: runOutputModePlain,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := selectRunOutputMode(tt.jsonMode, tt.interactive, tt.stdinTTY, tt.stdoutTTY)
			if got != tt.wantOutput {
				t.Fatalf("selectRunOutputMode(%v, %v, %v, %v) = %q, want %q", tt.jsonMode, tt.interactive, tt.stdinTTY, tt.stdoutTTY, got, tt.wantOutput)
			}
		})
	}
}

func TestIsTTY(t *testing.T) {
	t.Parallel()

	if isTTY(nil) {
		t.Fatalf("isTTY(nil) = true, want false")
	}

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	defer reader.Close()
	defer writer.Close()

	if isTTY(reader) {
		t.Fatalf("isTTY(pipe reader) = true, want false")
	}
	if isTTY(writer) {
		t.Fatalf("isTTY(pipe writer) = true, want false")
	}
}
