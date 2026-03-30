package ralphloop

import (
	"bytes"
	"encoding/json"
	"os"
	"regexp"
	"strings"
)

var eventPrefixPattern = regexp.MustCompile(`^__RALPH_LOOP_EVENT__\s+`)

func executeMainCommand(runCtx runContext) int {
	request := map[string]any{
		"command":           "main",
		"prompt":            runCtx.command.MainOptions.Prompt,
		"model":             runCtx.command.MainOptions.Model,
		"base_branch":       runCtx.command.MainOptions.BaseBranch,
		"max_iterations":    runCtx.command.MainOptions.MaxIterations,
		"work_branch":       runCtx.command.MainOptions.WorkBranch,
		"timeout":           runCtx.command.MainOptions.TimeoutSeconds,
		"approval_policy":   runCtx.command.MainOptions.ApprovalPolicy,
		"sandbox":           runCtx.command.MainOptions.Sandbox,
		"preserve_worktree": runCtx.command.MainOptions.PreserveWorktree,
		"dry_run":           runCtx.command.MainOptions.DryRun,
	}
	sideEffects := []string{
		"create or reuse a worktree",
		"run the setup agent",
		"iterate the coding loop until completion",
		"run the PR agent",
		"write logs under .worktree/<id>/logs/ralph-loop.log",
	}
	if runCtx.command.MainOptions.DryRun {
		return writeDryRun(runCtx, request, sideEffects)
	}

	if runCtx.command.Common.Output == OutputText {
		_, err := runMain(runCtx.ctx, runCtx.repoRoot, runCtx.command.MainOptions, runCtx.stdout, runCtx.stderr)
		if err != nil {
			return writeCommandError(runCtx.stdout, runCtx.stderr, runCtx.command.Common.Output, string(runCtx.command.Kind), err)
		}
		return 0
	}

	stdoutBuffer := &bytes.Buffer{}
	stderrBuffer := &bytes.Buffer{}
	restore := os.Getenv("RALPH_LOOP_EMIT_JSON_EVENTS")
	_ = os.Setenv("RALPH_LOOP_EMIT_JSON_EVENTS", "1")
	result, err := runMain(runCtx.ctx, runCtx.repoRoot, runCtx.command.MainOptions, stdoutBuffer, stderrBuffer)
	if restore == "" {
		_ = os.Unsetenv("RALPH_LOOP_EMIT_JSON_EVENTS")
	} else {
		_ = os.Setenv("RALPH_LOOP_EMIT_JSON_EVENTS", restore)
	}
	if err != nil {
		return writeCommandError(runCtx.stdout, runCtx.stderr, runCtx.command.Common.Output, string(runCtx.command.Kind), err)
	}

	events := parseBufferedEvents(stdoutBuffer.String(), result)
	if runCtx.command.Common.Output == OutputNDJSON {
		return writeCommandResult(runCtx, events)
	}
	payload := map[string]any{
		"command":           "main",
		"status":            "completed",
		"worktree_id":       result.WorktreeID,
		"worktree_path":     result.WorktreePath,
		"work_branch":       result.WorkBranch,
		"base_branch":       result.BaseBranch,
		"runtime_root":      result.RuntimeRoot,
		"log_path":          result.LogPath,
		"plan_path":         result.PlanPath,
		"iterations":        result.Iterations,
		"pr_url":            result.PRURL,
		"final_status":      result.FinalStatus,
		"events":            events,
		"naming_source":     result.NamingSource,
		"naming_reason":     result.NamingReason,
		"preserve_worktree": result.Preserved,
	}
	if strings.TrimSpace(stderrBuffer.String()) != "" {
		payload["stderr"] = sanitizeText(stderrBuffer.String())
	}
	return writeCommandResult(runCtx, payload)
}

func parseBufferedEvents(buffer string, result mainRunResult) []map[string]any {
	lines := strings.Split(strings.TrimSpace(buffer), "\n")
	events := make([]map[string]any, 0, len(lines)+1)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if eventPrefixPattern.MatchString(line) {
			payload := eventPrefixPattern.ReplaceAllString(line, "")
			record := map[string]any{}
			if err := json.Unmarshal([]byte(payload), &record); err == nil {
				record["command"] = "main"
				record["status"] = "running"
				events = append(events, record)
				continue
			}
		}
		events = append(events, map[string]any{
			"command": "main",
			"event":   "log",
			"status":  "running",
			"message": sanitizeText(line),
		})
	}
	events = append(events, map[string]any{
		"command":       "main",
		"event":         "run.completed",
		"status":        result.FinalStatus,
		"worktree_path": result.WorktreePath,
		"work_branch":   result.WorkBranch,
		"plan_path":     result.PlanPath,
		"iterations":    result.Iterations,
		"pr_url":        result.PRURL,
	})
	return events
}
