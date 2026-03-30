package ralphloop

import (
	"strings"
	"testing"
)

func TestParseMainCommandFromFlags(t *testing.T) {
	command, err := ParseCommand([]string{"implement the feature", "--max-iterations", "5"}, strings.NewReader(""), OutputJSON)
	if err != nil {
		t.Fatalf("ParseCommand() error = %v", err)
	}
	if command.Kind != commandMain {
		t.Fatalf("kind = %s, want %s", command.Kind, commandMain)
	}
	if command.MainOptions.Prompt != "implement the feature" {
		t.Fatalf("prompt = %q", command.MainOptions.Prompt)
	}
	if command.MainOptions.MaxIterations != 5 {
		t.Fatalf("max iterations = %d", command.MainOptions.MaxIterations)
	}
}

func TestParseInitCommandFromJSONPayload(t *testing.T) {
	command, err := ParseCommand([]string{"init", "--json", "-"}, strings.NewReader(`{"command":"init","base_branch":"dev","work_branch":"ralph-agent","dry_run":true,"output":"json"}`), OutputText)
	if err != nil {
		t.Fatalf("ParseCommand() error = %v", err)
	}
	if command.Kind != commandInit {
		t.Fatalf("kind = %s, want %s", command.Kind, commandInit)
	}
	if command.InitOptions.BaseBranch != "dev" {
		t.Fatalf("base branch = %q", command.InitOptions.BaseBranch)
	}
	if !command.InitOptions.DryRun {
		t.Fatal("expected dry_run to be true")
	}
	if command.Common.Output != OutputJSON {
		t.Fatalf("output = %s, want %s", command.Common.Output, OutputJSON)
	}
}

func TestDetectOutputFormatDefaultsToJSONForNonTTY(t *testing.T) {
	if got := detectOutputFormat(&strings.Builder{}); got != OutputJSON {
		t.Fatalf("detectOutputFormat() = %s, want %s", got, OutputJSON)
	}
}

func TestSandboxOutputPathRejectsEscape(t *testing.T) {
	if _, err := sandboxOutputPath("/tmp/project", "../escape.json"); err == nil {
		t.Fatal("expected escape path to be rejected")
	}
}
